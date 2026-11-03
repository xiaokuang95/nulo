package main

// Test fixture used by sdk/plugin_integration_test.go. It exercises the hook
// surface: tool/command registration, dynamic tools, input transforms, system
// prompt replacement, and message patching.
//
//go:generate go build -buildmode=c-shared -o nulo-example.dll .
//go:generate go build -buildmode=c-shared -o nulo-example.dylib . (darwin)
//go:generate go build -buildmode=c-shared -o nulo-example.so . (linux)

import (
	"github.com/xiaokuang95/nulo/plugin/sdk"
	_ "github.com/xiaokuang95/nulo/plugin/sdk/cabi"
)

func main() {}

func init() {
	sdk.ServeNamed("test-fixture", "1.0.0", func(api *sdk.API) {
		api.RegisterTool(sdk.ToolDefinition{
			Name:        "echo",
			Label:       "Echo",
			Description: "Echo the text back, reporting the tools the session declares.",
			Parameters:  []byte(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
			Execute: func(ctx *sdk.Context, callID string, args map[string]any) (sdk.ToolResult, error) {
				text, _ := args["text"].(string)
				tools, _ := ctx.API().GetActiveTools()
				return sdk.ToolResult{Content: []sdk.ContentBlock{{Type: "text", Text: text + " tools=" + join(tools)}}}, nil
			},
		})
		api.RegisterCommand("hello", sdk.CommandOptions{
			Description: "Say hello from the plugin",
			Handler: func(ctx *sdk.CommandContext, args string) error {
				return ctx.API().SendMessage(map[string]any{
					"customType": "example-greeting",
					"content":    "hello " + args,
					"display":    true,
				}, nil)
			},
		})
		api.OnSessionStart(func(ctx *sdk.Context, event sdk.Event) (any, error) {
			_ = ctx.API().SetSessionName("plugin session")
			ctx.API().RegisterTool(sdk.ToolDefinition{
				Name: "dynamic", Description: "registered at session start",
				Execute: func(ctx *sdk.Context, id string, args map[string]any) (sdk.ToolResult, error) {
					return sdk.ToolResult{Content: []sdk.ContentBlock{{Type: "text", Text: "dynamic ok"}}}, nil
				},
			})
			return nil, nil
		})
		api.OnSessionInfoChanged(func(ctx *sdk.Context, event sdk.Event) (any, error) {
			// Re-enter the host from inside the event it emitted.
			_, _ = ctx.API().GetSessionName()
			return nil, nil
		})
		api.OnInput(func(ctx *sdk.Context, event sdk.Event) (any, error) {
			if event["text"] == "rename" {
				return sdk.InputResult{Action: "transform", Text: "renamed"}, nil
			}
			return nil, nil
		})
		api.OnBeforeAgentStart(func(ctx *sdk.Context, event sdk.Event) (any, error) {
			return sdk.BeforeAgentStartPatch{SystemPrompt: "PLUGIN SYSTEM PROMPT"}, nil
		})
		api.OnMessageEnd(func(ctx *sdk.Context, event sdk.Event) (any, error) {
			msg, _ := event["message"].(map[string]any)
			if msg != nil && msg["role"] == "assistant" {
				msg["content"] = "patched:" + sdkText(msg["content"])
				return sdk.MessageEndPatch{Message: msg}, nil
			}
			return nil, nil
		})
		api.OnToolCall(func(ctx *sdk.Context, event sdk.Event) (any, error) {
			if event["toolName"] == "echo" {
				if input, ok := event["input"].(map[string]any); ok {
					if text, ok := input["text"].(string); ok {
						input["text"] = "[" + text + "]"
					}
				}
			}
			return nil, nil
		})
	})
}

func sdkText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		out := ""
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					out += text
				}
			}
		}
		return out
	}
	return ""
}

func join(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ","
		}
		out += value
	}
	return out
}
