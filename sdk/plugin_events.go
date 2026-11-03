package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// pluginInput runs Pi's input event before prompt expansion. The returned text
// is what the agent will actually receive.
func (s *Session) pluginInput(ctx context.Context, text, source, streamingBehavior string) (string, bool) {
	data := map[string]any{
		"type":              "input",
		"text":              text,
		"source":            source,
		"streamingBehavior": streamingBehavior,
	}
	result := s.emitPlugin(ctx, "input", data)
	if updated, ok := data["text"].(string); ok {
		text = updated
	}
	switch fmt.Sprint(result["action"]) {
	case "handled":
		return "", false
	case "transform":
		if transformed, ok := result["text"].(string); ok {
			text = transformed
		}
	}
	return text, true
}

func (s *Session) pluginBeforeAgentStart(ctx context.Context, prompt string) {
	data := map[string]any{
		"type":         "before_agent_start",
		"messageType":  "user",
		"prompt":       prompt,
		"systemPrompt": s.currentSystemPrompt(),
	}
	result := s.emitPlugin(ctx, "before_agent_start", data)
	if system, ok := result["systemPrompt"].(string); ok && system != "" {
		s.mu.Lock()
		s.pluginSystem = system
		s.mu.Unlock()
	}
	if message, ok := result["message"].(map[string]any); ok {
		customType, _ := message["customType"].(string)
		content, err := json.Marshal(message["content"])
		if err == nil && customType != "" {
			display, _ := message["display"].(bool)
			details, _ := json.Marshal(message["details"])
			_, _ = s.manager.AppendCustomMessage(customType, content, display, details)
		}
	}
}

func (s *Session) currentSystemPrompt() string {
	if s.manager == nil {
		return ""
	}
	sys := sessionSys(s.manager.Project())
	if sys == nil {
		return ""
	}
	return ai.MessageText(*sys)
}

// pluginPrepareRequest applies the context and context_with_system interceptors.
func (s *Session) pluginPrepareRequest(ctx context.Context, messages []ai.Message) []ai.Message {
	if s.services == nil || s.services.plugins == nil {
		return messages
	}
	// context sees the conversation without system messages, as Pi does.
	nonSystem := make([]ai.Message, 0, len(messages))
	systemMessages := make([]ai.Message, 0, 2)
	for _, message := range messages {
		if message.Role == "system" {
			systemMessages = append(systemMessages, message)
		} else {
			nonSystem = append(nonSystem, message)
		}
	}
	contextData := map[string]any{"type": "context", "messages": messagesJSON(nonSystem)}
	contextResult := s.emitPlugin(ctx, "context", contextData)
	if patched, ok := contextResult["messages"].([]any); ok {
		if decoded, err := messagesFromJSON(patched); err == nil {
			nonSystem = decoded
		}
	}
	messages = append(append([]ai.Message{}, systemMessages...), nonSystem...)
	fullData := map[string]any{"type": "context_with_system", "messages": messagesJSON(messages)}
	fullResult := s.emitPlugin(ctx, "context_with_system", fullData)
	if patched, ok := fullResult["messages"].([]any); ok {
		if decoded, err := messagesFromJSON(patched); err == nil {
			messages = decoded
		}
	}
	s.mu.Lock()
	override := s.pluginSystem
	s.mu.Unlock()
	if override != "" {
		messages = replaceSystemPrompt(messages, override)
	}
	return messages
}

func replaceSystemPrompt(messages []ai.Message, prompt string) []ai.Message {
	out := append([]ai.Message(nil), messages...)
	seen := false
	for i := range out {
		if out[i].Role == "system" {
			out[i] = ai.SystemMessage(prompt, out[i].Timestamp)
			seen = true
			break
		}
	}
	if !seen {
		out = append([]ai.Message{ai.SystemMessage(prompt, time.Now().UnixMilli())}, out...)
	}
	return out
}

