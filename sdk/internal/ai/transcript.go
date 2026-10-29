package ai

import "encoding/json"

func NormalizeContext(systemPrompt string, tools []Tool, messages []Message) Transcript {
	var initial *Message
	hasPrompt := systemPrompt != ""
	hasTools := len(tools) > 0
	if hasPrompt || hasTools {
		m := Message{Role: "system", Timestamp: 0, Content: EncodeContentString(systemPrompt)}
		if hasTools {
			m.ToolsAdded = append([]Tool(nil), tools...)
		}
		initial = &m
	}
	out := make([]Message, 0, len(messages)+1)
	if initial != nil {
		out = append(out, *initial)
	}
	out = append(out, messages...)
	return Transcript{Messages: out}
}

func GetCurrentTools(messages []Message) []Tool {
	tools := map[string]Tool{}
	var order []string
	for _, m := range messages {
		if m.Role != "system" {
			continue
		}
		for _, r := range m.ToolsRemoved {
			delete(tools, r.Name)
		}
		for _, t := range m.ToolsAdded {
			if _, ok := tools[t.Name]; !ok {
				order = append(order, t.Name)
			}
			tools[t.Name] = t
		}
	}
	out := make([]Tool, 0, len(tools))
	seen := map[string]bool{}
	for _, name := range order {
		if t, ok := tools[name]; ok && !seen[name] {
			out = append(out, t)
			seen[name] = true
		}
	}
	for name, t := range tools {
		if !seen[name] {
			out = append(out, t)
		}
	}
	return out
}

func GetCurrentSystemMessage(messages []Message) *Message {
	var content []string
	sections := map[string]*string{}
	var ts int64
	var hasSys bool
	for _, m := range messages {
		if m.Role != "system" {
			continue
		}
		hasSys = true
		if ts == 0 {
			ts = m.Timestamp
		}
		if t := m.ContentString(); t != "" {
			content = append(content, t)
		}
		for name, v := range m.Sections {
			if v == nil {
				delete(sections, name)
			} else {
				cp := *v
				sections[name] = &cp
			}
		}
	}
	tools := GetCurrentTools(messages)
	if !hasSys && len(tools) == 0 {
		return nil
	}
	joined := ""
	for i, c := range content {
		if i > 0 {
			joined += "\n\n"
		}
		joined += c
	}
	out := Message{Role: "system", Timestamp: ts, Content: EncodeContentString(joined)}
	if len(sections) > 0 {
		out.Sections = sections
	}
	if len(tools) > 0 {
		out.ToolsAdded = tools
	}
	return &out
}

func ToolStateChanges(previous, current []Tool) (added []Tool, removed []ToolRef) {
	prev := map[string]Tool{}
	for _, t := range previous {
		prev[t.Name] = t
	}
	cur := map[string]Tool{}
	for _, t := range current {
		cur[t.Name] = t
	}
	for _, t := range current {
		old, ok := prev[t.Name]
		if !ok || !declarationsEqual(old, t) {
			added = append(added, t)
		}
	}
	for _, t := range previous {
		now, ok := cur[t.Name]
		if !ok || !declarationsEqual(t, now) {
			removed = append(removed, ToolRef{Name: t.Name})
		}
	}
	return added, removed
}

func declarationsEqual(a, b Tool) bool {
	if a.Name != b.Name || a.Description != b.Description {
		return false
	}
	return jsonEqual(a.Parameters, b.Parameters)
}

func jsonEqual(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		return string(a) == string(b)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		return false
	}
	ab, _ := json.Marshal(x)
	bb, _ := json.Marshal(y)
	return string(ab) == string(bb)
}
