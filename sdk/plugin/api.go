package sdk

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/xiaokuang95/nulo/sdk/internal/plugin"
)

// API is the surface passed to a Factory. Its method names follow Pi's
// ExtensionAPI in Go casing.
type API struct {
	server *server
	mu     sync.Mutex
}

type toolEntry struct {
	spec    plugin.ToolSpec
	execute func(ctx *Context, toolCallID string, args map[string]any) (ToolResult, error)
}

type commandEntry struct {
	spec    plugin.CommandSpec
	handler func(ctx *CommandContext, args string) error
}

type shortcutEntry struct {
	spec    plugin.ShortcutSpec
	handler func(ctx *Context) error
}

func newAPI(s *server) *API { return &API{server: s} }

// On registers an event handler and returns an unsubscribe function.
func (a *API) On(event string, handler Handler) func() {
	if !plugin.IsEvent(event) {
		panic("plugin: unknown event " + event)
	}
	return a.server.addHandler(event, handler)
}

// OnProjectTrust is the typed form of On("project_trust").
func (a *API) OnProjectTrust(h Handler) func() { return a.On("project_trust", h) }

// OnResourcesDiscover is the typed form of On("resources_discover").
func (a *API) OnResourcesDiscover(h Handler) func() { return a.On("resources_discover", h) }

// OnSessionStart is the typed form of On("session_start").
func (a *API) OnSessionStart(h Handler) func() { return a.On("session_start", h) }

// OnSessionInfoChanged is the typed form of On("session_info_changed").
func (a *API) OnSessionInfoChanged(h Handler) func() { return a.On("session_info_changed", h) }

// OnSessionBeforeSwitch is the typed form of On("session_before_switch").
func (a *API) OnSessionBeforeSwitch(h Handler) func() { return a.On("session_before_switch", h) }

// OnSessionBeforeFork is the typed form of On("session_before_fork").
func (a *API) OnSessionBeforeFork(h Handler) func() { return a.On("session_before_fork", h) }

// OnSessionBeforeCompact is the typed form of On("session_before_compact").
func (a *API) OnSessionBeforeCompact(h Handler) func() { return a.On("session_before_compact", h) }

// OnSessionCompact is the typed form of On("session_compact").
func (a *API) OnSessionCompact(h Handler) func() { return a.On("session_compact", h) }

// OnSessionCompactFailed is the typed form of On("session_compact_failed").
func (a *API) OnSessionCompactFailed(h Handler) func() { return a.On("session_compact_failed", h) }

// OnSessionShutdown is the typed form of On("session_shutdown").
func (a *API) OnSessionShutdown(h Handler) func() { return a.On("session_shutdown", h) }

// OnMcpServersChange is the typed form of On("mcp_servers_change").
func (a *API) OnMcpServersChange(h Handler) func() { return a.On("mcp_servers_change", h) }

// OnSessionBeforeTree is the typed form of On("session_before_tree").
func (a *API) OnSessionBeforeTree(h Handler) func() { return a.On("session_before_tree", h) }

// OnSessionTree is the typed form of On("session_tree").
func (a *API) OnSessionTree(h Handler) func() { return a.On("session_tree", h) }

// OnContext is the typed form of On("context").
func (a *API) OnContext(h Handler) func() { return a.On("context", h) }

// OnContextWithSystem is the typed form of On("context_with_system").
func (a *API) OnContextWithSystem(h Handler) func() { return a.On("context_with_system", h) }

// OnCacheWarmingDecision is the typed form of On("cache_warming_decision").
func (a *API) OnCacheWarmingDecision(h Handler) func() { return a.On("cache_warming_decision", h) }

// OnBeforeProviderRequest is the typed form of On("before_provider_request").
func (a *API) OnBeforeProviderRequest(h Handler) func() { return a.On("before_provider_request", h) }

// OnBeforeProviderHeaders is the typed form of On("before_provider_headers").
func (a *API) OnBeforeProviderHeaders(h Handler) func() { return a.On("before_provider_headers", h) }

// OnAfterProviderResponse is the typed form of On("after_provider_response").
func (a *API) OnAfterProviderResponse(h Handler) func() { return a.On("after_provider_response", h) }

