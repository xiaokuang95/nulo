// Legacy plugin adapter. It adapts nulo's older native-library and stdio
// plugin host to the SDK Plugin/Registry API.
//
// The adapter is intentionally load-time oriented. It exposes the plugin's
// static tools, commands, providers, MCP declarations and UI declarations. The
// old host remains the right path for plugins that depend on session-bound
// events or dynamic tool/command registration; those operations are reported
// through OnDiagnostic instead of being silently dropped.
package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/mcp"
	"github.com/xiaokuang95/nulo/sdk/internal/plugin"
)

// HostCallFunc handles one host call that the adapter does not implement
// itself. A nil handler makes unsupported calls fail with a descriptive error.
type LegacyHostCallFunc func(context.Context, string, json.RawMessage) (any, error)

// LegacyPluginOptions configures a legacy plugin adapter.
type LegacyPluginOptions struct {
	// Path is a shared library, a plugin.json manifest, or a directory
	// containing plugins. Required.
	Path string
	// CWD and AgentDir are sent to the plugin in HostInfo and included in
	// tool/command context. CWD defaults to ".".
	CWD      string
	AgentDir string
	Mode     string
	HasUI    bool

	// ReplaceExisting makes adapter tools and providers replace same-named
	// built-in or earlier plugin capabilities. The default is to skip a
	// conflicting tool and report a diagnostic.
	ReplaceExisting bool

	// HostCall handles host methods not implemented by the adapter. The
	// adapter implements registration calls, basic tool/command queries and
	// session.cwd, so most tools work without a callback.
	HostCall LegacyHostCallFunc

	// ContextData adds session-specific fields to the context sent with every
	// tool and command call. The adapter calls it for each invocation.
	ContextData func(context.Context) map[string]any

	// OnDiagnostic receives non-fatal load and capability messages.
	OnDiagnostic func(string)
}

// LegacyPlugin is a loaded legacy plugin exposed as a Plugin. Close releases
// the legacy plugin host.
type LegacyPlugin struct {
	opts LegacyPluginOptions
	name string
	host *plugin.Host

	mu       sync.Mutex
	insts    []*plugin.Instance
	closed   bool
	registry *Registry

	providers map[string]ai.ProviderConfig
	mcps      map[string]mcp.ServerConfig
	commands  []commandBinding
	ui        UIDeclaration
}

var _ Plugin = (*LegacyPlugin)(nil)

type commandBinding struct {
	inst *plugin.Instance
	spec plugin.CommandSpec
}

// LoadLegacyPlugin discovers and loads the legacy plugin(s) at opts.Path.
// Loading errors for one source do not prevent other sources from loading; the
// returned adapter reports them through OnDiagnostic.
func LoadLegacyPlugin(opts LegacyPluginOptions) (*LegacyPlugin, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, errors.New("legacy plugin: Path is required")
	}
	if opts.CWD == "" {
		opts.CWD = "."
	}
	if cwd, err := filepath.Abs(opts.CWD); err == nil {
		opts.CWD = cwd
	}
	if opts.AgentDir != "" {
		if abs, err := filepath.Abs(opts.AgentDir); err == nil {
			opts.AgentDir = abs
		}
	}
	sources, err := plugin.Discover(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("legacy plugin: %w", err)
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("legacy plugin: no plugins found at %s", opts.Path)
	}

	a := &LegacyPlugin{
		opts:      opts,
		name:      "legacy:" + filepath.Base(opts.Path),
		providers: map[string]ai.ProviderConfig{},
		mcps:      map[string]mcp.ServerConfig{},
	}
	a.host = plugin.NewHost(plugin.HostOptions{
		Info: plugin.HostInfo{
			APIVersion: plugin.APIVersion,
			Host:       "nulo",
			CWD:        opts.CWD,
			AgentDir:   opts.AgentDir,
			Mode:       opts.Mode,
			HasUI:      opts.HasUI,
		},
		Handler: a.hostCall,
		OnDiagnostic: func(d plugin.Diagnostic) {
			a.diagnostic(d.Error())
		},
	})

	var loadErrs []error
	for _, source := range sources {
		var inst *plugin.Instance
		var loadErr error
		switch source.Kind {
		case plugin.SourceProtocol:
			inst, loadErr = a.host.LoadProtocol(source.Path)
		default:
			inst, loadErr = a.host.Load(source.Path)
		}
		if loadErr != nil {
			loadErrs = append(loadErrs, fmt.Errorf("%s: %w", source.Path, loadErr))
			a.diagnostic(fmt.Sprintf("%s: %v", source.Path, loadErr))
			continue
		}
		a.insts = append(a.insts, inst)
	}
	if len(a.insts) == 0 {
		_ = a.host.Close()
		return nil, errors.Join(loadErrs...)
	}
	return a, nil
}

