package ai

import (
	"encoding/json"
	"strings"
)

// openaiMessageOptions carries the per-model compat that changes message shape.
type openaiMessageOptions struct {
	// ToolResultName adds the name field to tool result messages, which some
	// OpenAI-compatible providers require.
	ToolResultName bool
}

func openaiMessages(tr Transcript) (system string, msgs []map[string]any) {
	return openaiMessagesWith(tr, openaiMessageOptions{})
}

func openaiMessagesWith(tr Transcript, o openaiMessageOptions) (system string, msgs []map[string]any) {
	sys := GetCurrentSystemMessage(tr.Messages)
	if sys != nil {
		system = SystemMessageText(*sys)
	}
	for _, m := range tr.Messages {
		switch m.Role {
		case "system":
			continue
		case "user":
			msgs = append(msgs, map[string]any{"role": "user", "content": openaiContent(m)})
		case "assistant":
			om := map[string]any{"role": "assistant"}
			var text string
			var tools []map[string]any
			for _, b := range m.ContentBlocks() {
				switch b.Type {
				case "text":
					text += b.Text
				case "toolCall":
					args, _ := jsonBytes(b.Arguments)
					tools = append(tools, map[string]any{
						"id":   b.ID,
						"type": "function",
						"function": map[string]any{
							"name":      b.Name,
							"arguments": string(args),
						},
					})
				}
			}
			if text != "" {
				om["content"] = text
			}
			if len(tools) > 0 {
				om["tool_calls"] = tools
			}
			msgs = append(msgs, om)
		case "toolResult":
			result := map[string]any{
				"role":         "tool",
				"tool_call_id": m.ToolCallID,
				"content":      openaiContent(m),
			}
			if o.ToolResultName && m.ToolName != "" {
				result["name"] = m.ToolName
			}
			msgs = append(msgs, result)
		}
	}
	return system, msgs
}

func jsonBytes(v any) ([]byte, error) {
	return marshalJSON(v)
}

func openaiTools(tools []Tool) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		var params any
		if len(t.Parameters) > 0 {
			_ = unmarshalJSON(t.Parameters, &params)
		}
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			},
		})
	}
	return out
}

func anthropicPayload(tr Transcript) (system string, msgs []map[string]any, tools []map[string]any) {
	sys := GetCurrentSystemMessage(tr.Messages)
	if sys != nil {
		system = SystemMessageText(*sys)
		for _, t := range sys.ToolsAdded {
			var schema any
			if len(t.Parameters) > 0 {
				_ = unmarshalJSON(t.Parameters, &schema)
			}
			if schema == nil {
				schema = map[string]any{"type": "object"}
			}
			tools = append(tools, map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": schema,
			})
		}
	}
	for i := 0; i < len(tr.Messages); i++ {
		m := tr.Messages[i]
		switch m.Role {
		case "system":
			continue
		case "user":
			msgs = append(msgs, map[string]any{"role": "user", "content": anthropicContent(m, false)})
		case "assistant":
			var blocks []map[string]any
			for _, b := range m.ContentBlocks() {
				switch b.Type {
				case "text":
					blocks = append(blocks, map[string]any{"type": "text", "text": b.Text})
				case "thinking":
					if b.Redacted {
						blocks = append(blocks, map[string]any{"type": "redacted_thinking", "data": b.ThinkingSignature})
						continue
					}
					if b.Thinking == "" && b.ThinkingSignature == "" {
						continue
					}
					thinking := map[string]any{"type": "thinking", "thinking": b.Thinking}
					if b.ThinkingSignature != "" {
						thinking["signature"] = b.ThinkingSignature
					}
					blocks = append(blocks, thinking)
				case "toolCall":
					blocks = append(blocks, map[string]any{"type": "tool_use", "id": b.ID, "name": b.Name, "input": b.Arguments})
				}
			}
			msgs = append(msgs, map[string]any{"role": "assistant", "content": blocks})
		case "toolResult":
			// Anthropic wants the answers to one assistant turn together, so
			// consecutive results share one user message.
			results := make([]map[string]any, 0, 2)
			for ; i < len(tr.Messages) && tr.Messages[i].Role == "toolResult"; i++ {
				r := tr.Messages[i]
				results = append(results, map[string]any{
					"type":        "tool_result",
					"tool_use_id": r.ToolCallID,
					"content":     anthropicContent(r, true),
					"is_error":    r.IsError,
				})
			}
			i--
			msgs = append(msgs, map[string]any{"role": "user", "content": results})
		}
	}
	return system, msgs, tools
}

