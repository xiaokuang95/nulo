package sdk

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestPromptImageAttachment(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("I see it"))
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []AgentTool{}})
	if err != nil {
		t.Fatal(err)
	}
	data := base64.StdEncoding.EncodeToString([]byte("not-a-real-png"))
	if err := sess.Prompt(context.Background(), "describe this", PromptOptions{
		Images: []ImageAttachment{{Data: data, MIMEType: "image/png"}},
	}); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, m := range sess.Messages() {
		if m.Role != "user" {
			continue
		}
		for _, block := range m.ContentBlocks() {
			if block.Type == "image" && block.Data == data && block.MIMEType == "image/png" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("image block missing from messages: %+v", sess.Messages())
	}
}

func TestPromptImageRejectsInvalidBase64(t *testing.T) {
	p := ai.NewFauxProvider()
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []AgentTool{}})
	if err != nil {
		t.Fatal(err)
	}
	err = sess.Prompt(context.Background(), "bad", PromptOptions{
		Images: []ImageAttachment{{Data: "%%%", MIMEType: "image/png"}},
	})
	if err == nil {
		t.Fatal("invalid image data was accepted")
	}
}