func (a *LegacyPlugin) Name() string { return a.name }

// Register adds the legacy plugin's static capabilities to r.
func (a *LegacyPlugin) Register(r *Registry) error {
	if r == nil {
		return errors.New("legacy plugin: nil registry")
	}
	a.mu.Lock()
	if a.registry != nil {
		a.mu.Unlock()
		return errors.New("legacy plugin: already registered")
	}
	a.registry = r
	insts := append([]*plugin.Instance(nil), a.insts...)
	commands := append([]commandBinding(nil), a.commands...)
	ui := a.ui
	providers := make(map[string]ai.ProviderConfig, len(a.providers))
	for name, cfg := range a.providers {
		providers[name] = cfg
	}
	mcps := make(map[string]mcp.ServerConfig, len(a.mcps))
	for name, cfg := range a.mcps {
		mcps[name] = cfg
	}
	a.mu.Unlock()

	for _, inst := range insts {
		reg := inst.Snapshot()
		for _, spec := range reg.Tools {
			a.registerTool(r, inst, spec)
		}
		for _, spec := range reg.Commands {
			a.registerCommand(r, inst, spec)
		}
		for _, renderer := range reg.Renderers {
			ui.Renderers = append(ui.Renderers, Renderer{Plugin: reg.Name, Kind: renderer.Kind, Name: renderer.Name})
		}
		for _, shortcut := range reg.Shortcuts {
			ui.Shortcuts = append(ui.Shortcuts, Shortcut{Plugin: reg.Name, Key: shortcut.Key, Description: shortcut.Description})
		}
	}
	for _, binding := range commands {
		a.registerCommand(r, binding.inst, binding.spec)
	}
	for name, cfg := range providers {
		if err := a.addProvider(r, name, cfg); err != nil {
			return err
		}
	}
	for name, cfg := range mcps {
		if err := a.addMCPServer(r, name, cfg); err != nil {
			a.diagnostic(fmt.Sprintf("mcp server %s: %v", name, err))
		}
	}
	r.AddUI(ui)
	return nil
}

func (a *LegacyPlugin) registerTool(r *Registry, inst *plugin.Instance, spec plugin.ToolSpec) {
	exposure := spec.ToolExposure()
	if exposure == "hidden" {
		return
	}
	if (exposure == "direct" || exposure == "model-only") && !spec.ToolActive() {
		a.diagnostic(fmt.Sprintf("%s: tool %s is inactive by default; the registry adapter only adds active direct tools", inst.Registration.Name, spec.Name))
		return
	}
	tool := a.toolFor(inst, spec)
	var err error
	if a.opts.ReplaceExisting {
		err = r.ReplaceTool(tool)
	} else {
		err = r.AddTool(tool)
	}
	if err != nil {
		a.diagnostic(fmt.Sprintf("%s: tool %s: %v", inst.Registration.Name, spec.Name, err))
	}
}

func (a *LegacyPlugin) toolFor(inst *plugin.Instance, spec plugin.ToolSpec) Tool {
	parameters := append(json.RawMessage(nil), spec.Parameters...)
	return ToolFunc{
		Def: ToolDefinition{
			Name:             spec.Name,
			Label:            spec.Label,
			Description:      spec.Description,
			Parameters:       parameters,
			ExecutionMode:    spec.ExecutionMode,
			PromptSnippet:    spec.PromptSnippet,
			PromptGuidelines: append([]string(nil), spec.PromptGuidelines...),
			Exposure:         exposureForDefinition(spec.ToolExposure()),
		},
		Run: func(ctx context.Context, call ToolCall) (ToolResult, error) {
			result, err := inst.ExecuteTool(ctx, call.ID, spec.Name, call.Arguments, a.contextData(ctx))
			if err != nil {
				return ToolResult{}, err
			}
			return convertResult(result), nil
		},
	}
}

func (a *LegacyPlugin) registerCommand(r *Registry, inst *plugin.Instance, spec plugin.CommandSpec) {
	command := Command{
		Name:         spec.Name,
		Description:  spec.Description,
		ArgumentHint: spec.ArgumentHint,
		Run: func(ctx context.Context, sess *Session, args string) error {
			data := a.contextData(ctx)
			if sess != nil {
				data["sessionFile"] = sess.Manager().File()
			}
			return inst.ExecuteCommand(ctx, spec.Name, args, data)
		},
	}
	if err := r.AddCommand(command); err != nil {
		a.diagnostic(fmt.Sprintf("%s: command %s: %v", inst.Registration.Name, spec.Name, err))
	}
}

