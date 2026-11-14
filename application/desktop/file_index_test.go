package main

import (
	"path/filepath"
	"testing"
)

func TestScanWorkspaceFilesIncludesTrackedAndUntracked(t *testing.T) {
	root := gitTestRepo(t)
	writeFile(t, filepath.Join(root, "nested", "a.go"), "package nested\n")
	writeFile(t, filepath.Join(root, "b.txt"), "b\n")
	files := scanWorkspaceFiles(root)
	seen := map[string]bool{}
	for _, file := range files {
		seen[file] = true
	}
	for _, want := range []string{"README.md", "b.txt", "nested/a.go"} {
		if !seen[want] {
			t.Errorf("missing %q in %v", want, files)
		}
	}
	a := baseApp()
	a.draft = "@nested/"
	matches := a.filteredMentions(root)
	if len(matches) != 1 || matches[0] != "nested/a.go" {
		t.Fatalf("matches = %v", matches)
	}
}
