package sdk

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// TestProtocolPluginCreatesChildSession proves the plugin ABI can implement a
// subagent without the host hardcoding an Agent type, prompt, or result shape.
func TestProtocolPluginCreatesChildSession(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a protocol plugin")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "child-fixture")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	build := exec.Command("go", "build", "-o", exe, ".")
	build.Dir = "testdata/plugin-child"
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build child fixture: %v\n%s", err, output)
	}

	manifest, err := json.Marshal(map[string]any{
		"name":     "child-fixture",
		"version":  "1.0.0",
		"protocol": "stdio",
		"command":  exe,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}

	provider := ai.NewFauxProvider()
	provider.SetResponses(
		ai.FauxAssistantBlocks([]ai.ContentBlock{
			ai.ToolCallBlock("call-1", "child_echo", map[string]any{"prompt": "hello child"}),
		}, ai.StopToolUse),
		ai.FauxAssistant("child answer"),
		ai.FauxAssistant("parent done"),
	)
	stream := func(ctx context.Context, model ai.Model, tr ai.Transcript, opts ai.StreamOptions) *ai.EventStream {
		return provider.StreamSimple(ctx, model, tr, opts)
	}

	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD:         dir,
		AgentDir:    t.TempDir(),
		InMemory:    true,
		StreamFn:    stream,
		PluginPaths: []string{manifestPath},
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
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	if diags := sess.PluginDiagnostics(); len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if _, err := sess.Send(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	text := sess.LastAssistantText()
	if text == nil || *text != "parent done" {
		t.Fatalf("parent text = %v", text)
	}

	found := false
	for _, entry := range sess.Manager().Branch() {
		if entry.Message == nil || entry.Message.Role != "toolResult" {
			continue
		}
		if strings.Contains(ai.MessageText(*entry.Message), "child:child answer") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("child result missing from parent transcript: %+v", sess.Manager().Branch())
	}
}
