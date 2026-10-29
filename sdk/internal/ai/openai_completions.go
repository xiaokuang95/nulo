package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func StreamOpenAICompletions(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
	key := LookupAPIKey(model.Provider, "", opts)
	if key == "" {
		return errorStream(model, "provider is not configured: "+model.Provider, StopError)
	}
	system, msgs := openaiMessages(tr)
	if system != "" {
		msgs = append([]map[string]any{{"role": "system", "content": system}}, msgs...)
	}
	body := map[string]any{
		"model":    model.ID,
		"messages": msgs,
		"stream":   true,
	}
	if tools := openaiTools(GetCurrentTools(tr.Messages)); len(tools) > 0 {
		body["tools"] = tools
	}
	if opts.MaxTokens > 0 {
		body["max_tokens"] = opts.MaxTokens
	}
	raw, _ := json.Marshal(body)
	base := strings.TrimRight(model.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return errorStream(model, err.Error(), StopError)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}
	return streamHTTP(ctx, model, req, parseOpenAICompletions)
}

type oaiDelta struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func parseOpenAICompletions(s *EventStream, model Model, r io.Reader) {
	partial := AssistantMessage(nil, model, StopPending, time.Now().UnixMilli())
	blocks := []ContentBlock{}
	textIdx := -1
	type tcAcc struct {
		idx  int
		id   string
		name string
		args string
	}
	tools := map[int]*tcAcc{}
	started := false
	finish := ""
	_ = readSSE(r, func(_, data string) error {
		if data == "[DONE]" {
			return nil
		}
		var d oaiDelta
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return nil
		}
		if !started {
			started = true
			s.Push(AssistantMessageEvent{Type: "start", Partial: cloneMsg(&partial)})
		}
		if len(d.Choices) == 0 {
			return nil
		}
		ch := d.Choices[0]
		if ch.FinishReason != "" {
			finish = ch.FinishReason
		}
		if ch.Delta.Content != "" {
			if textIdx < 0 {
				textIdx = len(blocks)
				blocks = append(blocks, ContentBlock{Type: "text"})
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "text_start", ContentIndex: textIdx, Partial: cloneMsg(&partial)})
			}
			blocks[textIdx].Text += ch.Delta.Content
			partial.Content = EncodeContentBlocks(blocks)
			s.Push(AssistantMessageEvent{Type: "text_delta", ContentIndex: textIdx, Delta: ch.Delta.Content, Partial: cloneMsg(&partial)})
		}
		for _, tc := range ch.Delta.ToolCalls {
			acc, ok := tools[tc.Index]
			if !ok {
				acc = &tcAcc{idx: len(blocks)}
				tools[tc.Index] = acc
				blocks = append(blocks, ContentBlock{Type: "toolCall", Arguments: map[string]any{}})
				if tc.ID != "" {
					acc.id = tc.ID
					blocks[acc.idx].ID = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
					blocks[acc.idx].Name = tc.Function.Name
				}
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "toolcall_start", ContentIndex: acc.idx, Partial: cloneMsg(&partial)})
			}
			if tc.ID != "" {
				acc.id = tc.ID
				blocks[acc.idx].ID = tc.ID
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
				blocks[acc.idx].Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				acc.args += tc.Function.Arguments
				var parsed map[string]any
				_ = json.Unmarshal([]byte(acc.args), &parsed)
				if parsed != nil {
					blocks[acc.idx].Arguments = parsed
				}
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "toolcall_delta", ContentIndex: acc.idx, Delta: tc.Function.Arguments, Partial: cloneMsg(&partial)})
			}
		}
		return nil
	})
	if !started {
		msg := AssistantMessage(nil, model, StopError, time.Now().UnixMilli())
		msg.ErrorMessage = "empty completions stream"
		s.Push(AssistantMessageEvent{Type: "error", Reason: StopError, Error: &msg})
		return
	}
	if textIdx >= 0 {
		s.Push(AssistantMessageEvent{Type: "text_end", ContentIndex: textIdx, Content: blocks[textIdx].Text, Partial: cloneMsg(&partial)})
	}
	reason := StopStop
	for _, acc := range tools {
		var parsed map[string]any
		_ = json.Unmarshal([]byte(acc.args), &parsed)
		if parsed == nil {
			parsed = map[string]any{}
		}
		blocks[acc.idx].Arguments = parsed
		tc := blocks[acc.idx]
		partial.Content = EncodeContentBlocks(blocks)
		s.Push(AssistantMessageEvent{Type: "toolcall_end", ContentIndex: acc.idx, ToolCall: &tc, Partial: cloneMsg(&partial)})
		reason = StopToolUse
	}
	if finish == "length" {
		reason = StopLength
	}
	if finish == "content_filter" {
		reason = StopStop
	}
	partial.Content = EncodeContentBlocks(blocks)
	partial.StopReason = reason
	s.Push(AssistantMessageEvent{Type: "done", Reason: reason, Message: cloneMsg(&partial)})
}

func streamHTTP(ctx context.Context, model Model, req *http.Request, parse func(*EventStream, Model, io.Reader)) *EventStream {
	s := NewEventStream()
	go func() {
		defer s.End(nil)
		if ctx.Err() != nil {
			msg := AssistantMessage(nil, model, StopAborted, time.Now().UnixMilli())
			msg.ErrorMessage = "Request was aborted"
			s.Push(AssistantMessageEvent{Type: "error", Reason: StopAborted, Error: &msg})
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			reason := StopError
			if ctx.Err() != nil {
				reason = StopAborted
			}
			s.Push(AssistantMessageEvent{Type: "error", Reason: reason, Error: errMsg(model, err.Error(), reason)})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			s.Push(AssistantMessageEvent{Type: "error", Reason: StopError, Error: errMsg(model, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, b), StopError)})
			return
		}
		parse(s, model, resp.Body)
	}()
	return s
}

func errMsg(model Model, text string, reason StopReason) *Message {
	m := AssistantMessage(nil, model, reason, time.Now().UnixMilli())
	m.ErrorMessage = text
	return &m
}
