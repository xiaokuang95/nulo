package sdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListMCPServersIncludesDisabled(t *testing.T) {
	agentDir := t.TempDir()
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(agentDir, "mcp.json"), []byte(`{"mcpServers":{
		"z":{"command":"echo","description":"last"},
		"a":{"command":"echo","enabled":false}
	}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	servers := ListMCPServers(agentDir, cwd, false)
	if len(servers) != 2 || servers[0].Name != "a" || servers[0].Enabled || servers[1].Name != "z" || !servers[1].Enabled {
		t.Fatalf("servers = %+v", servers)
	}
	if servers[1].Description != "last" {
		t.Fatalf("description = %q", servers[1].Description)
	}
	if err := os.MkdirAll(filepath.Join(cwd, ".nulo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".nulo", "mcp.json"), []byte(`{"mcpServers":{"a":{"enabled":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	servers = ListMCPServers(agentDir, cwd, true)
	if !servers[0].Enabled {
		t.Fatalf("project override did not enable: %+v", servers)
	}
	servers = ListMCPServers(agentDir, cwd, false)
	if servers[0].Enabled {
		t.Fatalf("untrusted project applied an override: %+v", servers)
	}
}
