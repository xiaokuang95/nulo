package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xiaokuang95/nulo/plugin"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/mcp"
)

// PluginUI is the optional host-side UI bridge for native plugins. A nil
// callback means the operation is unavailable; plugins receive an empty or
// cancelled result instead of crashing the host.
type PluginUI struct {
	Select                 func(title string, options []string, timeoutMs int) (string, bool, error)
	Confirm                func(title, message string, timeoutMs int) (bool, error)
	Input                  func(title, placeholder string, timeoutMs int) (string, bool, error)
	Editor                 func(title, prefill string) (string, bool, error)
	Notify                 func(message, kind string)
	SetStatus              func(key, text string)
	SetWorkingMessage      func(message string)
	SetWorkingVisible      func(visible bool)
	SetWorkingIndicator    func(frames []string, intervalMs int)
	SetHiddenThinkingLabel func(label string)
	SetWidget              func(key string, lines []string, placement string)
	SetTitle               func(title string)
	PasteToEditor          func(text string)
	SetEditorText          func(text string)
	GetEditorText          func() string
	GetToolsExpanded       func() bool
	SetToolsExpanded       func(expanded bool)
	SetTheme               func(theme string) (bool, string)
}

// PluginDiagnostic is a non-fatal plugin load or runtime problem.
type PluginDiagnostic struct {
	Path    string
	Plugin  string
	Message string
}

func (d PluginDiagnostic) Error() string {
	if d.Plugin != "" {
		return d.Plugin + ": " + d.Message
	}
	if d.Path != "" {
		return d.Path + ": " + d.Message
	}
	return d.Message
}

// PluginCommand is one command a plugin registered.
type PluginCommand struct {
	Name         string
	Description  string
	ArgumentHint string
	Plugin       string
}

// PluginRenderer is a renderer declaration retained for a UI host.
type PluginRenderer struct {
	Plugin string
	Kind   string
	Name   string
}

// PluginFlag is a flag declaration retained for a CLI host.
type PluginFlag struct {
	Plugin      string
	Name        string
	Description string
	Type        string
	Default     any
}

// PluginShortcut is a shortcut declaration retained for a UI host.
type PluginShortcut struct {
	Plugin      string
	Key         string
	Description string
}

func (s *AgentSessionServices) initPluginMaps() {
	s.pluginMu.Lock()
	if s.pluginFlags == nil {
		s.pluginFlags = map[string]any{}
	}
	if s.pluginMCP == nil {
		s.pluginMCP = map[string]mcp.ServerConfig{}
	}
	if s.virtualModels == nil {
		s.virtualModels = map[string]map[string]any{}
	}
	if s.busSubscribers == nil {
		s.busSubscribers = map[string][]func(any){}
	}
	s.pluginMu.Unlock()
	s.providersMu.Lock()
	if s.dynamicProviders == nil {
		s.dynamicProviders = ai.Providers{}
	}
	s.providersMu.Unlock()
}

func (s *AgentSessionServices) ensurePlugins() {
	s.pluginOnce.Do(func() {
		s.pluginDiags = nil
		if s.NoPlugins {
			return
		}
		info := plugin.HostInfo{
			APIVersion: plugin.APIVersion,
			Host:       "nulo",
			CWD:        s.CWD,
			AgentDir:   s.AgentDir,
			Mode:       s.pluginMode,
			HasUI:      s.uiBridge != nil,
		}
		host := plugin.NewHost(plugin.HostOptions{
			Info: info,
			Handler: func(ctx context.Context, method string, params json.RawMessage) (any, error) {
				return s.pluginHostCall(ctx, method, params)
			},
			OnDiagnostic: func(d plugin.Diagnostic) {
				s.addPluginDiagnostic(PluginDiagnostic{Path: d.Path, Plugin: d.Plugin, Message: d.Message})
			},
		})
		s.plugins = host
		for _, path := range s.pluginSources() {
			libs, err := plugin.DiscoverPath(path)
			if err != nil {
				s.addPluginDiagnostic(PluginDiagnostic{Path: path, Message: err.Error()})
				continue
			}
			for _, lib := range libs {
				if _, err := host.Load(lib); err != nil {
					s.addPluginDiagnostic(PluginDiagnostic{Path: lib, Message: err.Error()})
				}
			}
		}
	})
}

