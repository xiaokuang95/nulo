package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitChangesReadsTrackedAndUntracked(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	writeFile(t, filepath.Join(dir, "tracked.go"), "package main\n\nfunc main() {}\n")
	runGit(t, dir, "add", "tracked.go")
	runGit(t, dir, "commit", "-m", "initial")
	writeFile(t, filepath.Join(dir, "tracked.go"), "package main\n\nfunc main() { println(\"hi\") }\n")
	writeFile(t, filepath.Join(dir, "new.txt"), "one\ntwo\n")

	changes, err := gitChanges(dir)
	if err != nil {
		t.Fatal(err)
	}
	if changes == nil || len(changes.Files) != 2 {
		t.Fatalf("changes = %+v", changes)
	}
	byPath := map[string]changeFile{}
	for _, f := range changes.Files {
		byPath[f.Path] = f
	}
	tracked := byPath["tracked.go"]
	if tracked.Status != "M" || tracked.Add != 1 || tracked.Del != 1 || tracked.Language != "go" {
		t.Fatalf("tracked = %+v", tracked)
	}
	if len(tracked.Diff) == 0 || !strings.Contains(strings.Join(tracked.Diff, "\n"), "println") {
		t.Fatalf("tracked diff = %+v", tracked.Diff)
	}
	added := byPath["new.txt"]
	if added.Status != "A" || added.Add != 2 || added.Language != "text" {
		t.Fatalf("added = %+v", added)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
