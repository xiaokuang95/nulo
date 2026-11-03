//go:build plugin_integration

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

// Run with: go test -tags plugin_integration ./sdk -run Plugin -v
// Requires a C toolchain because the fixture is built with -buildmode=c-shared.
func TestNativePluginEndToEnd(t *testing.T) {
	ext := ".so"
	if runtime.GOOS == "windows" {
		ext = ".dll"
	} else if runtime.GOOS == "darwin" {
		ext = ".dylib"
	}
	dir, err := os.MkdirTemp("", "nulo-plugin-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "nulo-test-plugin"+ext)
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", out, ".")
	cmd.Dir = "testdata/plugin"
	if build, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build plugin: %v\n%s", err, build)
	}

	p := ai.NewFauxProvider()
	var captured ai.Transcript
	stream := func(ctx context.Context, model ai.Model, tr ai.Transcript, opts ai.StreamOptions) *ai.EventStream {
		captured = tr
		p.SetResponses(ai.FauxAssistant("ok"))
		return p.StreamSimple(ctx, model, tr, opts)
	}
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true,
		StreamFn: stream, PluginPaths: []string{out},
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
	if diags := sess.PluginDiagnostics(); len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if !hasTool(sess.tools, "echo") || !hasTool(sess.tools, "dynamic") {
		t.Fatalf("plugin tools missing: %v", sess.ActiveToolNames())
	}
	if sess.Name() != "plugin session" {
		t.Fatalf("session name %q", sess.Name())
	}
	if _, err := sess.Send(context.Background(), "rename"); err != nil {
		t.Fatal(err)
	}
	sys := ai.GetCurrentSystemMessage(captured.Messages)
	if sys == nil || !strings.Contains(ai.MessageText(*sys), "PLUGIN SYSTEM PROMPT") {
		t.Fatalf("system prompt: %+v", sys)
	}
	var user, assistant string
	for _, m := range captured.Messages {
		if m.Role == "user" {
			user = m.ContentString()
		}
	}
	for _, entry := range sess.Manager().Branch() {
		if entry.Message != nil && entry.Message.Role == "assistant" {
			assistant = entry.Message.ContentString()
		}
	}
	if user != "renamed" {
		t.Fatalf("transformed user = %q", user)
	}
	if !strings.HasPrefix(assistant, "patched:") {
		t.Fatalf("patched assistant = %q", assistant)
	}
}

func TestProtocolPluginEndToEnd(t *testing.T) {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	dir, err := os.MkdirTemp("", "nulo-protocol-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	exe := filepath.Join(dir, "nulo-protocol-fixture"+ext)
	cmd := exec.Command("go", "build", "-o", exe, ".")
	cmd.Dir = "testdata/plugin-protocol"
	if build, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build protocol plugin: %v\n%s", err, build)
	}
	manifest, _ := json.Marshal(map[string]any{
		"name": "protocol-fixture", "version": "1.0.0",
		"protocol": "stdio", "command": exe,
	})
	manifestPath := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}

	p := ai.NewFauxProvider()
	stream := func(ctx context.Context, model ai.Model, tr ai.Transcript, opts ai.StreamOptions) *ai.EventStream {
		return p.StreamSimple(ctx, model, tr, opts)
	}
	p.SetResponses(
		ai.FauxAssistantBlocks([]ai.ContentBlock{ai.ToolCallBlock("c1", "protocol_echo", map[string]any{"text": "hi"})}, ai.StopToolUse),
		ai.FauxAssistant("done"),
	)
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: dir, AgentDir: t.TempDir(), InMemory: true,
		StreamFn: stream, PluginPaths: []string{manifestPath},
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
	if diags := sess.PluginDiagnostics(); len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if sess.Name() != "protocol session" {
		t.Fatalf("session name %q", sess.Name())
	}
	if !hasTool(sess.tools, "protocol_echo") {
		t.Fatalf("protocol tool missing: %v", sess.ActiveToolNames())
	}
	if _, err := sess.Send(context.Background(), "run it"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range sess.Manager().Branch() {
		if entry.Message != nil && entry.Message.Role == "toolResult" && strings.Contains(ai.MessageText(*entry.Message), "protocol:hi") {
			found = true
		}
	}
	if !found {
		t.Fatal("protocol tool result not found")
	}
}
