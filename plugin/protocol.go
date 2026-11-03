package plugin

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// APIVersion is the C ABI plus JSON protocol version. A plugin declares the
// version it was built against in its registration; the host refuses a
// mismatch instead of guessing.
const APIVersion = 1

// Symbol names exported by every native plugin library.
const (
	InitSymbol = "NuloPluginInit"
	CallSymbol = "NuloPluginCall"
	FreeSymbol = "NuloPluginFree"
)

// HostInfo tells a plugin where it is running. It is sent before the plugin
// registers anything.
type HostInfo struct {
	APIVersion int    `json:"apiVersion"`
	Host       string `json:"host"`
	Version    string `json:"version,omitempty"`
	CWD        string `json:"cwd,omitempty"`
	AgentDir   string `json:"agentDir,omitempty"`
	Session    string `json:"sessionFile,omitempty"`
	Mode       string `json:"mode,omitempty"`
	HasUI      bool   `json:"hasUI,omitempty"`
}

// Registration is what a plugin returns from Init.
type Registration struct {
	APIVersion  int            `json:"apiVersion"`
	Name        string         `json:"name"`
	Version     string         `json:"version,omitempty"`
	Description string         `json:"description,omitempty"`
	Tools       []ToolSpec     `json:"tools,omitempty"`
	Commands    []CommandSpec  `json:"commands,omitempty"`
	Flags       []FlagSpec     `json:"flags,omitempty"`
	Shortcuts   []ShortcutSpec `json:"shortcuts,omitempty"`
	Renderers   []RendererSpec `json:"renderers,omitempty"`
	Events      []string       `json:"events,omitempty"`
	// Error reports a factory failure. A plugin still loads, but the host
	// reports this diagnostic and registers nothing from it.
	Error string `json:"error,omitempty"`
}

// ToolSpec is the host-side half of plugin/sdk.ToolDefinition.
type ToolSpec struct {
	Name             string          `json:"name"`
	Label            string          `json:"label,omitempty"`
	Description      string          `json:"description,omitempty"`
	PromptSnippet    string          `json:"promptSnippet,omitempty"`
	PromptGuidelines []string        `json:"promptGuidelines,omitempty"`
	Parameters       json.RawMessage `json:"parameters,omitempty"`
	Exposure         string          `json:"exposure,omitempty"`
	ExecutionMode    string          `json:"executionMode,omitempty"`
	DefaultActive    *bool           `json:"defaultActive,omitempty"`
	RenderShell      string          `json:"renderShell,omitempty"`
	Annotations      map[string]bool `json:"annotations,omitempty"`
}

// CommandSpec describes a slash command. The handler stays in the plugin.
type CommandSpec struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	ArgumentHint string `json:"argumentHint,omitempty"`
}

// FlagSpec describes a CLI flag. Plugin flags are exposed to plugin code and
// to the embedding host; nulo itself has no CLI parser.
type FlagSpec struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Default     any    `json:"default,omitempty"`
}

// ShortcutSpec describes a keyboard shortcut the host may bind.
type ShortcutSpec struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
}

// RendererSpec records a renderer registration. Rendering belongs to the host;
// nulo only preserves the declaration so a TUI host can consume it.
type RendererSpec struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

// Event names accepted by On. They follow Pi's extension event names.
var EventNames = []string{
	"project_trust",
	"resources_discover",
	"session_start",
	"session_info_changed",
	"session_before_switch",
	"session_before_fork",
	"session_before_compact",
	"session_compact",
	"session_compact_failed",
	"session_shutdown",
	"mcp_servers_change",
	"session_before_tree",
	"session_tree",
	"context",
	"context_with_system",
	"cache_warming_decision",
	"before_provider_request",
	"before_provider_headers",
	"after_provider_response",
	"provider_stream_event",
	"before_agent_start",
	"agent_start",
	"agent_end",
	"agent_before_settle",
	"agent_settled",
	"ui_prompt_start",
	"ui_prompt_end",
	"turn_start",
	"turn_end",
	"message_start",
	"message_update",
	"message_end",
	"tool_execution_start",
	"tool_execution_update",
	"tool_execution_end",
	"model_select",
	"thinking_level_select",
	"tool_call",
	"tool_result",
	"user_bash",
	"input",
}

