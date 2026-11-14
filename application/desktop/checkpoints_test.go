package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckpointRestoresChangedFiles(t *testing.T) {
	root := gitTestRepo(t)
	agentDir := t.TempDir()
	s := &session{ID: "s1", CWD: root, Real: true}
	writeFile(t, filepath.Join(root, "README.md"), "checkpoint\n")
	writeFile(t, filepath.Join(root, "new.txt"), "new at checkpoint\n")
	h := &desktopHost{agentDir: agentDir}
	if err := h.captureCheckpoint(s, "root"); err != nil {
		t.Fatal(err)
	}
	files, note, ok := checkpointInfo(agentDir, s.ID, "root")
	if !ok || files < 2 || note != "" {
		t.Fatalf("snapshot info = %d %q %v", files, note, ok)
	}
	writeFile(t, filepath.Join(root, "README.md"), "later\n")
	if err := os.Remove(filepath.Join(root, "new.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "later.txt"), "leave me\n")
	restored, err := restoreCheckpoint(agentDir, s, "root")
	if err != nil {
		t.Fatal(err)
	}
	if restored < 2 {
		t.Fatalf("restored = %d", restored)
	}
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || string(data) != "checkpoint\n" {
		t.Fatalf("README = %q, %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(root, "new.txt"))
	if err != nil || string(data) != "new at checkpoint\n" {
		t.Fatalf("new.txt = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "later.txt")); err != nil {
		t.Fatalf("later untracked file should stay: %v", err)
	}
}

func TestCheckpointDoesNotOverwriteExistingSnapshot(t *testing.T) {
	root := gitTestRepo(t)
	agentDir := t.TempDir()
	s := &session{ID: "s1", CWD: root, Real: true}
	writeFile(t, filepath.Join(root, "README.md"), "first\n")
	h := &desktopHost{agentDir: agentDir}
	if err := h.captureCheckpoint(s, "root"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "README.md"), "second\n")
	if err := h.captureCheckpoint(s, "root"); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreCheckpoint(agentDir, s, "root"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || string(data) != "first\n" {
		t.Fatalf("README = %q, %v", data, err)
	}
}

func TestCheckpointRejectsMovedHead(t *testing.T) {
	root := gitTestRepo(t)
	agentDir := t.TempDir()
	s := &session{ID: "s1", CWD: root, Real: true}
	writeFile(t, filepath.Join(root, "README.md"), "checkpoint\n")
	h := &desktopHost{agentDir: agentDir}
	if err := h.captureCheckpoint(s, "root"); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "README.md")
	runGitTest(t, root, "-c", "user.name=Nulo Test", "-c", "user.email=nulo@example.test", "commit", "-m", "move head")
	writeFile(t, filepath.Join(root, "README.md"), "later\n")
	if _, err := restoreCheckpoint(agentDir, s, "root"); err == nil || !strings.Contains(err.Error(), "HEAD moved") {
		t.Fatalf("restore error = %v", err)
	}
}

func TestCheckpointSubdirectoryPaths(t *testing.T) {
	root := gitTestRepo(t)
	sub := filepath.Join(root, "sub")
	writeFile(t, filepath.Join(sub, "a.txt"), "one\n")
	writeFile(t, filepath.Join(root, "outside.txt"), "outside\n")
	runGitTest(t, root, "add", ".")
	runGitTest(t, root, "-c", "user.name=Nulo Test", "-c", "user.email=nulo@example.test", "commit", "-m", "sub")
	writeFile(t, filepath.Join(sub, "a.txt"), "two\n")
	writeFile(t, filepath.Join(root, "outside.txt"), "outside changed\n")
	writeFile(t, filepath.Join(sub, "new.txt"), "new\n")
	agentDir := t.TempDir()
	s := &session{ID: "s1", CWD: sub, Real: true}
	h := &desktopHost{agentDir: agentDir}
	if err := h.captureCheckpoint(s, "root"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "a.txt"), "later\n")
	if _, err := restoreCheckpoint(agentDir, s, "root"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(sub, "a.txt"))
	if err != nil || string(data) != "two\n" {
		t.Fatalf("a.txt = %q, %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(root, "outside.txt"))
	if err != nil || string(data) != "outside changed\n" {
		t.Fatalf("outside file should not be restored: %q, %v", data, err)
	}
}

func TestCheckpointUnavailableOutsideGit(t *testing.T) {
	dir := t.TempDir()
	agentDir := t.TempDir()
	s := &session{ID: "s1", CWD: dir, Real: true}
	h := &desktopHost{agentDir: agentDir}
	if err := h.captureCheckpoint(s, "root"); err != nil {
		t.Fatal(err)
	}
	if _, note, ok := checkpointInfo(agentDir, s.ID, "root"); ok || !strings.Contains(note, "not a git repository") {
		t.Fatalf("info = %q %v", note, ok)
	}
	if _, err := restoreCheckpoint(agentDir, s, "root"); err == nil {
		t.Fatal("restore should fail without a git repository")
	}
}

func TestCheckpointCopyForFork(t *testing.T) {
	root := gitTestRepo(t)
	agentDir := t.TempDir()
	s := &session{ID: "parent", CWD: root, Real: true}
	writeFile(t, filepath.Join(root, "README.md"), "fork checkpoint\n")
	h := &desktopHost{agentDir: agentDir}
	if err := h.captureCheckpoint(s, "root"); err != nil {
		t.Fatal(err)
	}
	if err := copyCheckpointSnapshots(agentDir, "parent", "child"); err != nil {
		t.Fatal(err)
	}
	child := &session{ID: "child", CWD: root, Real: true}
	if files, _, ok := checkpointInfo(agentDir, child.ID, "root"); !ok || files == 0 {
		t.Fatalf("copied snapshot info = %d %v", files, ok)
	}
	writeFile(t, filepath.Join(root, "README.md"), "later\n")
	if _, err := restoreCheckpoint(agentDir, child, "root"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || string(data) != "fork checkpoint\n" {
		t.Fatalf("README = %q, %v", data, err)
	}
}

func TestCheckpointRejectsOtherWorkspace(t *testing.T) {
	root := gitTestRepo(t)
	other := gitTestRepo(t)
	agentDir := t.TempDir()
	s := &session{ID: "s1", CWD: root, Real: true}
	writeFile(t, filepath.Join(root, "README.md"), "checkpoint\n")
	h := &desktopHost{agentDir: agentDir}
	if err := h.captureCheckpoint(s, "root"); err != nil {
		t.Fatal(err)
	}
	moved := &session{ID: "s1", CWD: other, Real: true}
	if _, err := restoreCheckpoint(agentDir, moved, "root"); err == nil || !strings.Contains(err.Error(), "another workspace") {
		t.Fatalf("restore error = %v", err)
	}
}