// OnProviderStreamEvent is the typed form of On("provider_stream_event").
func (a *API) OnProviderStreamEvent(h Handler) func() { return a.On("provider_stream_event", h) }

// OnBeforeAgentStart is the typed form of On("before_agent_start").
func (a *API) OnBeforeAgentStart(h Handler) func() { return a.On("before_agent_start", h) }

// OnAgentStart is the typed form of On("agent_start").
func (a *API) OnAgentStart(h Handler) func() { return a.On("agent_start", h) }

// OnAgentEnd is the typed form of On("agent_end").
func (a *API) OnAgentEnd(h Handler) func() { return a.On("agent_end", h) }

// OnAgentBeforeSettle is the typed form of On("agent_before_settle").
func (a *API) OnAgentBeforeSettle(h Handler) func() { return a.On("agent_before_settle", h) }

// OnAgentSettled is the typed form of On("agent_settled").
func (a *API) OnAgentSettled(h Handler) func() { return a.On("agent_settled", h) }

// OnUiPromptStart is the typed form of On("ui_prompt_start").
func (a *API) OnUiPromptStart(h Handler) func() { return a.On("ui_prompt_start", h) }

// OnUiPromptEnd is the typed form of On("ui_prompt_end").
func (a *API) OnUiPromptEnd(h Handler) func() { return a.On("ui_prompt_end", h) }

// OnTurnStart is the typed form of On("turn_start").
func (a *API) OnTurnStart(h Handler) func() { return a.On("turn_start", h) }

// OnTurnEnd is the typed form of On("turn_end").
func (a *API) OnTurnEnd(h Handler) func() { return a.On("turn_end", h) }

// OnMessageStart is the typed form of On("message_start").
func (a *API) OnMessageStart(h Handler) func() { return a.On("message_start", h) }

// OnMessageUpdate is the typed form of On("message_update").
func (a *API) OnMessageUpdate(h Handler) func() { return a.On("message_update", h) }

// OnMessageEnd is the typed form of On("message_end").
func (a *API) OnMessageEnd(h Handler) func() { return a.On("message_end", h) }

// OnToolExecutionStart is the typed form of On("tool_execution_start").
func (a *API) OnToolExecutionStart(h Handler) func() { return a.On("tool_execution_start", h) }

// OnToolExecutionUpdate is the typed form of On("tool_execution_update").
func (a *API) OnToolExecutionUpdate(h Handler) func() { return a.On("tool_execution_update", h) }

// OnToolExecutionEnd is the typed form of On("tool_execution_end").
func (a *API) OnToolExecutionEnd(h Handler) func() { return a.On("tool_execution_end", h) }

// OnModelSelect is the typed form of On("model_select").
func (a *API) OnModelSelect(h Handler) func() { return a.On("model_select", h) }

// OnThinkingLevelSelect is the typed form of On("thinking_level_select").
func (a *API) OnThinkingLevelSelect(h Handler) func() { return a.On("thinking_level_select", h) }

// OnToolCall is the typed form of On("tool_call").
func (a *API) OnToolCall(h Handler) func() { return a.On("tool_call", h) }

// OnToolResult is the typed form of On("tool_result").
func (a *API) OnToolResult(h Handler) func() { return a.On("tool_result", h) }

// OnUserBash is the typed form of On("user_bash").
func (a *API) OnUserBash(h Handler) func() { return a.On("user_bash", h) }

// OnInput is the typed form of On("input").
func (a *API) OnInput(h Handler) func() { return a.On("input", h) }

// RegisterTool registers a tool that the model can call.
func (a *API) RegisterTool(def ToolDefinition) {
	if def.Name == "" {
		panic("plugin: tool name is required")
	}
	if def.Execute == nil {
		panic("plugin: tool " + def.Name + " has no Execute function")
	}
	spec := plugin.ToolSpec{
		Name:             def.Name,
		Label:            def.Label,
		Description:      def.Description,
		PromptSnippet:    def.PromptSnippet,
		PromptGuidelines: append([]string(nil), def.PromptGuidelines...),
		Parameters:       def.Parameters,
		Exposure:         def.Exposure,
		ExecutionMode:    def.ExecutionMode,
		DefaultActive:    def.DefaultActive,
		RenderShell:      def.RenderShell,
		Annotations:      def.Annotations,
	}
	a.server.mu.Lock()
	if _, exists := a.server.tools[def.Name]; exists {
		a.server.mu.Unlock()
		panic("plugin: duplicate tool " + def.Name)
	}
	a.server.tools[def.Name] = &toolEntry{spec: spec, execute: def.Execute}
	ready := a.server.ready
	a.server.mu.Unlock()
	if ready {
		if err := a.server.hostCallVoid("registerTool", spec); err != nil {
			panic("plugin: registerTool: " + err.Error())
		}
	}
}

