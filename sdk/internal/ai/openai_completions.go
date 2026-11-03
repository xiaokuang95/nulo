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
	key, err := LookupAPIKey(model, "", opts)
	if err != nil {
		return errorStream(model, err.Error(), StopError)
	}
	if key == "" {
		return errorStream(model, "provider is not configured: "+model.Provider, StopError)
	}
	tr = withResizedImages(tr, model.InputLimits)
	system, msgs := openaiMessagesWith(tr, openaiMessageOptions{ToolResultName: model.Compat.ToolResultName()})
	if system != "" {
		role := "system"
		if model.Compat.DeveloperRole(false) {
			role = "developer"
		}
		msgs = append([]map[string]any{{"role": role, "content": system}}, msgs...)
	}
	body := map[string]any{
		"model":    model.ID,
		"messages": msgs,
		"stream":   true,
	}
	// Without this the provider sends no usage at all on a streamed response.
	if model.Compat.UsageInStreaming() {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	if tools := openaiTools(GetCurrentTools(tr.Messages)); len(tools) > 0 {
		body["tools"] = tools
	}
	if opts.MaxTokens > 0 {
		body[model.Compat.TokensField()] = opts.MaxTokens
	}
	if model.Compat.Store() {
		body["store"] = false
	}
	applyCompletionsThinking(body, model, opts)
	applySamplingParams(body, model, opts)
	if opts.OnPayload != nil {
		if replacement := opts.OnPayload(body); replacement != nil {
			if updated, ok := replacement.(map[string]any); ok {
				body = updated
			}
		}
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
	headers, err := RequestHeaders(model, opts)
	if err != nil {
		return errorStream(model, err.Error(), StopError)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if model.AuthHeader {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if opts.OnHeaders != nil {
		headers := headerMap(req.Header)
		opts.OnHeaders(headers)
		applyHeaderMap(req.Header, headers)
	}
	return streamHTTP(ctx, model, req, opts, parseOpenAICompletions)
}

type oaiDelta struct {
	Choices []oaiChoice     `json:"choices"`
	Usage   *oaiUsageFields `json:"usage"`
}

type oaiChoice struct {
	Delta        oaiDeltaBody `json:"delta"`
	FinishReason string       `json:"finish_reason"`
	// Some providers (Moonshot, for one) report usage here instead of on the chunk.
	Usage *oaiUsageFields `json:"usage"`
}

type oaiDeltaBody struct {
	Content string `json:"content"`
	// Reasoning models disagree on the field, and some send a list of parts.
	ReasoningContent string          `json:"reasoning_content"`
	Reasoning        json.RawMessage `json:"reasoning"`
	ToolCalls        []struct {
		Index    int    `json:"index"`
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

func parseOpenAICompletions(s *EventStream, model Model, r io.Reader, opts StreamOptions) {
	partial := AssistantMessage(nil, model, StopPending, time.Now().UnixMilli())
	blocks := []ContentBlock{}
	textIdx := -1
	thinkIdx := -1
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
		if opts.OnStreamEvent != nil {
			var eventData any
			_ = json.Unmarshal([]byte(data), &eventData)
			opts.OnStreamEvent(eventData)
		}
		if !started {
			started = true
			s.Push(AssistantMessageEvent{Type: "start", Partial: cloneMsg(&partial)})
		}
		// Usage arrives on its own final chunk, which carries no choices, so it has to
		// be read before bailing out on an empty choice list.
		if d.Usage != nil {
			partial.Usage = parseChunkUsage(*d.Usage, model)
		}
		if len(d.Choices) == 0 {
			return nil
		}
		ch := d.Choices[0]
		if d.Usage == nil && ch.Usage != nil {
			partial.Usage = parseChunkUsage(*ch.Usage, model)
		}
		if ch.FinishReason != "" {
			finish = ch.FinishReason
		}
		if reasoning := reasoningDelta(ch.Delta); reasoning != "" {
			if thinkIdx < 0 {
				thinkIdx = len(blocks)
				blocks = append(blocks, ContentBlock{Type: "thinking"})
				partial.Content = EncodeContentBlocks(blocks)
				s.Push(AssistantMessageEvent{Type: "thinking_start", ContentIndex: thinkIdx, Partial: cloneMsg(&partial)})
			}
			blocks[thinkIdx].Thinking += reasoning
			partial.Content = EncodeContentBlocks(blocks)
			s.Push(AssistantMessageEvent{Type: "thinking_delta", ContentIndex: thinkIdx, Delta: reasoning, Partial: cloneMsg(&partial)})
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
	if thinkIdx >= 0 {
		s.Push(AssistantMessageEvent{Type: "thinking_end", ContentIndex: thinkIdx, Content: blocks[thinkIdx].Thinking, Partial: cloneMsg(&partial)})
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

// reasoningDelta reads the reasoning text of a chat chunk: a string, or a list of
// parts that carries it in "text".
func reasoningDelta(d oaiDeltaBody) string {
	if d.ReasoningContent != "" {
		return d.ReasoningContent
	}
	if len(d.Reasoning) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(d.Reasoning, &text) == nil {
		return text
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(d.Reasoning, &parts) == nil {
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(part.Text)
		}
		return b.String()
	}
	return ""
}

func streamHTTP(ctx context.Context, model Model, req *http.Request, opts StreamOptions, parse func(*EventStream, Model, io.Reader, StreamOptions)) *EventStream {
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
		if opts.OnResponse != nil {
			opts.OnResponse(resp.StatusCode, headerMap(resp.Header))
		}
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			s.Push(AssistantMessageEvent{Type: "error", Reason: StopError, Error: errMsg(model, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, b), StopError)})
			return
		}
		parse(s, model, resp.Body, opts)
	}()
	return s
}

func errMsg(model Model, text string, reason StopReason) *Message {
	m := AssistantMessage(nil, model, reason, time.Now().UnixMilli())
	m.ErrorMessage = text
	return &m
}

func headerMap(header http.Header) map[string]string {
	out := make(map[string]string, len(header))
	for key, values := range header {
		out[key] = strings.Join(values, ", ")
	}
	return out
}

func applyHeaderMap(header http.Header, values map[string]string) {
	for key := range header {
		if _, ok := values[key]; !ok {
			header.Del(key)
		}
	}
	for key, value := range values {
		if value == "" {
			header.Del(key)
			continue
		}
		header.Set(key, value)
	}
}
