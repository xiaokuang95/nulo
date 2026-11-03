package sdk

import (
	"encoding/json"

	"github.com/xiaokuang95/nulo/sdk/internal/plugin"
)

// ToolResult is what a plugin tool returns.
type ToolResult = plugin.ToolResult

// ContentBlock is a text or image block.
type ContentBlock = plugin.ContentBlock

// Event is one extension event. Keys and values follow Pi's event types; using a
// map keeps the protocol forward-compatible when nulo adds a field.
type Event map[string]any

// Handler handles one event. A handler may return a result object for events
// that define one (tool_call, tool_result, context, input, ...). Returning nil
// means "no change".
type Handler func(ctx *Context, event Event) (any, error)

// Factory registers a plugin's tools, commands and event handlers.
type Factory func(api *API)

// ToolDefinition registers one model-callable tool. Execute runs inside the
// plugin; the host never sees its implementation.
type ToolDefinition struct {
	Name             string
	Label            string
	Description      string
	PromptSnippet    string
	PromptGuidelines []string
	// Parameters is a JSON Schema object. Nil means an empty object schema.
	Parameters json.RawMessage
	// Exposure is direct, model-only, codemode, deferred or hidden. Empty means
	// direct.
	Exposure string
	// ExecutionMode is parallel or sequential. Empty means parallel.
	ExecutionMode string
	// DefaultActive overrides whether a direct/model-only tool is active at
	// registration. Other exposures are never active by default in Pi.
	DefaultActive *bool
	RenderShell   string
	Annotations   map[string]bool

	Execute func(ctx *Context, toolCallID string, args map[string]any) (ToolResult, error)
}

// CommandOptions configures a slash command.
type CommandOptions struct {
	Description  string
	ArgumentHint string
	Handler      func(ctx *CommandContext, args string) error
}

// FlagOptions configures a host CLI flag.
type FlagOptions struct {
	Description string
	Type        string
	Default     any
}

// ShortcutOptions configures a keyboard shortcut.
type ShortcutOptions struct {
	Description string
	Handler     func(ctx *Context) error
}

// RendererOptions records a renderer declaration for a UI host.
type RendererOptions struct {
	Kind string
	Name string
}

// DialogOptions mirrors Pi's ExtensionUIDialogOptions.
type DialogOptions struct {
	Timeout int
}

// WidgetOptions mirrors Pi's ExtensionWidgetOptions.
type WidgetOptions struct {
	Placement string
}

// WorkingIndicatorOptions mirrors Pi's working indicator settings.
type WorkingIndicatorOptions struct {
	Frames     []string
	IntervalMs int
}

// CompactOptions mirrors Pi's CompactOptions.
type CompactOptions struct {
	CustomInstructions string
}

// ExecOptions mirrors the options nulo can honour for a host shell command.
type ExecOptions struct {
	CWD   string
	Env   map[string]string
	Stdin string
}

// ExecResult mirrors Pi's ExecResult.
type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

// Model is a JSON-friendly model snapshot, matching nulo's ai.Model fields.
type Model map[string]any

// Settings is a JSON-friendly settings snapshot.
type Settings map[string]any

// ProviderConfig is Pi's registerProvider config. nulo accepts the model and
// endpoint fields it can route; UI/OAuth-only fields are preserved but ignored.
type ProviderConfig map[string]any

// McpServerConfig is Pi's registerMcpServer config.
type McpServerConfig map[string]any

// RegisteredMcpServer is a server registered by an extension.
type RegisteredMcpServer struct {
	Name          string          `json:"name"`
	Config        McpServerConfig `json:"config"`
	ExtensionPath string          `json:"extensionPath,omitempty"`
}

// VirtualModel is Pi's registerVirtualModel input. nulo stores the declaration
// and applies it when its router supports virtual models.
type VirtualModel map[string]any

// ToolInfo describes a registered tool for getAllTools.
type ToolInfo struct {
	Name             string          `json:"name"`
	Label            string          `json:"label,omitempty"`
	Description      string          `json:"description,omitempty"`
	Parameters       json.RawMessage `json:"parameters,omitempty"`
	Exposure         string          `json:"exposure,omitempty"`
	Source           string          `json:"source,omitempty"`
	PromptSnippet    string          `json:"promptSnippet,omitempty"`
	PromptGuidelines []string        `json:"promptGuidelines,omitempty"`
}

// SlashCommandInfo is a command visible to the host.
type SlashCommandInfo struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	ArgumentHint   string `json:"argumentHint,omitempty"`
	Source         string `json:"source,omitempty"`
	InvocationName string `json:"invocationName,omitempty"`
}

// ContextUsage mirrors Pi's context usage snapshot.
type ContextUsage struct {
	Tokens        *int     `json:"tokens"`
	ContextWindow int      `json:"contextWindow"`
	Percent       *float64 `json:"percent"`
}

// InputResult is Pi's input event result.
type InputResult struct {
	Action string `json:"action"`
	Text   string `json:"text,omitempty"`
}

// ToolCallResult is Pi's tool_call event result.
type ToolCallResult struct {
	Block     bool   `json:"block,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Terminate bool   `json:"terminate,omitempty"`
}

// ToolResultPatch is Pi's tool_result event result.
type ToolResultPatch struct {
	Content []ContentBlock `json:"content,omitempty"`
	Details any            `json:"details,omitempty"`
	IsError *bool          `json:"isError,omitempty"`
	Usage   any            `json:"usage,omitempty"`
}

// MessageEndPatch replaces a finalized message.
type MessageEndPatch struct {
	Message map[string]any `json:"message,omitempty"`
}

// ContextPatch replaces the conversation sent to the model.
type ContextPatch struct {
	Messages []any `json:"messages,omitempty"`
}

// BeforeAgentStartPatch replaces the system prompt or injects a custom message.
type BeforeAgentStartPatch struct {
	SystemPrompt string         `json:"systemPrompt,omitempty"`
	Message      map[string]any `json:"message,omitempty"`
}

// CompactPatch supplies a compaction result from an extension.
type CompactPatch struct {
	Cancel     bool           `json:"cancel,omitempty"`
	Compaction map[string]any `json:"compaction,omitempty"`
}

// SessionBeforeSwitchResult cancels a session switch.
type SessionBeforeSwitchResult struct {
	Cancel bool `json:"cancel,omitempty"`
}

// SessionBeforeForkResult cancels a fork and can skip conversation restore.
type SessionBeforeForkResult struct {
	Cancel                  bool `json:"cancel,omitempty"`
	SkipConversationRestore bool `json:"skipConversationRestore,omitempty"`
}

// SessionBeforeTreeResult cancels or supplies a tree summary.
type SessionBeforeTreeResult struct {
	Cancel              bool           `json:"cancel,omitempty"`
	Summary             map[string]any `json:"summary,omitempty"`
	CustomInstructions  string         `json:"customInstructions,omitempty"`
	ReplaceInstructions bool           `json:"replaceInstructions,omitempty"`
	Label               string         `json:"label,omitempty"`
}

// CacheWarmingDecisionResult overrides a scheduled cache refresh.
type CacheWarmingDecisionResult struct {
	Action string `json:"action,omitempty"`
}

// BeforeProviderRequestPatch replaces the provider request body.
type BeforeProviderRequestPatch struct {
	Payload any `json:"payload,omitempty"`
}

// BeforeProviderHeadersPatch replaces provider request headers.
type BeforeProviderHeadersPatch struct {
	Headers map[string]string `json:"headers,omitempty"`
}
