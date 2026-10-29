package session

import (
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
	thinking := "off"
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
	var msgs []ai.Message
	for i, e := range ctxEntries {
		if e.Type == "compaction" && i > 0 {
			continue
		}
		msgs = append(msgs, entryMessages(e)...)
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

func fileTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15-04-05-000Z")
}