func messagesJSON(messages []ai.Message) []any {
	raw, _ := json.Marshal(messages)
	var out []any
	_ = json.Unmarshal(raw, &out)
	return out
}

func messagesFromJSON(values []any) ([]ai.Message, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	var out []ai.Message
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Session) pluginBeforeToolCall(ctx context.Context, assistant ai.Message, call ai.ContentBlock, args map[string]any) (*agent.BeforeToolCallResult, error) {
	before, _ := json.Marshal(args)
	data := map[string]any{
		"type":       "tool_call",
		"toolCallId": call.ID,
		"toolName":   call.Name,
		"input":      args,
	}
	result := s.emitPlugin(ctx, "tool_call", data)
	after, _ := json.Marshal(data["input"])
	if string(before) != string(after) {
		if input, ok := data["input"].(map[string]any); ok {
			replaceMap(args, input)
		}
	}
	blocked, _ := result["block"].(bool)
	if !blocked {
		return nil, nil
	}
	reason, _ := result["reason"].(string)
	terminate, _ := result["terminate"].(bool)
	return &agent.BeforeToolCallResult{Block: true, Reason: reason, Terminate: terminate}, nil
}

func (s *Session) pluginAfterToolCall(ctx context.Context, assistant ai.Message, call ai.ContentBlock, args map[string]any, result agent.ToolResult, isError bool) (*agent.AfterToolCallResult, error) {
	content, _ := json.Marshal(result.Content)
	var contentRaw []any
	_ = json.Unmarshal(content, &contentRaw)
	data := map[string]any{
		"type":       "tool_result",
		"toolCallId": call.ID,
		"toolName":   call.Name,
		"input":      args,
		"content":    contentRaw,
		"details":    result.Details,
		"isError":    isError,
	}
	patch := s.emitPlugin(ctx, "tool_result", data)
	out := &agent.AfterToolCallResult{}
	changed := false
	if content, ok := patch["content"].([]any); ok {
		raw, _ := json.Marshal(content)
		var blocks []ai.ContentBlock
		if json.Unmarshal(raw, &blocks) == nil {
			out.Content = blocks
			changed = true
		}
	}
	if details, ok := patch["details"]; ok {
		out.Details = details
		changed = true
	}
	if value, ok := patch["isError"].(bool); ok {
		out.IsError = &value
		changed = true
	}
	if !changed {
		return nil, nil
	}
	return out, nil
}

func replaceMap(dst, src map[string]any) {
	for key := range dst {
		delete(dst, key)
	}
	for key, value := range src {
		dst[key] = value
	}
}

// pluginOnAgent forwards the agent loop's events to plugins. It returns a
// replacement message for message_end when a handler supplied one.
func (s *Session) pluginOnAgent(event agent.Event) *ai.Message {
	if s.services == nil || s.services.plugins == nil {
		return nil
	}
	ctx := s.activeRunContext()
	data := map[string]any{"type": event.Type}
	var replacement *ai.Message
	switch event.Type {
	case "agent_start":
	case "agent_end", "agent_settled":
		data["messages"] = messagesJSON(event.Messages)
	case "turn_start":
		s.turnIndex++
		data["turnIndex"] = s.turnIndex
		data["timestamp"] = time.Now().UnixMilli()
	case "turn_end":
		if event.Message != nil {
			data["message"] = messageJSON(*event.Message)
		}
		data["toolResults"] = messagesJSON(event.ToolResults)
		data["turnIndex"] = s.turnIndex
		data["entries"] = []any{}
		data["continue"] = false
		data["outcome"] = "completed"
	case "message_start", "message_end":
		if event.Message == nil {
			return nil
		}
		data["message"] = messageJSON(*event.Message)
	case "message_update":
		if event.Message == nil {
			return nil
		}
		data["message"] = messageJSON(*event.Message)
		data["assistantMessageEvent"] = event.AssistantMessageEvent
	case "tool_execution_start":
		data["toolCallId"] = event.ToolCallID
		data["toolName"] = event.ToolName
		data["args"] = event.Args
	case "tool_execution_end":
		data["toolCallId"] = event.ToolCallID
		data["toolName"] = event.ToolName
		data["result"] = event.Result
		data["isError"] = event.IsError
		if event.DurationMs != nil {
			data["durationMs"] = *event.DurationMs
		}
	}
	result := s.emitPlugin(ctx, event.Type, data)
	if event.Type == "turn_end" {
		s.applyBoundaryEntries(result)
	}
	if event.Type == "message_end" {
		if value, ok := data["message"].(map[string]any); ok {
			var message ai.Message
			raw, _ := json.Marshal(value)
			if json.Unmarshal(raw, &message) == nil && message.Role == event.Message.Role {
				if !sameMessage(*event.Message, message) {
					replacement = &message
				}
			}
		}
	}
	return replacement
}

