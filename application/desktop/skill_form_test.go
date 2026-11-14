package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillFormAddEditDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	agentDir := t.TempDir()
	a := newAppWithHost(hostOptions{Workspace: t.TempDir(), AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	if s == nil {
		t.Fatal("no active session")
	}
	a.openSkillForm(s)
	a.skillFormTarget = "user"
	a.skillFormName = "demo-skill"
	a.skillFormDescription = "Use for the demo"
	a.skillFormBody = "Read the demo files first."
	a.finishSkillForm()
	if a.skillFormError != "" {
		t.Fatalf("form error = %s", a.skillFormError)
	}
	path := filepath.Join(agentDir, "skills", "demo-skill", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `description: "Use for the demo"`) || !strings.Contains(string(data), "Read the demo files first.") {
		t.Fatalf("skill content = %q", data)
	}
	var added skillInfo
	for _, skill := range s.SkillAll {
		if skill.Name == "demo-skill" {
			added = skill
		}
	}
	if added.Name == "" || !added.Enabled || added.Path != path {
		t.Fatalf("added skill = %+v", added)
	}
	a.editSkill(s, added)
	if !a.configOpen {
		t.Fatal("skill editor did not open")
	}
	a.configText = strings.Replace(a.configText, "Read the demo files first.", "Updated instructions.", 1)
	a.saveConfigEditor()
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Updated instructions.") {
		t.Fatalf("edited skill = %q", data)
	}
	if err := a.deleteSkill(s, added); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("skill directory still exists: %v", err)
	}
}
