package main

// This is the whole ABI surface a native plugin main package needs. The host
// resolves these three exported symbols in the shared library.
//
//go:generate go build -buildmode=c-shared -o nulo-example.dll .
//go:generate go build -buildmode=c-shared -o nulo-example.dylib . (darwin)
//go:generate go build -buildmode=c-shared -o nulo-example.so . (linux)

import (
	"strings"

	"github.com/xiaokuang95/nulo/plugin/sdk"
	_ "github.com/xiaokuang95/nulo/plugin/sdk/cabi"
)

func main() {}

func init() {
	sdk.ServeNamed("example", "1.0.0", func(api *sdk.API) {
		api.RegisterTool(sdk.ToolDefinition{
			Name:        "echo",
			Label:       "Echo",
			Description: "Echo the text back, reporting the tools the session declares.",
			Parameters:  []byte(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
			Execute: func(ctx *sdk.Context, callID string, args map[string]any) (sdk.ToolResult, error) {
				text, _ := args["text"].(string)
				tools, _ := ctx.API().GetActiveTools()
				return sdk.ToolResult{Content: []sdk.ContentBlock{{Type: "text", Text: text + " tools=" + strings.Join(tools, ",")}}}, nil
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
			return nil, ctx.API().SetSessionName("example session")
		})
		api.OnToolCall(func(ctx *sdk.Context, event sdk.Event) (any, error) {
			if input, ok := event["input"].(map[string]any); ok && event["toolName"] == "echo" {
				if text, ok := input["text"].(string); ok {
					input["text"] = "[" + text + "]"
				}
			}
			return nil, nil
		})
	})
}
