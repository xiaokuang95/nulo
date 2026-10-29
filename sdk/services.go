package sdk

import (
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
)

type CreateAgentSessionServicesOptions struct {
	CWD            string
	AgentDir       string
	InMemory       bool
	ProjectTrusted bool
	NoContextFiles bool
	StreamFn       ai.StreamFn
	SessionManager *session.Manager
}

type AgentSessionServices struct {
	CWD            string
	AgentDir       string
	InMemory       bool
	ProjectTrusted bool
	NoContextFiles bool
	StreamFn       ai.StreamFn
	Sessions       *session.Manager
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
	s := &AgentSessionServices{
		CWD:            cwd,
		AgentDir:       agentDir,
		InMemory:       opt.InMemory,
		ProjectTrusted: opt.ProjectTrusted,
		NoContextFiles: opt.NoContextFiles,
		StreamFn:       opt.StreamFn,
		Sessions:       opt.SessionManager,
	}
	return s, nil
}

func (s *AgentSessionServices) Close() error { return nil }
