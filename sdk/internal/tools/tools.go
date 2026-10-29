package tools

import (
	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func errResult(msg string) (agent.ToolResult, error) {
	return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(msg)}, IsError: true}, nil
}

func Default(cwd string) []agent.AgentTool {
	q := newMutationQueue()
	return []agent.AgentTool{
		Read(cwd),
		Bash(cwd),
		Edit(cwd, q),
		Write(cwd, q),
	}
}

func Names(tools []agent.AgentTool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name
	}
	return out
}
