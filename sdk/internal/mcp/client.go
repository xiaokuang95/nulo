package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	CWD     string            `json:"cwd"`
	URL     string            `json:"url"`
	Type    string            `json:"type"`
	Headers map[string]string `json:"headers"`
	Enabled *bool             `json:"enabled"`
	Timeout float64           `json:"timeout"`
}

func (c ServerConfig) on() bool {
	return c.Enabled == nil || *c.Enabled
}

type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Server      string
	call        func(ctx context.Context, args map[string]any) (string, bool, error)
}

type Client struct {
	name   string
	cfg    ServerConfig
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex
	wait   map[int64]chan response
	http   *http.Client
	closed bool
}

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
	if _, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "nulo", "version": "0.0.0"},
	}); err != nil {
		c.Close()
		return nil, nil, err
	}
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
		tools = append(tools, Tool{
			Name:        mcpToolName(name, t.Name),
			Description: t.Description,
			InputSchema: t.InputSchema,
			Server:      name,
			call: func(ctx context.Context, args map[string]any) (string, bool, error) {
				res, err := c.call(ctx, "tools/call", map[string]any{"name": t.Name, "arguments": args})
				if err != nil {
					return err.Error(), true, nil
				}
				var out struct {
					IsError bool `json:"isError"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				}
				_ = json.Unmarshal(res, &out)
				var texts []string
				for _, b := range out.Content {
					if b.Text != "" {
						texts = append(texts, b.Text)
					}
				}
				return strings.Join(texts, "\n"), out.IsError, nil
			},
		})
	}
	return c, tools, nil
}

func mcpToolName(server, tool string) string {
	clean := func(s string) string {
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
	return "mcp__" + clean(server) + "__" + clean(tool)
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
