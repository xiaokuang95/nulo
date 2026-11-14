package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/xiaokuang95/nulo/sdk"
)

func gitTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitTest(t, root, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "README.md")
	runGitTest(t, root, "-c", "user.name=Nulo Test", "-c", "user.email=nulo@example.test", "commit", "-m", "initial")
	return root
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestWorktreeIndexRoundTrip(t *testing.T) {
	dir := t.TempDir()
	records := map[string]worktreeRecord{
		"s1": {SessionID: "s1", Root: "/repo", Path: "/repo-wt", Branch: "nulo/wt-s1", Active: true},
	}
	if err := saveWorktreeIndex(dir, records); err != nil {
		t.Fatal(err)
	}
	got, err := loadWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["s1"] != records["s1"] {
		t.Fatalf("loaded = %+v, want %+v", got, records)
	}
	info, err := os.Stat(worktreeIndexPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestSDKHostSwitchesToWorktreeAndBack(t *testing.T) {
	root := gitTestRepo(t)
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	a := newAppWithHost(hostOptions{Workspace: root, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	if s == nil || s.CWD != root {
		t.Fatalf("active = %+v", s)
	}
	if err := a.host.switchEnvironment(a, s, "Worktree"); err != nil {
		t.Fatal(err)
	}
	if s.Env != "Worktree" || s.Worktree == "" {
		t.Fatalf("session env = %+v", s)
	}
	if s.CWD == root || !strings.HasPrefix(s.Branch, "nulo/wt-") {
		t.Fatalf("worktree = %+v", s)
	}
	if got := runGitTest(t, s.CWD, "rev-parse", "--show-toplevel"); !samePath(got, s.CWD) {
		t.Fatalf("worktree root = %q, cwd = %q", got, s.CWD)
	}
	if s.BaseCWD != root {
		t.Fatalf("base cwd = %q", s.BaseCWD)
	}
	if a.activeID != s.ID {
		t.Fatalf("active ID = %q, session = %q", a.activeID, s.ID)
	}
	engine := a.host.engine(s)
	if engine == nil {
		t.Fatal("worktree session has no engine")
	}
	if engine.rt.Services.CWD != s.CWD {
		t.Fatalf("runtime cwd = %q, session cwd = %q", engine.rt.Services.CWD, s.CWD)
	}
	worktreePath := s.CWD
	if err := a.host.switchEnvironment(a, s, "Local"); err != nil {
		t.Fatal(err)
	}
	if s.Env != "Local" || s.CWD != root || s.Worktree != "" {
		t.Fatalf("local session = %+v", s)
	}
	if err := a.host.switchEnvironment(a, s, "Worktree"); err != nil {
		t.Fatal(err)
	}
	if s.CWD != worktreePath {
		t.Fatalf("worktree was not reused: %q, want %q", s.CWD, worktreePath)
	}
}

func TestWorktreeRestoredAfterRestart(t *testing.T) {
	root := gitTestRepo(t)
	agentDir := t.TempDir()
	sessionDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", sessionDir)

	a := newAppWithHost(hostOptions{Workspace: root, AgentDir: agentDir, Trusted: true, Sync: true})
	s := a.active()
	e := a.host.engine(s)
	if e == nil {
		t.Fatal("engine missing")
	}
	settled := make(chan struct{}, 1)
	e.sess.Subscribe(func(ev sdk.Event) {
		if ev.Type == "agent_settled" {
			select {
			case settled <- struct{}{}:
			default:
			}
		}
	})
	a.send("persist this session")
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for agent_settled")
	}
	if err := a.host.switchEnvironment(a, s, "Worktree"); err != nil {
		t.Fatal(err)
	}
	worktreePath, branch := s.CWD, s.Branch
	if !fileExists(s.Path) {
		t.Fatalf("session file missing: %s", s.Path)
	}
	a.closeHost()

	b := newAppWithHost(hostOptions{Workspace: root, AgentDir: agentDir, Trusted: true, Sync: true})
	defer b.closeHost()
	restored := b.sessionByID(s.ID)
	if restored == nil {
		t.Fatalf("session %q missing after restart: %+v", s.ID, b.sessions)
	}
	if restored.Env != "Worktree" || restored.CWD != worktreePath || restored.Branch != branch {
		t.Fatalf("restored = %+v", restored)
	}
	if restored.BaseCWD != root {
		t.Fatalf("restored base = %q", restored.BaseCWD)
	}
	found := false
	for _, group := range b.projects {
		if group.CWD == root && group.Key == restored.Project {
			found = true
		}
	}
	if !found {
		t.Fatalf("worktree session is not grouped under %q: %+v", root, b.projects)
	}
	if engine := b.host.engine(restored); engine == nil || engine.rt.Services.CWD != worktreePath {
		t.Fatalf("restored runtime cwd = %+v", engine)
	}
	if err := b.host.switchEnvironment(b, restored, "Local"); err != nil {
		t.Fatal(err)
	}
	if restored.CWD != root || restored.Env != "Local" || restored.ID != s.ID {
		t.Fatalf("saved session did not return to local: %+v", restored)
	}
	if err := b.host.switchEnvironment(b, restored, "Worktree"); err != nil {
		t.Fatal(err)
	}
	if restored.CWD != worktreePath || restored.ID != s.ID {
		t.Fatalf("saved session did not reuse worktree: %+v", restored)
	}
}

func samePath(a, b string) bool {
	eval := func(value string) string {
		if path, err := filepath.EvalSymlinks(value); err == nil {
			return path
		}
		return filepath.Clean(value)
	}
	return eval(a) == eval(b)
}

func TestWorktreeCreatedSessionReturnsToLocalAfterRestart(t *testing.T) {
	root := gitTestRepo(t)
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())

	a := newAppWithHost(hostOptions{Workspace: root, AgentDir: agentDir, Trusted: true, Sync: true})
	s := a.active()
	if err := a.host.switchEnvironment(a, s, "Worktree"); err != nil {
		t.Fatal(err)
	}
	worktreePath := s.CWD
	e := a.host.engine(s)
	if e == nil {
		t.Fatal("engine missing")
	}
	settled := make(chan struct{}, 1)
	e.sess.Subscribe(func(ev sdk.Event) {
		if ev.Type == "agent_settled" {
			select {
			case settled <- struct{}{}:
			default:
			}
		}
	})
	a.send("work in the worktree")
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for agent_settled")
	}
	if info, err := sdk.ListAllSessions(agentDir); err != nil || len(info) != 1 || !samePath(info[0].CWD, worktreePath) {
		t.Fatalf("stored header cwd = %+v, err %v", info, err)
	}
	if err := a.host.switchEnvironment(a, s, "Local"); err != nil {
		t.Fatal(err)
	}
	if s.Env != "Local" || s.CWD != root {
		t.Fatalf("local session = %+v", s)
	}
	a.closeHost()

	b := newAppWithHost(hostOptions{Workspace: root, AgentDir: agentDir, Trusted: true, Sync: true})
	defer b.closeHost()
	restored := b.sessionByID(s.ID)
	if restored == nil {
		t.Fatalf("session %q missing after restart: %+v", s.ID, b.sessions)
	}
	if restored.Env != "Local" || restored.CWD != root {
		t.Fatalf("restored worktree session = %+v", restored)
	}
	b.host.dropEngine(restored)
	if err := b.host.rename(b, restored, "Renamed from worktree"); err != nil {
		t.Fatal(err)
	}
	if restored.Title != "Renamed from worktree" {
		t.Fatalf("title = %q", restored.Title)
	}
	for _, group := range b.projects {
		if group.CWD == worktreePath {
			t.Fatalf("worktree leaked as a project: %+v", b.projects)
		}
	}
}

func TestWorktreeMergeAndRemove(t *testing.T) {
	root := gitTestRepo(t)
	runGitTest(t, root, "config", "user.name", "Nulo Test")
	runGitTest(t, root, "config", "user.email", "nulo@example.test")
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	a := newAppWithHost(hostOptions{Workspace: root, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	if err := a.host.switchEnvironment(a, s, "Worktree"); err != nil {
		t.Fatal(err)
	}
	worktreePath, branch := s.CWD, s.Branch
	writeFile(t, filepath.Join(worktreePath, "worktree.txt"), "from worktree\n")
	runGitTest(t, worktreePath, "add", "worktree.txt")
	runGitTest(t, worktreePath, "commit", "-m", "worktree change")
	if err := a.host.mergeWorktree(a, s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "worktree.txt"))
	if err != nil || string(data) != "from worktree\n" {
		t.Fatalf("merged file = %q, %v", data, err)
	}
	if err := a.host.removeWorktree(a, s); err != nil {
		t.Fatal(err)
	}
	if s.Env != "Local" || s.CWD != root {
		t.Fatalf("session after remove = %+v", s)
	}
	if fileExists(worktreePath) {
		t.Fatalf("worktree still exists: %s", worktreePath)
	}
	if _, ok := a.host.worktreeFor(s.ID); ok {
		t.Fatal("worktree record was not removed")
	}
	if _, err := gitOutput(root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		t.Fatalf("branch %s still exists", branch)
	}
}

func TestWorktreeMergeConflictResolve(t *testing.T) {
	root := gitTestRepo(t)
	runGitTest(t, root, "config", "user.name", "Nulo Test")
	runGitTest(t, root, "config", "user.email", "nulo@example.test")
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	a := newAppWithHost(hostOptions{Workspace: root, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	if err := a.host.switchEnvironment(a, s, "Worktree"); err != nil {
		t.Fatal(err)
	}
	worktreePath := s.CWD
	writeFile(t, filepath.Join(worktreePath, "README.md"), "worktree\n")
	runGitTest(t, worktreePath, "add", "README.md")
	runGitTest(t, worktreePath, "commit", "-m", "worktree change")
	writeFile(t, filepath.Join(root, "README.md"), "local\n")
	runGitTest(t, root, "add", "README.md")
	runGitTest(t, root, "commit", "-m", "local change")
	if err := a.host.mergeWorktree(a, s); err == nil {
		t.Fatal("merge should have conflicted")
	}
	if !a.mergeConflictOpen || len(a.mergeConflictFiles) != 1 || a.mergeConflictFiles[0] != "README.md" {
		t.Fatalf("conflicts = %+v open=%v", a.mergeConflictFiles, a.mergeConflictOpen)
	}
	if !mergeInProgress(root) {
		t.Fatal("merge state missing")
	}
	tt := ui.NewTester(a.view, 1440, 900)
	tt.Frame()
	if !tt.HasText("Merge conflicts") || !tt.HasText("README.md") {
		t.Fatalf("conflict dialog missing: %q", tt.Texts())
	}
	if err := tt.Click("Resolve"); err != nil {
		t.Fatal(err)
	}
	if !a.mergeEditorOpen {
		t.Fatal("merge editor did not open")
	}
	if a.mergeEditorBase != "hello\n" || a.mergeEditorOurs != "local\n" || a.mergeEditorTheirs != "worktree\n" {
		t.Fatalf("three-way stages = base %q ours %q theirs %q", a.mergeEditorBase, a.mergeEditorOurs, a.mergeEditorTheirs)
	}
	tt.Frame()
	for _, label := range []string{"Base", "Local", "Worktree", "Result", "Save & resolve"} {
		if !tt.HasText(label) {
			t.Fatalf("merge editor missing %q: %q", label, tt.Texts())
		}
	}
	a.mergeEditorResult = "<<<<<<< HEAD\nlocal\n=======\nworktree\n>>>>>>> branch\n"
	a.saveMergeEditor()
	if a.mergeEditorError == "" {
		t.Fatal("unresolved markers should be rejected")
	}
	a.mergeEditorResult = "merged\n"
	a.saveMergeEditor()
	if a.mergeEditorError != "" {
		t.Fatalf("save merge editor error = %s", a.mergeEditorError)
	}
	if a.mergeEditorOpen {
		t.Fatal("merge editor did not close")
	}
	if len(a.mergeConflictFiles) != 0 {
		t.Fatalf("conflicts after resolve = %+v", a.mergeConflictFiles)
	}
	if err := a.completeMerge(); err != nil {
		t.Fatal(err)
	}
	if mergeInProgress(root) {
		t.Fatal("merge still in progress")
	}
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || string(data) != "merged\n" {
		t.Fatalf("merged file = %q, %v", data, err)
	}
}

func TestMergeConflictPathValidation(t *testing.T) {
	a := baseApp()
	a.mergeConflictRoot = t.TempDir()
	for _, path := range []string{"../escape", "/tmp/escape", ".."} {
		if _, _, err := a.mergeConflictFullPath(path); err == nil {
			t.Fatalf("path %q should be rejected", path)
		}
	}
	full, clean, err := a.mergeConflictFullPath(filepath.Join("dir", "file.txt"))
	if err != nil || clean != filepath.Join("dir", "file.txt") || !strings.HasPrefix(full, a.mergeConflictRoot) {
		t.Fatalf("valid path = %q %q %v", full, clean, err)
	}
}

func TestDeleteSessionCleansWorktree(t *testing.T) {
	root := gitTestRepo(t)
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	a := newAppWithHost(hostOptions{Workspace: root, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	if err := a.host.switchEnvironment(a, s, "Worktree"); err != nil {
		t.Fatal(err)
	}
	worktreePath, branch := s.CWD, s.Branch
	if err := a.host.deleteSession(a, s); err != nil {
		t.Fatal(err)
	}
	if fileExists(worktreePath) {
		t.Fatalf("worktree still exists: %s", worktreePath)
	}
	if _, ok := a.host.worktreeFor(s.ID); ok {
		t.Fatal("worktree record was not removed")
	}
	if _, err := gitOutput(root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		t.Fatalf("branch %s still exists", branch)
	}
}