func (s *AgentSessionServices) pluginSources() []string {
	var paths []string
	if s.SettingsManager != nil {
		paths = append(paths, s.SettingsManager.GetExtensionPaths()...)
	}
	if s.AgentDir != "" {
		for _, name := range []string{"plugins", "extensions"} {
			if path := filepath.Join(s.AgentDir, name); pathExists(path) {
				paths = append(paths, path)
			}
		}
	}
	if s.ProjectTrusted && s.CWD != "" {
		for _, name := range []string{"plugins", "extensions"} {
			if path := filepath.Join(s.CWD, ".nulo", name); pathExists(path) {
				paths = append(paths, path)
			}
		}
	}
	paths = append(paths, s.PluginPaths...)
	seen := map[string]bool{}
	var out []string
	for _, path := range paths {
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.CWD, path)
		}
		abs, err := filepath.Abs(path)
		if err == nil {
			path = abs
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *AgentSessionServices) addPluginDiagnostic(d PluginDiagnostic) {
	s.pluginMu.Lock()
	s.pluginDiags = append(s.pluginDiags, d)
	s.pluginMu.Unlock()
}

func (s *AgentSessionServices) pluginDiagnostics() []PluginDiagnostic {
	s.pluginMu.Lock()
	defer s.pluginMu.Unlock()
	return append([]PluginDiagnostic(nil), s.pluginDiags...)
}

func (s *AgentSessionServices) bindPluginSession(sess *Session) {
	s.pluginMu.Lock()
	s.pluginSession = sess
	s.pluginMu.Unlock()
}

func (s *AgentSessionServices) unbindPluginSession(sess *Session) {
	s.pluginMu.Lock()
	if s.pluginSession == sess {
		s.pluginSession = nil
	}
	s.pluginMu.Unlock()
}

func (s *AgentSessionServices) boundPluginSession() *Session {
	s.pluginMu.Lock()
	defer s.pluginMu.Unlock()
	return s.pluginSession
}

// PluginDiagnostics returns plugin loading and runtime diagnostics.
func (s *AgentSessionServices) PluginDiagnostics() []PluginDiagnostic {
	s.ensurePlugins()
	return s.pluginDiagnostics()
}

func (s *Session) pluginDiagnostics() []PluginDiagnostic {
	if s.services == nil {
		return nil
	}
	return s.services.pluginDiagnostics()
}

// PluginDiagnostics returns plugin loading and runtime diagnostics for this
// session, including errors from plugins that ran during the session.
func (s *Session) PluginDiagnostics() []PluginDiagnostic { return s.pluginDiagnostics() }

// PluginCommands returns the commands registered by loaded plugins.
func (s *Session) PluginCommands() []PluginCommand {
	if s.services == nil || s.services.plugins == nil {
		return nil
	}
	var out []PluginCommand
	for _, inst := range s.services.plugins.Instances() {
		for _, cmd := range inst.Snapshot().Commands {
			out = append(out, PluginCommand{Name: cmd.Name, Description: cmd.Description, ArgumentHint: cmd.ArgumentHint, Plugin: inst.Registration.Name})
		}
	}
	return out
}

// PluginRenderers returns renderer declarations from loaded plugins.
func (s *Session) PluginRenderers() []PluginRenderer {
	if s.services == nil || s.services.plugins == nil {
		return nil
	}
	var out []PluginRenderer
	for _, inst := range s.services.plugins.Instances() {
		for _, r := range inst.Snapshot().Renderers {
			out = append(out, PluginRenderer{Plugin: inst.Registration.Name, Kind: r.Kind, Name: r.Name})
		}
	}
	return out
}

// PluginFlags returns flag declarations from loaded plugins.
func (s *Session) PluginFlags() []PluginFlag {
	if s.services == nil || s.services.plugins == nil {
		return nil
	}
	var out []PluginFlag
	for _, inst := range s.services.plugins.Instances() {
		for _, f := range inst.Snapshot().Flags {
			out = append(out, PluginFlag{Plugin: inst.Registration.Name, Name: f.Name, Description: f.Description, Type: f.Type, Default: f.Default})
		}
	}
	return out
}

