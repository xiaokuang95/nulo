package sdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultAgentDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	t.Setenv(envHome, tmp)
	t.Setenv(envAgentDir, filepath.Join(tmp, "from-agent-env"))
	if got := DefaultAgentDir(); got != filepath.Join(tmp, "from-agent-env") {
		t.Fatalf("agent env wins: %q", got)
	}
	os.Unsetenv(envAgentDir)
	t.Setenv(envHome, tmp)
	if got := DefaultAgentDir(); got != filepath.Join(tmp, "agent") {
		t.Fatalf("home/agent: %q", got)
	}
	_ = home
}

func TestDefaultSessionDirEnv(t *testing.T) {
	tmp := t.TempDir()
	os.Unsetenv(envSessionDir)
	os.Unsetenv(legacyEnvSessionDir)
	if got := DefaultSessionDir(filepath.Join(tmp, "agent")); got != filepath.Join(tmp, "agent", "sessions") {
		t.Fatalf("default session dir = %q", got)
	}
	t.Setenv(legacyEnvSessionDir, filepath.Join(tmp, "legacy"))
	if got := DefaultSessionDir(filepath.Join(tmp, "agent")); got != filepath.Join(tmp, "legacy") {
		t.Fatalf("legacy env = %q", got)
	}
	t.Setenv(envSessionDir, filepath.Join(tmp, "current"))
	if got := DefaultSessionDir(filepath.Join(tmp, "agent")); got != filepath.Join(tmp, "current") {
		t.Fatalf("current env = %q", got)
	}
}