var eventNameSet = func() map[string]bool {
	m := make(map[string]bool, len(EventNames))
	for _, name := range EventNames {
		m[name] = true
	}
	return m
}()

// IsEvent reports whether name is an event in the Pi extension surface.
func IsEvent(name string) bool { return eventNameSet[name] }

// ProtocolMessage is one JSON frame on a protocol plugin's stdin/stdout.
// Frames are newline-delimited JSON objects.
type ProtocolMessage struct {
	Type         string        `json:"type"`
	ID           uint64        `json:"id,omitempty"`
	HostInfo     *HostInfo     `json:"hostInfo,omitempty"`
	Registration *Registration `json:"registration,omitempty"`
	Invoke       *Invoke       `json:"invoke,omitempty"`
	Result       *InvokeResult `json:"result,omitempty"`
	Call         *HostCall     `json:"call,omitempty"`
	Reply        *HostReply    `json:"reply,omitempty"`
	Error        string        `json:"error,omitempty"`
}

// Invoke is one call into a loaded plugin.
type Invoke struct {
	// Type is "event", "tool", "command", "shortcut" or "host".
	Type string `json:"type"`
	// Event is set for event calls.
	Event string `json:"event,omitempty"`
	// Name is the tool or command name for tool and command calls.
	Name string `json:"name,omitempty"`
	// ToolCallID is stable for one model-issued tool call.
	ToolCallID string `json:"toolCallId,omitempty"`
	// Input is the tool input object or command argument string.
	Input json.RawMessage `json:"input,omitempty"`
	// Context is a snapshot supplied by the host for this call.
	Context json.RawMessage `json:"context,omitempty"`
	// Data is the event payload.
	Data json.RawMessage `json:"data,omitempty"`
}