// PluginShortcuts returns shortcut declarations from loaded plugins.
func (s *Session) PluginShortcuts() []PluginShortcut {
	if s.services == nil || s.services.plugins == nil {
		return nil
	}
	var out []PluginShortcut
	for _, inst := range s.services.plugins.Instances() {
		for _, sc := range inst.Snapshot().Shortcuts {
			out = append(out, PluginShortcut{Plugin: inst.Registration.Name, Key: sc.Key, Description: sc.Description})
		}
	}
	return out
}

// RunPluginShortcut invokes a shortcut handler registered by a plugin.
func (s *Session) RunPluginShortcut(ctx context.Context, key string) error {
	if s.services == nil || s.services.plugins == nil {
		return fmt.Errorf("no plugins loaded")
	}
	for _, inst := range s.services.plugins.Instances() {
		for _, shortcut := range inst.Snapshot().Shortcuts {
			if shortcut.Key == key {
				return inst.ExecuteShortcut(ctx, key, s.pluginContextData(""))
			}
		}
	}
	return fmt.Errorf("shortcut not found: %s", key)
}

// SetPluginFlag sets a plugin flag's host value.
func (s *Session) SetPluginFlag(name string, value any) {
	s.services.pluginMu.Lock()
	s.services.pluginFlags[name] = value
	s.services.pluginMu.Unlock()
}

// ExecutePluginCommand runs a command registered by a plugin. Unknown command
// names return an error.
func (s *Session) ExecutePluginCommand(ctx context.Context, name, args string) error {
	if s.services == nil || s.services.plugins == nil {
		return fmt.Errorf("no plugins loaded")
	}
	for _, inst := range s.services.plugins.Instances() {
		for _, cmd := range inst.Snapshot().Commands {
			if cmd.Name != name {
				continue
			}
			return inst.ExecuteCommand(ctx, name, args, s.pluginContextData(""))
		}
	}
	return fmt.Errorf("command not found: %s", name)
}

// pluginContextData is the snapshot sent with every event, tool and command.
func (s *Session) pluginContextData(extra string) map[string]any {
	data := map[string]any{
		"cwd":            s.services.CWD,
		"mode":           s.services.pluginMode,
		"hasUI":          s.services.uiBridge != nil,
		"projectTrusted": s.services.ProjectTrusted,
	}
	if s.manager != nil {
		data["sessionFile"] = s.manager.File()
	}
	data["isIdle"] = s.isIdle()
	data["thinkingLevel"] = string(s.ThinkingLevel())
	data["model"] = modelJSON(s.Model())
	data["hasPendingMessages"] = s.hasPendingMessages()
	if extra != "" {
		data["extra"] = extra
	}
	return data
}

func (s *Session) isIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.streaming
}

func (s *Session) hasPendingMessages() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.steering) > 0 || len(s.followup) > 0
}

