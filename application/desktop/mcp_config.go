package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaokuang95/nulo/sdk"
)

type mcpServerInfo struct {
	Name        string
	Enabled     bool
	Connected   bool
	Description string
}

func (a *app) refreshMCPInfo(s *session) {
	if a == nil || a.host == nil || s == nil || !s.Real {
		return
	}
	servers := sdk.ListMCPServers(a.host.agentDir, s.CWD, a.host.sessionTrusted(s))
	connected := make(map[string]bool, len(s.MCPServers))
	for _, name := range s.MCPServers {
		connected[name] = true
	}
	out := make([]mcpServerInfo, 0, len(servers))
	for _, server := range servers {
		out = append(out, mcpServerInfo{Name: server.Name, Enabled: server.Enabled, Connected: connected[server.Name], Description: server.Description})
	}
	s.MCPAll = out
}

func applyMCPHealth(s *session) {
	if s == nil {
		return
	}
	connected := make(map[string]bool, len(s.MCPServers))
	for _, name := range s.MCPServers {
		connected[name] = true
	}
	seen := make(map[string]bool, len(s.MCPAll))
	for i := range s.MCPAll {
		server := &s.MCPAll[i]
		server.Connected = connected[server.Name]
		seen[server.Name] = true
	}
	for _, name := range s.MCPServers {
		if name == "" || seen[name] {
			continue
		}
		s.MCPAll = append(s.MCPAll, mcpServerInfo{
			Name: name, Enabled: true, Connected: true, Description: "connected by the SDK",
		})
	}
}

func (a *app) toggleMCPServer(s *session, name string, enabled bool) {
	if a == nil || a.host == nil || s == nil {
		return
	}
	if s.Status == "running" || s.Status == "awaiting" {
		a.pendingToast = "Stop the current turn before changing MCP servers"
		return
	}
	if !a.host.sessionTrusted(s) {
		a.pendingToast = "Trust this project before overriding MCP servers"
		return
	}
	if err := a.host.validateConfigReload(a); err != nil {
		a.pendingToast = "MCP: " + err.Error()
		return
	}
	path := filepath.Join(s.CWD, ".nulo", "mcp.json")
	previous, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		a.pendingToast = "MCP: " + readErr.Error()
		return
	}
	if err := setMCPProjectEnabled(path, name, enabled); err != nil {
		a.pendingToast = "MCP: " + err.Error()
		return
	}
	if err := a.reloadConfig("MCP configuration reloaded"); err != nil {
		if existed {
			_ = writeConfigFile(path, previous)
		} else {
			_ = os.Remove(path)
		}
		a.pendingToast = "MCP reload: " + err.Error()
		return
	}
	a.refreshMCPInfo(s)
	if enabled {
		a.pendingToast = "Enabled MCP server · " + name
	} else {
		a.pendingToast = "Disabled MCP server · " + name
	}
}

func setMCPProjectEnabled(path, name string, enabled bool) error {
	if path == "" || strings.TrimSpace(name) == "" {
		return errors.New("project path and MCP server name are required")
	}
	root := map[string]any{}
	data, err := os.ReadFile(path)
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	entry, _ := servers[name].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	entry["enabled"] = enabled
	servers[name] = entry
	root["mcpServers"] = servers
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return writeConfigFile(path, encoded)
}
