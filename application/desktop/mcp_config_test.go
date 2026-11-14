package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetMCPProjectEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".nulo", "mcp.json")
	if err := setMCPProjectEnabled(path, "filesystem", false); err != nil {
		t.Fatal(err)
	}
	if err := setMCPProjectEnabled(path, "browser", true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if root.MCPServers["filesystem"]["enabled"] != false || root.MCPServers["browser"]["enabled"] != true {
		t.Fatalf("servers = %+v", root.MCPServers)
	}
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"local":{"command":"echo","enabled":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := setMCPProjectEnabled(path, "local", false); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	entry := root.MCPServers["local"]
	if entry["command"] != "echo" || entry["enabled"] != false {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestApplyMCPHealth(t *testing.T) {
	s := &session{
		MCPServers: []string{"filesystem", "plugin-search"},
		MCPAll: []mcpServerInfo{
			{Name: "filesystem", Enabled: true},
			{Name: "browser", Enabled: false},
		},
	}
	applyMCPHealth(s)
	if !s.MCPAll[0].Connected || s.MCPAll[1].Connected {
		t.Fatalf("health = %+v", s.MCPAll)
	}
	if len(s.MCPAll) != 3 || s.MCPAll[2].Name != "plugin-search" || !s.MCPAll[2].Connected {
		t.Fatalf("plugin MCP missing = %+v", s.MCPAll)
	}
}
