package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MoeForge/moejs"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/toolsearch"
)

// Result is what one script produced.
type Result struct {
	OK bool
	// Content is the formatted output: text items, images, and the console block.
	Content []ai.ContentBlock
	// Text is Content's text, for callers that only need a string.
	Text string
	// Error is the structured error the prelude reported, as JSON.
	Error string
	Store map[string]any
	Wrote bool
	// MaxOutputTokens is the script's `// @options` budget, or 0 for the default.
	MaxOutputTokens int64
	Elapsed         time.Duration
}

// ToolInfo is what a script sees about one callable tool.
type ToolInfo struct {
	Name        string
	Description string
	Namespace   string
	// Sample is what describeTool() and a search result's `description` show:
	// the TypeScript declaration of the tool, as codemode's prompt lists it.
	// Empty means Description.
	Sample string
}

// Namespace groups callable tools, such as one MCP server. It is what
// describeNamespace() resolves to.
type Namespace struct {
	Name         string
	Description  string
	Instructions string
	Tools        []string // tool names belonging to this namespace
}

// RunOptions is the environment a script runs in.
type RunOptions struct {
	// Tools are the callable functions, keyed by tool name.
	Tools map[string]func(map[string]any) (string, error)
	// ToolInfo is what ALL_TOOLS, searchTools and describeTool report.
	ToolInfo []ToolInfo
	// Namespaces are the tool groups describeNamespace() knows about.
	Namespaces []Namespace
	Store      map[string]any
	// MemoryLimit caps what one script may keep and allocate, in bytes. Zero
	// means defaultMemoryLimit and a negative value no limit.
	MemoryLimit int64
}

// normNamespace folds the spellings a script may use for a namespace: an optional
// "mcp__" prefix and dashes versus underscores.
func normNamespace(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "mcp__")
	return strings.ReplaceAll(s, "-", "_")
}

// Run executes one script. The script may start with `// @options:`.
func Run(ctx context.Context, source string, opt RunOptions) (Result, error) {
	parsed, err := ParseSource(source)
	if err != nil {
		return Result{}, err
	}

	store := opt.Store
	if store == nil {
		store = map[string]any{}
	}
	info := map[string]ToolInfo{}
	for _, ti := range opt.ToolInfo {
		info[ti.Name] = ti
	}
	h := &Host{store: store, tools: opt.Tools, info: info, namespaces: opt.Namespaces}

	names := make([]string, 0, len(opt.Tools))
	for name := range opt.Tools {
		names = append(names, name)
	}
	// A stable order keeps ALL_TOOLS, and so searchTools tie-breaks, reproducible.
	sort.Strings(names)

	type toolEntry struct {
		Name        string `json:"name"`
		Call        string `json:"call"`
		Description string `json:"description"`
	}
	entries := make([]toolEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, toolEntry{
			Name:        toIdent(name),
			Call:        name,
			Description: sample(info[name]),
		})
	}
	toolsJSON, _ := json.Marshal(entries)
	storeJSON, _ := json.Marshal(store)
	namespacesJSON, _ := json.Marshal(namespaceTable(opt.Namespaces))

	var maxTokens int64
	if parsed.Options.MaxOutputTokens != nil {
		maxTokens = *parsed.Options.MaxOutputTokens
	}

	start := time.Now()
	v := newJSVM(opt.MemoryLimit, h)
	if err := v.installBridge(); err != nil {
		return Result{}, err
	}
	if err := v.runPrelude(); err != nil {
		return Result{}, err
	}

	// The script's deadline starts once the engine is ready, so startup does not
	// eat the script's budget. Interrupt works from another goroutine and stops
	// an endless loop.
	var timedOut atomic.Bool
	if parsed.Options.TimeoutMs != nil {
		timer := time.AfterFunc(time.Duration(*parsed.Options.TimeoutMs)*time.Millisecond, func() {
			timedOut.Store(true)
			v.interrupt("deadline")
		})
		defer timer.Stop()
	}
	if stop := watchCancel(ctx, v); stop != nil {
		defer stop()
	}

	wrapped := wrapScript(parsed.Code, string(toolsJSON), string(storeJSON), string(namespacesJSON))
	if err := v.eval(wrapped); err != nil {
		return engineFailure(parsed, opt.MemoryLimit, &timedOut, start, err)
	}

	h.mu.Lock()
	items := append([]OutputItem(nil), h.out...)
	ok := h.doneOK
	donePayload := h.donePayload
	finished := h.doneSeen
	st := h.store
	wrote := h.wrote
	h.mu.Unlock()

	if !finished {
		ok = false
		donePayload = mustFailure(scriptFailure{
			Name:    "Error",
			Message: "Script ended before it completed: an await never settled.",
		})
	}

	content := formatOutput(items)
	var text []string
	for _, block := range content {
		if block.Type == "text" {
			text = append(text, block.Text)
		}
	}
	return Result{
		OK:              ok,
		MaxOutputTokens: maxTokens,
		Content:         content,
		Text:            strings.Join(text, "\n"),
		Error:           donePayload,
		Store:           st,
		Wrote:           wrote,
		Elapsed:         time.Since(start),
	}, nil
}