// InvokeResult is the plugin's answer to an Invoke.
type InvokeResult struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// HostCall is a plugin-initiated API request.
type HostCall struct {
	ID     uint64          `json:"id"`
	Plugin string          `json:"plugin,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// HostReply is the host's answer to a HostCall. Exactly one of Result and Error
// is set.
type HostReply struct {
	ID     uint64    `json:"id"`
	Result any       `json:"result,omitempty"`
	Error  *APIError `json:"error,omitempty"`
}

// APIError is a JSON-friendly error returned across the plugin boundary.
type APIError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ToolResult is the value a plugin tool returns. It deliberately mirrors the
// useful subset of Pi's AgentToolResult and nulo's agent.ToolResult.
type ToolResult struct {
	Content           []ContentBlock `json:"content,omitempty"`
	Details           any            `json:"details,omitempty"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	Usage             any            `json:"usage,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	Terminate         bool           `json:"terminate,omitempty"`
}

// ContentBlock is a text or image content block. Other Pi block types are
// carried as JSON so a plugin can round-trip them without nulo knowing them.
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	Raw      any    `json:"-"`
}

// MarshalJSON keeps unknown blocks intact.
func (b ContentBlock) MarshalJSON() ([]byte, error) {
	if b.Raw != nil {
		return json.Marshal(b.Raw)
	}
	type plain ContentBlock
	return json.Marshal(plain(b))
}

// UnmarshalJSON preserves blocks nulo does not model.
func (b *ContentBlock) UnmarshalJSON(data []byte) error {
	type plain ContentBlock
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*b = ContentBlock(p)
	if b.Type != "text" && b.Type != "image" {
		var raw any
		if err := json.Unmarshal(data, &raw); err == nil {
			b.Raw = raw
		}
	}
	return nil
}

func validateRegistration(reg *Registration) error {
	if reg.Error != "" {
		return fmt.Errorf("plugin init failed: %s", reg.Error)
	}
	if reg.APIVersion != APIVersion {
		return fmt.Errorf("plugin API version %d is not supported (host API version %d)", reg.APIVersion, APIVersion)
	}
	if strings.TrimSpace(reg.Name) == "" {
		return fmt.Errorf("plugin registration has no name")
	}
	seenTools := map[string]bool{}
	for i := range reg.Tools {
		t := &reg.Tools[i]
		if strings.TrimSpace(t.Name) == "" {
			return fmt.Errorf("plugin %q: tool %d has no name", reg.Name, i)
		}
		if seenTools[t.Name] {
			return fmt.Errorf("plugin %q: duplicate tool %q", reg.Name, t.Name)
		}
		seenTools[t.Name] = true
		if len(t.Parameters) == 0 {
			t.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		var schema any
		if err := json.Unmarshal(t.Parameters, &schema); err != nil {
			return fmt.Errorf("plugin %q: tool %q parameters: %w", reg.Name, t.Name, err)
		}
		if _, ok := schema.(map[string]any); !ok {
			return fmt.Errorf("plugin %q: tool %q parameters must be a JSON object", reg.Name, t.Name)
		}
		switch t.Exposure {
		case "", "direct", "model-only", "codemode", "deferred", "hidden":
		default:
			return fmt.Errorf("plugin %q: tool %q has unknown exposure %q", reg.Name, t.Name, t.Exposure)
		}
		switch t.ExecutionMode {
		case "", "parallel", "sequential":
		default:
			return fmt.Errorf("plugin %q: tool %q has unknown execution mode %q", reg.Name, t.Name, t.ExecutionMode)
		}
		if t.ExecutionMode == "" {
			t.ExecutionMode = "parallel"
		}
	}
	seenCommands := map[string]bool{}
	for i := range reg.Commands {
		c := &reg.Commands[i]
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("plugin %q: command %d has no name", reg.Name, i)
		}
		if seenCommands[c.Name] {
			return fmt.Errorf("plugin %q: duplicate command %q", reg.Name, c.Name)
		}
		seenCommands[c.Name] = true
	}
	seenEvents := map[string]bool{}
	for _, name := range reg.Events {
		if !IsEvent(name) {
			return fmt.Errorf("plugin %q: unknown event %q", reg.Name, name)
		}
		if seenEvents[name] {
			continue
		}
		seenEvents[name] = true
	}
	return nil
}

// ToolActive reports whether a tool with this exposure is declared to the model
// immediately after registration, matching Pi's ToolDefinition.defaultActive.
func (t ToolSpec) ToolActive() bool {
	if t.DefaultActive != nil {
		if !*t.DefaultActive {
			return false
		}
	}
	exposure := t.Exposure
	if exposure == "" {
		exposure = "direct"
	}
	return exposure == "direct" || exposure == "model-only"
}

// ToolExposure returns the effective exposure, defaulting to direct.
func (t ToolSpec) ToolExposure() string {
	if t.Exposure == "" {
		return "direct"
	}
	return t.Exposure
}

// DecodeRegistration decodes and validates Init output.
func DecodeRegistration(data []byte) (Registration, error) {
	var reg Registration
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&reg); err != nil {
		return Registration{}, fmt.Errorf("decode plugin registration: %w", err)
	}
	if err := validateRegistration(&reg); err != nil {
		return Registration{}, err
	}
	return reg, nil
}

// DecodeInvokeResult decodes a Call result.
func DecodeInvokeResult(data []byte) (InvokeResult, error) {
	var res InvokeResult
	if err := json.Unmarshal(data, &res); err != nil {
		return InvokeResult{}, fmt.Errorf("decode plugin result: %w", err)
	}
	return res, nil
}

// NumberAsInt converts a JSON number produced by encoding/json or UseNumber.
func NumberAsInt(v any) (int, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := strconv.Atoi(n.String())
		return i, err == nil
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}
