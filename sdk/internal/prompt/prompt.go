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
	NoContextFiles bool
	ProjectTrusted bool
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
			snip := t.Description
			if snip == "" {
				snip = t.Name
			}
			lines = append(lines, "- "+t.Name+": "+snip)
		}
		tools := "(none)"
		if len(lines) > 0 {
			tools = strings.Join(lines, "\n")
		}
		sec["tools"] = tools + "\n\nIn addition to the tools above, you may have access to other custom tools depending on the project."
		sec["rules"] = "- Be concise in your responses\n- Show file paths clearly when working with files"
		if hasBash(opt.Tools) {
			sec["rules"] = "- Use bash for file operations like ls, rg, find\n" + sec["rules"]
		}
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
	if body := skills.FormatForPrompt(opt.Skills); body != "" {
		sec["skills"] = body
	}
	if opt.CWD != "" {
		sec["cwd"] = filepath.ToSlash(opt.CWD)
	}
	return sec
}

func hasBash(tools []agent.AgentTool) bool {
	for _, t := range tools {
		if t.Name == "bash" {
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
	order := []string{"tools", "rules", "addendum", "project_context", "skills", "cwd"}
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
