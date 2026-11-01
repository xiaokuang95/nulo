package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/skills"
)

func TestAGENTSAndSkills(t *testing.T) {
	cwd := t.TempDir()
	os.WriteFile(filepath.Join(cwd, "AGENTS.md"), []byte("always 中文"), 0o644)
	skillDir := filepath.Join(cwd, ".nulo", "skills", "pdf")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: nulo-pdf-test\ndescription: PDF helper\n---\n\nDo PDF things.\n"), 0o644)
	sk := skills.LoadSkills(skills.LoadOptions{Cwd: cwd, ProjectTrusted: true, IncludeDefaults: true}).Skills
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
	sec := BuildSections(Options{
		CWD:            cwd,
		Skills:         sk,
		ProjectTrusted: true,
		// Pi only advertises skills when a tool can load their files.
		Tools: []agent.AgentTool{{Tool: ai.Tool{Name: "read"}}},
	})
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

func TestRulesFollowSelectedShellTools(t *testing.T) {
	tool := func(name string) agent.AgentTool {
		return agent.AgentTool{Tool: ai.Tool{Name: name}}
	}
	cases := []struct {
		name  string
		tools []agent.AgentTool
		want  string
	}{
		{"no shell tool", []agent.AgentTool{tool("read")}, ""},
		{"bash only", []agent.AgentTool{tool("bash")}, "Use bash for file operations like ls, rg, find"},
		{"powershell only", []agent.AgentTool{tool("powershell")}, "Use PowerShell for file operations like listing, searching, and finding files"},
		{"both", []agent.AgentTool{tool("bash"), tool("powershell")}, "Use bash or PowerShell for file operations like listing, searching, and finding files"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := BuildSections(Options{CWD: t.TempDir(), Tools: tc.tools, NoContextFiles: true})["rules"]
			if tc.want == "" {
				if strings.Contains(rules, "for file operations") {
					t.Fatalf("unexpected shell rule in %q", rules)
				}
			} else if !strings.Contains(rules, tc.want) {
				t.Fatalf("rules %q missing %q", rules, tc.want)
			}
			if !strings.Contains(rules, "- Be concise in your responses") {
				t.Fatalf("rules %q dropped the standing rules", rules)
			}
		})
	}
}
