package sdk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
)

func TestRuntimeSwitchRebind(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("one"), ai.FauxAssistant("two"))
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	s1, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []AgentTool{}})
	if err != nil {
		t.Fatal(err)
	}
	var oldHits int
	s1.Subscribe(func(Event) { oldHits++ })
	if _, err := s1.Send(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if oldHits == 0 {
		t.Fatal("expected events on s1")
	}
	before := oldHits
	s2, err := rt.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Send(context.Background(), "b"); err != nil {
		t.Fatal(err)
	}
	if oldHits != before {
		t.Fatalf("old subscribe still firing: %d -> %d", before, oldHits)
	}
	if s2.LastAssistantText() == nil {
		t.Fatal("s2 empty")
	}
}

func TestSessionDirEnvOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv(envSessionDir, root)

	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("x"))
	agentDir := t.TempDir()
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: "/tmp/proj", AgentDir: agentDir, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []AgentTool{}})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Dispose()
	if _, err := sess.Send(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}

	file := sess.Manager().File()
	if !strings.HasPrefix(file, root) {
		t.Fatalf("session file %q is not under %q", file, root)
	}
	if _, err := os.Stat(filepath.Join(agentDir, "sessions")); !os.IsNotExist(err) {
		t.Fatalf("sessions were still written under the agent dir: %v", err)
	}
}

func TestRuntimeFork(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("x"))
	dir := t.TempDir()
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: "/tmp/proj", AgentDir: dir, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	s1, _ := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []AgentTool{}})
	_, _ = s1.Send(context.Background(), "hi")
	oldFile := s1.Manager().File()
	s2, err := rt.Fork()
	if err != nil {
		t.Fatal(err)
	}
	if s2.Manager().Header().ParentSession != oldFile {
		t.Fatalf("parent %q want %q", s2.Manager().Header().ParentSession, oldFile)
	}
}

func TestListSessionsIncludesLegacyDirectory(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("x"))
	cwd := t.TempDir()
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: t.TempDir(), StreamFn: p.StreamSimple,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	old := session.Create(cwd, session.LegacySessionDirIn(DefaultSessionDir(svcs.AgentDir), cwd))
	if _, err := old.AppendMessage(ai.UserMessage("old", 1)); err != nil {
		t.Fatal(err)
	}
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Send(context.Background(), "new"); err != nil {
		t.Fatal(err)
	}
	infos, err := rt.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, info := range infos {
		seen[info.Path] = true
	}
	if !seen[old.File()] || !seen[sess.File()] {
		t.Fatalf("sessions = %+v, want legacy %q and new %q", infos, old.File(), sess.File())
	}
}

func TestForkFromLegacySessionUsesNewDirectory(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("x"))
	cwd := t.TempDir()
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: t.TempDir(), StreamFn: p.StreamSimple,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	legacy := session.Create(cwd, session.LegacySessionDirIn(DefaultSessionDir(svcs.AgentDir), cwd))
	if _, err := legacy.AppendMessage(ai.UserMessage("hi", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.AppendMessage(ai.AssistantMessage([]ai.ContentBlock{ai.TextBlock("yo")}, DefaultFauxModel(), ai.StopStop, 2)); err != nil {
		t.Fatal(err)
	}
	resumed, err := rt.ResumeSession(legacy.File())
	if err != nil {
		t.Fatal(err)
	}
	forked, err := rt.Fork()
	if err != nil {
		t.Fatal(err)
	}
	want := session.SessionDirIn(DefaultSessionDir(svcs.AgentDir), cwd)
	if filepath.Dir(forked.File()) != want {
		t.Fatalf("fork dir = %q, want %q (resumed %q)", filepath.Dir(forked.File()), want, resumed.File())
	}
	if forked.Manager().Header().ParentSession != legacy.File() {
		t.Fatalf("parent = %q", forked.Manager().Header().ParentSession)
	}
}
