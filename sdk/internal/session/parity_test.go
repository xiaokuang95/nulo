package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestNewSessionIDIsUUIDv7(t *testing.T) {
	id := newSessionID()
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(id) {
		t.Fatalf("not a UUIDv7: %s", id)
	}
	if again := newSessionID(); again == id {
		t.Fatal("session ids repeat")
	}
}

func TestMigrateV1ToV3(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.jsonl")
	body := strings.Join([]string{
		`{"type":"session","version":1,"id":"old","timestamp":"2024-01-01T00:00:00.000Z","cwd":"/t"}`,
		`{"type":"message","timestamp":"2024-01-01T00:00:01.000Z","message":{"role":"user","content":"hi","timestamp":1}}`,
		`{"type":"compaction","timestamp":"2024-01-01T00:00:02.000Z","summary":"sum","firstKeptEntryIndex":1,"tokensBefore":10}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	entries := m.Entries()
	if len(entries) != 2 {
		t.Fatalf("entries %d", len(entries))
	}
	if entries[0].ID == "" || entries[1].ID == "" || entries[0].ID == entries[1].ID {
		t.Fatalf("ids %+v", entries)
	}
	if entries[0].ParentID != nil {
		t.Fatalf("root parent %v", *entries[0].ParentID)
	}
	if entries[1].ParentID == nil || *entries[1].ParentID != entries[0].ID {
		t.Fatalf("chain %+v", entries[1])
	}
	if entries[1].FirstKeptEntryID != entries[0].ID || entries[1].FirstKeptEntryIndex != nil {
		t.Fatalf("firstKept %+v", entries[1])
	}
	if m.Header().Version != CurrentVersion {
		t.Fatalf("version %d", m.Header().Version)
	}
	// The migrated file is rewritten and still loads.
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Header().Version != CurrentVersion || len(reopened.Entries()) != 2 {
		t.Fatalf("reopen %+v", reopened.Header())
	}
}

func TestMigrateV2HookMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.jsonl")
	body := strings.Join([]string{
		`{"type":"session","version":2,"id":"old","timestamp":"2024-01-01T00:00:00.000Z","cwd":"/t"}`,
		`{"type":"message","id":"a","parentId":null,"timestamp":"2024-01-01T00:00:01.000Z","message":{"role":"hookMessage","content":"hi","timestamp":1}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if role := m.Project().Messages[0].Role; role != "custom" {
		t.Fatalf("role %q", role)
	}
}

func TestOpenAddsMissingTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	body := `{"type":"session","version":3,"id":"x","timestamp":"2024-01-01T00:00:00.000Z","cwd":"/t"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AppendMessage(ai.UserMessage("hi", 1)); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Project().Messages; len(got) != 1 || got[0].ContentString() != "hi" {
		t.Fatalf("%+v", got)
	}
}

func TestCustomMessageProjection(t *testing.T) {
	m := InMemory("/tmp/proj")
	display := false
	_ = display
	if _, err := m.AppendCustomMessage("note", json.RawMessage(`"hello"`), false, json.RawMessage(`{"k":1}`)); err != nil {
		t.Fatal(err)
	}
	projection := m.Project()
	if len(projection.Messages) != 1 || projection.Messages[0].Role != "custom" {
		t.Fatalf("%+v", projection.Messages)
	}
	converted := ConvertToLlm(projection.Messages)
	if len(converted) != 1 || converted[0].Role != "user" || converted[0].ContentString() != "hello" {
		t.Fatalf("%+v", converted)
	}
	entry := m.Entries()[0]
	if entry.CustomType != "note" || entry.Display == nil || *entry.Display || string(entry.Details) != `{"k":1}` {
		t.Fatalf("entry %+v", entry)
	}
}

func TestLabels(t *testing.T) {
	m := InMemory("/tmp/proj")
	id, _ := m.AppendMessage(ai.UserMessage("hi", 1))
	label := "checkpoint"
	if _, err := m.AppendLabel(id, &label); err != nil {
		t.Fatal(err)
	}
	if got, ok := m.Label(id); !ok || got != "checkpoint" {
		t.Fatalf("label %q %v", got, ok)
	}
	if _, err := m.AppendLabel(id, nil); err != nil {
		t.Fatal(err)
	}
	if got, ok := m.Label(id); !ok || got != "" {
		t.Fatalf("cleared label %q %v", got, ok)
	}
	if _, err := m.AppendLabel("missing", &label); err == nil {
		t.Fatal("labeling a missing entry must fail")
	}
}

func TestUsageEntry(t *testing.T) {
	m := InMemory("/tmp/proj")
	usage := ai.Usage{Input: 10, Output: 2, TotalTokens: 12}
	if _, err := m.AppendUsage("cache_warm", "p", "m", usage, "note"); err != nil {
		t.Fatal(err)
	}
	entry := m.Entries()[0]
	if entry.Type != "usage" || entry.Kind != "cache_warm" || entry.Provider != "p" || entry.Model != "m" || entry.Usage == nil || entry.Usage.Input != 10 || entry.Note != "note" {
		t.Fatalf("entry %+v", entry)
	}
	if len(m.Project().Messages) != 0 {
		t.Fatal("usage must not enter context")
	}
}

func TestContextEditProjection(t *testing.T) {
	m := InMemory("/tmp/proj")
	userID, _ := m.AppendMessage(ai.UserMessage("original", 1))
	assistantID, _ := m.AppendMessage(ai.AssistantMessage([]ai.ContentBlock{ai.TextBlock("answer")}, ai.DefaultFauxModel(), ai.StopStop, 2))
	replacement := &ContextEditReplacement{Content: json.RawMessage(`"changed"`)}
	if _, err := m.AppendContextEdit(userID, replacement); err != nil {
		t.Fatal(err)
	}
	projection := m.Project()
	if len(projection.Messages) != 2 || projection.Messages[0].ContentString() != "changed" {
		t.Fatalf("%+v", projection.Messages)
	}
	if _, err := m.AppendContextEdit(assistantID, nil); err != nil {
		t.Fatal(err)
	}
	projection = m.Project()
	if len(projection.Messages) != 1 || projection.Messages[0].Role != "user" {
		t.Fatalf("omitted target %+v", projection.Messages)
	}

	// A string replacement for an assistant is stored as one text block.
	assistantID2, _ := m.AppendMessage(ai.AssistantMessage([]ai.ContentBlock{ai.TextBlock("second")}, ai.DefaultFauxModel(), ai.StopStop, 3))
	editID, err := m.AppendContextEdit(assistantID2, replacement)
	if err != nil {
		t.Fatal(err)
	}
	_ = editID
	for _, e := range m.Entries() {
		if e.Type == "context_edit" && e.TargetID == assistantID2 {
			if e.Replacement == nil || string(e.Replacement.Content) != `[{"type":"text","text":"changed"}]` {
				t.Fatalf("assistant replacement %+v", e.Replacement)
			}
		}
	}
	projection = m.Project()
	last := projection.Messages[len(projection.Messages)-1]
	if last.Role != "assistant" || last.ContentString() != "changed" {
		t.Fatalf("assistant edit %+v", last)
	}
}

func TestContextEditRejectsAbandonedTarget(t *testing.T) {
	m := InMemory("/tmp/proj")
	if _, err := m.AppendMessage(ai.UserMessage("one", 1)); err != nil {
		t.Fatal(err)
	}
	abandoned, _ := m.AppendMessage(ai.UserMessage("two", 2))
	if err := m.SetLeaf(""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AppendMessage(ai.UserMessage("root", 3)); err != nil {
		t.Fatal(err)
	}
	_, err := m.AppendContextEdit(abandoned, &ContextEditReplacement{Content: json.RawMessage(`"x"`)})
	if err == nil || !strings.Contains(err.Error(), "not on the active branch") {
		t.Fatalf("err %v", err)
	}
}

func TestCompactionAndBranchSummaryShape(t *testing.T) {
	m := InMemory("/tmp/proj")
	if _, err := m.AppendMessage(ai.UserMessage("hi", 1)); err != nil {
		t.Fatal(err)
	}
	id, err := m.AppendCompaction("sum", "", 42, json.RawMessage(`{"k":1}`), false, &ai.Usage{Input: 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	entry := m.Entries()[len(m.Entries())-1]
	if entry.FirstKeptEntryID != id || entry.Details == nil || entry.FromHook == nil || *entry.FromHook || entry.Usage == nil || entry.Usage.Input != 3 {
		t.Fatalf("compaction %+v", entry)
	}
	raw, _ := json.Marshal(entry)
	if !strings.Contains(string(raw), `"fromHook":false`) || !strings.Contains(string(raw), `"firstKeptEntryId":"`+id+`"`) {
		t.Fatalf("wire %s", raw)
	}

	summaryID, err := m.AppendBranchSummary("root", "branch", json.RawMessage(`{"a":1}`), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	summary := m.Entries()[len(m.Entries())-1]
	if summary.ID != summaryID || summary.FromID != "root" || summary.FromHook == nil {
		t.Fatalf("branch summary %+v", summary)
	}
}

func TestSessionInfoName(t *testing.T) {
	m := InMemory("/tmp/proj")
	if _, err := m.AppendSessionInfo("  first\n\nsecond  "); err != nil {
		t.Fatal(err)
	}
	if got := m.Name(); got != "first second" {
		t.Fatalf("name %q", got)
	}
	if _, err := m.AppendSessionInfo(""); err != nil {
		t.Fatal(err)
	}
	if got := m.Name(); got != "" {
		t.Fatalf("cleared name %q", got)
	}
}

func TestBashExecutionProjection(t *testing.T) {
	m := InMemory("/tmp/proj")
	exit := 2
	message := ai.Message{Role: "bashExecution", Command: "make", Output: "boom", ExitCode: &exit, Timestamp: 1}
	if _, err := m.AppendBashExecution(message); err != nil {
		t.Fatal(err)
	}
	converted := ConvertToLlm(m.Project().Messages)
	if len(converted) != 1 || !strings.Contains(converted[0].ContentString(), "Ran `make`") || !strings.Contains(converted[0].ContentString(), "exited with code 2") {
		t.Fatalf("%+v", converted)
	}
	excluded := true
	message.ExcludeFromContext = &excluded
	if _, err := m.AppendBashExecution(message); err != nil {
		t.Fatal(err)
	}
	if converted := ConvertToLlm(m.Project().Messages); len(converted) != 1 {
		t.Fatalf("excluded entry entered context: %+v", converted)
	}
}

func TestListSessions(t *testing.T) {
	dir := t.TempDir()
	write := func(name, id, text string, ts int64, sessionName string) string {
		lines := []string{
			fmt.Sprintf(`{"type":"session","version":3,"id":%q,"timestamp":"2024-01-01T00:00:00.000Z","cwd":"/t"}`, id),
			fmt.Sprintf(`{"type":"message","id":"a","parentId":null,"timestamp":"2024-01-01T00:00:01.000Z","message":{"role":"user","content":%q,"timestamp":%d}}`, text, ts),
		}
		if sessionName != "" {
			lines = append(lines, fmt.Sprintf(`{"type":"session_info","id":"b","parentId":"a","timestamp":"2024-01-01T00:00:02.000Z","name":%q}`, sessionName))
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(strings.Join(append(lines, ""), "\n")), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	older := write("older.jsonl", "one", "hello one", 1000, "named")
	newer := write("newer.jsonl", "two", "hello two", 2000, "")
	if err := os.WriteFile(filepath.Join(dir, "broken.jsonl"), []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	infos, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatalf("infos %+v", infos)
	}
	if infos[0].Path != newer || infos[1].Path != older {
		t.Fatalf("order %+v", infos)
	}
	if infos[1].Name != "named" || infos[1].FirstMessage != "hello one" || infos[1].MessageCount != 1 {
		t.Fatalf("summary %+v", infos[1])
	}
	if infos[0].FirstMessage != "hello two" {
		t.Fatalf("summary %+v", infos[0])
	}
}

func TestOpenOrCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "created.jsonl")
	m, err := OpenOrCreate(path, "/tmp/proj")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AppendMessage(ai.UserMessage("hi", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	// An empty file is also a new session.
	empty := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenOrCreate(empty, "/tmp/proj"); err != nil {
		t.Fatal(err)
	}
	// A non-empty file without a header is not.
	bad := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenOrCreate(bad, "/tmp/proj"); err == nil {
		t.Fatal("a non-empty invalid file must fail")
	}
}

func TestSessionJSONKeepsHTMLLiteral(t *testing.T) {
	dir := t.TempDir()
	m := Create("/tmp/proj", dir)
	if _, err := m.AppendMessage(ai.UserMessage("<tag> & <other>", 1)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(m.File())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "<tag> & <other>") || strings.Contains(string(raw), `\u003c`) {
		t.Fatalf("HTML escaped: %s", raw)
	}
}

func TestTreeLabelsAndBranches(t *testing.T) {
	m := InMemory("/tmp/proj")
	first, _ := m.AppendMessage(ai.UserMessage("one", 1))
	abandoned, _ := m.AppendMessage(ai.AssistantMessage([]ai.ContentBlock{ai.TextBlock("a")}, ai.DefaultFauxModel(), ai.StopStop, 2))
	if err := m.SetLeaf(first); err != nil {
		t.Fatal(err)
	}
	second, _ := m.AppendMessage(ai.UserMessage("two", 3))
	label := "kept"
	if _, err := m.AppendLabel(second, &label); err != nil {
		t.Fatal(err)
	}
	roots := m.Tree()
	if len(roots) != 1 || roots[0].Entry.ID != first {
		t.Fatalf("roots %+v", roots)
	}
	children := roots[0].Children
	if len(children) != 2 {
		t.Fatalf("children %+v", children)
	}
	if children[0].Entry.ID != abandoned || children[1].Entry.ID != second || children[1].Label != "kept" {
		t.Fatalf("children %+v", children)
	}
}
