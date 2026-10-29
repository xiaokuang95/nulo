package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

type fileCfg struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

func Load(agentDir, cwd string, projectTrusted bool) map[string]ServerConfig {
	out := map[string]ServerConfig{}
	if agentDir != "" {
		merge(out, readFile(filepath.Join(agentDir, "mcp.json")), false)
	}
	if projectTrusted && cwd != "" {
		merge(out, readFile(filepath.Join(cwd, ".nulo", "mcp.json")), true)
	}
	return out
}

func readFile(path string) map[string]ServerConfig {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f fileCfg
	if err := json.Unmarshal(b, &f); err != nil {
		return nil
	}
	return f.MCPServers
}

func merge(dst, src map[string]ServerConfig, overlay bool) {
	for name, cfg := range src {
		if overlay {
			if existing, ok := dst[name]; ok && cfg.Command == "" && cfg.URL == "" && cfg.Type == "" {
				if cfg.Enabled != nil {
					existing.Enabled = cfg.Enabled
				}
				dst[name] = existing
				continue
			}
		}
		dst[name] = cfg
	}
}

func ConnectAll(ctx context.Context, cfgs map[string]ServerConfig) ([]*Client, []agent.AgentTool) {
	var clients []*Client
	var tools []agent.AgentTool
	for name, cfg := range cfgs {
		if !cfg.on() {
			continue
		}
		if strings.EqualFold(cfg.Type, "sse") {
			continue
		}
		cl, mcpTools, err := Connect(ctx, name, cfg)
		if err != nil {
			continue
		}
		clients = append(clients, cl)
		for _, t := range mcpTools {
			t := t
			params := t.InputSchema
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object"}`)
			}
			tools = append(tools, agent.AgentTool{
				Tool:  ai.Tool{Name: t.Name, Description: t.Description, Parameters: params},
				Label: t.Name,
				Execute: func(execCtx context.Context, _ string, args map[string]any) (agent.ToolResult, error) {
					text, isErr, err := t.call(execCtx, args)
					if err != nil {
						return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(err.Error())}, IsError: true}, nil
					}
					return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(text)}, IsError: isErr}, nil
				},
			})
		}
	}
	return clients, tools
}
