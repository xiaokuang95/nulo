package sdk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestToolApprovalBlocksAndAllows(t *testing.T) {
	cwd := t.TempDir()
	target := filepath.Join(cwd, "blocked.txt")
	p := ai.NewFauxProvider()
	p.SetResponses(
		ai.FauxAssistantBlocks([]ai.ContentBlock{
			ai.ToolCallBlock("call-1", "bash", map[string]any{"command": "touch " + target}),
		}, ai.StopToolUse),
		ai.FauxAssistant("done"),
	)
	var requests []ToolApprovalRequest
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{
		Model: DefaultFauxModel(),
		ApproveTool: func(_ context.Context, req ToolApprovalRequest) (ToolApprovalDecision, error) {
			requests = append(requests, req)
			return ToolApprovalDecision{Allow: false, Reason: "not this time"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Dispose()
	if _, err := sess.Send(context.Background(), "write it"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].ToolName != "bash" || requests[0].CWD != cwd {
		t.Fatalf("approval requests = %+v", requests)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("denied command still ran: %v", err)
	}
	foundDenial := false
	for _, m := range sess.Messages() {
		if m.Role == "toolResult" && m.IsError && strings.Contains(m.ContentString(), "not this time") {
			foundDenial = true
		}
	}
	if !foundDenial {
		t.Fatalf("denial not recorded: %+v", sess.Messages())
	}
}

func TestListAllSessionsAndProviderModels(t *testing.T) {
	cwd := t.TempDir()
	agentDir := t.TempDir()
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("hello"))
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: agentDir, StreamFn: p.StreamSimple,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Send(context.Background(), "persist me"); err != nil {
		t.Fatal(err)
	}
	file := sess.File()
	if file == "" || sess.ID() == "" {
		t.Fatalf("session file/id missing: %q %q", file, sess.ID())
	}
	_ = rt.Close()

	infos, err := ListAllSessions(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Path != file || infos[0].CWD != cwd {
		t.Fatalf("sessions = %+v", infos)
	}

	models := filepath.Join(agentDir, "models.json")
	if err := os.WriteFile(models, []byte(`{"providers":{"test":{"name":"Test","api":"openai-completions","baseUrl":"https://example.test/v1","models":[{"id":"b","name":"B","contextWindow":64000},{"id":"a","name":"A"}]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	providers, err := ListProviders(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].Name != "Test" || len(providers[0].Models) != 2 {
		t.Fatalf("providers = %+v", providers)
	}
	if providers[0].Models[0].ID != "a" || providers[0].Models[1].ID != "b" {
		t.Fatalf("models not sorted: %+v", providers[0].Models)
	}
	if providers[0].Models[1].ContextWindow != 64000 {
		t.Fatalf("context window = %d", providers[0].Models[1].ContextWindow)
	}
}

func TestRuntimeApprovalAppliesToNewSessions(t *testing.T) {
	cwd := t.TempDir()
	target := filepath.Join(cwd, "runtime-blocked.txt")
	p := ai.NewFauxProvider()
	p.SetResponses(
		ai.FauxAssistantBlocks([]ai.ContentBlock{
			ai.ToolCallBlock("call-1", "bash", map[string]any{"command": "touch " + target}),
		}, ai.StopToolUse),
		ai.FauxAssistant("done"),
	)
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{
		Services: svcs,
		ApproveTool: func(context.Context, ToolApprovalRequest) (ToolApprovalDecision, error) {
			return ToolApprovalDecision{Allow: false, Reason: "blocked by runtime"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Send(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("runtime approval did not block the tool: %v", err)
	}
}
