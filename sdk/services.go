package sdk

import (
	"errors"
	"strings"
	"sync"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/mcp"
	"github.com/xiaokuang95/nulo/sdk/internal/plugin"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
	settingspkg "github.com/xiaokuang95/nulo/sdk/internal/settings"
)

type CreateAgentSessionServicesOptions struct {
	CWD             string
	AgentDir        string
	InMemory        bool
	ProjectTrusted  bool
	NoContextFiles  bool
	StreamFn        ai.StreamFn
	SessionManager  *session.Manager
	SettingsManager *SettingsManager
	// PluginPaths are extra shared libraries, plugin directories or
	// plugin.json manifests. Settings "extensions", <agentDir>/plugins and the
	// trusted project .nulo/plugins are loaded too.
	PluginPaths []string
	NoPlugins   bool
	PluginUI    *PluginUI
	PluginMode  string
	// DisabledSkills removes matching skills from the model-facing skill list.
	// Hosts persist their own enable switches and pass the disabled names here.
	DisabledSkills []string

	// Plugins are SDK plugins loaded after the built-in tool and provider
	// plugins. A plugin may contribute tools, providers, hooks, commands,
	// prompt sections, MCP servers and UI declarations.
	Plugins []Plugin
	// NoBuiltinPlugins disables the built-in tool and provider plugins. The
	// caller is then responsible for registering those capabilities.
	NoBuiltinPlugins bool
}

type AgentSessionServices struct {
	CWD             string
	AgentDir        string
	InMemory        bool
	ProjectTrusted  bool
	NoContextFiles  bool
	StreamFn        ai.StreamFn
	Sessions        *session.Manager
	SettingsManager *SettingsManager
	PluginPaths     []string
	NoPlugins       bool
	DisabledSkills  map[string]bool
	Registry        *Registry

	plugins       *plugin.Host
	pluginOnce    sync.Once
	pluginMu      sync.Mutex
	pluginDiags   []PluginDiagnostic
	pluginSession *Session
	pluginRuntime *Runtime
	childSessions map[string]*pluginChildSession
	pluginMode    string
	uiBridge      *PluginUI

	providersMu      sync.Mutex
	dynamicProviders ai.Providers
	pluginMCP        map[string]mcp.ServerConfig
	virtualModels    map[string]map[string]any
	pluginFlags      map[string]any
	busMu            sync.Mutex
	busSubscribers   map[string][]func(any)
}

func disabledSkillSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]bool, len(names))
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			out[name] = true
		}
	}
	return out
}

func CreateAgentSessionServices(opt CreateAgentSessionServicesOptions) (*AgentSessionServices, error) {
	cwd := opt.CWD
	if cwd == "" {
		cwd = "."
	}
	agentDir := opt.AgentDir
	if agentDir == "" {
		agentDir = DefaultAgentDir()
	}
	settingsManager := opt.SettingsManager
	if settingsManager == nil {
		trusted := opt.ProjectTrusted
		settingsManager = settingspkg.NewWithOptions(cwd, agentDir, settingspkg.Options{ProjectTrusted: &trusted})
	}
	s := &AgentSessionServices{
		CWD:              cwd,
		AgentDir:         agentDir,
		InMemory:         opt.InMemory,
		ProjectTrusted:   opt.ProjectTrusted,
		NoContextFiles:   opt.NoContextFiles,
		StreamFn:         opt.StreamFn,
		Sessions:         opt.SessionManager,
		SettingsManager:  settingsManager,
		PluginPaths:      append([]string(nil), opt.PluginPaths...),
		NoPlugins:        opt.NoPlugins,
		pluginMode:       opt.PluginMode,
		uiBridge:         opt.PluginUI,
		DisabledSkills:   disabledSkillSet(opt.DisabledSkills),
		dynamicProviders: ai.Providers{},
		pluginMCP:        map[string]mcp.ServerConfig{},
		virtualModels:    map[string]map[string]any{},
		busSubscribers:   map[string][]func(any){},
		pluginFlags:      map[string]any{},
	}
	if s.pluginMode == "" {
		s.pluginMode = "print"
	}
	registry := NewRegistry()
	if !opt.NoBuiltinPlugins {
		if err := registry.Register(builtinToolsPlugin(cwd)); err != nil {
			_ = registry.Close()
			return nil, err
		}
		if err := registry.Register(builtinProvidersPlugin(agentDir)); err != nil {
			_ = registry.Close()
			return nil, err
		}
	}
	for _, p := range opt.Plugins {
		if err := registry.Register(p); err != nil {
			_ = registry.Close()
			return nil, err
		}
	}
	s.Registry = registry
	s.initPluginMaps()
	s.ensurePlugins()
	return s, nil
}

// GetSettingsManager returns the services settings view. It is an accessor
// for callers that prefer Pi's method style; SettingsManager is the field.
func (s *AgentSessionServices) GetSettingsManager() *SettingsManager {
	return s.SettingsManager
}

func (s *AgentSessionServices) Close() error {
	s.closePluginChildren()
	var errs []error
	if s.plugins != nil {
		if err := s.plugins.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.Registry != nil {
		if err := s.Registry.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
