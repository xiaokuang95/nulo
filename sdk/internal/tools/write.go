package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func Write(cwd string, q *mutationQueue) agent.AgentTool {
	return agent.AgentTool{
		Tool: ai.Tool{
			Name:                "write",
			Description:         "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
			Parameters:          json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
			ConstrainedSampling: map[string]any{"type": "json_schema", "strict": "prefer"},
		},
		Label: "write",
		Execute: func(ctx context.Context, _ string, args map[string]any) (agent.ToolResult, error) {
			path, _ := args["path"].(string)
			content, _ := args["content"].(string)
			if path == "" {
				return errResult("path is required")
			}
			abs := resolvePath(cwd, path)
			err := q.do(abs, func() error {
				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
					return err
				}
				return os.WriteFile(abs, []byte(content), 0o644)
			})
			if err != nil {
				return errResult(err.Error())
			}
			return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock("Wrote " + abs)}}, nil
		},
	}
}
