package sdk

import (
	"context"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestSteerDuringSend(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("one"), ai.FauxAssistant("two"))
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	unsub := sess.Subscribe(func(ev Event) {
		if ev.Type == "message_end" && ev.Message != nil && ev.Message.Role == "assistant" && ai.MessageText(*ev.Message) == "one" {
			_ = sess.Steer(context.Background(), ai.UserMessage("steer", 2))
		}
	})
	defer unsub()
	msgs, err := sess.Send(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, m := range msgs {
		if m.Role == "assistant" {
			texts = append(texts, ai.MessageText(m))
		}
	}
	if len(texts) < 2 || texts[1] != "two" {
		t.Fatalf("steer did not continue: %v %+v", texts, msgs)
	}
}

func TestSetActiveToolsAllowlist(t *testing.T) {
	p := ai.NewFauxProvider()
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	sess.SetActiveTools([]string{"read"})
	names := toolNames(sess)
	if len(names) != 1 || names[0] != "read" {
		t.Fatalf("allowlist %v", names)
	}
	sess.SetActiveTools([]string{"+bash"})
	names = toolNames(sess)
	want := map[string]bool{"read": true, "bash": true}
	if len(names) != 2 {
		t.Fatalf("+bash %v", names)
	}
	for _, n := range names {
		if !want[n] {
			t.Fatalf("unexpected %s in %v", n, names)
		}
	}
}