// RegisterCommand registers a slash command.
func (a *API) RegisterCommand(name string, options CommandOptions) {
	if name == "" {
		panic("plugin: command name is required")
	}
	if options.Handler == nil {
		panic("plugin: command " + name + " has no Handler")
	}
	a.server.mu.Lock()
	if _, exists := a.server.commands[name]; exists {
		a.server.mu.Unlock()
		panic("plugin: duplicate command " + name)
	}
	entry := &commandEntry{
		spec:    plugin.CommandSpec{Name: name, Description: options.Description, ArgumentHint: options.ArgumentHint},
		handler: options.Handler,
	}
	a.server.commands[name] = entry
	ready := a.server.ready
	a.server.mu.Unlock()
	if ready {
		if err := a.server.hostCallVoid("registerCommand", entry.spec); err != nil {
			panic("plugin: registerCommand: " + err.Error())
		}
	}
}

// RegisterShortcut registers a keyboard shortcut for a UI host.
func (a *API) RegisterShortcut(shortcut string, options ShortcutOptions) {
	a.server.mu.Lock()
	spec := plugin.ShortcutSpec{Key: shortcut, Description: options.Description}
	a.server.shortcuts = append(a.server.shortcuts, spec)
	if options.Handler != nil {
		a.server.shortcutHandlers[shortcut] = options.Handler
	}
	ready := a.server.ready
	a.server.mu.Unlock()
	if ready {
		_ = a.server.hostCallVoid("registerShortcut", spec)
	}
}

// RegisterFlag registers a CLI flag. The host exposes its value through GetFlag.
func (a *API) RegisterFlag(name string, options FlagOptions) {
	kind := options.Type
	if kind == "" {
		kind = "boolean"
	}
	if kind != "boolean" && kind != "string" {
		panic("plugin: flag " + name + " type must be boolean or string")
	}
	a.server.mu.Lock()
	if _, exists := a.server.flags[name]; exists {
		a.server.mu.Unlock()
		panic("plugin: duplicate flag " + name)
	}
	spec := plugin.FlagSpec{Name: name, Description: options.Description, Type: kind, Default: options.Default}
	a.server.flags[name] = spec
	if options.Default != nil {
		a.server.flagValues[name] = options.Default
	}
	ready := a.server.ready
	a.server.mu.Unlock()
	if ready {
		_ = a.server.hostCallVoid("registerFlag", spec)
	}
}

// GetFlag returns the effective value of a registered flag.
func (a *API) GetFlag(name string) any {
	a.server.mu.Lock()
	ready := a.server.ready
	a.server.mu.Unlock()
	if ready {
		var value any
		if err := a.server.hostCall("getFlag", map[string]any{"name": name}, &value); err == nil && value != nil {
			return value
		}
	}
	a.server.mu.Lock()
	defer a.server.mu.Unlock()
	return a.server.flagValues[name]
}

// SetFlagValue lets a host set a flag before or after registration.
func (a *API) SetFlagValue(name string, value any) {
	a.server.mu.Lock()
	a.server.flagValues[name] = value
	ready := a.server.ready
	a.server.mu.Unlock()
	if ready {
		_ = a.server.hostCallVoid("setFlag", map[string]any{"name": name, "value": value})
	}
}

// RegisterMessageRenderer records a custom message renderer declaration.
func (a *API) RegisterMessageRenderer(customType string, options RendererOptions) {
	a.addRenderer("message", customType)
}

// RegisterEntryRenderer records a custom entry renderer declaration.
func (a *API) RegisterEntryRenderer(customType string, options RendererOptions) {
	a.addRenderer("entry", customType)
}

