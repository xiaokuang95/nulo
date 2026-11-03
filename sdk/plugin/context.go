package sdk

import (
	"encoding/json"
	"fmt"
)

// Context is passed to event handlers and tools. It is a live view: methods
// that read session state ask the host at call time.
type Context struct {
	api  *API
	data map[string]any
}

func contextFrom(raw json.RawMessage) *Context {
	c := &Context{api: &API{server: current}, data: map[string]any{}}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &c.data)
	}
	return c
}

// Raw returns the snapshot the host sent with this call.
func (c *Context) Raw() map[string]any {
	out := make(map[string]any, len(c.data))
	for k, v := range c.data {
		out[k] = v
	}
	return out
}

// CWD is the session working directory.
func (c *Context) CWD() string {
	if v, ok := c.data["cwd"].(string); ok {
		return v
	}
	return ""
}

// Mode is the host mode: tui, rpc, json or print.
func (c *Context) Mode() string {
	if v, ok := c.data["mode"].(string); ok {
		return v
	}
	return ""
}

// HasUI reports whether dialog-capable UI is attached.
func (c *Context) HasUI() bool {
	if v, ok := c.data["hasUI"].(bool); ok {
		return v
	}
	return false
}

// IsProjectTrusted reports whether project-local resources are trusted.
func (c *Context) IsProjectTrusted() bool {
	var out bool
	_ = c.api.server.hostCall("isProjectTrusted", nil, &out)
	return out
}

// IsIdle reports whether the agent is not streaming.
func (c *Context) IsIdle() bool {
	var out bool
	_ = c.api.server.hostCall("isIdle", nil, &out)
	return out
}

// HasPendingMessages reports whether steering or follow-up messages are queued.
func (c *Context) HasPendingMessages() bool {
	var out bool
	_ = c.api.server.hostCall("hasPendingMessages", nil, &out)
	return out
}

// Signal reports whether a run is currently active and can be aborted.
func (c *Context) Signal() bool {
	var out bool
	_ = c.api.server.hostCall("hasSignal", nil, &out)
	return out
}

// Abort cancels the current agent operation.
func (c *Context) Abort() error { return c.api.server.hostCallVoid("abort", nil) }

// Shutdown asks the host to exit gracefully.
func (c *Context) Shutdown() error { return c.api.server.hostCallVoid("shutdown", nil) }

// Model returns the current model, or nil when none is selected.
func (c *Context) Model() (Model, error) {
	var out Model
	err := c.api.server.hostCall("context.model", nil, &out)
	return out, err
}

// ThinkingLevel returns the current thinking level.
func (c *Context) ThinkingLevel() string {
	var out string
	_ = c.api.server.hostCall("getThinkingLevel", nil, &out)
	return out
}

// GetContextUsage returns the latest context-usage snapshot.
func (c *Context) GetContextUsage() (*ContextUsage, error) {
	var out *ContextUsage
	err := c.api.server.hostCall("getContextUsage", nil, &out)
	return out, err
}

// GetSystemPrompt returns the effective system prompt.
func (c *Context) GetSystemPrompt() (string, error) {
	var out string
	err := c.api.server.hostCall("getSystemPrompt", nil, &out)
	return out, err
}

// Compact requests compaction. It returns when the host has accepted the job.
func (c *Context) Compact(options *CompactOptions) error {
	return c.api.server.hostCallVoid("compact", map[string]any{"options": options})
}

// SendMessage sends a custom message to the session.
func (c *Context) SendMessage(message map[string]any, options map[string]any) error {
	return c.api.SendMessage(message, options)
}

// SendUserMessage sends a user message.
func (c *Context) SendUserMessage(content any, options map[string]any) error {
	return c.api.SendUserMessage(content, options)
}

// UI returns the host UI bridge. Calls fail cleanly when the host has no UI.
func (c *Context) UI() *UIContext { return &UIContext{api: c.api} }

// SessionManager returns the read-only session view.
func (c *Context) SessionManager() *SessionManager { return &SessionManager{api: c.api} }

// ModelRegistry returns the model catalogue view.
func (c *Context) ModelRegistry() *ModelRegistry { return &ModelRegistry{api: c.api} }

// CommandContext is the additional context available to slash commands.
type CommandContext struct {
	*Context
}

func commandContextFrom(raw json.RawMessage) *CommandContext {
	return &CommandContext{Context: contextFrom(raw)}
}

// WaitForIdle waits until the agent finishes streaming.
func (c *CommandContext) WaitForIdle() error {
	return c.api.server.hostCallVoid("command.waitForIdle", nil)
}

