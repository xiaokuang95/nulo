package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

type Tool struct {
	Name         string
	OriginalName string // the name the server knows it by
	Description  string
	InputSchema  json.RawMessage
	Server       string
	call         func(ctx context.Context, args map[string]any) ([]ai.ContentBlock, bool, error)
}

type Client struct {
	name         string
	cfg          ServerConfig
	instructions string
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	mu           sync.Mutex
	wait         map[int64]chan response
	http         *http.Client
	closed       bool
}

// Instructions is the server's own initialize instructions, used as the summary
// in the mcp_servers system prompt section when no description is configured.
func (c *Client) Instructions() string { return c.instructions }

func (c *Client) Close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
}

func Connect(ctx context.Context, name string, cfg ServerConfig) (*Client, []Tool, error) {
	return connect(ctx, name, cfg, nil)
}

// connect takes the set of tool names already used by other servers, so a
// sanitized name that would collide gets Pi's hash suffix.
func connect(ctx context.Context, name string, cfg ServerConfig, taken map[string]bool) (*Client, []Tool, error) {
	if strings.EqualFold(cfg.Type, "sse") {
		return nil, nil, fmt.Errorf("mcp sse transport is not supported")
	}
	c := &Client{name: name, cfg: cfg, wait: map[int64]chan response{}, http: &http.Client{Timeout: 60 * time.Second}}
	if cfg.Timeout > 0 {
		c.http.Timeout = time.Duration(cfg.Timeout * float64(time.Second))
	}
	var err error
	if cfg.URL != "" || cfg.Type == "http" || cfg.Type == "streamable-http" {
		err = c.initHTTP(ctx)
	} else if cfg.Command != "" {
		err = c.initStdio(ctx)
	} else {
		return nil, nil, fmt.Errorf("mcp %s: need command or url", name)
	}
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	rawInit, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "nulo", "version": "0.0.0"},
	})
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	var initRes struct {
		Instructions string `json:"instructions"`
	}
	_ = json.Unmarshal(rawInit, &initRes)
	c.instructions = initRes.Instructions
	_ = c.notify("notifications/initialized", nil)
	raw, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	var list struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(raw, &list)
	var tools []Tool
	for _, t := range list.Tools {
		t := t
		toolName := createMcpToolName(name, t.Name, func(candidate string) bool {
			return taken != nil && taken[candidate]
		})
		if taken != nil {
			taken[toolName] = true
		}
		tools = append(tools, Tool{
			Name:         toolName,
			OriginalName: t.Name,
			Description:  t.Description,
			InputSchema:  t.InputSchema,
			Server:       name,
			call: func(ctx context.Context, args map[string]any) ([]ai.ContentBlock, bool, error) {
				res, err := c.call(ctx, "tools/call", map[string]any{"name": t.Name, "arguments": args})
				if err != nil {
					return []ai.ContentBlock{ai.TextBlock(err.Error())}, true, nil
				}
				blocks, isErr := convertResult(res)
				return blocks, isErr, nil
			},
		})
	}
	return c, tools, nil
}

func clean(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// maxToolNameLength is the provider limit on tool names.
const maxToolNameLength = 64

// createMcpToolName builds `mcp__<server>__<tool>`, mapping everything outside
// [A-Za-z0-9_] to "_". A name that is too long, or that another MCP tool already
// uses after sanitizing, gets a short sha256 suffix.
func createMcpToolName(server, tool string, isTaken func(string) bool) string {
	name := clean("mcp__" + server + "__" + tool)
	if len(name) <= maxToolNameLength && !isTaken(name) {
		return name
	}
	sum := sha256.Sum256([]byte(server + string([]byte{0}) + tool))
	hash := hex.EncodeToString(sum[:])[:8]
	prefix := name
	if limit := maxToolNameLength - len(hash) - 1; len(prefix) > limit {
		prefix = prefix[:limit]
	}
	return prefix + "_" + hash
}

func (c *Client) initStdio(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Handshake timeout lives on ctx for call(); the process must outlive that ctx.
	cmd := exec.Command(c.cfg.Command, c.cfg.Args...)
	if c.cfg.CWD != "" {
		cmd.Dir = expandEnv(c.cfg.CWD, nil)
	}
	cmd.Env = os.Environ()
	for k, v := range c.cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+expandEnv(v, nil))
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	c.stdin = stdin
	go c.readLoop(stdout)
	return nil
}

func (c *Client) initHTTP(ctx context.Context) error {
	if c.cfg.URL == "" {
		return fmt.Errorf("http mcp needs url")
	}
	return nil
}

func (c *Client) readLoop(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var resp response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		if resp.Method != "" && len(resp.ID) == 0 {
			continue
		}
		var id int64
		_ = json.Unmarshal(resp.ID, &id)
		c.mu.Lock()
		ch := c.wait[id]
		c.mu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
}

func (c *Client) notify(method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	return c.send(msg)
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := nextID()
	req := request{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	if c.cfg.URL != "" {
		return c.callHTTP(ctx, req)
	}
	ch := make(chan response, 1)
	c.mu.Lock()
	c.wait[id] = ch
	c.mu.Unlock()
	if err := c.send(req); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

func (c *Client) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return fmt.Errorf("mcp not started")
	}
	_, err = c.stdin.Write(append(b, '\n'))
	return err
}

func (c *Client) callHTTP(ctx context.Context, req request) (json.RawMessage, error) {
	b, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, expandEnv(c.cfg.URL, nil), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.cfg.Headers {
		httpReq.Header.Set(k, expandEnv(v, nil))
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		var result json.RawMessage
		err := readSSELike(resp.Body, func(data string) {
			var r response
			if json.Unmarshal([]byte(data), &r) == nil && r.Error == nil && len(r.Result) > 0 {
				result = r.Result
			}
		})
		return result, err
	}
	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, r.Error
	}
	return r.Result, nil
}

func readSSELike(r io.Reader, fn func(string)) error {
	sc := bufio.NewScanner(r)
	var data strings.Builder
	flush := func() {
		d := strings.TrimSpace(data.String())
		data.Reset()
		if d != "" {
			fn(d)
		}
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			flush()
			continue
		}
		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(rest))
		}
	}
	flush()
	return sc.Err()
}

func expandEnv(s string, extra map[string]string) string {
	return os.Expand(s, func(k string) string {
		if extra != nil {
			if v, ok := extra[k]; ok {
				return v
			}
		}
		return os.Getenv(k)
	})
}
