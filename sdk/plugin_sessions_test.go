package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestPluginChildSessionLifecycle(t *testing.T) {
	provider := ai.NewFauxProvider()
	provider.SetResponses(ai.FauxAssistant("child says hi"))
	stream := func(ctx context.Context, model ai.Model, tr ai.Transcript, opts ai.StreamOptions) *ai.EventStream {
		return provider.StreamSimple(ctx, model, tr, opts)
	}

	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer svcs.Close()
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if _, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()}); err != nil {
		t.Fatal(err)
	}

	created := pluginHostCallForTest(t, svcs, "session.create", map[string]any{
		"systemPrompt": "You are a child agent.",
		"noTools":      true,
	})
	createMap, ok := created.(map[string]any)
	if !ok {
		t.Fatalf("create result = %#v", created)
	}
	id, _ := createMap["id"].(string)
	if id == "" {
		t.Fatalf("missing child id: %#v", created)
	}

	statusAny := pluginHostCallForTest(t, svcs, "session.status", map[string]any{"sessionId": id})
	status, ok := statusAny.(pluginChildStatus)
	if !ok {
		t.Fatalf("status = %#v", statusAny)
	}
	if status.Running {
		t.Fatalf("initial status = %+v", status)
	}

	sent := pluginHostCallForTest(t, svcs, "session.send", map[string]any{
		"sessionId": id,
		"prompt":    "say hi",
	})
	result, ok := sent.(pluginChildResult)
	if !ok {
		t.Fatalf("send result = %#v", sent)
	}
	if !strings.Contains(result.Text, "child says hi") {
		t.Fatalf("text = %q", result.Text)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected child messages")
	}

	last := pluginHostCallForTest(t, svcs, "session.result", map[string]any{"sessionId": id})
	lastResult, ok := last.(pluginChildResult)
	if !ok || lastResult.Text != result.Text {
		t.Fatalf("last result = %#v", last)
	}

	messagesAny := pluginHostCallForTest(t, svcs, "session.messages", map[string]any{"sessionId": id})
	if messages, ok := messagesAny.([]map[string]any); !ok || len(messages) == 0 {
		t.Fatalf("messages = %#v", messagesAny)
	}

	listAny := pluginHostCallForTest(t, svcs, "session.list", nil)
	if list, ok := listAny.([]pluginChildStatus); !ok || len(list) != 1 {
		t.Fatalf("list = %#v", listAny)
	}

	pluginHostCallForTest(t, svcs, "session.dispose", map[string]any{"sessionId": id})
	listAny = pluginHostCallForTest(t, svcs, "session.list", nil)
	if list, ok := listAny.([]pluginChildStatus); !ok || len(list) != 0 {
		t.Fatalf("list after dispose = %#v", listAny)
	}
}

func TestPluginChildSessionRejectsUnknownTool(t *testing.T) {
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer svcs.Close()

	raw, _ := json.Marshal(map[string]any{"tools": []string{"does_not_exist"}})
	if _, err := svcs.pluginHostCall(context.Background(), "session.create", raw); err == nil {
		t.Fatal("expected unknown tool error")
	}
}

func pluginHostCallForTest(t *testing.T, svcs *AgentSessionServices, method string, params map[string]any) any {
	t.Helper()
	var raw json.RawMessage
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		raw = encoded
	}
	value, err := svcs.pluginHostCall(context.Background(), method, raw)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return value
}

func TestPluginChildSessionResume(t *testing.T) {
	provider := ai.NewFauxProvider()
	provider.SetResponses(ai.FauxAssistant("first answer"), ai.FauxAssistant("second answer"))
	stream := func(ctx context.Context, model ai.Model, tr ai.Transcript, opts ai.StreamOptions) *ai.EventStream {
		return provider.StreamSimple(ctx, model, tr, opts)
	}

	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), StreamFn: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer svcs.Close()
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if _, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()}); err != nil {
		t.Fatal(err)
	}

	created := pluginHostCallForTest(t, svcs, "session.create", map[string]any{
		"noTools": true,
		"persist": true,
	})
	createMap := created.(map[string]any)
	id := createMap["id"].(string)
	pluginHostCallForTest(t, svcs, "session.send", map[string]any{"sessionId": id, "prompt": "first"})
	status := pluginHostCallForTest(t, svcs, "session.status", map[string]any{"sessionId": id}).(pluginChildStatus)
	if status.File == "" {
		t.Fatal("expected persisted child file")
	}
	pluginHostCallForTest(t, svcs, "session.dispose", map[string]any{"sessionId": id})

	resumed := pluginHostCallForTest(t, svcs, "session.create", map[string]any{
		"noTools":    true,
		"resumeFrom": status.File,
	})
	resumeMap := resumed.(map[string]any)
	resumedID := resumeMap["id"].(string)
	sent := pluginHostCallForTest(t, svcs, "session.send", map[string]any{"sessionId": resumedID, "prompt": "second"}).(pluginChildResult)
	if sent.Text != "second answer" {
		t.Fatalf("resumed text = %q", sent.Text)
	}
	joined := ""
	for _, message := range sent.Messages {
		joined += fmt.Sprint(message["content"])
	}
	if !strings.Contains(joined, "first answer") {
		t.Fatalf("resumed history missing first answer: %v", sent.Messages)
	}
}
