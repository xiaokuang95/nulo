package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

const storeType = "codemode-store"

// Tool builds the codemode tool. other is the full catalogue a script may call,
// which includes tools deliberately left out of the model's tool list; namespaces
// group those tools for describeNamespace() and searchTools({namespace}).
func Tool(other []agent.AgentTool, namespaces []Namespace, loadStore func() map[string]any, saveStore func(map[string]any)) agent.AgentTool {
	return agent.AgentTool{
		Tool: ai.Tool{
			Name:        "codemode",
			Description: CreateDescription(other, namespaces, DefaultInlineBudget),
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
			nsOf := map[string]string{}
			for _, ns := range namespaces {
				for _, tn := range ns.Tools {
					nsOf[tn] = ns.Name
				}
			}
			info := make([]ToolInfo, 0, len(fns))
			for _, t := range other {
				if t.Name == "codemode" {
					continue
				}
				info = append(info, ToolInfo{Name: t.Name, Description: t.Description, Namespace: nsOf[t.Name], Sample: renderToolSection(t)})
			}
			r, err := Run(ctx, src, RunOptions{Tools: fns, ToolInfo: info, Namespaces: namespaces, Store: st})
			if err != nil {
				return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(err.Error())}, IsError: true}, nil
			}
			if r.Wrote && saveStore != nil {
				saveStore(r.Store)
			}

			// Like Pi: the header names the outcome, the output follows it, and on
			// failure the error goes last, inside the same budget.
			items := r.Content
			if !r.OK {
				items = append(items, ai.TextBlock("Script error:\n"+formatErrorJSON(r.Error)))
			}
			maxTokens := int64(defaultMaxOutputTokens)
			if r.MaxOutputTokens > 0 {
				maxTokens = r.MaxOutputTokens
			}
			truncated, _ := truncateOutput(joinAdjacentText(items), maxTokens)
			output := joinAdjacentText(truncated)

			title := "Script failed"
			if r.OK {
				title = "Script completed"
			}
			header := title + "\nWall time " + strconv.FormatFloat(r.Elapsed.Seconds(), 'f', 1, 64) + " seconds\nOutput:\n"
			content := append([]ai.ContentBlock{ai.TextBlock(header)}, output...)
			return agent.ToolResult{Content: content, IsError: !r.OK}, nil
		},
	}
}