func modelJSON(model ai.Model) map[string]any {
	raw, err := json.Marshal(model)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

// emitPlugin sends one event to every subscribed plugin and returns the merged
// result object. Per-plugin failures become diagnostics and never abort the
// agent loop.
func (s *Session) emitPlugin(ctx context.Context, event string, data map[string]any) map[string]any {
	if s.services == nil || s.services.plugins == nil {
		return nil
	}
	combined := map[string]any{}
	for _, inst := range s.services.plugins.Instances() {
		raw, delivered, err := inst.Emit(ctx, event, data, s.pluginContextData(""))
		if !delivered {
			continue
		}
		if err != nil {
			s.services.addPluginDiagnostic(PluginDiagnostic{Path: inst.Path, Plugin: inst.Registration.Name, Message: event + ": " + err.Error()})
			continue
		}
		var envelope struct {
			Event  map[string]any `json:"event"`
			Result map[string]any `json:"result"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			s.services.addPluginDiagnostic(PluginDiagnostic{Path: inst.Path, Plugin: inst.Registration.Name, Message: event + ": " + err.Error()})
			continue
		}
		for k, v := range envelope.Event {
			data[k] = v
		}
		for k, v := range envelope.Result {
			combined[k] = v
		}
	}
	return combined
}

// pluginHostCall is the host side of the plugin API bridge.
func (s *AgentSessionServices) pluginHostCall(ctx context.Context, method string, params json.RawMessage) (any, error) {
	var arg map[string]any
	if len(params) > 0 {
		_ = json.Unmarshal(params, &arg)
	}
	sess := s.boundPluginSession()
	needSession := func() (*Session, error) {
		if sess == nil {
			return nil, fmt.Errorf("%s needs a bound session", method)
		}
		return sess, nil
	}
	switch method {
	case "isIdle":
		if sess == nil {
			return true, nil
		}
		return sess.isIdle(), nil
	case "isProjectTrusted":
		return s.ProjectTrusted, nil
	case "hasPendingMessages":
		if sess == nil {
			return false, nil
		}
		return sess.hasPendingMessages(), nil
	case "hasSignal":
		if sess == nil {
			return false, nil
		}
		sess.mu.Lock()
		defer sess.mu.Unlock()
		return sess.streaming, nil
	case "context.model":
		if sess == nil {
			return nil, nil
		}
		return modelJSON(sess.Model()), nil
	case "context.scopedModels":
		return s.availableModels(), nil
	case "ui.getAllThemes":
		return []any{}, nil
	case "ui.getTheme":
		return nil, nil
	case "setModel":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(arg["model"])
		if err != nil {
			return nil, err
		}
		var model ai.Model
		if err := json.Unmarshal(raw, &model); err != nil {
			return nil, err
		}
		if err := sess.SetModel(model); err != nil {
			return nil, err
		}
		return true, nil
	case "getThinkingLevel":
		if sess == nil {
			return "", nil
		}
		return string(sess.ThinkingLevel()), nil
	case "setThinkingLevel":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		return nil, sess.SetThinkingLevel(ai.ThinkingLevel(stringArg(arg, "level")))
	case "getActiveTools":
		if sess == nil {
			return []string{}, nil
		}
		return sess.ActiveToolNames(), nil
	case "getAllTools":
		if sess == nil {
			return []map[string]any{}, nil
		}
		return sess.toolInfos(), nil
	case "setActiveTools":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		sess.SetActiveTools(stringSliceArg(arg, "tools"))
		return nil, nil
	case "getCommands":
		if sess == nil {
			return []any{}, nil
		}
		return sess.commandInfos(), nil
	case "getSettings":
		return settingsSnapshot(s.SettingsManager), nil
	case "getFlag":
		s.pluginMu.Lock()
		value, ok := s.pluginFlags[stringArg(arg, "name")]
		s.pluginMu.Unlock()
		if !ok {
			return nil, nil
		}
		return value, nil
	case "setFlag":
		s.pluginMu.Lock()
		s.pluginFlags[stringArg(arg, "name")] = arg["value"]
		s.pluginMu.Unlock()
		return nil, nil
	case "getSessionName":
		if sess == nil {
			return "", nil
		}
		return sess.Name(), nil
	case "setSessionName":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		return nil, sess.SetName(stringArg(arg, "name"))
	case "setLabel":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		label := stringArg(arg, "label")
		var ptr *string
		if label != "" {
			ptr = &label
		}
		return nil, sess.SetLabel(stringArg(arg, "entryId"), ptr)
	case "appendEntry":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(arg["data"])
		if err != nil {
			return nil, err
		}
		_, err = sess.manager.AppendCustom(stringArg(arg, "customType"), data)
		return nil, err
	case "sendMessage":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		return nil, sess.pluginSendMessage(arg)
	case "sendUserMessage":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		return nil, sess.pluginSendUserMessage(ctx, arg)
	case "abort":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		return nil, sess.Abort(ctx)
	case "shutdown":
		if sess != nil {
			sess.Dispose()
		}
		return nil, nil
	case "getContextUsage":
		if sess == nil {
			return nil, nil
		}
		usage := sess.contextUsage()
		return usage, nil
	case "getSystemPrompt":
		if sess == nil {
			return "", nil
		}
		return sess.currentSystemPrompt(), nil
	case "compact":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		instructions := ""
		if opts, ok := arg["options"].(map[string]any); ok {
			instructions = stringArg(opts, "customInstructions")
		}
		_, err = sess.Compact(ctx, instructions)
		return nil, err
	case "exec":
		return s.pluginExec(ctx, arg)
	case "registerProvider":
		return nil, s.registerPluginProvider(arg)
	case "unregisterProvider":
		s.unregisterPluginProvider(stringArg(arg, "name"))
		return nil, nil
	case "registerMcpServer":
		return nil, s.registerPluginMCP(stringArg(arg, "name"), arg["config"])
	case "unregisterMcpServer":
		if sess != nil {
			return nil, sess.unregisterPluginMCP(stringArg(arg, "name"))
		}
		s.unregisterPluginMCPConfig(stringArg(arg, "name"))
		return nil, nil
	case "getMcpServers":
		return s.pluginMCPList(), nil
	case "registerVirtualModel":
		return nil, s.registerVirtualModel(arg["model"])
	case "unregisterVirtualModel":
		return nil, s.unregisterVirtualModel(stringArg(arg, "provider"), stringArg(arg, "id"))
	case "events.emit":
		s.emitBusEvent(stringArg(arg, "event"), arg["data"])
		return nil, nil
	case "registerTool":
		inst := s.instanceByName(plugin.NameFromContext(ctx))
		if inst == nil {
			return nil, fmt.Errorf("registerTool: unknown plugin %q", plugin.NameFromContext(ctx))
		}
		raw, _ := json.Marshal(arg)
		var spec plugin.ToolSpec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, err
		}
		if err := inst.AddTool(spec); err != nil {
			return nil, err
		}
		if sess != nil {
			if err := sess.addPluginTool(inst, spec); err != nil {
				return nil, err
			}
		}
		return nil, nil
	case "registerEvent":
		inst := s.instanceByName(plugin.NameFromContext(ctx))
		if inst == nil {
			return nil, fmt.Errorf("registerEvent: unknown plugin %q", plugin.NameFromContext(ctx))
		}
		return nil, inst.AddEvent(stringArg(arg, "event"))
	case "registerCommand":
		inst := s.instanceByName(plugin.NameFromContext(ctx))
		if inst == nil {
			return nil, fmt.Errorf("registerCommand: unknown plugin %q", plugin.NameFromContext(ctx))
		}
		raw, _ := json.Marshal(arg)
		var spec plugin.CommandSpec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, err
		}
		return nil, inst.AddCommand(spec)
	case "registerFlag":
		inst := s.instanceByName(plugin.NameFromContext(ctx))
		if inst == nil {
			return nil, fmt.Errorf("registerFlag: unknown plugin %q", plugin.NameFromContext(ctx))
		}
		raw, _ := json.Marshal(arg)
		var spec plugin.FlagSpec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, err
		}
		return nil, inst.AddFlag(spec)
	case "registerShortcut":
		inst := s.instanceByName(plugin.NameFromContext(ctx))
		if inst == nil {
			return nil, fmt.Errorf("registerShortcut: unknown plugin %q", plugin.NameFromContext(ctx))
		}
		raw, _ := json.Marshal(arg)
		var spec plugin.ShortcutSpec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, err
		}
		return nil, inst.AddShortcut(spec)
	case "registerRenderer":
		inst := s.instanceByName(plugin.NameFromContext(ctx))
		if inst == nil {
			return nil, fmt.Errorf("registerRenderer: unknown plugin %q", plugin.NameFromContext(ctx))
		}
		raw, _ := json.Marshal(arg)
		var spec plugin.RendererSpec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, err
		}
		return nil, inst.AddRenderer(spec)
	case "session.file":
		if sess == nil {
			return "", nil
		}
		return sess.manager.File(), nil
	case "session.cwd":
		return s.CWD, nil
	case "session.label":
		if sess == nil {
			return "", nil
		}
		value, _ := sess.Label(stringArg(arg, "entryId"))
		return value, nil
	case "session.entries":
		if sess == nil {
			return []any{}, nil
		}
		return sessionEntriesJSON(sess), nil
	case "session.leaf":
		if sess == nil {
			return "", nil
		}
		return sess.manager.LeafID(), nil
	case "session.tree":
		if sess == nil {
			return []any{}, nil
		}
		raw, _ := json.Marshal(sess.Tree())
		var out []any
		_ = json.Unmarshal(raw, &out)
		return out, nil
	case "modelRegistry.find":
		return BuildModel(stringArg(arg, "spec"), &AgentSessionServices{AgentDir: s.AgentDir, SettingsManager: s.SettingsManager, dynamicProviders: s.providerSnapshot()})
	case "modelRegistry.available":
		return s.availableModels(), nil
	case "modelRegistry.apiKey":
		model, err := s.findModelByProvider(stringArg(arg, "provider"))
		if err != nil {
			return "", err
		}
		return ai.LookupAPIKey(model, s.AgentDir, ai.StreamOptions{})
	case "command.waitForIdle":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		return nil, sess.WaitIdle(ctx)
	case "command.newSession":
		if s.pluginRuntime == nil {
			return false, nil
		}
		_, err := s.pluginRuntime.NewSession()
		return err == nil, err
	case "command.fork":
		if s.pluginRuntime == nil {
			return false, nil
		}
		_, err := s.pluginRuntime.Fork()
		return err == nil, err
	case "command.navigateTree":
		sess, err := needSession()
		if err != nil {
			return nil, err
		}
		opts := map[string]any{}
		if m, ok := arg["options"].(map[string]any); ok {
			opts = m
		}
		err = sess.NavigateTree(ctx, stringArg(arg, "targetId"), NavigateTreeOptions{
			Summarize:          boolArg(opts, "summarize"),
			CustomInstructions: stringArg(opts, "customInstructions"),
		})
		return err == nil, err
	case "command.switchSession":
		if s.pluginRuntime == nil {
			return false, nil
		}
		_, err := s.pluginRuntime.SwitchSession(stringArg(arg, "path"))
		return err == nil, err
	case "command.reload":
		return nil, nil
	case "command.getSystemPromptOptions":
		if sess == nil {
			return map[string]any{}, nil
		}
		return map[string]any{"cwd": s.CWD, "customPrompt": sess.system, "agentDir": s.AgentDir}, nil
	}
	if strings.HasPrefix(method, "ui.") {
		return s.pluginUIHandle(method, arg)
	}
	return nil, fmt.Errorf("plugin API method %q is not supported", method)
}

func stringArg(arg map[string]any, key string) string {
	if v, ok := arg[key].(string); ok {
		return v
	}
	return ""
}

func boolArg(arg map[string]any, key string) bool {
	if v, ok := arg[key].(bool); ok {
		return v
	}
	return false
}

func stringSliceArg(arg map[string]any, key string) []string {
	raw, ok := arg[key].([]any)
	if !ok {
		if values, ok := arg[key].([]string); ok {
			return values
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (s *AgentSessionServices) pluginUIHandle(method string, arg map[string]any) (any, error) {
	ui := s.uiBridge
	if ui == nil {
		return nil, fmt.Errorf("host has no plugin UI")
	}
	timeout := 0
	if opts, ok := arg["dialogOptions"].(map[string]any); ok {
		if n, ok := plugin.NumberAsInt(opts["timeout"]); ok {
			timeout = n
		}
	}
	switch method {
	case "ui.select":
		if ui.Select == nil {
			return nil, nil
		}
		value, ok, err := ui.Select(stringArg(arg, "title"), stringSliceArg(arg, "options"), timeout)
		if err != nil || !ok {
			return nil, err
		}
		return value, nil
	case "ui.confirm":
		if ui.Confirm == nil {
			return false, nil
		}
		return ui.Confirm(stringArg(arg, "title"), stringArg(arg, "message"), timeout)
	case "ui.input":
		if ui.Input == nil {
			return nil, nil
		}
		value, ok, err := ui.Input(stringArg(arg, "title"), stringArg(arg, "placeholder"), timeout)
		if err != nil || !ok {
			return nil, err
		}
		return value, nil
	case "ui.editor":
		if ui.Editor == nil {
			return nil, nil
		}
		value, ok, err := ui.Editor(stringArg(arg, "title"), stringArg(arg, "prefill"))
		if err != nil || !ok {
			return nil, err
		}
		return value, nil
	case "ui.notify":
		if ui.Notify != nil {
			ui.Notify(stringArg(arg, "message"), stringArg(arg, "type"))
		}
		return nil, nil
	case "ui.setStatus":
		if ui.SetStatus != nil {
			ui.SetStatus(stringArg(arg, "key"), stringArg(arg, "text"))
		}
		return nil, nil
	case "ui.setWorkingMessage":
		if ui.SetWorkingMessage != nil {
			ui.SetWorkingMessage(stringArg(arg, "message"))
		}
		return nil, nil
	case "ui.setWorkingVisible":
		if ui.SetWorkingVisible != nil {
			ui.SetWorkingVisible(boolArg(arg, "visible"))
		}
		return nil, nil
	case "ui.setWorkingIndicator":
		if ui.SetWorkingIndicator != nil {
			frames := stringSliceArg(arg, "frames")
			interval := 0
			if opts, ok := arg["options"].(map[string]any); ok {
				frames = stringSliceArg(opts, "frames")
				interval, _ = plugin.NumberAsInt(opts["intervalMs"])
			}
			ui.SetWorkingIndicator(frames, interval)
		}
		return nil, nil
	case "ui.setHiddenThinkingLabel":
		if ui.SetHiddenThinkingLabel != nil {
			ui.SetHiddenThinkingLabel(stringArg(arg, "label"))
		}
		return nil, nil
	case "ui.setWidget":
		if ui.SetWidget != nil {
			placement := ""
			if opts, ok := arg["options"].(map[string]any); ok {
				placement = stringArg(opts, "placement")
			}
			ui.SetWidget(stringArg(arg, "key"), stringSliceArg(arg, "content"), placement)
		}
		return nil, nil
	case "ui.setTitle":
		if ui.SetTitle != nil {
			ui.SetTitle(stringArg(arg, "title"))
		}
		return nil, nil
	case "ui.pasteToEditor":
		if ui.PasteToEditor != nil {
			ui.PasteToEditor(stringArg(arg, "text"))
		}
		return nil, nil
	case "ui.setEditorText":
		if ui.SetEditorText != nil {
			ui.SetEditorText(stringArg(arg, "text"))
		}
		return nil, nil
	case "ui.getEditorText":
		if ui.GetEditorText != nil {
			return ui.GetEditorText(), nil
		}
		return "", nil
	case "ui.getToolsExpanded":
		if ui.GetToolsExpanded != nil {
			return ui.GetToolsExpanded(), nil
		}
		return false, nil
	case "ui.setToolsExpanded":
		if ui.SetToolsExpanded != nil {
			ui.SetToolsExpanded(boolArg(arg, "expanded"))
		}
		return nil, nil
	case "ui.setTheme":
		if ui.SetTheme == nil {
			return map[string]any{"success": false, "error": "host has no theme support"}, nil
		}
		ok, msg := ui.SetTheme(stringArg(arg, "theme"))
		return map[string]any{"success": ok, "error": msg}, nil
	}
	return nil, fmt.Errorf("plugin API method %q is not supported", method)
}

func (s *AgentSessionServices) instanceByName(name string) *plugin.Instance {
	if s.plugins == nil {
		return nil
	}
	for _, inst := range s.plugins.Instances() {
		if inst.Snapshot().Name == name {
			return inst
		}
	}
	return nil
}

func (s *Session) addPluginTool(inst *plugin.Instance, spec plugin.ToolSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.pluginToolByName[spec.Name]; exists {
		return fmt.Errorf("tool %q is already registered", spec.Name)
	}
	for _, tool := range s.tools {
		if tool.Name == spec.Name {
			return fmt.Errorf("tool %q conflicts with an active tool", spec.Name)
		}
	}
	if spec.ExecutionMode == "" {
		spec.ExecutionMode = "parallel"
	}
	binding := pluginToolBinding{instance: inst, spec: spec}
	s.pluginToolByName[spec.Name] = binding
	tool := s.pluginAgentTool(binding)
	if spec.ToolExposure() != "hidden" {
		s.catalog = append(s.catalog, tool)
		if spec.ToolActive() {
			s.tools = append(s.tools, tool)
		}
	}
	return nil
}

func settingsSnapshot(manager *SettingsManager) map[string]any {
	if manager == nil {
		return map[string]any{}
	}
	raw := manager.Raw()
	out := make(map[string]any, len(raw))
	for key, value := range raw {
		var decoded any
		if json.Unmarshal(value, &decoded) == nil {
			out[key] = decoded
		}
	}
	return out
}

func sessionEntriesJSON(s *Session) []any {
	entries := s.manager.Branch()
	raw, _ := json.Marshal(entries)
	var out []any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = []any{}
	}
	return out
}