// RegisterMarkdownTransformer records a markdown transformer declaration.
func (a *API) RegisterMarkdownTransformer(options RendererOptions) {
	a.addRenderer("markdown", options.Name)
}

// RegisterToolRenderer records a tool renderer declaration.
func (a *API) RegisterToolRenderer(options RendererOptions) {
	a.addRenderer("tool", options.Name)
}

func (a *API) addRenderer(kind, name string) {
	a.server.mu.Lock()
	spec := plugin.RendererSpec{Kind: kind, Name: name}
	a.server.renderers = append(a.server.renderers, spec)
	ready := a.server.ready
	a.server.mu.Unlock()
	if ready {
		_ = a.server.hostCallVoid("registerRenderer", spec)
	}
}

// SendMessage sends a custom message to the session.
func (a *API) SendMessage(message map[string]any, options map[string]any) error {
	return a.server.hostCallVoid("sendMessage", map[string]any{"message": message, "options": options})
}

// SendUserMessage sends a user message to the agent.
func (a *API) SendUserMessage(content any, options map[string]any) error {
	return a.server.hostCallVoid("sendUserMessage", map[string]any{"content": content, "options": options})
}

// AppendEntry appends a custom entry for persistence, not LLM context.
func (a *API) AppendEntry(customType string, data any) error {
	return a.server.hostCallVoid("appendEntry", map[string]any{"customType": customType, "data": data})
}

// SetSessionName sets the session display name.
func (a *API) SetSessionName(name string) error {
	return a.server.hostCallVoid("setSessionName", map[string]any{"name": name})
}

// GetSessionName returns the session display name.
func (a *API) GetSessionName() (string, error) {
	var out string
	err := a.server.hostCall("getSessionName", nil, &out)
	return out, err
}

// SetLabel sets or clears a label on an entry.
func (a *API) SetLabel(entryID, label string) error {
	return a.server.hostCallVoid("setLabel", map[string]any{"entryId": entryID, "label": label})
}

// Exec runs a shell command.
func (a *API) Exec(command string, args []string, options *ExecOptions) (ExecResult, error) {
	var out ExecResult
	err := a.server.hostCall("exec", map[string]any{"command": command, "args": args, "options": options}, &out)
	return out, err
}

// GetActiveTools returns the names declared to the model.
func (a *API) GetActiveTools() ([]string, error) {
	var out []string
	err := a.server.hostCall("getActiveTools", nil, &out)
	return out, err
}

// GetAllTools returns every registered tool with metadata.
func (a *API) GetAllTools() ([]ToolInfo, error) {
	var out []ToolInfo
	err := a.server.hostCall("getAllTools", nil, &out)
	return out, err
}

// GetSettings returns the effective settings snapshot.
func (a *API) GetSettings() (Settings, error) {
	var out Settings
	err := a.server.hostCall("getSettings", nil, &out)
	return out, err
}

// SetActiveTools sets the tools declared to the model.
func (a *API) SetActiveTools(names []string) error {
	return a.server.hostCallVoid("setActiveTools", map[string]any{"tools": names})
}

// GetCommands returns the slash commands visible to the host.
func (a *API) GetCommands() ([]SlashCommandInfo, error) {
	var out []SlashCommandInfo
	err := a.server.hostCall("getCommands", nil, &out)
	return out, err
}

// SetModel changes the current session's model.
func (a *API) SetModel(model Model) (bool, error) {
	var out bool
	err := a.server.hostCall("setModel", map[string]any{"model": model}, &out)
	return out, err
}

// GetThinkingLevel returns the current thinking level.
func (a *API) GetThinkingLevel() (string, error) {
	var out string
	err := a.server.hostCall("getThinkingLevel", nil, &out)
	return out, err
}

// SetThinkingLevel changes the thinking level.
func (a *API) SetThinkingLevel(level string) error {
	return a.server.hostCallVoid("setThinkingLevel", map[string]any{"level": level})
}

// RegisterProvider registers or overrides a model provider.
func (a *API) RegisterProvider(name string, config ProviderConfig) error {
	return a.server.hostCallVoid("registerProvider", map[string]any{"name": name, "config": config})
}

// UnregisterProvider removes a provider registered by an extension.
func (a *API) UnregisterProvider(name string) error {
	return a.server.hostCallVoid("unregisterProvider", map[string]any{"name": name})
}

