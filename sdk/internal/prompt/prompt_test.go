package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/skills"
)

func TestAGENTSAndSkills(t *testing.T) {
	cwd := t.TempDir()
	os.WriteFile(filepath.Join(cwd, "AGENTS.md"), []byte("always 中文"), 0o644)
	skillDir := filepath.Join(cwd, ".nulo", "skills", "pdf")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: nulo-pdf-test\ndescription: PDF helper\n---\n\nDo PDF things.\n"), 0o644)
	sk := skills.Discover(cwd, "", true)
	var found skills.Skill
	for _, s := range sk {
		if s.Name == "nulo-pdf-test" {
			found = s
		}
	}
	if found.Name == "" {
		t.Fatalf("missing project skill in %+v", sk)
	}
	sk = []skills.Skill{found}
	sec := BuildSections(Options{CWD: cwd, Skills: sk, ProjectTrusted: true})
	if !strings.Contains(sec["project_context"], "always") {
		t.Fatalf("context %q", sec["project_context"])
	}
	if !strings.Contains(sec["skills"], "<name>nulo-pdf-test</name>") {
		t.Fatalf("skills %q", sec["skills"])
	}
	expanded, ok := skills.ExpandCommand("/skill:nulo-pdf-test extract", sk)
	if !ok || !strings.Contains(expanded, "Do PDF things") || !strings.Contains(expanded, "extract") {
		t.Fatalf("%v %q", ok, expanded)
	}
}
