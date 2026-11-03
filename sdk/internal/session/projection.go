package session

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

type Projection struct {
	Messages      []ai.Message
	ThinkingLevel string
	ModelProvider string
	ModelID       string
}

func (m *Manager) Branch() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return buildPath(m.entries, m.leafID)
}

func buildPath(entries []Entry, leafID string) []Entry {
	byID := map[string]Entry{}
	for _, e := range entries {
		if e.Type == "session" {
			continue
		}
		byID[e.ID] = e
	}
	if leafID == "" {
		return nil
	}
	var path []Entry
	cur, ok := byID[leafID]
	for ok {
		path = append(path, cur)
		pid := parentID(cur)
		if pid == "" {
			break
		}
		cur, ok = byID[pid]
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

func (m *Manager) Project() Projection {
	m.mu.Lock()
	path := buildPath(m.entries, m.leafID)
	m.mu.Unlock()
	thinking := ""
	var provider, modelID string
	for _, e := range path {
		switch e.Type {
		case "thinking_level_change":
			thinking = e.ThinkingLevel
		case "model_change":
			provider, modelID = e.Provider, e.ModelID
		case "message":
			if e.Message != nil && e.Message.Role == "assistant" {
				provider, modelID = e.Message.Provider, e.Message.Model
			}
		}
	}
	ctxEntries := contextEntries(path)
	edits := map[string]Entry{}
	for _, e := range ctxEntries {
		if e.Type == "context_edit" && e.TargetID != "" {
			edits[e.TargetID] = e
		}
	}
	var msgs []ai.Message
	for i, e := range ctxEntries {
		switch e.Type {
		case "context_edit":
			continue
		case "compaction":
			// Only the newest compaction, first in the context, contributes
			// a checkpoint; an older one retained in the kept range is state.
			if i > 0 {
				continue
			}
		}
		messages := entryMessages(e)
		if edit, ok := edits[e.ID]; ok {
			messages = applyContextEdit(messages, edit)
		}
		for j := range messages {
			if messages[j].Content == nil {
				messages[j].Content = ai.EncodeContentBlocks(nil)
			}
		}
		msgs = append(msgs, messages...)
	}
	return Projection{Messages: msgs, ThinkingLevel: thinking, ModelProvider: provider, ModelID: modelID}
}

func contextEntries(path []Entry) []Entry {
	var compaction *Entry
	for i := range path {
		if path[i].Type == "compaction" {
			compaction = &path[i]
		}
	}
	if compaction == nil {
		return path
	}
	idx := -1
	for i := range path {
		if path[i].ID == compaction.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return path
	}
	out := []Entry{*compaction}
	found := false
	for i := 0; i < idx; i++ {
		e := path[i]
		if e.ID == compaction.FirstKeptEntryID {
			found = true
		}
		if found && !(e.Type == "message" && e.Message != nil && e.Message.Role == "system") {
			out = append(out, e)
		}
	}
	out = append(out, path[idx+1:]...)
	return out
}

func entryMessages(e Entry) []ai.Message {
	switch e.Type {
	case "message":
		if e.Message == nil {
			return nil
		}
		return []ai.Message{*e.Message}
	case "custom_message":
		content := e.Content
		if content == nil {
			content = ai.EncodeContentBlocks(nil)
		}
		message := ai.Message{Role: "custom", Timestamp: parseISO(e.Timestamp).UnixMilli(), Content: content, CustomType: e.CustomType, Details: e.Details}
		if e.Display != nil {
			message.Display = *e.Display
		}
		return []ai.Message{message}
	case "branch_summary":
		if e.Summary == "" {
			return nil
		}
		return []ai.Message{branchSummaryMessage(e.Summary, parseISO(e.Timestamp).UnixMilli())}
	case "compaction":
		ts := parseISO(e.Timestamp).UnixMilli()
		sum := compactionMessage(e.Summary, ts)
		if e.SystemMessage != nil {
			return []ai.Message{*e.SystemMessage, sum}
		}
		return []ai.Message{sum}
	default:
		return nil
	}
}

// applyContextEdit replaces the content of the messages a context_edit
// targets, or removes them when the replacement is null.
func applyContextEdit(messages []ai.Message, edit Entry) []ai.Message {
	if edit.Replacement == nil {
		return nil
	}
	content := edit.Replacement.Content
	out := make([]ai.Message, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "user", "custom":
			message.Content = content
		case "assistant", "toolResult":
			if isJSONString(content) {
				var text string
				if json.Unmarshal(content, &text) == nil {
					content = ai.EncodeContentBlocks([]ai.ContentBlock{{Type: "text", Text: text}})
				}
			}
			message.Content = content
		default:
			out = append(out, message)
			continue
		}
		out = append(out, message)
	}
	return out
}

func isJSONString(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(trimmed) > 0 && trimmed[0] == '"'
}

func fileTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15-04-05-000Z")
}
