package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// Plugin is the unit of SDK extension. A plugin registers any combination of
// tools, providers, hooks, commands, prompt sections, MCP servers and UI
// declarations. Built-in capabilities use the same interface, so the runtime
// does not need a second extension mechanism.
type Plugin interface {
	Name() string
	Register(*Registry) error
}

// PluginFunc adapts a function to Plugin.
type PluginFunc struct {
	PluginName string
	Func       func(*Registry) error
}

func (p PluginFunc) Name() string {
	if p.PluginName == "" {
		return "plugin"
	}
	return p.PluginName
}

func (p PluginFunc) Register(r *Registry) error {
	if p.Func == nil {
		return nil
	}
	return p.Func(r)
}

// Tool is one model-callable tool. Definition is static; Execute runs one
// call. Built-in tools and plugin tools use the same interface.
type Tool interface {
	Definition() ToolDefinition
	Execute(context.Context, ToolCall) (ToolResult, error)
}

type ToolDefinition struct {
	Name             string          `json:"name"`
	Label            string          `json:"label,omitempty"`
	Description      string          `json:"description,omitempty"`
	Parameters       json.RawMessage `json:"parameters,omitempty"`
	ExecutionMode    string          `json:"executionMode,omitempty"`
	PromptSnippet    string          `json:"promptSnippet,omitempty"`
	PromptGuidelines []string        `json:"promptGuidelines,omitempty"`
	// Exposure is direct (default), codemode, deferred or hidden. codemode
	// and deferred tools stay in the catalogue but are not declared to the
	// model until codemode or tool_search reaches them.
	Exposure string `json:"exposure,omitempty"`
}

type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolFunc adapts a function to Tool.
type ToolFunc struct {
	Def ToolDefinition
	Run func(context.Context, ToolCall) (ToolResult, error)
}

func (t ToolFunc) Definition() ToolDefinition { return t.Def }
func (t ToolFunc) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	if t.Run == nil {
		return ToolResult{}, errors.New("tool has no Execute function")
	}
	return t.Run(ctx, call)
}

// Provider is one model supplier. The built-in chat, responses and messages
// protocols implement it; provider plugins can add endpoints or route to a
// custom supplier using the same interface.
type Provider interface {
	ID() string
	Models() []Model
	Stream(context.Context, Model, Transcript, StreamOptions) *EventStream
}

// ProviderFunc adapts a function to Provider.
type ProviderFunc struct {
	ProviderID string
	ModelList  []Model
	Fn         StreamFn
}

func (p ProviderFunc) ID() string      { return p.ProviderID }
func (p ProviderFunc) Models() []Model { return append([]Model(nil), p.ModelList...) }
func (p ProviderFunc) Stream(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
	if p.Fn == nil {
		stream := NewEventStream()
		msg := Message{
			Role:         "assistant",
			Timestamp:    time.Now().UnixMilli(),
			StopReason:   StopError,
			ErrorMessage: "provider has no Stream function",
		}
		stream.Push(AssistantMessageEvent{Type: "error", Reason: StopError, Error: &msg})
		stream.End(nil)
		return stream
	}
	return p.Fn(ctx, model, tr, opts)
}

// PromptInput is the text and attachments before a prompt is sent.
type PromptInput struct {
	Text   string
	Source string
	Images []ImageAttachment
}

// ToolDecision lets a BeforeTool hook block a tool call.
type ToolDecision struct {
	Block  bool
	Reason string
}

// Hooks are optional lifecycle callbacks. A hook runs in registration order.
// Returning an error aborts the operation for the error-returning hooks.
type Hooks struct {
	SessionStart func(context.Context, *Session) error
	SessionEnd   func(context.Context, *Session) error
	BeforeSend   func(context.Context, *Session, *PromptInput) error
	BeforeTool   func(context.Context, *Session, ToolCall) (*ToolDecision, error)
	AfterTool    func(context.Context, *Session, ToolCall, ToolResult) error
	MessageEnd   func(context.Context, *Session, Message) error
}

// Command is a slash command exposed by the host.
type Command struct {
	Name         string
	Description  string
	ArgumentHint string
	Run          func(context.Context, *Session, string) error
}

// PromptSection contributes one named system-prompt section. Build is called
// whenever the session's system prompt is built.
type PromptSection struct {
	Name  string
	Build func(context.Context, *Session) (string, error)
}

// MCPServer is a declarative MCP tool source registered by a plugin.
type MCPServer struct {
	Name      string
	Transport string // "stdio" or "http"
	Command   string
	Args      []string
	Env       map[string]string
	CWD       string
	URL       string
	Headers   map[string]string
	Tools     []string
	// Exposure defaults to "direct" for plugin declarations. It accepts the
	// same values as mcp.json: direct, codemode, deferred or hidden.
	Exposure     string
	ToolExposure map[string]string
	Description  string
}

// Renderer and Shortcut are host-facing UI declarations. Nulo only preserves
// them; the host decides how to render them.
type Renderer struct {
	Plugin string
	Kind   string
	Name   string
}

