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

// minResponsesOutputTokens is OpenAI's floor for max_output_tokens; smaller values
// are rejected outright.
const minResponsesOutputTokens = 16

func StreamOpenAIResponses(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
	key, err := LookupAPIKey(model, "", opts)
	if err != nil {
		return errorStream(model, err.Error(), StopError)
	}
	if key == "" {
		return errorStream(model, "provider is not configured: "+model.Provider, StopError)
	}
	tr = withResizedImages(tr, model.InputLimits)
	system, input := responsesInput(tr)
	if system != "" {
		role := "system"
		if model.Compat.DeveloperRole(true) {
			role = "developer"
		}
		input = append([]map[string]any{{"role": role, "content": system}}, input...)
	}
	body := map[string]any{
		"model":  model.ID,
		"input":  input,
		"stream": true,
		// Like Pi: do not keep the response on OpenAI's side.
		"store": false,
	}
	if tools := responsesTools(GetCurrentTools(tr.Messages)); len(tools) > 0 {
		body["tools"] = tools
	}
	// Unlike chat completions, this API accepts a cap, so honour the same option
	// the other two providers take. Some Codex-protocol gateways reject the field.
	if opts.MaxTokens > 0 && model.Compat.MaxOutputTokens() {
		body["max_output_tokens"] = max(opts.MaxTokens, minResponsesOutputTokens)
	}
	applyResponsesThinking(body, model, opts)
	applySamplingParams(body, model, opts)
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
	return streamHTTP(ctx, model, req, parseOpenAIResponses)
}

// responsesStream maps the item_id of a streamed output item to its block, so
// deltas reach the right call when the model emits several at once.
type responsesStream struct {
	s       *EventStream
	model   Model
	partial Message
	blocks  []ContentBlock
	started bool
	reason  StopReason

	textIdx  int
	thinkIdx int
	byItem   map[string]int
	finished map[int]bool
}

func (r *responsesStream) ensure() {
	if !r.started {
		r.started = true
		r.s.Push(AssistantMessageEvent{Type: "start", Partial: cloneMsg(&r.partial)})
	}
}

func (r *responsesStream) push() {
	r.partial.Content = EncodeContentBlocks(r.blocks)
}

func (r *responsesStream) textStart() {
	if r.textIdx >= 0 {
		return
	}
	r.textIdx = r.block(ContentBlock{Type: "text"})
	r.push()
	r.s.Push(AssistantMessageEvent{Type: "text_start", ContentIndex: r.textIdx, Partial: cloneMsg(&r.partial)})
}

func (r *responsesStream) thinkStart() {
	if r.thinkIdx >= 0 {
		return
	}
	r.thinkIdx = r.block(ContentBlock{Type: "thinking"})
	r.push()
	r.s.Push(AssistantMessageEvent{Type: "thinking_start", ContentIndex: r.thinkIdx, Partial: cloneMsg(&r.partial)})
}

func (r *responsesStream) block(b ContentBlock) int {
	r.blocks = append(r.blocks, b)
	return len(r.blocks) - 1
}

