package sdk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestCodemodeReadTool(t *testing.T) {
	cwd := t.TempDir()
	os.WriteFile(filepath.Join(cwd, "x.txt"), []byte("hello-nulo"), 0o644)
	p := ai.NewFauxProvider()
	p.SetResponses(
		ai.FauxAssistantBlocks([]ai.ContentBlock{ai.ToolCallBlock("c1", "codemode", map[string]any{
			"source": "return await tools.read({path:'x.txt'})",
		})}, ai.StopToolUse),
		ai.FauxAssistant("done"),
	)
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	// default tools must not include codemode until enabled
	for _, tname := range toolNames(sess) {
		if tname == "codemode" {
			t.Fatal("codemode should be inactive by default")
		}
	}
	sess.SetActiveTools([]string{"+codemode"})
	msgs, err := sess.Send(context.Background(), "run")
	if err != nil {
		t.Fatal(err)
	}
	var tr *ai.Message
	for i := range msgs {
		if msgs[i].Role == "toolResult" {
			tr = &msgs[i]
		}
	}
	if tr == nil || !strings.Contains(tr.ContentString(), "hello-nulo") {
		t.Fatalf("codemode result %+v", tr)
	}
}

func toolNames(s *Session) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n []string
	for _, t := range s.tools {
		n = append(n, t.Name)
	}
	return n
}
