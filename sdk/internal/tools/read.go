package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

const (
	maxReadLines = 2000
	maxReadBytes = 50 * 1024
)

func Read(cwd string) agent.AgentTool {
	return agent.AgentTool{
		Tool: ai.Tool{
			Name:        "read",
			Description: "Read file contents",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"number"},"limit":{"type":"number"}},"required":["path"]}`),
		},
		Label: "read",
		Execute: func(ctx context.Context, _ string, args map[string]any) (agent.ToolResult, error) {
			path, _ := args["path"].(string)
			if path == "" {
				return errResult("path is required")
			}
			abs := resolvePath(cwd, path)
			data, err := os.ReadFile(abs)
			if err != nil {
				return errResult(err.Error())
			}
			text := string(data)
			lines := strings.Split(text, "\n")
			offset := 1
			if v, ok := args["offset"].(float64); ok && int(v) > 0 {
				offset = int(v)
			}
			limit := maxReadLines
			if v, ok := args["limit"].(float64); ok && int(v) > 0 {
				limit = int(v)
			}
			start := offset - 1
			if start < 0 {
				start = 0
			}
			if start > len(lines) {
				start = len(lines)
			}
			end := start + limit
			if end > len(lines) {
				end = len(lines)
			}
			out := strings.Join(lines[start:end], "\n")
			if len(out) > maxReadBytes {
				out = out[:maxReadBytes] + fmt.Sprintf("\n\n[truncated %s]", abs)
			}
			return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(out)}}, nil
		},
	}
}
