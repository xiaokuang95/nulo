package prompt

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/skills"
)

type Options struct {
	CWD            string
	AgentDir       string
	CustomPrompt   string
	ForcePrompt    string
	Append         string
	Tools          []agent.AgentTool
	Skills         []skills.Skill
	MCPServers     []MCPServer
	NoContextFiles bool
	ProjectTrusted bool
}

// toolSnippet is the one-line summary of a tool for the system prompt's tool
// list. Descriptions may be long and multi-line (codemode's lists its globals);
// the list keeps one line per tool so it stays scannable.
func toolSnippet(description, name string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return name
	}
	if i := strings.Index(description, "\n\n"); i >= 0 {
		description = description[:i]
	}
	description = strings.Join(strings.Fields(description), " ")
	const limit = 240
	if len(description) <= limit {
		return description
	}
	cut := limit
	if space := strings.LastIndexByte(description[:limit], ' '); space > limit/2 {
		cut = space
	}
	return strings.TrimRight(description[:cut], " ,.;:") + "…"
}

func BuildSections(opt Options) map[string]string {
	if opt.ForcePrompt != "" {
		return map[string]string{"preamble": opt.ForcePrompt}
	}
	sec := map[string]string{}
	if opt.CustomPrompt != "" {
		sec["preamble"] = opt.CustomPrompt
	} else {
		sec["preamble"] = "You are an expert coding assistant operating inside nulo, a coding agent harness. You help users by reading files, executing commands, editing code, and writing new files."
		var lines []string
		for _, t := range opt.Tools {
			lines = append(lines, "- "+t.Name+": "+toolSnippet(t.Description, t.Name))
		}
		tools := "(none)"
		if len(lines) > 0 {
			tools = strings.Join(lines, "\n")
		}
		sec["tools"] = tools + "\n\nIn addition to the tools above, you may have access to other custom tools depending on the project."
		sec["rules"] = buildRules(opt.Tools)
	}
	// Emitted regardless of a custom prompt: servers whose tools are not declared
	// to the model still need to be reachable through codemode, and Pi appends
	// this section separately for the same reason.
	if body := renderMCPServers(opt.MCPServers); body != "" {
		sec["mcp_servers"] = body
	}
	if opt.Append != "" {
		sec["addendum"] = opt.Append
	}
	if !opt.NoContextFiles {
		if files := LoadContextFiles(opt.CWD, opt.AgentDir); len(files) > 0 {
			var b strings.Builder
			b.WriteString("Project-specific instructions and guidelines:\n\n")
			for _, f := range files {
				b.WriteString("<project_instructions path=\"" + f.Path + "\">\n")
				b.WriteString(f.Content)
				b.WriteString("\n</project_instructions>\n")
			}
			sec["project_context"] = strings.TrimSpace(b.String())
		}
	}
	// Pi only advertises skills when a tool can load their files, and names that
	// tool in the instructions so the model knows how to read one.
	if reader := skillFileReadTool(opt.Tools); reader != "" {
		if body := strings.TrimSpace(skills.FormatForPrompt(opt.Skills, reader)); body != "" {
			sec["skills"] = body
		}
	}
	if opt.CWD != "" {
		sec["cwd"] = filepath.ToSlash(opt.CWD)
	}
	return sec
}

// buildRules renders the tool-derived guidelines. Pi's buildRules guards its file
// operation rule on "some shell tool is selected and no dedicated search tool is";
// nulo ships no grep/find/ls tools, so that guard reduces to the shell half.
func buildRules(ts []agent.AgentTool) string {
	hasBash := hasTool(ts, "bash")
	hasPowerShell := hasTool(ts, "powershell")
	var rules []string
	switch {
	case hasBash && hasPowerShell:
		rules = append(rules, "Use bash or PowerShell for file operations like listing, searching, and finding files")
	case hasPowerShell:
		rules = append(rules, "Use PowerShell for file operations like listing, searching, and finding files")
	case hasBash:
		rules = append(rules, "Use bash for file operations like ls, rg, find")
	}
	rules = append(rules, "Be concise in your responses", "Show file paths clearly when working with files")
	return "- " + strings.Join(rules, "\n- ")
}

// skillFileReadTool mirrors Pi's selection: the first of read/bash that is
// declared to the model, or "indirect" when the only way to reach a skill file
// is through another tool. Without a reader, no skills section is emitted.
func skillFileReadTool(ts []agent.AgentTool) string {
	for _, name := range []string{"read", "bash"} {
		if hasTool(ts, name) {
			return name
		}
	}
	if hasTool(ts, "codemode") {
		return "indirect"
	}
	return ""
}

func hasTool(ts []agent.AgentTool, name string) bool {
	for _, t := range ts {
		if t.Name == name {
			return true
		}
	}
	return false
}

type ContextFile struct {
	Path, Content string
}

func LoadContextFiles(cwd, agentDir string) []ContextFile {
	var out []ContextFile
	seen := map[string]bool{}
	add := func(dir string) {
		if f := loadContextFileFromDir(dir); f != nil && !seen[f.Path] {
			seen[f.Path] = true
			out = append(out, *f)
		}
	}
	if agentDir != "" {
		add(agentDir)
	}
	if cwd == "" {
		return out
	}
	dir, _ := filepath.Abs(cwd)
	var stack []ContextFile
	for i := 0; i < 16; i++ {
		if f := loadContextFileFromDir(dir); f != nil && !seen[f.Path] {
			seen[f.Path] = true
			stack = append([]ContextFile{*f}, stack...)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	out = append(out, stack...)
	return out
}

func loadContextFileFromDir(dir string) *ContextFile {
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md", "AGENTS.MD", "CLAUDE.md", "CLAUDE.MD"} {
		p := filepath.Join(dir, name)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		return &ContextFile{Path: p, Content: string(b)}
	}
	return nil
}

func WrapSections(sec map[string]string) map[string]*string {
	out := map[string]*string{}
	if p, ok := sec["preamble"]; ok {
		cp := p
		out["preamble"] = &cp
	}
	order := []string{"tools", "mcp_servers", "rules", "addendum", "project_context", "skills", "cwd"}
	for _, name := range order {
		v, ok := sec[name]
		if !ok || v == "" {
			continue
		}
		wrapped := "<" + name + ">\n" + v + "\n</" + name + ">"
		out[name] = &wrapped
	}
	for name, v := range sec {
		if _, ok := out[name]; ok || name == "preamble" {
			continue
		}
		wrapped := "<" + name + ">\n" + v + "\n</" + name + ">"
		out[name] = &wrapped
	}
	return out
}
