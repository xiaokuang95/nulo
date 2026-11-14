package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHunkPatchRevertsOneHunk(t *testing.T) {
	root := gitTestRepo(t)
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line " + string(rune('A'+i))
	}
	writeFile(t, filepath.Join(root, "README.md"), strings.Join(lines, "\n")+"\n")
	runGitTest(t, root, "add", "README.md")
	runGitTest(t, root, "-c", "user.name=Nulo Test", "-c", "user.email=nulo@example.test", "commit", "-m", "lines")
	lines[1] = "changed early"
	lines[17] = "changed late"
	writeFile(t, filepath.Join(root, "README.md"), strings.Join(lines, "\n")+"\n")
	diff, err := fileDiff(root, "README.md", "M")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.Join(diff, "\n"), "@@") < 2 {
		t.Fatalf("expected two hunks: %v", diff)
	}
	patch, ok := hunkPatch(diff, 0)
	if !ok {
		t.Fatal("hunk patch missing")
	}
	hunks := 0
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "@@") {
			hunks++
		}
	}
	if hunks != 1 || strings.Contains(patch, "changed late") {
		t.Fatalf("patch contains the wrong hunks:\n%s", patch)
	}
	if err := gitApply(root, patch, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "changed early") || !strings.Contains(text, "changed late") {
		t.Fatalf("content after hunk revert:\n%s", text)
	}
}

func TestStageAndRevertFiles(t *testing.T) {
	root := gitTestRepo(t)
	writeFile(t, filepath.Join(root, "new.txt"), "new\n")
	writeFile(t, filepath.Join(root, "README.md"), "changed\n")
	a := baseApp()
	a.host = &desktopHost{sync: true}
	s := &session{ID: "s", CWD: root, Real: true}
	a.sessions = []*session{s}
	a.activeID = s.ID
	a.stageChangeFile(s, changeFile{Path: "new.txt"})
	status := runGitTest(t, root, "status", "--porcelain")
	if !strings.Contains(status, "A  new.txt") {
		t.Fatalf("new file was not staged: %q", status)
	}
	a.unstageChangeFile(s, changeFile{Path: "new.txt"})
	if status := runGitTest(t, root, "status", "--porcelain"); !strings.Contains(status, "?? new.txt") {
		t.Fatalf("new file was not unstaged: %q", status)
	}
	a.stageChangeFile(s, changeFile{Path: "new.txt"})
	a.revertChangeFile(s, changeFile{Path: "new.txt"})
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new file still exists: %v", err)
	}
	if status := runGitTest(t, root, "status", "--porcelain"); strings.Contains(status, "new.txt") {
		t.Fatalf("new file still in status: %q", status)
	}
	a.revertChangeFile(s, changeFile{Path: "README.md"})
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("README = %q", data)
	}
}

func TestUnstageInRepositoryWithoutHead(t *testing.T) {
	root := t.TempDir()
	runGitTest(t, root, "init", "-b", "main")
	writeFile(t, filepath.Join(root, "first.txt"), "first\n")
	a := baseApp()
	a.host = &desktopHost{sync: true}
	s := &session{ID: "s", CWD: root, Real: true}
	a.sessions = []*session{s}
	a.activeID = s.ID
	a.stageChangeFile(s, changeFile{Path: "first.txt"})
	a.unstageChangeFile(s, changeFile{Path: "first.txt"})
	if status := runGitTest(t, root, "status", "--porcelain"); !strings.Contains(status, "?? first.txt") {
		t.Fatalf("status = %q", status)
	}
}
