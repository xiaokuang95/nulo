package sdk

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/plugin"
)

func TestServerLifecycle(t *testing.T) {
	ServeNamed("test", "1", func(api *API) {
		api.RegisterTool(ToolDefinition{
			Name: "echo", Description: "echo",
			Execute: func(ctx *Context, callID string, args map[string]any) (ToolResult, error) {
				return ToolResult{Content: []ContentBlock{{Type: "text", Text: args["text"].(string)}}}, nil
			},
		})
		api.RegisterCommand("hello", CommandOptions{Handler: func(ctx *CommandContext, args string) error { return nil }})
		api.OnInput(func(ctx *Context, event Event) (any, error) {
			if event["text"] == "rename" {
				return InputResult{Action: "transform", Text: "renamed"}, nil
			}
			return nil, nil
		})
		api.OnSessionStart(func(ctx *Context, event Event) (any, error) { return nil, nil })
	})

	raw := Init(`{"cwd":"C:/tmp"}`, 0)
	var reg plugin.Registration
	if err := json.Unmarshal([]byte(raw), &reg); err != nil {
		t.Fatal(err)
	}
	if reg.Name != "test" || reg.APIVersion != plugin.APIVersion || len(reg.Tools) != 1 || len(reg.Commands) != 1 {
		t.Fatalf("registration: %s", raw)
	}

	result := Call(`{"type":"event","event":"input","data":{"text":"rename"}}`)
	if !strings.Contains(result, `"text":"renamed"`) || !strings.Contains(result, `"action":"transform"`) {
		t.Fatalf("input: %s", result)
	}
	result = Call(`{"type":"event","event":"session_start","data":{}}`)
	if !strings.Contains(result, `"result"`) || strings.Contains(result, `"error"`) {
		t.Fatalf("session_start: %s", result)
	}
	result = Call(`{"type":"tool","name":"echo","toolCallId":"1","input":{"text":"hi"}}`)
	if !strings.Contains(result, `"text":"hi"`) {
		t.Fatalf("echo: %s", result)
	}
}
