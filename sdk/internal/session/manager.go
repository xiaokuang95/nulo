package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

type Manager struct {
	mu         sync.Mutex
	cwd        string
	sessionDir string
	persist    bool
	sessionID  string
	file       string
	header     Header
	entries    []Entry // tree entries only (no header)
	leafID     string
	flushed    bool
}

func InMemory(cwd string) *Manager {
	return newManager(cwd, "", false, "")
}

func Create(cwd, sessionDir string) *Manager {
	return newManager(cwd, sessionDir, true, "")
}

func Open(path string) (*Manager, error) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	records, terminated, err := readSessionRecords(path)
	if err != nil {
		return nil, err
	}
	var header Header
	var entries []Entry
	for _, raw := range records {
		if header.Type == "" {
			var probe struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(raw, &probe) != nil || probe.Type != "session" {
				return nil, fmt.Errorf("Session file is not a valid pi session: %s", path)
			}
			if err := json.Unmarshal(raw, &header); err != nil {
				return nil, fmt.Errorf("session header: %w", err)
			}
			continue
		}
		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		if e.Type == "session" {
			continue
		}
		entries = append(entries, e)
	}
	if header.Type == "" {
		return nil, fmt.Errorf("empty session file: %s", path)
	}
	migrated := migrateEntries(&header, entries)
	m := &Manager{
		cwd:        header.CWD,
		sessionDir: filepath.Dir(path),
		persist:    true,
		sessionID:  header.ID,
		file:       path,
		header:     header,
		entries:    entries,
		flushed:    true,
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].ID != "" {
			m.leafID = entries[i].ID
			break
		}
	}
	if migrated {
		if err := m.rewrite(); err != nil {
			return nil, err
		}
		// The rewrite always ends every record with a newline.
		terminated = true
	}
	if !terminated {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		if _, err := f.WriteString("\n"); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// readSessionRecords returns the JSON records of a session file, one per line.
// Malformed lines are skipped, as Pi's parseSessionEntries does, and lines have
// no length limit. terminated reports that the file ends with a newline.
func readSessionRecords(path string) ([]json.RawMessage, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, 64*1024)
	var records []json.RawMessage
	terminated := true
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			terminated = line[len(line)-1] == '\n'
			line = bytes.TrimSuffix(line, []byte("\n"))
			line = bytes.TrimSuffix(line, []byte("\r"))
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed) {
				records = append(records, append(json.RawMessage(nil), trimmed...))
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, false, readErr
		}
	}
	return records, terminated, nil
}

// migrateEntries applies Pi's v1 and v2 session migrations in memory:
// version 1 entries get fresh IDs and a linear parent chain, and a
// compaction's firstKeptEntryIndex becomes firstKeptEntryId; version 2's
// hookMessage role becomes custom.
func migrateEntries(header *Header, entries []Entry) bool {
	version := header.Version
	if version == 0 {
		version = 1
	}
	if version >= CurrentVersion {
		return false
	}
	if version < 2 {
		used := map[string]Entry{}
		for i := range entries {
			id := generateID(used)
			used[id] = entries[i]
			entries[i].ID = id
		}
		previous := ""
		for i := range entries {
			entries[i].ParentID = parentPtr(previous)
			previous = entries[i].ID
			if entries[i].Type != "compaction" || len(entries[i].FirstKeptEntryIndex) == 0 {
				continue
			}
			rawIndex := strings.TrimSpace(string(entries[i].FirstKeptEntryIndex))
			// Pi only migrates a numeric index; a string or null is left
			// alone, and an out-of-range number just drops the member.
			if rawIndex != "null" && !strings.HasPrefix(rawIndex, "\"") {
				var number float64
				if json.Unmarshal(entries[i].FirstKeptEntryIndex, &number) == nil {
					// Pi indexes the file's records including the header;
					// entries here start after it.
					index := int(number) - 1
					if float64(index+1) == number && index >= 0 && index < len(entries) && entries[index].ID != "" {
						entries[i].FirstKeptEntryID = entries[index].ID
					} else {
						entries[i].FirstKeptEntryID = ""
					}
					entries[i].FirstKeptEntryIndex = nil
				}
			}
		}
	}
	if version < 3 {
		for i := range entries {
			if entries[i].Type == "message" && entries[i].Message != nil && entries[i].Message.Role == "hookMessage" {
				entries[i].Message.Role = "custom"
			}
		}
	}
	header.Version = CurrentVersion
	return true
}

// OpenOrCreate opens an existing session, or starts a new session at path
// when the file is missing or empty, as Pi's SessionManager.open does.
func OpenOrCreate(path, cwd string) (*Manager, error) {
	m, err := Open(path)
	if err == nil {
		return m, nil
	}
	info, statErr := os.Stat(path)
	if statErr == nil && info.Size() > 0 {
		return nil, err
	}
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}
	abs, absErr := filepath.Abs(path)
	if absErr != nil {
		return nil, absErr
	}
	m = newManager(cwd, filepath.Dir(abs), true, "")
	m.file = abs
	m.flushed = false
	return m, nil
}

