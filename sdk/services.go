package sdk

import (
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
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
		CWD:             cwd,
		AgentDir:        agentDir,
		InMemory:        opt.InMemory,
		ProjectTrusted:  opt.ProjectTrusted,
		NoContextFiles:  opt.NoContextFiles,
		StreamFn:        opt.StreamFn,
		Sessions:        opt.SessionManager,
		SettingsManager: settingsManager,
	}
	return s, nil
}

// GetSettingsManager returns the services settings view. It is an accessor
// for callers that prefer Pi's method style; SettingsManager is the field.
func (s *AgentSessionServices) GetSettingsManager() *SettingsManager {
	return s.SettingsManager
}

func (s *AgentSessionServices) Close() error { return nil }
