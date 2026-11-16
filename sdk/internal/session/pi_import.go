package session

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ImportPi reads a Pi JSONL session and writes a nulo session in sessionDir.
// The importer preserves the message tree, model/thinking changes, compaction,
// branch summaries, labels and custom entries. Pi-only fields that nulo does
// not model are ignored; this is a one-way import, not a compatibility layer.
func ImportPi(src, sessionDir, cwd string) (*Manager, error) {
	abs, err := filepath.Abs(src)
	if err != nil {
		return nil, err
	}
	records, _, err := readSessionRecords(abs)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("empty Pi session: %s", abs)
	}

	var header Header
	if err := json.Unmarshal(records[0], &header); err != nil || header.Type != "session" {
		return nil, fmt.Errorf("not a Pi session: %s", abs)
	}
	version := header.Version
	if version == 0 {
		version = 1
	}

	entries := make([]Entry, 0, len(records)-1)
	for _, raw := range records[1:] {
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &probe) != nil || probe.Type == "" || probe.Type == "session" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	used := make(map[string]Entry, len(entries))
	for i := range entries {
		if entries[i].ID == "" {
			entries[i].ID = generateID(used)
		}
		used[entries[i].ID] = entries[i]
	}
	if version < 2 {
		previous := ""
		for i := range entries {
			entries[i].ParentID = parentPtr(previous)
			previous = entries[i].ID
		}
	}
	for i := range entries {
		if entries[i].Type == "message" && entries[i].Message != nil && entries[i].Message.Role == "hookMessage" {
			entries[i].Message.Role = "custom"
		}
		if entries[i].Timestamp == "" {
			entries[i].Timestamp = isoTime(time.Now())
		}
		// Drop dangling parents so every entry belongs to a real tree.
		if entries[i].ParentID != nil {
			if _, ok := used[*entries[i].ParentID]; !ok {
				entries[i].ParentID = nil
			}
		}
	}
	// Pi v1 compaction used a one-based file index instead of an entry ID.
	for i := range entries {
		if entries[i].Type != "compaction" || entries[i].FirstKeptEntryID != "" || len(entries[i].FirstKeptEntryIndex) == 0 {
			continue
		}
		raw := strings.TrimSpace(string(entries[i].FirstKeptEntryIndex))
		if raw == "null" || strings.HasPrefix(raw, "\"") {
			continue
		}
		var number float64
		if json.Unmarshal(entries[i].FirstKeptEntryIndex, &number) != nil {
			continue
		}
		index := int(number) - 1 // file index includes the header
		if float64(index+1) == number && index >= 0 && index < len(entries) {
			entries[i].FirstKeptEntryID = entries[index].ID
		}
		entries[i].FirstKeptEntryIndex = nil
	}

	importCWD := cwd
	if importCWD == "" {
		importCWD = header.CWD
	}
	m := newManager(importCWD, sessionDir, true, "")
	m.header.ImportedFrom = abs
	m.entries = entries
	m.leafID = ""
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].ID != "" {
			m.leafID = entries[i].ID
			break
		}
	}
	if m.file == "" {
		return nil, fmt.Errorf("import target directory is required")
	}
	if err := m.rewrite(); err != nil {
		return nil, err
	}
	m.flushed = true
	return m, nil
}