// NewSession starts a new session. Setup and withSession callbacks are not
// transported across the native boundary; use the persistent plugin state and
// session_start instead.
func (c *CommandContext) NewSession() (bool, error) {
	var out bool
	err := c.api.server.hostCall("command.newSession", nil, &out)
	return out, err
}

// Fork forks from an entry.
func (c *CommandContext) Fork(entryID string, position string) (bool, error) {
	var out bool
	err := c.api.server.hostCall("command.fork", map[string]any{"entryId": entryID, "position": position}, &out)
	return out, err
}

// NavigateTree changes the active leaf.
func (c *CommandContext) NavigateTree(targetID string, options map[string]any) (bool, error) {
	var out bool
	err := c.api.server.hostCall("command.navigateTree", map[string]any{"targetId": targetID, "options": options}, &out)
	return out, err
}

// SwitchSession opens another session file.
func (c *CommandContext) SwitchSession(path string) (bool, error) {
	var out bool
	err := c.api.server.hostCall("command.switchSession", map[string]any{"path": path}, &out)
	return out, err
}

// Reload re-reads resources and plugins.
func (c *CommandContext) Reload() error { return c.api.server.hostCallVoid("command.reload", nil) }

// GetSystemPromptOptions returns the prompt construction inputs.
func (c *CommandContext) GetSystemPromptOptions() (map[string]any, error) {
	var out map[string]any
	err := c.api.server.hostCall("command.getSystemPromptOptions", nil, &out)
	return out, err
}

// UIContext is Pi's ExtensionUIContext.
type UIContext struct{ api *API }

// Select shows a selector and reports whether a value was chosen.
func (u *UIContext) Select(title string, options []string, opts *DialogOptions) (string, bool, error) {
	var out *string
	err := u.api.server.hostCall("ui.select", map[string]any{"title": title, "options": options, "dialogOptions": opts}, &out)
	if err != nil || out == nil {
		return "", false, err
	}
	return *out, true, nil
}

// Confirm shows a confirmation dialog.
func (u *UIContext) Confirm(title, message string, opts *DialogOptions) (bool, error) {
	var out bool
	err := u.api.server.hostCall("ui.confirm", map[string]any{"title": title, "message": message, "options": opts}, &out)
	return out, err
}

// Input shows a text input dialog.
func (u *UIContext) Input(title, placeholder string, opts *DialogOptions) (string, bool, error) {
	var out *string
	err := u.api.server.hostCall("ui.input", map[string]any{"title": title, "placeholder": placeholder, "options": opts}, &out)
	if err != nil || out == nil {
		return "", false, err
	}
	return *out, true, nil
}

// Editor shows a multi-line editor.
func (u *UIContext) Editor(title, prefill string) (string, bool, error) {
	var out *string
	err := u.api.server.hostCall("ui.editor", map[string]any{"title": title, "prefill": prefill}, &out)
	if err != nil || out == nil {
		return "", false, err
	}
	return *out, true, nil
}

// Notify shows a notification.
func (u *UIContext) Notify(message, kind string) error {
	return u.api.server.hostCallVoid("ui.notify", map[string]any{"message": message, "type": kind})
}

// SetStatus sets or clears a footer status entry.
func (u *UIContext) SetStatus(key, text string) error {
	return u.api.server.hostCallVoid("ui.setStatus", map[string]any{"key": key, "text": text})
}

// SetWorkingMessage sets the streaming working message.
func (u *UIContext) SetWorkingMessage(message string) error {
	return u.api.server.hostCallVoid("ui.setWorkingMessage", map[string]any{"message": message})
}

// SetWorkingVisible shows or hides the working row.
func (u *UIContext) SetWorkingVisible(visible bool) error {
	return u.api.server.hostCallVoid("ui.setWorkingVisible", map[string]any{"visible": visible})
}

// SetWorkingIndicator configures the working animation.
func (u *UIContext) SetWorkingIndicator(options *WorkingIndicatorOptions) error {
	return u.api.server.hostCallVoid("ui.setWorkingIndicator", map[string]any{"options": options})
}

// SetHiddenThinkingLabel sets the hidden-thinking label.
func (u *UIContext) SetHiddenThinkingLabel(label string) error {
	return u.api.server.hostCallVoid("ui.setHiddenThinkingLabel", map[string]any{"label": label})
}

// SetWidget sets a text widget.
func (u *UIContext) SetWidget(key string, lines []string, options *WidgetOptions) error {
	return u.api.server.hostCallVoid("ui.setWidget", map[string]any{"key": key, "content": lines, "options": options})
}