func parseOpenAIResponses(s *EventStream, model Model, rd io.Reader) {
	r := &responsesStream{
		s: s, model: model,
		partial:  AssistantMessage(nil, model, StopPending, time.Now().UnixMilli()),
		textIdx:  -1,
		thinkIdx: -1,
		byItem:   map[string]int{},
		finished: map[int]bool{},
		reason:   StopStop,
	}
	_ = readSSE(rd, func(_, data string) error {
		if data == "[DONE]" {
			return nil
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return nil
		}
		typ, _ := ev["type"].(string)
		itemID, _ := ev["item_id"].(string)
		switch typ {
		case "response.created", "response.in_progress":
			r.ensure()
		case "response.output_text.delta":
			r.ensure()
			delta, _ := ev["delta"].(string)
			r.textStart()
			r.blocks[r.textIdx].Text += delta
			r.push()
			s.Push(AssistantMessageEvent{Type: "text_delta", ContentIndex: r.textIdx, Delta: delta, Partial: cloneMsg(&r.partial)})
		case "response.refusal.delta":
			// A refusal is the assistant's text as far as the transcript is concerned.
			r.ensure()
			delta, _ := ev["delta"].(string)
			r.textStart()
			r.blocks[r.textIdx].Text += delta
			r.push()
			s.Push(AssistantMessageEvent{Type: "text_delta", ContentIndex: r.textIdx, Delta: delta, Partial: cloneMsg(&r.partial)})
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			r.ensure()
			delta, _ := ev["delta"].(string)
			r.thinkStart()
			r.blocks[r.thinkIdx].Thinking += delta
			r.push()
			s.Push(AssistantMessageEvent{Type: "thinking_delta", ContentIndex: r.thinkIdx, Delta: delta, Partial: cloneMsg(&r.partial)})
		case "response.output_item.added":
			r.ensure()
			item, _ := ev["item"].(map[string]any)
			it, _ := item["type"].(string)
			switch it {
			case "function_call":
				id, _ := item["call_id"].(string)
				if id == "" {
					id, _ = item["id"].(string)
				}
				name, _ := item["name"].(string)
				idx := r.block(ContentBlock{Type: "toolCall", ID: id, Name: name, Arguments: map[string]any{}})
				if raw, _ := item["id"].(string); raw != "" {
					r.byItem[raw] = idx
				}
				if callID, _ := item["call_id"].(string); callID != "" {
					r.byItem[callID] = idx
				}
				r.push()
				s.Push(AssistantMessageEvent{Type: "toolcall_start", ContentIndex: idx, Partial: cloneMsg(&r.partial)})
				r.reason = StopToolUse
			case "reasoning":
				// Keep the item as the block's signature so a later request replays
				// the encrypted reasoning instead of dropping it.
				r.thinkStart()
				if raw, err := json.Marshal(item); err == nil {
					r.blocks[r.thinkIdx].ThinkingSignature = string(raw)
				}
			}
		case "response.function_call_arguments.delta":
			delta, _ := ev["delta"].(string)
			idx, ok := r.byItem[itemID]
			if !ok {
				break
			}
			r.blocks[idx].ArgumentsError += delta
			var parsed map[string]any
			_ = json.Unmarshal([]byte(r.blocks[idx].ArgumentsError), &parsed)
			if parsed != nil {
				r.blocks[idx].Arguments = parsed
			}
			r.push()
			s.Push(AssistantMessageEvent{Type: "toolcall_delta", ContentIndex: idx, Delta: delta, Partial: cloneMsg(&r.partial)})
		case "response.function_call_arguments.done", "response.output_item.done":
			r.finishCall(itemID, ev)
		case "response.completed":
			r.ensure()
			if resp, ok := ev["response"].(map[string]any); ok {
				var f responsesUsageFields
				if decodeAs(resp["usage"], &f) {
					r.partial.Usage = f.toUsage(model)
				}
			}
		case "response.incomplete":
			r.reason = StopLength
		case "response.failed":
			r.reason = StopError
			if resp, ok := ev["response"].(map[string]any); ok {
				if e, ok := resp["error"].(map[string]any); ok {
					if msg, _ := e["message"].(string); msg != "" {
						r.partial.ErrorMessage = msg
					}
				}
			}
		}
		return nil
	})
	if !r.started {
		s.Push(AssistantMessageEvent{Type: "error", Reason: StopError, Error: errMsg(model, "empty responses stream", StopError)})
		return
	}
	if r.textIdx >= 0 {
		s.Push(AssistantMessageEvent{Type: "text_end", ContentIndex: r.textIdx, Content: r.blocks[r.textIdx].Text, Partial: cloneMsg(&r.partial)})
	}
	if r.thinkIdx >= 0 {
		s.Push(AssistantMessageEvent{Type: "thinking_end", ContentIndex: r.thinkIdx, Content: r.blocks[r.thinkIdx].Thinking, Partial: cloneMsg(&r.partial)})
	}
	for i := range r.blocks {
		if r.blocks[i].Type != "toolCall" || r.finished[i] {
			continue
		}
		// A call whose done event never arrived still has to be reported, or the
		// tool batch would drop it.
		tc := r.finalCall(i)
		r.push()
		s.Push(AssistantMessageEvent{Type: "toolcall_end", ContentIndex: i, ToolCall: &tc, Partial: cloneMsg(&r.partial)})
		r.reason = StopToolUse
	}
	r.push()
	r.partial.StopReason = r.reason
	if r.reason == StopError {
		s.Push(AssistantMessageEvent{Type: "error", Reason: r.reason, Error: cloneMsg(&r.partial)})
		return
	}
	s.Push(AssistantMessageEvent{Type: "done", Reason: r.reason, Message: cloneMsg(&r.partial)})
}

// finishCall settles one streamed function_call block: a done event may carry the
// final arguments, and the item id or call id names it.
func (r *responsesStream) finishCall(itemID string, ev map[string]any) {
	idx, ok := r.byItem[itemID]
	if !ok {
		// output_item.done names the item, not its item_id.
		if item, isItem := ev["item"].(map[string]any); isItem {
			if id, _ := item["id"].(string); id != "" {
				idx, ok = r.byItem[id]
			}
			if !ok {
				if id, _ := item["call_id"].(string); id != "" {
					idx, ok = r.byItem[id]
				}
			}
			if ok {
				if args, _ := item["arguments"].(string); args != "" {
					r.blocks[idx].ArgumentsError = args
				}
			}
		}
	}
	if !ok || r.finished[idx] {
		return
	}
	r.finished[idx] = true
	tc := r.finalCall(idx)
	r.push()
	r.s.Push(AssistantMessageEvent{Type: "toolcall_end", ContentIndex: idx, ToolCall: &tc, Partial: cloneMsg(&r.partial)})
	r.reason = StopToolUse
}

func (r *responsesStream) finalCall(idx int) ContentBlock {
	if r.blocks[idx].ArgumentsError != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(r.blocks[idx].ArgumentsError), &parsed); err != nil || parsed == nil {
			parsed = map[string]any{}
		}
		r.blocks[idx].Arguments = parsed
		r.blocks[idx].ArgumentsError = ""
	}
	if r.blocks[idx].Arguments == nil {
		r.blocks[idx].Arguments = map[string]any{}
	}
	return r.blocks[idx]
}
