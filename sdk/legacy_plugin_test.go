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
	"github.com/xiaokuang95/nulo/sdk/internal/mcp"
)

func TestAdapterLoadsProtocolPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a protocol plugin")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "legacy-fixture")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	build := exec.Command("go", "build", "-o", exe, ".")
	build.Dir = filepath.Join("testdata", "plugin-protocol")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build protocol fixture: %v\n%s", err, output)
	}
	manifest, err := json.Marshal(map[string]any{
		"name": "legacy-fixture", "version": "1.0.0",
		"protocol": "stdio", "command": exe,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}

	var hostMethods []string
	adapter, err := LoadLegacyPlugin(LegacyPluginOptions{
		Path: manifestPath,
		CWD:  dir,
		HostCall: func(_ context.Context, method string, _ json.RawMessage) (any, error) {
			hostMethods = append(hostMethods, method)
			return nil, nil
		},
		OnDiagnostic: func(message string) {
			t.Logf("diagnostic: %s", message)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()

	registry := NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	tools := registry.Tools()
	if len(tools) != 1 || tools[0].Definition().Name != "protocol_echo" {
		t.Fatalf("tools = %+v", tools)
	}
	result, err := tools[0].Execute(context.Background(), ToolCall{
		ID: "call-1", Name: "protocol_echo", Arguments: map[string]any{"text": "hi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "protocol:hi") {
		t.Fatalf("result = %+v", result)
	}
	if len(registry.Commands()) != 1 || registry.Commands()[0].Name != "protocol-hello" {
		t.Fatalf("commands = %+v", registry.Commands())
	}
	if err := registry.Commands()[0].Run(context.Background(), nil, "x"); err != nil {
		t.Fatal(err)
	}
	foundSend := false
	for _, method := range hostMethods {
		if method == "sendMessage" {
			foundSend = true
		}
	}
	if !foundSend {
		t.Fatalf("host methods = %v, want sendMessage", hostMethods)
	}
}

func TestProviderFor(t *testing.T) {
	cfg := ai.ProviderConfig{
		API:     "chat",
		BaseURL: "https://example.invalid/v1",
		Models: []ai.ProviderModelConfig{
			{ID: "m1", Name: "M1", ContextWindow: 1000, MaxTokens: 100},
		},
	}
	provider, err := providerFor("acme", cfg)
	if err != nil {
		t.Fatal(err)
	}
	models := provider.Models()
	if len(models) != 1 || models[0].Provider != "acme" || models[0].ID != "m1" {
		t.Fatalf("models = %+v", models)
	}
}

func TestAdapterRejectsMissingPath(t *testing.T) {
	if _, err := LoadLegacyPlugin(LegacyPluginOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestAdapterMCPDeclaration(t *testing.T) {
	adapter := &LegacyPlugin{}
	registry := NewRegistry()
	cfg := mcp.ServerConfig{
		Command: "echo",
		Args:    []string{"hello"},
		Env:     map[string]string{"A": "B"},
	}
	if err := adapter.addMCPServer(registry, "legacy-mcp", cfg); err != nil {
		t.Fatal(err)
	}
	servers := registry.MCPServers()
	if len(servers) != 1 {
		t.Fatalf("servers = %+v", servers)
	}
	if servers[0].Transport != "stdio" || servers[0].Exposure != "codemode" || servers[0].Command != "echo" {
		t.Fatalf("server = %+v", servers[0])
	}
}

func TestAdapterBridgesInitialRegistrationCalls(t *testing.T) {
	adapter := &LegacyPlugin{
		providers: map[string]ai.ProviderConfig{},
		mcps:      map[string]mcp.ServerConfig{},
	}
	if _, err := adapter.hostCall(context.Background(), "registerProvider", json.RawMessage(`{
		"name":"acme",
		"config":{"api":"chat","baseUrl":"https://example.invalid/v1","models":[{"id":"m","name":"M"}]}
	}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.hostCall(context.Background(), "registerMcpServer", json.RawMessage(`{
		"name":"legacy-mcp","config":{"command":"echo","args":["hello"]}
	}`)); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := adapter.Register(registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.Providers()) != 1 || registry.Providers()[0].ID() != "acme" {
		t.Fatalf("providers = %+v", registry.Providers())
	}
	if len(registry.MCPServers()) != 1 || registry.MCPServers()[0].Name != "legacy-mcp" {
		t.Fatalf("mcp = %+v", registry.MCPServers())
	}
}
