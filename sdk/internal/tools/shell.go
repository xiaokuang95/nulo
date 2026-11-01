package tools

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// Limits and ceilings shared by the built-in shell tools, mirroring Pi's
// truncate.ts (DEFAULT_MAX_LINES / DEFAULT_MAX_BYTES) and bash.ts (MAX_TIMEOUT_MS).
const (
	maxShellLines     = 2000
	maxShellBytes     = 50 * 1024
	maxShellTimeoutMS = 2147483647
)

// shellSchema is the parameter schema both shell tools publish, matching Pi's bashSchema.
const shellSchema = `{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute"},"timeout":{"type":"number","description":"Timeout in seconds (optional, no default timeout)"}},"required":["command"]}`

// shellResolveResult is a resolved shell binary plus the arguments that precede the command.
type shellResolveResult struct {
	path string
	args []string
}

// shellConfig is everything that differs between the built-in shell tools. Pi models the
// same split as the config argument of createShellToolDefinition.
type shellConfig struct {
	name          string
	label         string
	shellName     string
	tempPrefix    string
	commandPrefix string
	resolve       func() (shellResolveResult, error)
}

// shellTool builds a shell tool from cfg. Behaviour matches Pi's
// createShellToolDefinition: combined stdout/stderr, tail truncation with the full
// output saved to a temp file, and an optional per-command timeout.
func shellTool(cwd string, cfg shellConfig) agent.AgentTool {
	description := fmt.Sprintf(
		"Execute a %s command in the current working directory. Returns stdout and stderr. "+
			"Output is truncated to last %d lines or %dKB (whichever is hit first). "+
			"If truncated, full output is saved to a temp file. Optionally provide a timeout in seconds.",
		cfg.shellName, maxShellLines, maxShellBytes/1024)
	return agent.AgentTool{
		Tool: ai.Tool{
			Name:        cfg.name,
			Description: description,
			Parameters:  json.RawMessage(shellSchema),
		},
		Label: cfg.label,
		Execute: func(ctx context.Context, _ string, args map[string]any) (agent.ToolResult, error) {
			return runShell(ctx, cwd, cfg, args)
		},
	}
}

func runShell(ctx context.Context, cwd string, cfg shellConfig, args map[string]any) (agent.ToolResult, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return errResult("command is required")
	}
	timeout, err := resolveShellTimeout(args["timeout"])
	if err != nil {
		return errResult(err.Error())
	}
	sh, err := cfg.resolve()
	if err != nil {
		return errResult(err.Error())
	}
	if _, err := os.Stat(cwd); err != nil {
		return errResult(fmt.Sprintf("Working directory does not exist: %s\nCannot execute %s commands.", cwd, cfg.shellName))
	}

	// One buffer for both streams: os/exec serialises the two pipes when Stdout and
	// Stderr are the same writer, so the interleaving stays race-free.
	var out bytes.Buffer
	cmd := exec.Command(sh.path, append(append([]string{}, sh.args...), cfg.commandPrefix+command)...)
	cmd.Dir = cwd
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return errResult(err.Error())
	}

	var timedOut atomic.Bool
	if timeout.duration > 0 {
		timer := time.AfterFunc(timeout.duration, func() {
			timedOut.Store(true)
			killProcessTree(cmd.Process.Pid)
		})
		defer timer.Stop()
	}
	abortDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			killProcessTree(cmd.Process.Pid)
		case <-abortDone:
		}
	}()
	waitErr := cmd.Wait()
	close(abortDone)

	truncation := tailTruncate(out.String(), maxShellLines, maxShellBytes)
	// Pi persists the full output whenever the result is truncated, on every exit path.
	appendTruncationNote := func(text string) string {
		if !truncation.truncated {
			return text
		}
		path, err := writeOutputFile(cfg.tempPrefix, out.Bytes())
		if err != nil {
			return text
		}
		return text + truncationNote(truncation, path)
	}

	// Abort wins over timeout, as in Pi's execute.
	switch {
	case ctx.Err() != nil:
		return errResult(appendStatus(appendTruncationNote(truncation.content), "Command aborted"))
	case timedOut.Load():
		status := fmt.Sprintf("Command timed out after %s seconds", timeout.raw)
		return errResult(appendStatus(appendTruncationNote(truncation.content), status))
	}

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(waitErr, &exitErr) {
			return errResult(appendStatus(appendTruncationNote(truncation.content), waitErr.Error()))
		}
		// A signal-killed shell reports -1; aborts and timeouts are already handled above.
		if exitCode = exitErr.ExitCode(); exitCode < 0 {
			exitCode = 1
		}
	}

	text := truncation.content
	if text == "" {
		text = "(no output)"
	}
	text = appendTruncationNote(text)
	if exitCode != 0 {
		return agent.ToolResult{
			Content: []ai.ContentBlock{ai.TextBlock(appendStatus(text, fmt.Sprintf("Command exited with code %d", exitCode)))},
			IsError: true,
		}, nil
	}
	return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(text)}}, nil
}

// shellTimeout is a validated timeout: the duration to enforce plus the user's own
// number, which Pi echoes back verbatim in the timeout message.
type shellTimeout struct {
	duration time.Duration
	raw      string
}

