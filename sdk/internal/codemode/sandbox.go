package codemode

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type Result struct {
	OK      bool
	Text    string
	Error   string
	Store   map[string]any
	Wrote   bool
	Elapsed time.Duration
}

func ParseOptions(src string) (code string, timeout time.Duration) {
	code = src
	timeout = 0
	if strings.HasPrefix(src, "// @options:") {
		line, rest, _ := strings.Cut(src, "\n")
		var opt struct {
			TimeoutMS int `json:"timeout_ms"`
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "// @options:"))
		_ = json.Unmarshal([]byte(raw), &opt)
		if opt.TimeoutMS > 0 {
			timeout = time.Duration(opt.TimeoutMS) * time.Millisecond
		}
		code = rest
	}
	return code, timeout
}

func Run(ctx context.Context, source string, tools map[string]func(map[string]any) (string, error), store map[string]any) (Result, error) {
	code, to := ParseOptions(source)
	if to > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, to)
		defer cancel()
	}
	if store == nil {
		store = map[string]any{}
	}
	h := &Host{store: store, tools: tools}
	start := time.Now()
	v, err := newJSVM(ctx, h)
	if err != nil {
		return Result{}, err
	}
	defer v.Close()
	if err := v.installBridge(); err != nil {
		return Result{}, err
	}
	var names []string
	desc := map[string]string{}
	for n := range tools {
		names = append(names, n)
		desc[n] = n
	}
	wrapped := wrapScript(code, names, desc)
	ptr, err := v.eval(wrapped, "<codemode>", evalAsync)
	if err != nil {
		return Result{}, err
	}
	if v.isException(ptr) {
		ex, _ := v.call("qjs_get_exception")
		return Result{OK: false, Error: v.toString(uint32(ex)), Elapsed: time.Since(start)}, nil
	}
	v.drainJobs()
	h.mu.Lock()
	defer h.mu.Unlock()
	text := strings.Join(h.out, "\n")
	if h.donePayload != "" {
		var payload struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(h.donePayload), &payload) == nil && payload.Text != "" {
			text = payload.Text
		} else if !h.doneOK {
			return Result{OK: false, Error: h.donePayload, Store: h.store, Wrote: h.wrote, Elapsed: time.Since(start)}, nil
		}
	}
	return Result{OK: h.doneOK || text != "" || h.donePayload == "", Text: text, Store: h.store, Wrote: h.wrote, Elapsed: time.Since(start)}, nil
}
