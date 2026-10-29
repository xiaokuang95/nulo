package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

const (
	maxBashLines = 2000
	maxBashBytes = 50 * 1024
)

func Bash(cwd string) agent.AgentTool {
	return agent.AgentTool{
		Tool: ai.Tool{
			Name:        "bash",
			Description: "Execute bash commands (ls, grep, find, etc.)",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"},"timeout":{"type":"number"}},"required":["command"]}`),
		},
		Label: "bash",
		Execute: func(ctx context.Context, _ string, args map[string]any) (agent.ToolResult, error) {
			command, _ := args["command"].(string)
			if command == "" {
				return errResult("command is required")
			}
			cctx := ctx
			if v, ok := args["timeout"].(float64); ok && v > 0 {
				var cancel context.CancelFunc
				cctx, cancel = context.WithTimeout(ctx, time.Duration(v*float64(time.Second)))
				defer cancel()
			}
			cmd := exec.CommandContext(cctx, "/bin/bash", "-lc", command)
			cmd.Dir = cwd
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			start := time.Now()
			err := cmd.Run()
			out := buf.String()
			if len(out) > maxBashBytes || strings.Count(out, "\n") > maxBashLines {
				lines := strings.Split(out, "\n")
				if len(lines) > maxBashLines {
					lines = lines[:maxBashLines]
				}
				out = strings.Join(lines, "\n")
				if len(out) > maxBashBytes {
					out = out[:maxBashBytes]
				}
				out += "\n[truncated]"
			}
			code := 0
			if err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					code = ee.ExitCode()
				} else {
					return errResult(err.Error())
				}
			}
			text := out
			if code != 0 {
				text = fmt.Sprintf("%s\n(exit %d, %.2fs)", out, code, time.Since(start).Seconds())
				return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(text)}, IsError: true}, nil
			}
			return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(text)}}, nil
		},
	}
}