func (s *Session) activeRunContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runCtx != nil {
		return s.runCtx
	}
	return context.Background()
}

func messageJSON(message ai.Message) map[string]any {
	raw, _ := json.Marshal(message)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func sameMessage(a, b ai.Message) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

// pluginStreamOptions installs the provider-boundary hooks.
func (s *Session) pluginStreamOptions() ai.StreamOptions {
	if s.services == nil || s.services.plugins == nil {
		return ai.StreamOptions{}
	}
	return ai.StreamOptions{
		OnPayload: func(payload any) any {
			data := map[string]any{"type": "before_provider_request", "payload": payload}
			result := s.emitPlugin(s.activeRunContext(), "before_provider_request", data)
			if replacement, ok := result["payload"]; ok {
				return replacement
			}
			return data["payload"]
		},
		OnHeaders: func(headers map[string]string) {
			data := map[string]any{"type": "before_provider_headers", "headers": headers}
			result := s.emitPlugin(s.activeRunContext(), "before_provider_headers", data)
			if updated, ok := result["headers"].(map[string]any); ok {
				for key := range headers {
					delete(headers, key)
				}
				for key, value := range updated {
					headers[key] = fmt.Sprint(value)
				}
			}
		},
		OnResponse: func(status int, headers map[string]string) {
			s.emitPlugin(s.activeRunContext(), "after_provider_response", map[string]any{
				"type": "after_provider_response", "status": status, "headers": headers,
			})
		},
		OnStreamEvent: func(data any) {
			model := s.Model()
			s.emitPlugin(s.activeRunContext(), "provider_stream_event", map[string]any{
				"type": "provider_stream_event", "provider": model.Provider,
				"api": model.API, "model": model.ID, "data": data,
			})
		},
	}
}

// applyBoundaryEntries persists the drafts a turn_end or agent_before_settle
// handler returned. Pi's continue flag needs another provider request; nulo's
// loop is already settling, so the drafts are applied and the run ends.
func (s *Session) applyBoundaryEntries(result map[string]any) {
	entries, _ := result["entries"].([]any)
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch fmt.Sprint(entry["type"]) {
		case "custom":
			data, _ := json.Marshal(entry["data"])
			_, _ = s.manager.AppendCustom(fmt.Sprint(entry["customType"]), data)
		case "custom_message":
			content, _ := json.Marshal(entry["content"])
			details, _ := json.Marshal(entry["details"])
			display, _ := entry["display"].(bool)
			_, _ = s.manager.AppendCustomMessage(fmt.Sprint(entry["customType"]), content, display, details)
		}
	}
}

// pluginBeforeSettle emits Pi's final settlement boundary before agent_settled.
func (s *Session) pluginBeforeSettle(messages []ai.Message) {
	data := map[string]any{
		"type":     "agent_before_settle",
		"entries":  []any{},
		"continue": false,
		"outcome":  "completed",
		"context": map[string]any{
			"contextMessages": messagesJSON(messages),
		},
	}
	result := s.emitPlugin(s.activeRunContext(), "agent_before_settle", data)
	s.applyBoundaryEntries(result)
}