// RegisterMcpServer registers an MCP server for this session.
func (a *API) RegisterMcpServer(name string, config McpServerConfig) error {
	return a.server.hostCallVoid("registerMcpServer", map[string]any{"name": name, "config": config})
}

// UnregisterMcpServer removes a server registered by this extension.
func (a *API) UnregisterMcpServer(name string) error {
	return a.server.hostCallVoid("unregisterMcpServer", map[string]any{"name": name})
}

// GetMcpServers returns every extension-registered MCP server.
func (a *API) GetMcpServers() ([]RegisteredMcpServer, error) {
	var out []RegisteredMcpServer
	err := a.server.hostCall("getMcpServers", nil, &out)
	return out, err
}

// RegisterVirtualModel registers a virtual model declaration.
func (a *API) RegisterVirtualModel(model VirtualModel) error {
	return a.server.hostCallVoid("registerVirtualModel", map[string]any{"model": model})
}

// UnregisterVirtualModel removes a virtual model declaration.
func (a *API) UnregisterVirtualModel(provider, id string) error {
	return a.server.hostCallVoid("unregisterVirtualModel", map[string]any{"provider": provider, "id": id})
}

// Events returns the shared extension event bus.
func (a *API) Events() *EventBus { return &EventBus{server: a.server} }

// EventBus lets plugins communicate without a session.
type EventBus struct{ server *server }

// On subscribes to a bus event.
func (b *EventBus) On(name string, handler func(data any)) func() {
	return b.server.addBusHandler(name, handler)
}

// Emit publishes a bus event.
func (b *EventBus) Emit(name string, data any) error {
	return b.server.hostCallVoid("events.emit", map[string]any{"event": name, "data": data})
}

func (a *API) registration() plugin.Registration {
	a.server.mu.Lock()
	defer a.server.mu.Unlock()
	reg := plugin.Registration{
		APIVersion: plugin.APIVersion,
		Name:       a.server.name,
		Version:    a.server.version,
	}
	for _, entry := range a.server.tools {
		reg.Tools = append(reg.Tools, entry.spec)
	}
	sort.Slice(reg.Tools, func(i, j int) bool { return reg.Tools[i].Name < reg.Tools[j].Name })
	for _, entry := range a.server.commands {
		reg.Commands = append(reg.Commands, entry.spec)
	}
	sort.Slice(reg.Commands, func(i, j int) bool { return reg.Commands[i].Name < reg.Commands[j].Name })
	for _, flag := range a.server.flags {
		reg.Flags = append(reg.Flags, flag)
	}
	sort.Slice(reg.Flags, func(i, j int) bool { return reg.Flags[i].Name < reg.Flags[j].Name })
	reg.Shortcuts = append(reg.Shortcuts, a.server.shortcuts...)
	reg.Renderers = append(reg.Renderers, a.server.renderers...)
	for event := range a.server.handlers {
		reg.Events = append(reg.Events, event)
	}
	sort.Strings(reg.Events)
	return reg
}

func (s *server) addHandler(event string, handler Handler) func() {
	if handler == nil {
		return func() {}
	}
	s.mu.Lock()
	s.handlers[event] = append(s.handlers[event], handler)
	index := len(s.handlers[event]) - 1
	ready := s.ready
	s.mu.Unlock()
	if ready {
		_ = s.hostCallVoid("registerEvent", map[string]any{"event": event})
	}
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		hs := s.handlers[event]
		if index < len(hs) {
			hs[index] = nil
		}
	}
}

func (s *server) addBusHandler(name string, handler func(any)) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busHandlers[name] = append(s.busHandlers[name], handler)
	index := len(s.busHandlers[name]) - 1
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		hs := s.busHandlers[name]
		if index < len(hs) {
			hs[index] = nil
		}
	}
}

// Serve is the only call a plugin needs besides its ABI wrappers.
func Serve(factory Factory) {
	current.serve(factory)
}

// ServeNamed is Serve with a name and version reported in diagnostics.
func ServeNamed(name, version string, factory Factory) {
	current.serveNamed(name, version, factory)
}

func decodeJSON(raw []byte, out any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("plugin: decode host value: %w", err)
	}
	return nil
}
