package main

import (
	"os"
	"strings"
	"testing"
)

func TestMCPFormAddEditDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	agentDir := t.TempDir()
	a := newAppWithHost(hostOptions{Workspace: t.TempDir(), AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	if s == nil {
		t.Fatal("no active session")
	}
	a.openMCPForm(s, "")
	a.mcpFormTarget = "global"
	a.mcpFormName = "demo"
	a.mcpFormDescription = "Demo server"
	a.mcpFormCommand = "false"
	a.mcpFormArgs = "--one two"
	a.mcpFormEnv = "A=1\nB = two"
	a.mcpFormAdvanced = `{"exposure":"on-demand","timeout":30}`
	a.mcpFormEnabled = false
	a.finishMCPForm()
	if a.mcpFormError != "" {
		t.Fatalf("add form error = %s", a.mcpFormError)
	}
	root, servers, _, _, err := mcpServersFromFile(a.mcpPath(s, "global"))
	if err != nil {
		t.Fatal(err)
	}
	_ = root
	entry, _ := servers["demo"].(map[string]any)
	if entry == nil || entry["command"] != "false" || entry["description"] != "Demo server" {
		t.Fatalf("entry = %#v", entry)
	}
	args, _ := entry["args"].([]any)
	if len(args) != 2 || args[0] != "--one" || args[1] != "two" {
		t.Fatalf("args = %#v", entry["args"])
	}
	env, _ := entry["env"].(map[string]any)
	if env["A"] != "1" || env["B"] != "two" {
		t.Fatalf("env = %#v", env)
	}
	if entry["exposure"] != "on-demand" || entry["timeout"] != float64(30) {
		t.Fatalf("advanced entry = %#v", entry)
	}
	a.openMCPForm(s, "demo")
	if !strings.Contains(a.mcpFormAdvanced, "on-demand") {
		t.Fatalf("advanced not loaded: %q", a.mcpFormAdvanced)
	}
	a.mcpFormDescription = "Updated"
	a.mcpFormAdvanced = `{"timeout":45}`
	a.mcpFormEnabled = false
	a.finishMCPForm()
	if a.mcpFormError != "" {
		t.Fatalf("edit form error = %s", a.mcpFormError)
	}
	_, servers, _, _, err = mcpServersFromFile(a.mcpPath(s, "global"))
	if err != nil {
		t.Fatal(err)
	}
	entry, _ = servers["demo"].(map[string]any)
	if entry["description"] != "Updated" || entry["enabled"] != false {
		t.Fatalf("edited entry = %#v", entry)
	}
	if entry["timeout"] != float64(45) {
		t.Fatalf("edited advanced entry = %#v", entry)
	}
	if _, ok := entry["exposure"]; ok {
		t.Fatalf("removed advanced key still present: %#v", entry)
	}
	if err := a.deleteMCPServer(s, "demo"); err != nil {
		t.Fatal(err)
	}
	_, servers, _, _, err = mcpServersFromFile(a.mcpPath(s, "global"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := servers["demo"]; ok {
		t.Fatalf("demo still present: %#v", servers)
	}
	if _, err := os.Stat(a.mcpPath(s, "global")); err != nil {
		t.Fatal(err)
	}
}
