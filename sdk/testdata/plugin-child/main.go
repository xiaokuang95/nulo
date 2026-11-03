package main

// Protocol fixture for the plugin-created child session API. The fixture is a
// normal plugin: it decides that "child_echo" means "run a fresh child session
// with this system prompt", then returns the child's final text. The host does
// not know about any agent type or prompt convention.

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
	sdk.ServeNamed("child-fixture", "1.0.0", func(api *sdk.API) {
		api.RegisterTool(sdk.ToolDefinition{
			Name:        "child_echo",
			Description: "Run a fresh child session and return its answer.",
			Parameters:  []byte(`{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"]}`),
			Execute: func(ctx *sdk.Context, callID string, args map[string]any) (sdk.ToolResult, error) {
				prompt, _ := args["prompt"].(string)
				child, err := ctx.API().CreateSession(sdk.SessionOptions{
					SystemPrompt: "You are a child agent.",
					NoTools:      true,
				})
				if err != nil {
					return sdk.ToolResult{}, err
				}
				defer child.Dispose()
				result, err := child.Send(prompt)
				if err != nil {
					return sdk.ToolResult{}, err
				}
				return sdk.ToolResult{
					Content: []sdk.ContentBlock{{Type: "text", Text: "child:" + result.Text}},
				}, nil
			},
		})
	})
}