type Shortcut struct {
	Plugin      string
	Key         string
	Description string
}

type UIDeclaration struct {
	Renderers []Renderer
	Shortcuts []Shortcut
}

// Registry holds the capabilities contributed by all loaded plugins.
type Registry struct {
	mu             sync.RWMutex
	plugins        []Plugin
	tools          map[string]Tool
	toolOrder      []string
	providers      map[string]Provider
	hooks          []Hooks
	commands       map[string]Command
	promptSections []PromptSection
	mcpServers     map[string]MCPServer
	ui             UIDeclaration
	closed         bool
}

func NewRegistry() *Registry {
	return &Registry{
		tools:      map[string]Tool{},
		providers:  map[string]Provider{},
		commands:   map[string]Command{},
		mcpServers: map[string]MCPServer{},
	}
}

func (r *Registry) ensure() {
	if r.tools == nil {
		r.tools = map[string]Tool{}
	}
	if r.providers == nil {
		r.providers = map[string]Provider{}
	}
	if r.commands == nil {
		r.commands = map[string]Command{}
	}
	if r.mcpServers == nil {
		r.mcpServers = map[string]MCPServer{}
	}
}

// Register loads one plugin. A plugin's contributions are added in call order.
func (r *Registry) Register(p Plugin) error {
	if p == nil {
		return errors.New("sdk: nil plugin")
	}
	if err := p.Register(r); err != nil {
		return fmt.Errorf("sdk: plugin %s: %w", p.Name(), err)
	}
	r.mu.Lock()
	r.plugins = append(r.plugins, p)
	r.mu.Unlock()
	return nil
}

func (r *Registry) Plugins() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Plugin(nil), r.plugins...)
}

func (r *Registry) AddTool(t Tool) error {
	if t == nil {
		return errors.New("sdk: nil tool")
	}
	def := t.Definition()
	if def.Name == "" {
		return errors.New("sdk: tool name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	if _, exists := r.tools[def.Name]; exists {
		return fmt.Errorf("sdk: duplicate tool %q", def.Name)
	}
	r.tools[def.Name] = t
	r.toolOrder = append(r.toolOrder, def.Name)
	return nil
}

// ReplaceTool installs t under its name, replacing a built-in or earlier
// plugin tool. Use AddTool when a duplicate should be an error.
func (r *Registry) ReplaceTool(t Tool) error {
	if t == nil {
		return errors.New("sdk: nil tool")
	}
	def := t.Definition()
	if def.Name == "" {
		return errors.New("sdk: tool name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	if _, exists := r.tools[def.Name]; !exists {
		r.toolOrder = append(r.toolOrder, def.Name)
	}
	r.tools[def.Name] = t
	return nil
}

func (r *Registry) Tools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.toolOrder))
	for _, name := range r.toolOrder {
		if t := r.tools[name]; t != nil {
			out = append(out, t)
		}
	}
	return out
}

// AgentTools converts registered tools to the runtime's internal tool shape.
func (r *Registry) AgentTools() []AgentTool {
	tools := r.Tools()
	out := make([]AgentTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, agentToolFromPlugin(tool))
	}
	return out
}

func agentToolFromPlugin(t Tool) agent.AgentTool {
	def := t.Definition()
	schema := def.Parameters
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return agent.AgentTool{
		Tool: ai.Tool{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  schema,
		},
		Label:         def.Label,
		ExecutionMode: def.ExecutionMode,
		Execute: func(ctx context.Context, toolCallID string, args map[string]any) (agent.ToolResult, error) {
			return t.Execute(ctx, ToolCall{ID: toolCallID, Name: def.Name, Arguments: args})
		},
	}
}