func newManager(cwd, sessionDir string, persist bool, parent string) *Manager {
	now := time.Now().UTC()
	id := newSessionID()
	cwd = ResolveCWD(cwd)
	if sessionDir != "" {
		if abs, err := filepath.Abs(sessionDir); err == nil {
			sessionDir = abs
		}
	}
	m := &Manager{
		cwd:        cwd,
		sessionDir: sessionDir,
		persist:    persist,
		sessionID:  id,
		header: Header{
			Type:          "session",
			Version:       CurrentVersion,
			ID:            id,
			Timestamp:     isoTime(now),
			CWD:           cwd,
			ParentSession: parent,
		},
	}
	if persist && sessionDir != "" {
		m.file = filepath.Join(sessionDir, fileTimestamp(now)+"_"+id+".jsonl")
	}
	return m
}

func (m *Manager) CWD() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cwd
}
func (m *Manager) ID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}
func (m *Manager) File() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.file
}
func (m *Manager) LeafID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.leafID
}
func (m *Manager) Persisted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.persist
}
func (m *Manager) Header() Header {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.header
}
func (m *Manager) Entries() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Entry(nil), m.entries...)
}

func (m *Manager) AppendMessage(msg ai.Message) (string, error) {
	return m.append(Entry{Type: "message", Message: &msg})
}

func (m *Manager) AppendModelChange(provider, modelID string) (string, error) {
	return m.append(Entry{Type: "model_change", Provider: provider, ModelID: modelID})
}

func (m *Manager) AppendThinkingLevel(level string) (string, error) {
	return m.append(Entry{Type: "thinking_level_change", ThinkingLevel: level})
}

func (m *Manager) AppendSessionInfo(name string) (string, error) {
	return m.append(Entry{Type: "session_info", Name: strings.Join(strings.Fields(name), " ")})
}

// Name is the latest session_info name on the active branch, or empty when
// none is set. A later empty name clears it.
func (m *Manager) Name() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Pi keeps the latest session_info of the whole log, not just the
	// active branch.
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].Type == "session_info" {
			return strings.TrimSpace(m.entries[i].Name)
		}
	}
	return ""
}

// Label is the latest label entry for targetID. ok is false when no label
// entry exists; an empty label with ok means the label was cleared.
func (m *Manager) Label(targetID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		if e.Type != "label" || e.TargetID != targetID {
			continue
		}
		if e.Label == nil {
			return "", true
		}
		return *e.Label, true
	}
	return "", false
}

// AppendCompaction records a compaction. An empty firstKept names the
// compaction itself, so it retains no preceding entries; fromHook is written
// as Pi writes it.
func (m *Manager) AppendCompaction(summary, firstKept string, tokensBefore int, details json.RawMessage, fromHook bool, usage *ai.Usage, sys *ai.Message) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := Entry{Type: "compaction", Summary: summary, TokensBefore: tokensBefore, Details: details, Usage: usage, SystemMessage: sys}
	if firstKept == "" {
		used := m.usedIDsLocked()
		e.ID = generateID(used)
		firstKept = e.ID
	}
	e.FirstKeptEntryID = firstKept
	hook := fromHook
	e.FromHook = &hook
	return m.appendLocked(e)
}

func (m *Manager) AppendBranchSummary(fromID, summary string, details json.RawMessage, fromHook bool, usage *ai.Usage) (string, error) {
	hook := fromHook
	return m.append(Entry{Type: "branch_summary", FromID: fromID, Summary: summary, Details: details, Usage: usage, FromHook: &hook})
}

func (m *Manager) AppendCustom(customType string, data json.RawMessage) (string, error) {
	return m.append(Entry{Type: "custom", CustomType: customType, Data: data})
}

// AppendCustomMessage records a message an extension injected into the
// conversation. display controls whether the host renders it; details is
// opaque metadata.
func (m *Manager) AppendCustomMessage(customType string, content json.RawMessage, display bool, details json.RawMessage) (string, error) {
	return m.append(Entry{Type: "custom_message", CustomType: customType, Content: content, Display: &display, Details: details})
}

// AppendUsage records model-attributed usage that does not enter the
// conversation, such as a cache-warming request.
func (m *Manager) AppendUsage(kind, provider, model string, usage ai.Usage, note string) (string, error) {
	return m.append(Entry{Type: "usage", Kind: kind, Provider: provider, Model: model, Usage: &usage, Note: note})
}

// AppendBashExecution records a user shell invocation as Pi does: a message
// entry with role bashExecution.
func (m *Manager) AppendBashExecution(msg ai.Message) (string, error) {
	msg.Role = "bashExecution"
	return m.append(Entry{Type: "message", Message: &msg})
}

