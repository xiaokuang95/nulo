package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

const storeType = "codemode-store"

func Tool(other []agent.AgentTool, loadStore func() map[string]any, saveStore func(map[string]any)) agent.AgentTool {
	return agent.AgentTool{
		Tool: ai.Tool{
			Name:        "codemode",
			Description: "Run a JavaScript script that calls other tools. Only the script output reaches the model.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"source":{"type":"string"}},"required":["source"]}`),
		},
		Label: "codemode",
		Execute: func(ctx context.Context, _ string, args map[string]any) (agent.ToolResult, error) {
			src, _ := args["source"].(string)
			if src == "" {
				return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock("source is required")}, IsError: true}, nil
			}
			fns := map[string]func(map[string]any) (string, error){}
			for _, t := range other {
				if t.Name == "codemode" {
					continue
				}
				t := t
				fns[t.Name] = func(a map[string]any) (string, error) {
					res, err := t.Execute(ctx, "codemode", a)
					if err != nil {
						return "", err
					}
					var b strings.Builder
					for _, c := range res.Content {
						if c.Type == "text" {
							b.WriteString(c.Text)
						}
					}
					if res.IsError {
						return b.String(), fmt.Errorf("%s", b.String())
					}
					return b.String(), nil
				}
			}
			st := map[string]any{}
			if loadStore != nil {
				if s := loadStore(); s != nil {
					st = s
				}
			}
			r, err := Run(ctx, src, fns, st)
			if err != nil {
				return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(err.Error())}, IsError: true}, nil
			}
			if r.Wrote && saveStore != nil {
				saveStore(r.Store)
			}
			prefix := "Script completed"
			if !r.OK {
				prefix = "Script failed"
			}
			text := fmt.Sprintf("%s in %.2fs\n%s", prefix, r.Elapsed.Seconds(), r.Text)
			if r.Error != "" {
				text += "\nScript error: " + r.Error
			}
			return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(text)}, IsError: !r.OK}, nil
		},
	}
}
