package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

func StreamAnthropicMessages(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
	key := LookupAPIKey(model.Provider, "", opts)
	if key == "" {
		return errorStream(model, "provider is not configured: "+model.Provider, StopError)
	}
	system, msgs, tools := anthropicPayload(tr)
	body := map[string]any{
		"model":      model.ID,
		"messages":   msgs,
		"max_tokens": model.MaxTokens,
		"stream":     true,
	}
	if body["max_tokens"] == 0 {
		body["max_tokens"] = 4096
	}
	if opts.MaxTokens > 0 {
		body["max_tokens"] = opts.MaxTokens
	}
	if system != "" {
		body["system"] = system
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	raw, _ := json.Marshal(body)
	base := strings.TrimRight(model.BaseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	url := base + "/v1/messages"
	if strings.HasSuffix(base, "/v1") {
		url = base + "/messages"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return errorStream(model, err.Error(), StopError)
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}
	return streamHTTP(ctx, model, req, parseAnthropic)
}

func parseAnthropic(s *EventStream, model Model, r io.Reader) {
	partial := AssistantMessage(nil, model, StopPending, time.Now().UnixMilli())
	blocks := []ContentBlock{}
	started := false
	stop := ""
	ensureStart := func() {
		if !started {
			started = true
			s.Push(AssistantMessageEvent{Type: "start", Partial: cloneMsg(&partial)})
		}
	}
	_ = readSSE(r, func(event, data string) error {
		var payload map[string]any
		_ = json.Unmarshal([]byte(data), &payload)
		if event == "" {
			if t, _ := payload["type"].(string); t != "" {
				event = t
			}
		}
		switch event {
		case "message_start":
			ensureStart()
		case "content_block_start":
			ensureStart()
			idx := intFrom(payload["index"])
			block, _ := payload["content_block"].(map[string]any)
			typ, _ := block["type"].(string)
			for len(blocks) <= idx {
				blocks = append(blocks, ContentBlock{})
			}
			switch typ {
			case "text":
				blocks[idx] = ContentBlock{Type: "text"}
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "text_start", ContentIndex: idx, Partial: cloneMsg(&partial)})
			case "thinking":
				blocks[idx] = ContentBlock{Type: "thinking"}
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "thinking_start", ContentIndex: idx, Partial: cloneMsg(&partial)})
			case "tool_use":
				id, _ := block["id"].(string)
				name, _ := block["name"].(string)
				blocks[idx] = ContentBlock{Type: "toolCall", ID: id, Name: name, Arguments: map[string]any{}}
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "toolcall_start", ContentIndex: idx, Partial: cloneMsg(&partial)})
			}
		case "content_block_delta":
			idx := intFrom(payload["index"])
			delta, _ := payload["delta"].(map[string]any)
			dt, _ := delta["type"].(string)
			if idx < 0 || idx >= len(blocks) {
				return nil
			}
			switch dt {
			case "text_delta":
				text, _ := delta["text"].(string)
				blocks[idx].Text += text
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "text_delta", ContentIndex: idx, Delta: text, Partial: cloneMsg(&partial)})
			case "thinking_delta":
				text, _ := delta["thinking"].(string)
				blocks[idx].Thinking += text
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "thinking_delta", ContentIndex: idx, Delta: text, Partial: cloneMsg(&partial)})
			case "input_json_delta":
				partialJSON, _ := delta["partial_json"].(string)
				var parsed map[string]any
				// accumulate in Name? store raw in TextSignature temporarily
				blocks[idx].ArgumentsError += partialJSON
				_ = json.Unmarshal([]byte(blocks[idx].ArgumentsError), &parsed)
				if parsed != nil {
					blocks[idx].Arguments = parsed
				}
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "toolcall_delta", ContentIndex: idx, Delta: partialJSON, Partial: cloneMsg(&partial)})
			}
		case "content_block_stop":
			idx := intFrom(payload["index"])
			if idx < 0 || idx >= len(blocks) {
				return nil
			}
			switch blocks[idx].Type {
			case "text":
				s.Push(AssistantMessageEvent{Type: "text_end", ContentIndex: idx, Content: blocks[idx].Text, Partial: cloneMsg(&partial)})
			case "thinking":
				s.Push(AssistantMessageEvent{Type: "thinking_end", ContentIndex: idx, Content: blocks[idx].Thinking, Partial: cloneMsg(&partial)})
			case "toolCall":
				var parsed map[string]any
				if blocks[idx].ArgumentsError != "" {
					_ = json.Unmarshal([]byte(blocks[idx].ArgumentsError), &parsed)
					if parsed != nil {
						blocks[idx].Arguments = parsed
					}
					blocks[idx].ArgumentsError = ""
				}
				tc := blocks[idx]
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "toolcall_end", ContentIndex: idx, ToolCall: &tc, Partial: cloneMsg(&partial)})
			}
		case "message_delta":
			if d, ok := payload["delta"].(map[string]any); ok {
				if sr, _ := d["stop_reason"].(string); sr != "" {
					stop = sr
				}
			}
		}
		return nil
	})
	if !started {
		s.Push(AssistantMessageEvent{Type: "error", Reason: StopError, Error: errMsg(model, "empty anthropic stream", StopError)})
		return
	}
	reason := StopStop
	switch stop {
	case "max_tokens":
		reason = StopLength
	case "tool_use":
		reason = StopToolUse
	case "error":
		reason = StopError
	}
	for _, b := range blocks {
		if b.Type == "toolCall" {
			reason = StopToolUse
		}
	}
	partial.Content = EncodeContentBlocks(nonzeroBlocks(blocks))
	partial.StopReason = reason
	if reason == StopError {
		s.Push(AssistantMessageEvent{Type: "error", Reason: reason, Error: cloneMsg(&partial)})
		return
	}
	s.Push(AssistantMessageEvent{Type: "done", Reason: reason, Message: cloneMsg(&partial)})
}

func intFrom(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func nonzeroBlocks(in []ContentBlock) []ContentBlock {
	var out []ContentBlock
	for _, b := range in {
		if b.Type != "" {
			out = append(out, b)
		}
	}
	return out
}
