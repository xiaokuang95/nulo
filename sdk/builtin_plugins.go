package sdk

import (
	"context"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/tools"
)

// builtinToolsPlugin registers the SDK's default file and shell tools through
// the same Tool interface plugin authors use.
func builtinToolsPlugin(cwd string) Plugin {
	return PluginFunc{
		PluginName: "builtin-tools",
		Func: func(r *Registry) error {
			for _, tool := range tools.Default(cwd) {
				if err := r.AddTool(agentToolAdapter{tool: tool}); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

type agentToolAdapter struct {
	tool agent.AgentTool
}

func (a agentToolAdapter) Definition() ToolDefinition {
	return ToolDefinition{
		Name:          a.tool.Name,
		Label:         a.tool.Label,
		Description:   a.tool.Description,
		Parameters:    a.tool.Parameters,
		ExecutionMode: a.tool.ExecutionMode,
	}
}

func (a agentToolAdapter) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	return a.tool.Execute(ctx, call.ID, call.Arguments)
}

// builtinProvidersPlugin registers the three wire protocols as providers. The
// provider lookup uses model.Provider first, then model.API, so the built-ins
// can be overridden by a plugin with the same ID.
func builtinProvidersPlugin(agentDir string) Plugin {
	stream := ai.StreamForDir(agentDir)
	return PluginFunc{
		PluginName: "builtin-providers",
		Func: func(r *Registry) error {
			for _, id := range []string{
				"openai-completions",
				"openai-responses",
				"anthropic-messages",
			} {
				if err := r.AddProvider(ProviderFunc{ProviderID: id, Fn: stream}); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
