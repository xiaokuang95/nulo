package sdk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
)

func writeModelCatalog(t *testing.T, dir string) {
	t.Helper()
	body := `{"providers": {"p": {"baseUrl": "https://p.example/v1", "api": "openai-completions", "models": [{"id": "m", "reasoning": true}]}}}`
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNewSessionRecordsInitialModel(t *testing.T) {
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	entries := sess.Manager().Entries()
	if len(entries) == 0 || entries[0].Type != "model_change" || entries[0].Provider != "faux" || entries[0].ModelID != "faux-1" {
		t.Fatalf("initial entries %+v", entries)
	}
}

func TestResumeSessionRestoresModelAndThinking(t *testing.T) {
	dir := t.TempDir()
	writeModelCatalog(t, dir)
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	model, err := BuildModel("p/m", svcs)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := rt.New(SessionStartOptions{Model: model, ThinkingLevel: ThinkingHigh})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Manager().AppendMessage(ai.UserMessage("hi", 1)); err != nil {
		t.Fatal(err)
	}
	path := sess.Manager().File()

	// Switch carries the live settings; resume restores the file's.
	switched, err := rt.SwitchSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := switched.ThinkingLevel(); got != ThinkingHigh {
		t.Fatalf("switch level %q", got)
	}
	if _, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), InMemory: true, ThinkingLevel: ThinkingLow}); err != nil {
		t.Fatal(err)
	}
	resumed, err := rt.ResumeSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := resumed.Model(); got.Provider != "p" || got.ID != "m" {
		t.Fatalf("resumed model %+v", got)
	}
	if got := resumed.ThinkingLevel(); got != ThinkingHigh {
		t.Fatalf("resumed level %q", got)
	}
}

func TestImportCopiesIntoSessionDir(t *testing.T) {
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	source := filepath.Join(t.TempDir(), "imported.jsonl")
	body := strings.Join([]string{
		`{"type":"session","version":3,"id":"imported","timestamp":"2024-01-01T00:00:00.000Z","cwd":"/elsewhere"}`,
		`{"type":"message","id":"a","parentId":null,"timestamp":"2024-01-01T00:00:01.000Z","message":{"role":"user","content":"hi","timestamp":1}}`,
		"",
	}, "\n")
	if err := os.WriteFile(source, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	imported, err := rt.ImportFromJsonl(source)
	if err != nil {
		t.Fatal(err)
	}
	first := imported.Manager().File()
	dir := session.SessionDirIn(DefaultSessionDir(svcs.AgentDir), svcs.CWD)
	if filepath.Dir(first) != dir {
		t.Fatalf("imported to %q, want %q", first, dir)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal(err)
	}
	second, err := rt.ImportFromJsonl(source)
	if err != nil {
		t.Fatal(err)
	}
	if second.Manager().File() == first {
		t.Fatal("a second import must not overwrite the first")
	}
}

func TestSessionNameAndListing(t *testing.T) {
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.SetName("my session"); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Manager().AppendMessage(ai.UserMessage("hello", 1)); err != nil {
		t.Fatal(err)
	}
	path := sess.Manager().File()

	list, err := rt.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "my session" || list[0].FirstMessage != "hello" {
		t.Fatalf("list %+v", list)
	}
	if err := rt.RenameSession(path, "renamed"); err != nil {
		t.Fatal(err)
	}
	list, err = rt.ListSessions()
	if err != nil || len(list) != 1 || list[0].Name != "renamed" {
		t.Fatalf("renamed list %+v err %v", list, err)
	}
	if err := rt.DeleteSession(path); err != nil {
		t.Fatal(err)
	}
	if list, err := rt.ListSessions(); err != nil || len(list) != 0 {
		t.Fatalf("after delete %+v err %v", list, err)
	}
	if err := rt.DeleteSession(path); err != nil {
		t.Fatalf("deleting a missing file must be fine: %v", err)
	}
	if err := rt.DeleteSession(filepath.Join(t.TempDir(), "outside.jsonl")); err == nil {
		t.Fatal("deleting outside the session directory must fail")
	}
}

func TestForkUnsavedSessionFails(t *testing.T) {
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if _, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()}); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Fork(); err == nil {
		t.Fatal("forking an unsaved session must fail")
	}
}
