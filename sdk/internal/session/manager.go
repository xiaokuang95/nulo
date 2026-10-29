package session

import (
	"bufio"
	"encoding/json"
	"fmt"
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
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var raw []json.RawMessage
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		raw = append(raw, json.RawMessage(line))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty session file: %s", path)
	}
	var header Header
	if err := json.Unmarshal(raw[0], &header); err != nil {
		return nil, fmt.Errorf("session header: %w", err)
	}
	if header.Type != "session" {
		return nil, fmt.Errorf("first line is not a session header")
	}
	m := &Manager{
		cwd:        header.CWD,
		sessionDir: filepath.Dir(path),
		persist:    true,
		sessionID:  header.ID,
		file:       path,
		header:     header,
		flushed:    true,
	}
	for i, line := range raw[1:] {
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("session %s line %d: %w", path, i+2, err)
		}
		if e.Type == "session" {
			continue
		}
		m.entries = append(m.entries, e)
		m.leafID = e.ID
	}
	return m, nil
}

func newManager(cwd, sessionDir string, persist bool, parent string) *Manager {
	now := time.Now().UTC()
	id := newSessionID()
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
	return m.append(Entry{Type: "session_info", Name: strings.TrimSpace(strings.ReplaceAll(name, "\n", " "))})
}

func (m *Manager) AppendCompaction(summary, firstKept string, tokensBefore int, sys *ai.Message) (string, error) {
	e := Entry{Type: "compaction", Summary: summary, FirstKeptEntryID: firstKept, TokensBefore: tokensBefore, SystemMessage: sys}
	id, err := m.append(e)
	if err != nil {
		return id, err
	}
	if firstKept == "" {
		m.mu.Lock()
		for i := range m.entries {
			if m.entries[i].ID == id {
				m.entries[i].FirstKeptEntryID = id
				break
			}
		}
		m.mu.Unlock()
	}
	return id, nil
}

func (m *Manager) AppendBranchSummary(fromID, summary string) (string, error) {
	return m.append(Entry{Type: "branch_summary", FromID: fromID, Summary: summary})
}

func (m *Manager) AppendCustom(customType string, data json.RawMessage) (string, error) {
	return m.append(Entry{Type: "custom", CustomType: customType, Data: data})
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
	used := map[string]Entry{}
	for _, x := range m.entries {
		used[x.ID] = x
	}
	e.ID = generateID(used)
	e.Timestamp = isoTime(time.Now())
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
	return json.NewEncoder(f).Encode(m.entries[len(m.entries)-1])
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
	enc := json.NewEncoder(f)
	if err := enc.Encode(m.header); err != nil {
		return err
	}
	for _, e := range m.entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}
