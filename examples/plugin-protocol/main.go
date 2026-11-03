package main

// A protocol plugin is a normal executable. The host starts it and exchanges
// newline-delimited JSON frames over stdin/stdout.
//
//go:generate go build -o plugin-protocol.exe .
//go:generate go build -o plugin-protocol .

import (
	"fmt"
	"os"

	"github.com/xiaokuang95/nulo/plugin/sdk"
)

func main() {
	if err := sdk.ServeProtocol(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	sdk.ServeNamed("example-protocol", "1.0.0", func(api *sdk.API) {
		api.RegisterTool(sdk.ToolDefinition{
			Name:        "protocol_echo",
			Description: "Echo text through the protocol plugin.",
			Parameters:  []byte(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
			Execute: func(ctx *sdk.Context, callID string, args map[string]any) (sdk.ToolResult, error) {
				text, _ := args["text"].(string)
				return sdk.ToolResult{Content: []sdk.ContentBlock{{Type: "text", Text: text}}}, nil
			},
		})
		api.OnSessionStart(func(ctx *sdk.Context, event sdk.Event) (any, error) {
			return nil, ctx.API().SetSessionName("protocol example")
		})
	})
}
