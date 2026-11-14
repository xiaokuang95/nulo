package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func mergeBranch(t *testing.T, root, branch string) {
	t.Helper()
	cmd := exec.Command("git", "-C", root, "merge", branch)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("merge unexpectedly succeeded:\n%s", out)
	}
}

func TestBinaryMergeConflictChooseSide(t *testing.T) {
	root := gitTestRepo(t)
	runGitTest(t, root, "config", "user.name", "Nulo Test")
	runGitTest(t, root, "config", "user.email", "nulo@example.test")
	runGitTest(t, root, "checkout", "-q", "-b", "wt")
	if err := os.WriteFile(filepath.Join(root, "image.bin"), []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "image.bin")
	runGitTest(t, root, "commit", "-qm", "base")
	runGitTest(t, root, "checkout", "-q", "main")
	writeFile(t, filepath.Join(root, "image.bin"), string([]byte{0, 1, 2}))
	runGitTest(t, root, "add", "image.bin")
	runGitTest(t, root, "commit", "-qm", "base on main")
	runGitTest(t, root, "checkout", "-q", "wt")
	if err := os.WriteFile(filepath.Join(root, "image.bin"), []byte{0, 3, 4}, 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "image.bin")
	runGitTest(t, root, "commit", "-qm", "worktree")
	runGitTest(t, root, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(root, "image.bin"), []byte{0, 5, 6}, 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "image.bin")
	runGitTest(t, root, "commit", "-qm", "local")
	mergeBranch(t, root, "wt")

	a := baseApp()
	a.openMergeConflicts(nil, root, "wt")
	if len(a.mergeConflictFiles) != 1 || a.mergeConflictFiles[0] != "image.bin" {
		t.Fatalf("conflicts = %+v", a.mergeConflictFiles)
	}
	a.openMergeEditor("image.bin")
	if !a.mergeEditorBinary {
		t.Fatal("binary conflict was not detected")
	}
	a.chooseMergeSide(true)
	a.saveMergeEditor()
	if a.mergeEditorError != "" {
		t.Fatal(a.mergeEditorError)
	}
	if len(a.mergeConflictFiles) != 0 {
		t.Fatalf("conflicts remain = %+v", a.mergeConflictFiles)
	}
	data, err := os.ReadFile(filepath.Join(root, "image.bin"))
	if err != nil || string(data) != string([]byte{0, 5, 6}) {
		t.Fatalf("binary = %v, %v", data, err)
	}
}

func TestRenameDeleteConflictChooseWorktree(t *testing.T) {
	root := gitTestRepo(t)
	runGitTest(t, root, "config", "user.name", "Nulo Test")
	runGitTest(t, root, "config", "user.email", "nulo@example.test")
	runGitTest(t, root, "checkout", "-q", "-b", "wt")
	runGitTest(t, root, "mv", "README.md", "RENAMED.md")
	runGitTest(t, root, "commit", "-qm", "rename")
	runGitTest(t, root, "checkout", "-q", "main")
	runGitTest(t, root, "rm", "-q", "README.md")
	runGitTest(t, root, "commit", "-qm", "delete")
	mergeBranch(t, root, "wt")

	a := baseApp()
	a.openMergeConflicts(nil, root, "wt")
	if len(a.mergeConflictFiles) != 1 || a.mergeConflictFiles[0] != "RENAMED.md" {
		t.Fatalf("conflicts = %+v", a.mergeConflictFiles)
	}
	a.openMergeEditor("RENAMED.md")
	a.chooseMergeSide(false)
	a.saveMergeEditor()
	if a.mergeEditorError != "" {
		t.Fatal(a.mergeEditorError)
	}
	if len(a.mergeConflictFiles) != 0 {
		t.Fatalf("conflicts remain = %+v", a.mergeConflictFiles)
	}
	if _, err := os.Stat(filepath.Join(root, "RENAMED.md")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
}