// AppendLabel names targetID; a nil label clears it.
func (m *Manager) AppendLabel(targetID string, label *string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for _, e := range m.entries {
		if e.ID == targetID {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("Entry %s not found", targetID)
	}
	return m.appendLocked(Entry{Type: "label", TargetID: targetID, Label: label})
}

// AppendContextEdit changes (or, with a nil replacement, removes) the model
// context one earlier entry contributes. The target must be on the active
// branch and carry editable content.
func (m *Manager) AppendContextEdit(targetID string, replacement *ContextEditReplacement) (string, error) {
	if replacement != nil && !isContextContent(replacement.Content) {
		return "", fmt.Errorf("Context edit replacement must be null or contain string/array content")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var target *Entry
	for i := range m.entries {
		if m.entries[i].ID == targetID {
			target = &m.entries[i]
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("Entry %s not found", targetID)
	}
	if !m.onBranchLocked(targetID) {
		return "", fmt.Errorf("Entry %s is not on the active branch", targetID)
	}
	role := editableRole(*target)
	if role == "" {
		return "", fmt.Errorf("Entry %s does not contribute editable model content", targetID)
	}
	if replacement != nil && (role == "assistant" || role == "toolResult") {
		content, err := textBlockContent(replacement.Content)
		if err != nil {
			return "", err
		}
		replacement = &ContextEditReplacement{Content: content}
	}
	return m.appendLocked(Entry{Type: "context_edit", TargetID: targetID, Replacement: replacement})
}

func (m *Manager) usedIDsLocked() map[string]Entry {
	used := map[string]Entry{}
	for _, e := range m.entries {
		used[e.ID] = e
	}
	return used
}

func (m *Manager) onBranchLocked(id string) bool {
	for cur := m.leafID; cur != ""; {
		if cur == id {
			return true
		}
		var parent string
		found := false
		for _, e := range m.entries {
			if e.ID == cur {
				parent, found = parentID(e), true
				break
			}
		}
		if !found {
			return false
		}
		cur = parent
	}
	return false
}

func editableRole(e Entry) string {
	switch e.Type {
	case "custom_message":
		return "custom"
	case "message":
		if e.Message == nil {
			return ""
		}
		switch e.Message.Role {
		case "user", "assistant", "toolResult":
			return e.Message.Role
		}
	}
	return ""
}

func isContextContent(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || (trimmed[0] != '"' && trimmed[0] != '[') {
		return false
	}
	return json.Valid([]byte(trimmed))
}

func textBlockContent(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed[0] != '"' {
		return raw, nil
	}
	var text string
	if err := json.Unmarshal([]byte(trimmed), &text); err != nil {
		return nil, err
	}
	return ai.EncodeContentBlocks([]ai.ContentBlock{{Type: "text", Text: text}}), nil
}

func (m *Manager) SetLeaf(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id == "" {
		m.leafID = ""
		return nil
	}
	for _, e := range m.entries {
		if e.ID == id {
			m.leafID = id
			return nil
		}
	}
	return fmt.Errorf("entry %s not found", id)
}

func (m *Manager) Fork() (*Manager, error) {
	m.mu.Lock()
	path := buildPath(m.entries, m.leafID)
	cwd, sessionDir, persist, file := m.cwd, m.sessionDir, m.persist, m.file
	m.mu.Unlock()
	child := newManager(cwd, sessionDir, persist, "")
	if persist {
		child.header.ParentSession = file
	}
	for _, e := range path {
		cp := e
		child.entries = append(child.entries, cp)
		child.leafID = cp.ID
	}
	if child.persist && child.hasConversation() {
		if err := child.rewrite(); err != nil {
			return nil, err
		}
		child.flushed = true
	}
	return child, nil
}

func (m *Manager) append(e Entry) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appendLocked(e)
}

// appendLocked appends e at the current leaf. The caller holds m.mu.
func (m *Manager) appendLocked(e Entry) (string, error) {
	if e.ID == "" {
		e.ID = generateID(m.usedIDsLocked())
	}
	if e.Timestamp == "" {
		e.Timestamp = isoTime(time.Now())
	}
	e.ParentID = parentPtr(m.leafID)
	m.entries = append(m.entries, e)
	m.leafID = e.ID
	if err := m.persistEntry(); err != nil {
		return e.ID, err
	}
	return e.ID, nil
}

func (m *Manager) hasConversation() bool {
	for _, e := range m.entries {
		if e.Type == "message" && e.Message != nil && (e.Message.Role == "user" || e.Message.Role == "assistant") {
			return true
		}
	}
	return false
}

func (m *Manager) persistEntry() error {
	if !m.persist || m.file == "" {
		return nil
	}
	if !m.hasConversation() {
		return nil
	}
	if !m.flushed {
		if err := os.MkdirAll(filepath.Dir(m.file), 0o755); err != nil {
			return err
		}
		if err := m.rewrite(); err != nil {
			return err
		}
		m.flushed = true
		return nil
	}
	f, err := os.OpenFile(m.file, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return encodeSessionLine(f, m.entries[len(m.entries)-1])
}

func (m *Manager) rewrite() error {
	if err := os.MkdirAll(filepath.Dir(m.file), 0o755); err != nil {
		return err
	}
	f, err := os.Create(m.file)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := encodeSessionLine(f, m.header); err != nil {
		return err
	}
	for _, e := range m.entries {
		if err := encodeSessionLine(f, e); err != nil {
			return err
		}
	}
	return nil
}

// encodeSessionLine writes one JSONL record without HTML escaping, as Pi's
// JSON.stringify does.
func encodeSessionLine(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