// SetTitle sets the terminal title.
func (u *UIContext) SetTitle(title string) error {
	return u.api.server.hostCallVoid("ui.setTitle", map[string]any{"title": title})
}

// PasteToEditor pastes text into the editor.
func (u *UIContext) PasteToEditor(text string) error {
	return u.api.server.hostCallVoid("ui.pasteToEditor", map[string]any{"text": text})
}

// SetEditorText replaces the editor text.
func (u *UIContext) SetEditorText(text string) error {
	return u.api.server.hostCallVoid("ui.setEditorText", map[string]any{"text": text})
}

// GetEditorText reads the editor text.
func (u *UIContext) GetEditorText() (string, error) {
	var out string
	err := u.api.server.hostCall("ui.getEditorText", nil, &out)
	return out, err
}

// GetToolsExpanded reports the tool-output expansion state.
func (u *UIContext) GetToolsExpanded() bool {
	var out bool
	_ = u.api.server.hostCall("ui.getToolsExpanded", nil, &out)
	return out
}

// SetToolsExpanded changes the tool-output expansion state.
func (u *UIContext) SetToolsExpanded(expanded bool) error {
	return u.api.server.hostCallVoid("ui.setToolsExpanded", map[string]any{"expanded": expanded})
}

// SetTheme switches the host theme.
func (u *UIContext) SetTheme(theme string) (bool, string, error) {
	var out struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	err := u.api.server.hostCall("ui.setTheme", map[string]any{"theme": theme}, &out)
	return out.Success, out.Error, err
}

// SessionManager is a read-only view of the session tree.
type SessionManager struct{ api *API }

// File is the session file path. It is empty for an in-memory session.
func (m *SessionManager) File() string {
	var out string
	_ = m.api.server.hostCall("session.file", nil, &out)
	return out
}

// Entries returns the session's entries.
func (m *SessionManager) Entries() ([]map[string]any, error) {
	var out []map[string]any
	err := m.api.server.hostCall("session.entries", nil, &out)
	return out, err
}

// Leaf returns the active entry id.
func (m *SessionManager) Leaf() (string, error) {
	var out string
	err := m.api.server.hostCall("session.leaf", nil, &out)
	return out, err
}

// Tree returns the branch tree.
func (m *SessionManager) Tree() ([]map[string]any, error) {
	var out []map[string]any
	err := m.api.server.hostCall("session.tree", nil, &out)
	return out, err
}

// Name returns the session name.
func (m *SessionManager) Name() (string, error) {
	return m.api.GetSessionName()
}

// SetName sets the session name.
func (m *SessionManager) SetName(name string) error { return m.api.SetSessionName(name) }

// ModelRegistry is the model catalogue view.
type ModelRegistry struct{ api *API }

// Find resolves a provider/model spec.
func (r *ModelRegistry) Find(spec string) (Model, error) {
	var out Model
	err := r.api.server.hostCall("modelRegistry.find", map[string]any{"spec": spec}, &out)
	return out, err
}

// Available returns the models the host can resolve.
func (r *ModelRegistry) Available() ([]Model, error) {
	var out []Model
	err := r.api.server.hostCall("modelRegistry.available", nil, &out)
	return out, err
}

// GetAPIKey returns the resolved key for a provider, or empty when none is set.
func (r *ModelRegistry) GetAPIKey(provider string) (string, error) {
	var out string
	err := r.api.server.hostCall("modelRegistry.apiKey", map[string]any{"provider": provider}, &out)
	return out, err
}

// API returns the registration surface for this plugin.
func (c *Context) API() *API { return c.api }

// ScopedModels returns the models scoped to this session.
func (c *Context) ScopedModels() ([]Model, error) {
	var out []Model
	err := c.api.server.hostCall("context.scopedModels", nil, &out)
	return out, err
}

// GetActiveTools returns the tools declared to the model.
func (c *Context) GetActiveTools() ([]string, error) { return c.api.GetActiveTools() }

// GetAllTools returns every registered tool.
func (c *Context) GetAllTools() ([]ToolInfo, error) { return c.api.GetAllTools() }

// GetCommands returns the slash commands visible to the host.
func (c *Context) GetCommands() ([]SlashCommandInfo, error) { return c.api.GetCommands() }

// SetActiveTools changes the model-visible tool set.
func (c *Context) SetActiveTools(names []string) error { return c.api.SetActiveTools(names) }

// SetModel changes the session model.
func (c *Context) SetModel(model Model) (bool, error) { return c.api.SetModel(model) }

// SetThinkingLevel changes the thinking level.
func (c *Context) SetThinkingLevel(level string) error { return c.api.SetThinkingLevel(level) }

