package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Skill struct {
	Name                   string
	Description            string
	FilePath               string
	DisableModelInvocation bool
}

func Discover(cwd, agentDir string, projectTrusted bool) []Skill {
	var paths []string
	if agentDir != "" {
		paths = append(paths, filepath.Join(agentDir, "skills"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".agents", "skills"))
	}
	if cwd != "" {
		if projectTrusted {
			paths = append(paths, filepath.Join(cwd, ".nulo", "skills"))
			paths = append(paths, ancestorAgentsSkills(cwd)...)
		}
	}
	seen := map[string]bool{}
	var out []Skill
	for _, dir := range paths {
		for _, s := range loadDir(dir) {
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			out = append(out, s)
		}
	}
	return out
}

func ancestorAgentsSkills(cwd string) []string {
	var dirs []string
	dir, _ := filepath.Abs(cwd)
	for i := 0; i < 12; i++ {
		dirs = append(dirs, filepath.Join(dir, ".agents", "skills"))
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return dirs
}

func loadDir(root string) []Skill {
	var out []Skill
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}
		if s, ok := loadFile(path); ok {
			out = append(out, s)
		}
		return nil
	})
	return out
}

func loadFile(path string) (Skill, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, false
	}
	name, desc, disable, bodyOK := parseFrontmatter(string(b))
	if !bodyOK {
		return Skill{}, false
	}
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	name = strings.ToLower(name)
	if !validName(name) {
		return Skill{}, false
	}
	if desc == "" {
		desc = name
	}
	return Skill{Name: name, Description: desc, FilePath: path, DisableModelInvocation: disable}, true
}

func parseFrontmatter(s string) (name, desc string, disable, ok bool) {
	s = strings.TrimPrefix(s, "\ufeff")
	if !strings.HasPrefix(s, "---") {
		return "", "", false, true
	}
	rest := s[3:]
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", false, true
	}
	fm := rest[:end]
	ok = true
	for _, line := range strings.Split(fm, "\n") {
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(strings.Trim(v, `"'`))
		switch k {
		case "name":
			name = v
		case "description":
			desc = v
		case "disable-model-invocation":
			disable = v == "true" || v == "yes"
		}
	}
	return name, desc, disable, ok
}

var nameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func validName(n string) bool {
	return len(n) > 0 && len(n) <= 64 && nameRe.MatchString(n)
}

func FormatForPrompt(skills []Skill) string {
	var vis []Skill
	for _, s := range skills {
		if !s.DisableModelInvocation {
			vis = append(vis, s)
		}
	}
	if len(vis) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The following skills provide specialized instructions for specific tasks.\n")
	b.WriteString("Use the read tool to load a skill's file when the task matches its description.\n")
	b.WriteString("When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.\n\n")
	b.WriteString("<available_skills>\n")
	for _, s := range vis {
		b.WriteString("  <skill>\n")
		b.WriteString("    <name>" + xmlEscape(s.Name) + "</name>\n")
		b.WriteString("    <description>" + xmlEscape(s.Description) + "</description>\n")
		b.WriteString("    <location>" + xmlEscape(s.FilePath) + "</location>\n")
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func LoadBody(s Skill) (string, error) {
	b, err := os.ReadFile(s.FilePath)
	if err != nil {
		return "", err
	}
	text := strings.TrimPrefix(string(b), "\ufeff")
	if strings.HasPrefix(text, "---") {
		rest := text[3:]
		if i := strings.Index(rest, "\n---"); i >= 0 {
			rest = rest[i+4:]
			return strings.TrimLeft(rest, "\n"), nil
		}
	}
	return text, nil
}

func ExpandCommand(text string, skills []Skill) (string, bool) {
	if !strings.HasPrefix(text, "/skill:") {
		return text, false
	}
	rest := strings.TrimPrefix(text, "/skill:")
	name, args, _ := strings.Cut(rest, " ")
	name = strings.TrimSpace(name)
	args = strings.TrimSpace(args)
	for _, s := range skills {
		if s.Name == name {
			body, err := LoadBody(s)
			if err != nil {
				return text, false
			}
			out := body
			if args != "" {
				out = body + "\n\n" + args
			}
			return out, true
		}
	}
	return text, false
}
