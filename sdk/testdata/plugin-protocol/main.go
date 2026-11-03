package main

import (
	"fmt"
	"os"

	"github.com/xiaokuang95/nulo/sdk/plugin"
)

func main() {
	if err := sdk.ServeProtocol(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	sdk.ServeNamed("protocol-fixture", "1.0.0", func(api *sdk.API) {
		api.RegisterTool(sdk.ToolDefinition{
			Name:        "protocol_echo",
			Description: "Echo through the stdio protocol.",
			Parameters:  []byte(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
			Execute: func(ctx *sdk.Context, callID string, args map[string]any) (sdk.ToolResult, error) {
				text, _ := args["text"].(string)
				tools, _ := ctx.API().GetActiveTools()
				return sdk.ToolResult{Content: []sdk.ContentBlock{{Type: "text", Text: "protocol:" + text + " tools=" + fmt.Sprint(len(tools))}}}, nil
			},
		})
		api.RegisterCommand("protocol-hello", sdk.CommandOptions{
			Handler: func(ctx *sdk.CommandContext, args string) error {
				return ctx.API().SendMessage(map[string]any{"customType": "protocol-greeting", "content": "hello"}, nil)
			},
		})
		api.OnSessionStart(func(ctx *sdk.Context, event sdk.Event) (any, error) {
			return nil, ctx.API().SetSessionName("protocol session")
		})
		api.OnSessionInfoChanged(func(ctx *sdk.Context, event sdk.Event) (any, error) {
			_, _ = ctx.API().GetSessionName()
			return nil, nil
		})
	})
}
