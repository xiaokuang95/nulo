package sdk

import (
	"sync"

	"github.com/xiaokuang95/nulo/plugin"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/mcp"
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

	plugins       *plugin.Host
	pluginOnce    sync.Once
	pluginMu      sync.Mutex
	pluginDiags   []PluginDiagnostic
	pluginSession *Session
	pluginRuntime *Runtime
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
		dynamicProviders: ai.Providers{},
		pluginMCP:        map[string]mcp.ServerConfig{},
		virtualModels:    map[string]map[string]any{},
		busSubscribers:   map[string][]func(any){},
		pluginFlags:      map[string]any{},
	}
	if s.pluginMode == "" {
		s.pluginMode = "print"
	}
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
	if s.plugins != nil {
		return s.plugins.Close()
	}
	return nil
}
