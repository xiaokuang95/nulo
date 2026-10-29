package sdk

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	envHome       = "NULO_HOME"
	envAgentDir   = "NULO_CODING_AGENT_DIR"
	envSessionDir = "NULO_CODING_AGENT_SESSION_DIR"
)

// DefaultHomeDir is ~/.nulo, or $NULO_HOME.
func DefaultHomeDir() string {
	if v := os.Getenv(envHome); v != "" {
		return expandTilde(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".nulo"
	}
	return filepath.Join(home, ".nulo")
}

// DefaultAgentDir is ~/.nulo/agent, or $NULO_CODING_AGENT_DIR.
func DefaultAgentDir() string {
	if v := os.Getenv(envAgentDir); v != "" {
		return expandTilde(v)
	}
	return filepath.Join(DefaultHomeDir(), "agent")
}

// DefaultSessionDir is <agent>/sessions, or $NULO_CODING_AGENT_SESSION_DIR.
func DefaultSessionDir(agentDir string) string {
	if v := os.Getenv(envSessionDir); v != "" {
		return expandTilde(v)
	}
	if agentDir == "" {
		agentDir = DefaultAgentDir()
	}
	return filepath.Join(agentDir, "sessions")
}

func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		if p == "~" {
			return home
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