func (a *LegacyPlugin) addProvider(r *Registry, name string, cfg ai.ProviderConfig) error {
	provider, err := providerFor(name, cfg)
	if err != nil {
		return err
	}
	if a.opts.ReplaceExisting {
		return r.ReplaceProvider(provider)
	}
	if err := r.AddProvider(provider); err != nil {
		a.diagnostic(fmt.Sprintf("provider %s: %v", name, err))
	}
	return nil
}

func providerFor(name string, cfg ai.ProviderConfig) (Provider, error) {
	providers := ai.Providers{name: cfg}
	models := make([]Model, 0, len(cfg.Models))
	for _, entry := range cfg.Models {
		model, err := providers.FindModel(name + "/" + entry.ID)
		if err != nil {
			continue
		}
		models = append(models, model)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("provider %s has no usable models", name)
	}
	return ProviderFunc{
		ProviderID: name,
		ModelList:  models,
		Fn: func(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
			return ai.StreamSimple(ctx, model, tr, opts)
		},
	}, nil
}

func (a *LegacyPlugin) addMCPServer(r *Registry, name string, cfg mcp.ServerConfig) error {
	transport := cfg.Type
	if transport == "" {
		if cfg.URL != "" {
			transport = "http"
		} else {
			transport = "stdio"
		}
	}
	exposure := cfg.Exposure
	if exposure == "" {
		exposure = string(mcp.ExposureCodemode)
	}
	server := MCPServer{
		Name:         name,
		Transport:    transport,
		Command:      cfg.Command,
		Args:         append([]string(nil), cfg.Args...),
		Env:          cloneMap(cfg.Env),
		CWD:          cfg.CWD,
		URL:          cfg.URL,
		Headers:      cloneMap(cfg.Headers),
		Exposure:     exposure,
		ToolExposure: cloneMap(cfg.ToolExposure),
		Description:  cfg.Description,
	}
	return r.AddMCPServer(server)
}

func (a *LegacyPlugin) contextData(ctx context.Context) map[string]any {
	data := map[string]any{
		"cwd":   a.opts.CWD,
		"mode":  a.opts.Mode,
		"hasUI": a.opts.HasUI,
	}
	if a.opts.AgentDir != "" {
		data["agentDir"] = a.opts.AgentDir
	}
	if a.opts.ContextData != nil {
		for key, value := range a.opts.ContextData(ctx) {
			data[key] = value
		}
	}
	return data
}

func (a *LegacyPlugin) hostCall(ctx context.Context, method string, params json.RawMessage) (any, error) {
	var arg map[string]any
	if len(params) > 0 {
		_ = json.Unmarshal(params, &arg)
	}
	switch method {
	case "isIdle":
		return true, nil
	case "isProjectTrusted":
		return false, nil
	case "getActiveTools":
		return a.toolNames(), nil
	case "getAllTools":
		return a.toolInfos(), nil
	case "getCommands":
		return a.commandInfos(), nil
	case "getSettings":
		return map[string]any{}, nil
	case "session.cwd":
		return a.opts.CWD, nil
	case "registerProvider":
		name, _ := arg["name"].(string)
		raw, err := json.Marshal(arg["config"])
		if err != nil {
			return nil, err
		}
		var cfg ai.ProviderConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
		a.mu.Lock()
		a.providers[name] = cfg
		reg := a.registry
		a.mu.Unlock()
		if reg != nil {
			return nil, a.addProvider(reg, name, cfg)
		}
		return nil, nil
	case "unregisterProvider":
		name, _ := arg["name"].(string)
		a.mu.Lock()
		delete(a.providers, name)
		a.mu.Unlock()
		return nil, nil
	case "registerMcpServer":
		name, _ := arg["name"].(string)
		raw, err := json.Marshal(arg["config"])
		if err != nil {
			return nil, err
		}
		var cfg mcp.ServerConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
		a.mu.Lock()
		a.mcps[name] = cfg
		reg := a.registry
		a.mu.Unlock()
		if reg != nil {
			return nil, a.addMCPServer(reg, name, cfg)
		}
		return nil, nil
	case "unregisterMcpServer":
		name, _ := arg["name"].(string)
		a.mu.Lock()
		delete(a.mcps, name)
		a.mu.Unlock()
		return nil, nil
	case "getMcpServers":
		a.mu.Lock()
		out := make([]map[string]any, 0, len(a.mcps))
		for name, cfg := range a.mcps {
			out = append(out, map[string]any{"name": name, "config": cfg})
		}
		a.mu.Unlock()
		sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]["name"]) < fmt.Sprint(out[j]["name"]) })
		return out, nil
	case "registerCommand":
		raw, _ := json.Marshal(arg)
		var spec plugin.CommandSpec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, err
		}
		inst := a.instanceByName(plugin.NameFromContext(ctx))
		if inst == nil && len(a.insts) == 1 {
			inst = a.insts[0]
		}
		if inst == nil {
			return nil, fmt.Errorf("registerCommand: unknown plugin")
		}
		a.mu.Lock()
		a.commands = append(a.commands, commandBinding{inst: inst, spec: spec})
		reg := a.registry
		a.mu.Unlock()
		if reg != nil {
			a.registerCommand(reg, inst, spec)
		}
		return nil, nil
	case "registerShortcut":
		raw, _ := json.Marshal(arg)
		var spec plugin.ShortcutSpec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, err
		}
		name := plugin.NameFromContext(ctx)
		a.mu.Lock()
		a.ui.Shortcuts = append(a.ui.Shortcuts, Shortcut{Plugin: name, Key: spec.Key, Description: spec.Description})
		reg := a.registry
		a.mu.Unlock()
		if reg != nil {
			reg.AddUI(UIDeclaration{Shortcuts: []Shortcut{{Plugin: name, Key: spec.Key, Description: spec.Description}}})
		}
		return nil, nil
	case "registerRenderer":
		raw, _ := json.Marshal(arg)
		var spec plugin.RendererSpec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, err
		}
		name := plugin.NameFromContext(ctx)
		a.mu.Lock()
		a.ui.Renderers = append(a.ui.Renderers, Renderer{Plugin: name, Kind: spec.Kind, Name: spec.Name})
		reg := a.registry
		a.mu.Unlock()
		if reg != nil {
			reg.AddUI(UIDeclaration{Renderers: []Renderer{{Plugin: name, Kind: spec.Kind, Name: spec.Name}}})
		}
		return nil, nil
	case "registerFlag":
		return nil, nil
	case "registerTool":
		return nil, errors.New("legacy plugin: dynamic registerTool is not supported by the registry adapter")
	case "events.emit":
		return nil, nil
	default:
		if a.opts.HostCall != nil {
			return a.opts.HostCall(ctx, method, params)
		}
		return nil, fmt.Errorf("legacy plugin: host method %q is not supported; set LegacyPluginOptions.HostCall", method)
	}
}

