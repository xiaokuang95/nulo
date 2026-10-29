package sdk

import (
	"context"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestRuntimeSwitchRebind(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("one"), ai.FauxAssistant("two"))
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	s1, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []Tool{}})
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

func TestRuntimeFork(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("x"))
	dir := t.TempDir()
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: "/tmp/proj", AgentDir: dir, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	s1, _ := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []Tool{}})
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
