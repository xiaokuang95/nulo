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

func Edit(cwd string, q *mutationQueue) agent.AgentTool {
	return agent.AgentTool{
		Tool: ai.Tool{
			Name:                "edit",
			Description:         "Make precise file edits with exact text replacement",
			Parameters:          json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"edits":{"type":"array","items":{"type":"object","properties":{"oldText":{"type":"string"},"newText":{"type":"string"}},"required":["oldText","newText"]}}},"required":["path","edits"]}`),
			ConstrainedSampling: map[string]any{"type": "json_schema", "strict": "prefer"},
		},
		Label: "edit",
		Execute: func(ctx context.Context, _ string, args map[string]any) (agent.ToolResult, error) {
			path, _ := args["path"].(string)
			if path == "" {
				return errResult("path is required")
			}
			abs := resolvePath(cwd, path)
			edits, err := parseEdits(args)
			if err != nil {
				return errResult(err.Error())
			}
			err = q.do(abs, func() error {
				raw, err := os.ReadFile(abs)
				if err != nil {
					return err
				}
				content := string(raw)
				next, err := applyEdits(content, edits)
				if err != nil {
					return err
				}
				return os.WriteFile(abs, []byte(next), 0o644)
			})
			if err != nil {
				return errResult(err.Error())
			}
			return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock("Edited " + abs)}}, nil
		},
	}
}

type textEdit struct{ old, new string }

func parseEdits(args map[string]any) ([]textEdit, error) {
	if old, ok := args["oldText"].(string); ok {
		nw, _ := args["newText"].(string)
		return []textEdit{{old, nw}}, nil
	}
	raw, ok := args["edits"].([]any)
	if !ok {
		return nil, fmt.Errorf("edits required")
	}
	var out []textEdit
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid edit")
		}
		old, _ := m["oldText"].(string)
		nw, _ := m["newText"].(string)
		out = append(out, textEdit{old, nw})
	}
	return out, nil
}

func applyEdits(content string, edits []textEdit) (string, error) {
	type span struct{ start, end int }
	var used []span
	type repl struct {
		start, end int
		new        string
	}
	var reps []repl
	for _, e := range edits {
		start := strings.Index(content, e.old)
		if start < 0 {
			return "", fmt.Errorf("oldText not found")
		}
		if strings.Index(content[start+len(e.old):], e.old) >= 0 {
			return "", fmt.Errorf("oldText is not unique")
		}
		end := start + len(e.old)
		for _, u := range used {
			if start < u.end && end > u.start {
				return "", fmt.Errorf("overlapping edits")
			}
		}
		used = append(used, span{start, end})
		reps = append(reps, repl{start, end, e.new})
	}
	// apply from the end so offsets stay valid
	for i := 0; i < len(reps); i++ {
		for j := i + 1; j < len(reps); j++ {
			if reps[j].start > reps[i].start {
				reps[i], reps[j] = reps[j], reps[i]
			}
		}
	}
	out := content
	for _, r := range reps {
		out = out[:r.start] + r.new + out[r.end:]
	}
	return out, nil
}