// AppendEntry persists a custom entry.
func (c *Context) AppendEntry(customType string, data any) error {
	return c.api.AppendEntry(customType, data)
}

// SetSessionName sets the session name.
func (c *Context) SetSessionName(name string) error { return c.api.SetSessionName(name) }

// GetSessionName gets the session name.
func (c *Context) GetSessionName() (string, error) { return c.api.GetSessionName() }

// SetLabel sets or clears an entry label.
func (c *Context) SetLabel(entryID, label string) error { return c.api.SetLabel(entryID, label) }

// Exec runs a shell command.
func (c *Context) Exec(command string, args []string, options *ExecOptions) (ExecResult, error) {
	return c.api.Exec(command, args, options)
}

// WaitForIdle waits until the active run finishes.
func (c *Context) WaitForIdle() error { return c.api.server.hostCallVoid("command.waitForIdle", nil) }

// GetSystemPromptOptions returns the prompt construction inputs.
func (c *Context) GetSystemPromptOptions() (map[string]any, error) {
	var out map[string]any
	err := c.api.server.hostCall("command.getSystemPromptOptions", nil, &out)
	return out, err
}

// GetSessionManager returns the read-only session manager.
func (c *Context) GetSessionManager() *SessionManager { return c.SessionManager() }

// GetModelRegistry returns the model registry.
func (c *Context) GetModelRegistry() *ModelRegistry { return c.ModelRegistry() }

// Additional SessionManager aliases used by ports from PiG.
func (m *SessionManager) GetEntries() ([]map[string]any, error) { return m.Entries() }
func (m *SessionManager) GetLeafID() (string, error)            { return m.Leaf() }
func (m *SessionManager) GetTree() ([]map[string]any, error)    { return m.Tree() }
func (m *SessionManager) GetName() (string, error)              { return m.Name() }
func (m *SessionManager) GetSessionFile() string                { return m.File() }
func (m *SessionManager) GetCwd() string {
	var out string
	_ = m.api.server.hostCall("session.cwd", nil, &out)
	return out
}
func (m *SessionManager) GetLabel(entryID string) (string, error) {
	var out string
	err := m.api.server.hostCall("session.label", map[string]any{"entryId": entryID}, &out)
	return out, err
}
func (m *SessionManager) SetLabel(entryID, label string) error { return m.api.SetLabel(entryID, label) }

// Additional ModelRegistry aliases.
func (r *ModelRegistry) GetAvailable() ([]Model, error)      { return r.Available() }
func (r *ModelRegistry) GetModel(spec string) (Model, error) { return r.Find(spec) }
func (r *ModelRegistry) GetAPIKeyForProvider(provider string) (string, error) {
	return r.GetAPIKey(provider)
}

// UI compatibility methods for hosts that implement the corresponding bridge.
func (u *UIContext) SetFooter(factory any)               {}
func (u *UIContext) SetHeader(factory any)               {}
func (u *UIContext) SetEditorComponent(factory any)      {}
func (u *UIContext) GetEditorComponent() any             { return nil }
func (u *UIContext) AddAutocompleteProvider(factory any) {}
func (u *UIContext) OnTerminalInput(handler any) func()  { return func() {} }
func (u *UIContext) Custom(factory any, options map[string]any) (any, error) {
	return nil, fmt.Errorf("custom UI components are not transportable across the native plugin boundary")
}
func (u *UIContext) GetAllThemes() ([]map[string]any, error) {
	var out []map[string]any
	err := u.api.server.hostCall("ui.getAllThemes", nil, &out)
	return out, err
}
func (u *UIContext) GetTheme(name string) (any, error) {
	var out any
	err := u.api.server.hostCall("ui.getTheme", map[string]any{"name": name}, &out)
	return out, err
}

// GetSettings returns the effective settings snapshot.
func (c *Context) GetSettings() (Settings, error) { return c.api.GetSettings() }

// RegisterProvider registers a provider from an event or command handler.
func (c *Context) RegisterProvider(name string, config ProviderConfig) error {
	return c.api.RegisterProvider(name, config)
}

// RegisterMcpServer registers an MCP server from an event or command handler.
func (c *Context) RegisterMcpServer(name string, config McpServerConfig) error {
	return c.api.RegisterMcpServer(name, config)
}

// GetMcpServers returns extension-registered MCP servers.
func (c *Context) GetMcpServers() ([]RegisteredMcpServer, error) { return c.api.GetMcpServers() }

// Events returns the shared plugin event bus.
func (c *Context) Events() *EventBus { return c.api.Events() }