// engineFailure words an engine error as a script failure the model can read: a
// syntax error, an interrupt (deadline, cancellation or memory limit), or a
// JavaScript exception with its stack.
func engineFailure(parsed ParsedSource, memoryLimit int64, timedOut *atomic.Bool, start time.Time, err error) (Result, error) {
	fail := func(name, message, stack string) (Result, error) {
		return Result{
			OK:      false,
			Error:   mustFailure(scriptFailure{Name: name, Message: message, Stack: stack}),
			Elapsed: time.Since(start),
		}, nil
	}

	var syntaxErr *moejs.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fail("SyntaxError", syntaxErr.Error(), "")
	}

	var interrupted *moejs.InterruptedError
	if errors.As(err, &interrupted) {
		switch {
		case errors.Is(err, moejs.ErrMemoryLimit):
			return fail("Error", "Script exceeded its memory limit of "+strconv.FormatInt(memoryLimitBytes(memoryLimit)>>20, 10)+" MiB.", "")
		case timedOut.Load() && parsed.Options.TimeoutMs != nil:
			return fail("Error", "Execution exceeded the deadline of "+strconv.FormatInt(*parsed.Options.TimeoutMs, 10)+" ms.", "")
		default:
			return fail("Error", "This operation was aborted.", "")
		}
	}

	var exc *moejs.Exception
	if errors.As(err, &exc) {
		return fail(exc.Name(), exc.Message(), exc.Stack)
	}

	return Result{}, err
}

// watchCancel interrupts a script when the caller's context ends. The returned
// function stops watching, so a finished run is not interrupted afterwards.
func watchCancel(ctx context.Context, v *jsVM) func() {
	if ctx.Done() == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			v.interrupt("aborted")
		case <-done:
		}
	}()
	return func() { close(done) }
}

func mustFailure(f scriptFailure) string {
	payload, err := json.Marshal(f)
	if err != nil {
		return `{"name":"Error","message":"the script failed"}`
	}
	return string(payload)
}

// sample is what discovery globals show for one tool.
func sample(ti ToolInfo) string {
	if ti.Sample != "" {
		return ti.Sample
	}
	return ti.Description
}

// namespaceTable is what describeNamespace() returns, keyed by the folded name.
func namespaceTable(namespaces []Namespace) map[string]any {
	out := make(map[string]any, len(namespaces))
	for _, ns := range namespaces {
		tools := make([]string, 0, len(ns.Tools))
		for _, name := range ns.Tools {
			tools = append(tools, toIdent(name))
		}
		out[normNamespace(ns.Name)] = map[string]any{
			"name":         ns.Name,
			"description":  ns.Description,
			"instructions": ns.Instructions,
			"tools":        tools,
		}
	}
	return out
}

type scriptFailure struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
}

// formatErrorJSON renders the structured failure the prelude reported: the stack
// when there is one, else "Name: message".
func formatErrorJSON(raw string) string {
	if raw == "" {
		return "Error"
	}
	var failure scriptFailure
	if err := json.Unmarshal([]byte(raw), &failure); err != nil {
		return raw
	}
	if failure.Stack != "" {
		return failure.Stack
	}
	name := failure.Name
	if name == "" {
		name = "Error"
	}
	if failure.Message == "" {
		return name
	}
	return name + ": " + failure.Message
}

// search resolves searchTools(). Ranking happens in Go so codemode and
// tool_search share one BM25.
func (h *Host) search(payload string) string {
	var req struct {
		Query     string   `json:"query"`
		Limit     *float64 `json:"limit"`
		Namespace *string  `json:"namespace"`
	}
	_ = json.Unmarshal([]byte(payload), &req)
	limit := 8
	if req.Limit != nil && *req.Limit > 0 {
		limit = int(*req.Limit)
	}
	var want string
	if req.Namespace != nil && *req.Namespace != "" {
		want = normNamespace(*req.Namespace)
	}

	names := make([]string, 0, len(h.tools))
	for name := range h.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	type entry struct {
		Name        string `json:"name"`
		Call        string `json:"call"`
		Description string `json:"description"`
	}
	docs := make([]string, 0, len(names))
	entries := make([]entry, 0, len(names))
	for _, name := range names {
		ti := h.info[name]
		if want != "" && normNamespace(ti.Namespace) != want {
			continue
		}
		var extra []string
		if ti.Namespace != "" {
			extra = append(extra, ti.Namespace)
			for _, ns := range h.namespaces {
				if normNamespace(ns.Name) == normNamespace(ti.Namespace) {
					extra = append(extra, ns.Description, ns.Instructions)
					break
				}
			}
		}
		docs = append(docs, toolsearch.Document(name, ti.Description, nil, extra...))
		entries = append(entries, entry{Name: toIdent(name), Call: name, Description: sample(ti)})
	}

	matches := toolsearch.Rank(req.Query, docs, limit)
	out := make([]entry, 0, len(matches))
	for _, match := range matches {
		out = append(out, entries[match.Index])
	}
	b, _ := json.Marshal(out)
	if b == nil {
		return "[]"
	}
	return string(b)
}