// responsesInput maps the transcript to the Responses API input array. Unlike
// chat completions, an assistant's tool call and a tool result are items of
// their own, and a result refers to the call by call_id.
func responsesInput(tr Transcript) (system string, input []map[string]any) {
	if sys := GetCurrentSystemMessage(tr.Messages); sys != nil {
		system = SystemMessageText(*sys)
	}
	for _, m := range tr.Messages {
		switch m.Role {
		case "system":
			continue
		case "user":
			var parts []map[string]any
			for _, b := range m.ContentBlocks() {
				switch b.Type {
				case "text":
					parts = append(parts, map[string]any{"type": "input_text", "text": b.Text})
				case "image":
					parts = append(parts, map[string]any{
						"type":      "input_image",
						"image_url": "data:" + b.MIMEType + ";base64," + b.Data,
					})
				}
			}
			if len(parts) == 0 {
				parts = []map[string]any{{"type": "input_text", "text": m.ContentString()}}
			}
			input = append(input, map[string]any{"role": "user", "content": parts})
		case "assistant":
			for _, b := range m.ContentBlocks() {
				switch b.Type {
				case "thinking":
					// A reasoning item round-trips verbatim; its signature holds the item.
					if b.ThinkingSignature == "" {
						continue
					}
					var item map[string]any
					if json.Unmarshal([]byte(b.ThinkingSignature), &item) == nil && item["type"] == "reasoning" {
						input = append(input, item)
					}
				case "text":
					if b.Text == "" {
						continue
					}
					input = append(input, map[string]any{
						"type": "message", "role": "assistant", "status": "completed",
						"content": []map[string]any{{"type": "output_text", "text": b.Text, "annotations": []any{}}},
					})
				case "toolCall":
					args, _ := jsonBytes(b.Arguments)
					input = append(input, map[string]any{
						"type": "function_call", "call_id": responsesCallID(b.ID), "name": b.Name,
						"arguments": string(args),
					})
				}
			}
		case "toolResult":
			input = append(input, map[string]any{
				"type": "function_call_output", "call_id": responsesCallID(m.ToolCallID),
				"output": responsesToolOutput(m),
			})
		}
	}
	return system, input
}

// responsesToolOutput is a tool result's output: text, or text and image parts
// when the tool returned images.
func responsesToolOutput(m Message) any {
	blocks := m.ContentBlocks()
	if !hasImage(blocks) {
		return m.ContentString()
	}
	parts := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				parts = append(parts, map[string]any{"type": "input_text", "text": b.Text})
			}
		case "image":
			parts = append(parts, map[string]any{"type": "input_image", "image_url": dataURI(b)})
		}
	}
	if len(parts) == 0 {
		return m.ContentString()
	}
	return parts
}

// responsesCallID drops the item-id half of the "call|item" ids OpenAI Responses
// streams and keeps only the call id the API matches results against.
func responsesCallID(id string) string {
	if before, _, ok := strings.Cut(id, "|"); ok {
		return before
	}
	return id
}

// responsesTools maps tools to the Responses API shape, which is flat rather
// than nested under "function".
func responsesTools(tools []Tool) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		var params any
		if len(t.Parameters) > 0 {
			_ = unmarshalJSON(t.Parameters, &params)
		}
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  params,
		})
	}
	return out
}

// dataURI is an image block as a data URL, the form both OpenAI APIs accept.
func dataURI(b ContentBlock) string {
	return "data:" + b.MIMEType + ";base64," + b.Data
}

func hasImage(blocks []ContentBlock) bool {
	for _, b := range blocks {
		if b.Type == "image" && b.Data != "" {
			return true
		}
	}
	return false
}

// openaiContent is a user or tool-result content value for chat completions: a
// plain string when nothing is an image, else text and image_url parts.
func openaiContent(m Message) any {
	blocks := m.ContentBlocks()
	if !hasImage(blocks) {
		return m.ContentString()
	}
	parts := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				parts = append(parts, map[string]any{"type": "text", "text": b.Text})
			}
		case "image":
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": dataURI(b)},
			})
		}
	}
	if len(parts) == 0 {
		return m.ContentString()
	}
	return parts
}

// anthropicContent is a message's content for Anthropic: a string, or a block
// list when it carries images. A tool result with only images gets a text block
// first, which the API requires.
func anthropicContent(m Message, toolResult bool) any {
	blocks := m.ContentBlocks()
	if !hasImage(blocks) {
		return m.ContentString()
	}
	out := make([]map[string]any, 0, len(blocks)+1)
	text := false
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				out = append(out, map[string]any{"type": "text", "text": b.Text})
				text = true
			}
		case "image":
			out = append(out, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type": "base64", "media_type": b.MIMEType, "data": b.Data,
				},
			})
		}
	}
	if toolResult && !text {
		out = append([]map[string]any{{"type": "text", "text": "(see attached image)"}}, out...)
	}
	if len(out) == 0 {
		return m.ContentString()
	}
	return out
}
