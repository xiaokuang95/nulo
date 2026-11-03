package main

// A protocol plugin that implements its own subagent convention. The host
// only creates child sessions; the agent types, prompts, tools and result
// format live here, in the plugin.
//
// Build:
//
//	go build -o plugin-subagent .
//
// The plugin.json beside this file points at that binary.
//
//go:generate go build -o plugin-subagent.exe .
//go:generate go build -o plugin-subagent .

import (
	"fmt"
	"os"

	"github.com/xiaokuang95/nulo/sdk/plugin"
)

type agentDefinition struct {
	SystemPrompt string
	Tools        []string
	Model        string
}

// Replace this map with a file loader if you want .nulo/agents/*.md or a
// remote agent registry. Nothing here is known to the host.
var agentTypes = map[string]agentDefinition{
	"scout": {
		SystemPrompt: "You are a read-only scout. Investigate the codebase and return concise findings with file paths.",
		Tools:        []string{"read"},
	},
	"worker": {
		SystemPrompt: "You are a worker. Make the requested code changes, verify them, and report exactly what changed.",
		Tools:        []string{"read", "edit", "write"},
	},
}

func main() {
	if err := sdk.ServeProtocol(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	sdk.ServeNamed("subagent-example", "1.0.0", func(api *sdk.API) {
		api.RegisterTool(sdk.ToolDefinition{
			Name:        "Agent",
			Description: "Run a plugin-defined subagent in an independent child session.",
			Parameters: []byte(`{
				"type":"object",
				"properties":{
					"subagent_type":{"type":"string","description":"scout or worker"},
					"prompt":{"type":"string","description":"The task for the child agent"}
				},
				"required":["subagent_type","prompt"]
			}`),
			Execute: func(ctx *sdk.Context, callID string, args map[string]any) (sdk.ToolResult, error) {
				agentType, _ := args["subagent_type"].(string)
				definition, ok := agentTypes[agentType]
				if !ok {
					return sdk.ToolResult{}, fmt.Errorf("unknown subagent_type %q", agentType)
				}
				prompt, _ := args["prompt"].(string)
				if prompt == "" {
					return sdk.ToolResult{}, fmt.Errorf("prompt is required")
				}

				child, err := ctx.API().CreateSession(sdk.SessionOptions{
					SystemPrompt: definition.SystemPrompt,
					Tools:        definition.Tools,
					Model:        definition.Model,
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
					Content: []sdk.ContentBlock{{Type: "text", Text: result.Text}},
				}, nil
			},
		})
	})
}
