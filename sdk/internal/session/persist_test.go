package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestPersistRewriteFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Create("/tmp/proj", blocker)
	_, err := m.AppendMessage(ai.UserMessage("hi", 1))
	if err == nil {
		t.Fatal("expected persist error")
	}
}

func TestOpenBadLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.jsonl")
	body := `{"type":"session","version":3,"id":"x","timestamp":"2024-01-01T00:00:00.000Z","cwd":"/t"}` + "\n" + "not-json\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(p); err == nil {
		t.Fatal("expected error on bad line")
	}
}
