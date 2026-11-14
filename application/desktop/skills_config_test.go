package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func skillDisabled(s *session, name string) (bool, bool) {
	for _, skill := range s.SkillAll {
		if skill.Name == name {
			return !skill.Enabled, true
		}
	}
	return false, false
}

func TestToggleSkillPersistsAndReloads(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	dir := filepath.Join(agentDir, "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: demo skill\n---\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	if s == nil {
		t.Fatal("no active session")
	}
	disabled, found := skillDisabled(s, "demo")
	if !found || disabled {
		t.Fatalf("demo before toggle = found:%v disabled:%v", found, disabled)
	}
	a.toggleSkill(s, "demo", false)
	disabled, found = skillDisabled(s, "demo")
	if !found || !disabled {
		t.Fatalf("demo after disable = found:%v disabled:%v", found, disabled)
	}
	if slices.Contains(s.SkillNames, "demo") {
		t.Fatalf("disabled skill still loaded: %v", s.SkillNames)
	}
	prefs, err := loadDesktopPreferences(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(prefs.DisabledSkills) != 1 || prefs.DisabledSkills[0] != "demo" {
		t.Fatalf("disabled skills = %v", prefs.DisabledSkills)
	}
	a.toggleSkill(s, "demo", true)
	disabled, found = skillDisabled(s, "demo")
	if !found || disabled {
		t.Fatalf("demo after enable = found:%v disabled:%v", found, disabled)
	}
	if !slices.Contains(s.SkillNames, "demo") {
		t.Fatalf("enabled skill not loaded: %v", s.SkillNames)
	}
}
