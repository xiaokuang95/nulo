package sdk

import (
	"context"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestSendFauxSettled(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("Hello from the test model."))
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD:      t.TempDir(),
		AgentDir: t.TempDir(),
		InMemory: true,
		StreamFn: p.StreamSimple,
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
	var types []string
	unsub := sess.Subscribe(func(ev Event) { types = append(types, ev.Type) })
	defer unsub()
	msgs, err := sess.Send(context.Background(), "Say hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		t.Fatal("no messages")
	}
	if types[0] != "agent_start" {
		t.Fatalf("first %v", types)
	}
	if types[len(types)-1] != "agent_settled" {
		t.Fatalf("last %v", types)
	}
	text := sess.LastAssistantText()
	if text == nil || *text != "Hello from the test model." {
		t.Fatalf("text=%v", text)
	}
}

func TestPromptErrorNilOnModelError(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxError("provider is not configured: mock"))
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	sess, _ := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	err := sess.Prompt(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Prompt err=%v", err)
	}
	text := sess.LastAssistantText()
	if text != nil {
		t.Fatalf("expected nil text, got %q", *text)
	}
}