func (a *LegacyPlugin) instanceByName(name string) *plugin.Instance {
	if name == "" {
		return nil
	}
	for _, inst := range a.insts {
		if inst.Registration.Name == name {
			return inst
		}
	}
	return nil
}

func (a *LegacyPlugin) toolNames() []string {
	var out []string
	for _, inst := range a.insts {
		for _, spec := range inst.Snapshot().Tools {
			if spec.ToolActive() {
				out = append(out, spec.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (a *LegacyPlugin) toolInfos() []map[string]any {
	var out []map[string]any
	for _, inst := range a.insts {
		for _, spec := range inst.Snapshot().Tools {
			out = append(out, map[string]any{
				"name":        spec.Name,
				"label":       spec.Label,
				"description": spec.Description,
				"parameters":  spec.Parameters,
				"exposure":    spec.ToolExposure(),
				"source":      "plugin",
			})
		}
	}
	return out
}

func (a *LegacyPlugin) commandInfos() []map[string]any {
	var out []map[string]any
	for _, inst := range a.insts {
		for _, spec := range inst.Snapshot().Commands {
			out = append(out, map[string]any{
				"name":         spec.Name,
				"description":  spec.Description,
				"argumentHint": spec.ArgumentHint,
				"source":       "plugin",
			})
		}
	}
	return out
}

// Close releases the legacy plugin host.
func (a *LegacyPlugin) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	host := a.host
	a.mu.Unlock()
	if host == nil {
		return nil
	}
	return host.Close()
}

func (a *LegacyPlugin) diagnostic(message string) {
	if a.opts.OnDiagnostic != nil && message != "" {
		a.opts.OnDiagnostic(message)
	}
}

func exposureForDefinition(exposure string) string {
	switch exposure {
	case "codemode", "deferred", "hidden":
		return exposure
	default:
		return "direct"
	}
}

func convertResult(result plugin.ToolResult) ToolResult {
	blocks := make([]ContentBlock, 0, len(result.Content))
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			blocks = append(blocks, ContentBlock{Type: "text", Text: block.Text})
		case "image":
			blocks = append(blocks, ContentBlock{Type: "image", Data: block.Data, MIMEType: block.MIMEType})
		default:
			raw, _ := json.Marshal(block)
			var converted ContentBlock
			if json.Unmarshal(raw, &converted) == nil {
				blocks = append(blocks, converted)
			}
		}
	}
	return ToolResult{Content: blocks, Details: result.Details, IsError: result.IsError, Terminate: result.Terminate}
}

func cloneMap[K comparable, V any](in map[K]V) map[K]V {
	if in == nil {
		return nil
	}
	out := make(map[K]V, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
