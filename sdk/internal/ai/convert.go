package ai

func openaiMessages(tr Transcript) (system string, msgs []map[string]any) {
	sys := GetCurrentSystemMessage(tr.Messages)
	if sys != nil {
		system = SystemMessageText(*sys)
	}
	for _, m := range tr.Messages {
		switch m.Role {
		case "system":
			continue
		case "user":
			msgs = append(msgs, map[string]any{"role": "user", "content": m.ContentString()})
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
			msgs = append(msgs, map[string]any{
				"role":         "tool",
				"tool_call_id": m.ToolCallID,
				"content":      m.ContentString(),
			})
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
	for _, m := range tr.Messages {
		switch m.Role {
		case "system":
			continue
		case "user":
			msgs = append(msgs, map[string]any{"role": "user", "content": m.ContentString()})
		case "assistant":
			var blocks []map[string]any
			for _, b := range m.ContentBlocks() {
				switch b.Type {
				case "text":
					blocks = append(blocks, map[string]any{"type": "text", "text": b.Text})
				case "thinking":
					blocks = append(blocks, map[string]any{"type": "thinking", "thinking": b.Thinking})
				case "toolCall":
					blocks = append(blocks, map[string]any{"type": "tool_use", "id": b.ID, "name": b.Name, "input": b.Arguments})
				}
			}
			msgs = append(msgs, map[string]any{"role": "assistant", "content": blocks})
		case "toolResult":
			msgs = append(msgs, map[string]any{
				"role": "user",
				"content": []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.ContentString(),
					"is_error":    m.IsError,
				}},
			})
		}
	}
	return system, msgs, tools
}