func resolveShellTimeout(v any) (shellTimeout, error) {
	f, ok := v.(float64)
	if !ok {
		return shellTimeout{}, nil
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 {
		return shellTimeout{}, errors.New("Invalid timeout: must be a finite number of seconds")
	}
	if f*1000 > maxShellTimeoutMS {
		return shellTimeout{}, fmt.Errorf("Invalid timeout: maximum is %s seconds",
			strconv.FormatFloat(float64(maxShellTimeoutMS)/1000, 'f', -1, 64))
	}
	return shellTimeout{duration: time.Duration(f * float64(time.Second)), raw: formatShellNumber(f)}, nil
}

func formatShellNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// appendStatus joins output and a trailing status line the way Pi does, dropping the
// separator when there is no output to begin with.
func appendStatus(text, status string) string {
	if text == "" {
		return status
	}
	return text + "\n\n" + status
}

// killProcessTree terminates a shell and everything it spawned. On Windows this needs
// taskkill (/T); elsewhere the direct child is enough, since the tools that use this
// core only run on Windows.
func killProcessTree(pid int) {
	if pid <= 0 {
		return
	}
	if runtime.GOOS == "windows" {
		// Use the trusted System32 executable so cleanup does not depend on PATH.
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = `C:\Windows`
		}
		taskkill := filepath.Join(systemRoot, "System32", "taskkill.exe")
		if err := exec.Command(taskkill, "/F", "/T", "/PID", strconv.Itoa(pid)).Run(); err == nil {
			return
		}
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

// writeOutputFile stores output the model was not shown in full. Like Pi, these files
// live in the OS temp directory and are readable only by their owner.
func writeOutputFile(prefix string, data []byte) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	path := filepath.Join(os.TempDir(), prefix+"-"+hex.EncodeToString(b[:])+".log")
	// O_EXCL never follows a link someone else placed at the path.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	return path, f.Close()
}

func formatSize(bytes int) string {
	switch {
	case bytes < 1024:
		return strconv.Itoa(bytes) + "B"
	case bytes < 1024*1024:
		return strconv.FormatFloat(float64(bytes)/1024, 'f', 1, 64) + "KB"
	default:
		return strconv.FormatFloat(float64(bytes)/(1024*1024), 'f', 1, 64) + "MB"
	}
}

// truncationNote renders Pi's trailer for a truncated result, which names the output
// path so the model can read the parts it was not shown.
func truncationNote(tr tailResult, path string) string {
	startLine := tr.totalLines - tr.outputLines + 1
	endLine := tr.totalLines
	switch {
	case tr.lastLinePartial:
		return fmt.Sprintf("\n\n[Showing last %s of line %d (line is %s). Full output: %s]",
			formatSize(tr.outputBytes), endLine, formatSize(tr.lastLineBytes), path)
	case tr.truncatedBy == "lines":
		return fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Full output: %s]",
			startLine, endLine, tr.totalLines, path)
	default:
		return fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Full output: %s]",
			startLine, endLine, tr.totalLines, formatSize(maxShellBytes), path)
	}
}

// tailResult is Pi's TailTruncationResult, restricted to the fields a caller needs.
type tailResult struct {
	content         string
	truncated       bool
	truncatedBy     string // "lines", "bytes", or "" when not truncated
	totalLines      int
	totalBytes      int
	outputLines     int
	outputBytes     int
	lastLinePartial bool
	lastLineBytes   int // byte length of the last line before truncation
}

// tailTruncate keeps the last maxLines lines / maxBytes bytes of content, ported from
// Pi's truncateTail. Shell output is truncated from the head because the tail carries
// the errors and the final result. A single line longer than maxBytes yields its own
// terminator as a partial line.
func tailTruncate(content string, maxLines, maxBytes int) tailResult {
	lines := splitLinesForCounting(content)
	tr := tailResult{
		content:       content,
		totalLines:    len(lines),
		totalBytes:    len(content),
		outputLines:   len(lines),
		outputBytes:   len(content),
		lastLineBytes: lastLineBytes(content),
	}
	if tr.totalLines <= maxLines && tr.totalBytes <= maxBytes {
		return tr
	}

	// Work backwards from the end, collecting lines in reverse order.
	var kept []string
	keptBytes := 0
	truncatedBy := "lines"
	for i := len(lines) - 1; i >= 0 && len(kept) < maxLines; i-- {
		lineBytes := len(lines[i])
		if len(kept) > 0 {
			lineBytes++ // the newline joining it to the line after it
		}
		if keptBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			if len(kept) == 0 {
				// The last line alone blows the byte budget: keep its end.
				partial := truncateStringToBytesFromEnd(lines[i], maxBytes)
				kept = append(kept, partial)
				keptBytes = len(partial)
				tr.lastLinePartial = true
			}
			break
		}
		kept = append(kept, lines[i])
		keptBytes += lineBytes
	}
	if len(kept) >= maxLines && keptBytes <= maxBytes {
		truncatedBy = "lines"
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	tr.content = strings.Join(kept, "\n")
	tr.truncated = true
	tr.truncatedBy = truncatedBy
	tr.outputLines = len(kept)
	tr.outputBytes = len(tr.content)
	return tr
}

// splitLinesForCounting matches Pi's splitLinesForCounting: empty content has no lines,
// and a trailing newline does not open a new one.
func splitLinesForCounting(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lastLineBytes is the byte length of the text after the final newline.
func lastLineBytes(content string) int {
	if i := strings.LastIndex(content, "\n"); i >= 0 {
		return len(content) - i - 1
	}
	return len(content)
}

// truncateStringToBytesFromEnd cuts to the last maxBytes bytes, backing up to a UTF-8
// rune boundary so a multi-byte character is never split.
func truncateStringToBytesFromEnd(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	start := len(s) - maxBytes
	for start < len(s) && s[start]&0xc0 == 0x80 {
		start++
	}
	return s[start:]
}
