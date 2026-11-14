package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDesktopPreferencesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	prefs := desktopPreferences{
		DefaultApproval:  "manual",
		ShowTimestamps:   false,
		CollapseThinking: false,
		CompactDensity:   true,
		Notifications:    false,
		TrustedProjects:  []string{"/a", "/b"},
		DisabledSkills:   []string{"vision"},
	}
	if err := saveDesktopPreferences(dir, prefs); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(desktopPreferencesPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
	got, err := loadDesktopPreferences(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, prefs) {
		t.Fatalf("loaded = %+v, want %+v", got, prefs)
	}
}

func TestHostLoadsDesktopDefaults(t *testing.T) {
	workspace := t.TempDir()
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	writeFile(t, filepath.Join(agentDir, "desktop-settings.json"), `{
		"defaultApproval":"manual","showTimestamps":false,"collapseThinking":false,
		"compactDensity":true,"notifications":false,"disabledSkills":["one"]
	}`)
	a := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	if s == nil {
		t.Fatal("no active session")
	}
	if s.Approval != "manual" {
		t.Fatalf("approval = %q, want manual", s.Approval)
	}
	if a.showTimestamps || a.collapseThinking || !a.compactDensity || a.notifyDone {
		t.Fatalf("appearance defaults = timestamps:%v collapse:%v compact:%v notify:%v", a.showTimestamps, a.collapseThinking, a.compactDensity, a.notifyDone)
	}
	if a.host.disabledSkills["one"] != true {
		t.Fatalf("disabled skills = %v", a.host.disabledSkills)
	}
}

func TestProjectTrustPersistsAndReloadsServices(t *testing.T) {
	workspace := gitTestRepo(t)
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	a := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: false, Sync: true})
	defer a.closeHost()
	s := a.active()
	if s == nil {
		t.Fatal("no active session")
	}
	if a.host.projectTrusted(workspace) {
		t.Fatal("project trusted by default")
	}
	if s.TrustRoot != workspace {
		t.Fatalf("trust root = %q, want %q", s.TrustRoot, workspace)
	}
	welcome, path := s.Welcome, s.Path
	s.Welcome, s.Path = false, ""
	a.setProjectTrusted(s, true)
	if a.host.projectTrusted(workspace) {
		t.Fatal("trust changed with unsaved session history")
	}
	s.Welcome, s.Path = welcome, path
	id := s.ID
	a.setProjectTrusted(s, true)
	if !a.host.projectTrusted(workspace) || !a.host.sessionTrusted(s) {
		t.Fatal("project was not trusted")
	}
	if s.ID != id {
		t.Fatalf("session id changed from %q to %q", id, s.ID)
	}
	if a.host.engine(s) == nil {
		t.Fatal("active session lost its engine after trust change")
	}
	svcs, err := a.host.servicesFor(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !svcs.ProjectTrusted {
		t.Fatal("project services were not rebuilt with trust")
	}
	prefs, err := loadDesktopPreferences(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prefs.TrustedProjects, []string{workspace}) {
		t.Fatalf("trusted projects = %v", prefs.TrustedProjects)
	}
	a.setProjectTrusted(s, false)
	if a.host.projectTrusted(workspace) {
		t.Fatal("project trust was not revoked")
	}
}