func (r *Registry) AddProvider(p Provider) error {
	if p == nil {
		return errors.New("sdk: nil provider")
	}
	if p.ID() == "" {
		return errors.New("sdk: provider ID is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	if _, exists := r.providers[p.ID()]; exists {
		return fmt.Errorf("sdk: duplicate provider %q", p.ID())
	}
	r.providers[p.ID()] = p
	return nil
}

// ReplaceProvider installs p under its ID, replacing a built-in or earlier
// plugin provider.
func (r *Registry) ReplaceProvider(p Provider) error {
	if p == nil {
		return errors.New("sdk: nil provider")
	}
	if p.ID() == "" {
		return errors.New("sdk: provider ID is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	r.providers[p.ID()] = p
	return nil
}

func (r *Registry) Providers() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

var protocolAliases = map[string]string{
	"chat":      "openai-completions",
	"responses": "openai-responses",
	"messages":  "anthropic-messages",
}

// StreamFn resolves a provider by model.Provider, then model.API. The short
// protocol IDs chat/responses/messages fall back to the built-in wire
// implementations when no plugin provides them. A nil result means the runtime
// should use its own fallback.
func (r *Registry) StreamFn(model Model) StreamFn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p := r.providers[model.Provider]; p != nil {
		return p.Stream
	}
	if p := r.providers[model.API]; p != nil {
		return p.Stream
	}
	if alias := protocolAliases[model.API]; alias != "" {
		if p := r.providers[alias]; p != nil {
			return func(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
				model.API = alias
				return p.Stream(ctx, model, tr, opts)
			}
		}
	}
	return nil
}

// Models returns every model declared by registered providers. Duplicate
// provider/model pairs keep the first declaration.
func (r *Registry) Models() []Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{}
	var out []Model
	for _, p := range r.providers {
		for _, m := range p.Models() {
			key := m.Provider + "\x00" + m.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider == out[j].Provider {
			return out[i].ID < out[j].ID
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

func (r *Registry) AddHooks(h Hooks) {
	r.mu.Lock()
	r.hooks = append(r.hooks, h)
	r.mu.Unlock()
}

func (r *Registry) Hooks() []Hooks {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Hooks(nil), r.hooks...)
}

func (r *Registry) AddCommand(c Command) error {
	if c.Name == "" {
		return errors.New("sdk: command name is required")
	}
	if c.Run == nil {
		return fmt.Errorf("sdk: command %q has no Run function", c.Name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	if _, exists := r.commands[c.Name]; exists {
		return fmt.Errorf("sdk: duplicate command %q", c.Name)
	}
	r.commands[c.Name] = c
	return nil
}

func (r *Registry) Commands() []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Command, 0, len(r.commands))
	for _, c := range r.commands {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) AddPromptSection(s PromptSection) error {
	if s.Name == "" {
		return errors.New("sdk: prompt section name is required")
	}
	if s.Build == nil {
		return fmt.Errorf("sdk: prompt section %q has no Build function", s.Name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.promptSections = append(r.promptSections, s)
	return nil
}

func (r *Registry) PromptSections() []PromptSection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]PromptSection(nil), r.promptSections...)
}

func (r *Registry) AddMCPServer(s MCPServer) error {
	if s.Name == "" {
		return errors.New("sdk: MCP server name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	if _, exists := r.mcpServers[s.Name]; exists {
		return fmt.Errorf("sdk: duplicate MCP server %q", s.Name)
	}
	r.mcpServers[s.Name] = s
	return nil
}

func (r *Registry) MCPServers() []MCPServer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]MCPServer, 0, len(r.mcpServers))
	for _, s := range r.mcpServers {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) AddUI(u UIDeclaration) {
	r.mu.Lock()
	r.ui.Renderers = append(r.ui.Renderers, u.Renderers...)
	r.ui.Shortcuts = append(r.ui.Shortcuts, u.Shortcuts...)
	r.mu.Unlock()
}

func (r *Registry) UI() UIDeclaration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return UIDeclaration{
		Renderers: append([]Renderer(nil), r.ui.Renderers...),
		Shortcuts: append([]Shortcut(nil), r.ui.Shortcuts...),
	}
}

// Close releases plugins that own external resources.
func (r *Registry) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	plugins := append([]Plugin(nil), r.plugins...)
	r.mu.Unlock()
	var errs []error
	for i := len(plugins) - 1; i >= 0; i-- {
		if closer, ok := plugins[i].(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("plugin %s: %w", plugins[i].Name(), err))
			}
		}
	}
	return errors.Join(errs...)
}

// RunSessionStart invokes SessionStart hooks in registration order.
func (r *Registry) RunSessionStart(ctx context.Context, s *Session) error {
	for _, h := range r.Hooks() {
		if h.SessionStart != nil {
			if err := h.SessionStart(ctx, s); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Registry) RunSessionEnd(ctx context.Context, s *Session) error {
	for _, h := range r.Hooks() {
		if h.SessionEnd != nil {
			if err := h.SessionEnd(ctx, s); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Registry) RunBeforeSend(ctx context.Context, s *Session, in *PromptInput) error {
	for _, h := range r.Hooks() {
		if h.BeforeSend != nil {
			if err := h.BeforeSend(ctx, s, in); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Registry) RunBeforeTool(ctx context.Context, s *Session, call ToolCall) (*ToolDecision, error) {
	for _, h := range r.Hooks() {
		if h.BeforeTool == nil {
			continue
		}
		decision, err := h.BeforeTool(ctx, s, call)
		if err != nil {
			return nil, err
		}
		if decision != nil && decision.Block {
			return decision, nil
		}
	}
	return nil, nil
}

func (r *Registry) RunAfterTool(ctx context.Context, s *Session, call ToolCall, result ToolResult) error {
	for _, h := range r.Hooks() {
		if h.AfterTool != nil {
			if err := h.AfterTool(ctx, s, call, result); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Registry) RunMessageEnd(ctx context.Context, s *Session, message Message) error {
	for _, h := range r.Hooks() {
		if h.MessageEnd != nil {
			if err := h.MessageEnd(ctx, s, message); err != nil {
				return err
			}
		}
	}
	return nil
}
