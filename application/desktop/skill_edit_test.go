package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructuredSkillEditPreservesFrontmatter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	agentDir := t.TempDir()
	a := newAppWithHost(hostOptions{Workspace: t.TempDir(), AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	path := filepath.Join(agentDir, "skills", "demo", "SKILL.md")
	writeFile(t, path, `---
name: demo
description: "old description"
license: MIT
metadata:
  owner: me
---

Old body.
`)
	a.openSkillEditForm(s, skillInfo{Name: "demo", Path: path})
	if !a.skillFormOpen || !a.skillFormEdit {
		t.Fatalf("edit form did not open: %+v", a)
	}
	if a.skillFormDescription != "old description" || a.skillFormBody != "Old body." {
		t.Fatalf("parsed skill = %q / %q", a.skillFormDescription, a.skillFormBody)
	}
	a.skillFormDescription = "new description"
	a.skillFormBody = "New body."
	a.finishSkillForm()
	if a.skillFormError != "" {
		t.Fatalf("form error = %s", a.skillFormError)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`description: "new description"`, "license: MIT", "owner: me", "New body."} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Old body.") {
		t.Fatalf("old body was not replaced:\n%s", text)
	}
}

func TestStructuredSkillEditFallsBackForMultilineDescription(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	agentDir := t.TempDir()
	a := newAppWithHost(hostOptions{Workspace: t.TempDir(), AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	path := filepath.Join(agentDir, "skills", "demo", "SKILL.md")
	writeFile(t, path, `---
name: demo
description: >
  folded description
---

Body.
`)
	a.openSkillEditForm(s, skillInfo{Name: "demo", Path: path})
	if !a.configOpen || a.skillFormOpen {
		t.Fatalf("expected raw editor fallback, config=%v form=%v", a.configOpen, a.skillFormOpen)
	}
}

func TestRenderSkillDocumentWithoutDescription(t *testing.T) {
	doc := skillDocument{Frontmatter: []string{"name: demo"}, DescLine: -1, Body: "old"}
	data := renderSkillDocument(doc, "new", "body")
	text := string(data)
	if !strings.Contains(text, `name: demo`) || !strings.Contains(text, `description: "new"`) || !strings.Contains(text, "body") {
		t.Fatalf("rendered:\n%s", text)
	}
}
