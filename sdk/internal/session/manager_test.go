package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestMemoryAppendAndProject(t *testing.T) {
	m := InMemory("/tmp/proj")
	if m.File() != "" {
		t.Fatal("memory has no file")
	}
	m.AppendMessage(ai.UserMessage("hi", 1))
	m.AppendMessage(ai.AssistantMessage([]ai.ContentBlock{ai.TextBlock("yo")}, ai.DefaultFauxModel(), ai.StopStop, 2))
	p := m.Project()
	if len(p.Messages) != 2 {
		t.Fatalf("got %d messages", len(p.Messages))
	}
	if p.Messages[0].Role != "user" || p.Messages[1].Role != "assistant" {
		t.Fatalf("%+v", p.Messages)
	}
}

func TestDiskCreateOnFirstUser(t *testing.T) {
	dir := t.TempDir()
	m := Create("/tmp/proj", dir)
	m.AppendThinkingLevel("medium")
	if _, err := os.Stat(m.File()); !os.IsNotExist(err) {
		t.Fatalf("file should not exist yet: %v", err)
	}
	m.AppendMessage(ai.UserMessage("hi", 1))
	if _, err := os.Stat(m.File()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(m.File())
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[0] != '{' {
		t.Fatal("expected jsonl")
	}
	opened, err := Open(m.File())
	if err != nil {
		t.Fatal(err)
	}
	p := opened.Project()
	if len(p.Messages) != 1 || p.Messages[0].ContentString() != "hi" {
		t.Fatalf("resume %+v", p.Messages)
	}
}

func TestForkParentSession(t *testing.T) {
	dir := t.TempDir()
	m := Create("/tmp/proj", dir)
	m.AppendMessage(ai.UserMessage("hi", 1))
	m.AppendMessage(ai.AssistantMessage([]ai.ContentBlock{ai.TextBlock("a")}, ai.DefaultFauxModel(), ai.StopStop, 2))
	child, err := m.Fork()
	if err != nil {
		t.Fatal(err)
	}
	if child.Header().ParentSession != m.File() {
		t.Fatalf("parentSession=%q want %q", child.Header().ParentSession, m.File())
	}
	if child.File() == m.File() {
		t.Fatal("fork must be a new file")
	}
	if _, err := os.Stat(m.File()); err != nil {
		t.Fatal("old file must remain")
	}
	if len(child.Project().Messages) != 2 {
		t.Fatalf("fork lost history: %+v", child.Project().Messages)
	}
}

func TestSessionProjectPath(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "a", "nulo")
	key := ProjectKey(cwd)
	if !strings.HasPrefix(key, "nulo-") || len(key) != len("nulo-")+12 {
		t.Fatalf("project key = %q", key)
	}
	if key != ProjectKey(filepath.Join(filepath.Dir(cwd), ".", "nulo")) {
		t.Fatal("project key is not stable")
	}
	other := filepath.Join(t.TempDir(), "a", "nulo")
	if key == ProjectKey(other) {
		t.Fatal("same basename must still be disambiguated by cwd")
	}
	dir := SessionDir(t.TempDir(), cwd)
	if filepath.Base(dir) != key {
		t.Fatalf("%s", dir)
	}
	legacy := filepath.Base(LegacySessionDirIn(t.TempDir(), "/Users/a/b:c"))
	if !strings.HasPrefix(legacy, "--") || !strings.HasSuffix(legacy, "--") {
		t.Fatalf("legacy dir = %q", legacy)
	}
}

func TestConvertToLlmCompactionPrefix(t *testing.T) {
	m := ai.Message{Role: "compactionSummary", Timestamp: 1, Content: ai.EncodeContentString("SUM")}
	out := ConvertToLlm([]ai.Message{m})
	if len(out) != 1 || out[0].Role != "user" {
		t.Fatalf("%+v", out)
	}
	text := out[0].ContentString()
	if text != CompactionSummaryPrefix+"SUM"+CompactionSummarySuffix {
		t.Fatalf("%q", text)
	}
}
