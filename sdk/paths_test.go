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
