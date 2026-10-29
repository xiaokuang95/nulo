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

func StreamOpenAIResponses(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
	key := LookupAPIKey(model.Provider, "", opts)
	if key == "" {
		return errorStream(model, "provider is not configured: "+model.Provider, StopError)
	}
	system, msgs := openaiMessages(tr)
	input := make([]map[string]any, 0, len(msgs)+1)
	if system != "" {
		input = append(input, map[string]any{"role": "system", "content": system})
	}
	input = append(input, msgs...)
	body := map[string]any{
		"model":  model.ID,
		"input":  input,
		"stream": true,
	}
	if tools := openaiTools(GetCurrentTools(tr.Messages)); len(tools) > 0 {
		body["tools"] = tools
	}
	raw, _ := json.Marshal(body)
	base := strings.TrimRight(model.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/responses", bytes.NewReader(raw))
	if err != nil {
		return errorStream(model, err.Error(), StopError)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}
	return streamHTTP(ctx, model, req, parseOpenAIResponses)
}

func parseOpenAIResponses(s *EventStream, model Model, r io.Reader) {
	partial := AssistantMessage(nil, model, StopPending, time.Now().UnixMilli())
	blocks := []ContentBlock{}
	textIdx := -1
	started := false
	reason := StopStop
	ensure := func() {
		if !started {
			started = true
			s.Push(AssistantMessageEvent{Type: "start", Partial: cloneMsg(&partial)})
		}
	}
	_ = readSSE(r, func(_, data string) error {
		if data == "[DONE]" {
			return nil
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return nil
		}
		typ, _ := ev["type"].(string)
		switch typ {
		case "response.created", "response.in_progress":
			ensure()
		case "response.output_text.delta":
			ensure()
			delta, _ := ev["delta"].(string)
			if textIdx < 0 {
				textIdx = len(blocks)
				blocks = append(blocks, ContentBlock{Type: "text"})
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "text_start", ContentIndex: textIdx, Partial: cloneMsg(&partial)})
			}
			blocks[textIdx].Text += delta
			partial.Content = EncodeContentBlocks(blocks)
			s.Push(AssistantMessageEvent{Type: "text_delta", ContentIndex: textIdx, Delta: delta, Partial: cloneMsg(&partial)})
		case "response.output_item.added":
			ensure()
			item, _ := ev["item"].(map[string]any)
			it, _ := item["type"].(string)
			if it == "function_call" || it == "tool_call" {
				id, _ := item["id"].(string)
				name, _ := item["name"].(string)
				idx := len(blocks)
				blocks = append(blocks, ContentBlock{Type: "toolCall", ID: id, Name: name, Arguments: map[string]any{}})
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "toolcall_start", ContentIndex: idx, Partial: cloneMsg(&partial)})
				reason = StopToolUse
			}
		case "response.function_call_arguments.delta":
			delta, _ := ev["delta"].(string)
			for i := range blocks {
				if blocks[i].Type == "toolCall" {
					blocks[i].ArgumentsError += delta
					var parsed map[string]any
					_ = json.Unmarshal([]byte(blocks[i].ArgumentsError), &parsed)
					if parsed != nil {
						blocks[i].Arguments = parsed
					}
					partial.Content = EncodeContentBlocks(blocks)
					s.Push(AssistantMessageEvent{Type: "toolcall_delta", ContentIndex: i, Delta: delta, Partial: cloneMsg(&partial)})
					break
				}
			}
		case "response.function_call_arguments.done", "response.output_item.done":
			for i := range blocks {
				if blocks[i].Type == "toolCall" {
					if blocks[i].ArgumentsError != "" {
						var parsed map[string]any
						_ = json.Unmarshal([]byte(blocks[i].ArgumentsError), &parsed)
						if parsed != nil {
							blocks[i].Arguments = parsed
						}
						blocks[i].ArgumentsError = ""
					}
					tc := blocks[i]
					partial.Content = EncodeContentBlocks(blocks)
					s.Push(AssistantMessageEvent{Type: "toolcall_end", ContentIndex: i, ToolCall: &tc, Partial: cloneMsg(&partial)})
					reason = StopToolUse
				}
			}
		case "response.completed":
			ensure()
		case "response.failed":
			reason = StopError
		}
		return nil
	})
	if !started {
		s.Push(AssistantMessageEvent{Type: "error", Reason: StopError, Error: errMsg(model, "empty responses stream", StopError)})
		return
	}
	if textIdx >= 0 {
		s.Push(AssistantMessageEvent{Type: "text_end", ContentIndex: textIdx, Content: blocks[textIdx].Text, Partial: cloneMsg(&partial)})
	}
	partial.Content = EncodeContentBlocks(blocks)
	partial.StopReason = reason
	if reason == StopError {
		s.Push(AssistantMessageEvent{Type: "error", Reason: reason, Error: cloneMsg(&partial)})
		return
	}
	s.Push(AssistantMessageEvent{Type: "done", Reason: reason, Message: cloneMsg(&partial)})
}
