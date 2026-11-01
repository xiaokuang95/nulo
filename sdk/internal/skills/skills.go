// Package skills discovers Agent Skills (SKILL.md directories and standalone
// markdown files) and expands /skill:name commands.
//
// The behaviour here is a port of Pi's dist/core/skills.ts plus the automatic
// skill locations its resource loader contributes, so a skill directory that
// works in Pi works the same way in nulo:
//
//   - a directory containing SKILL.md is a skill root and is not walked further
//   - a root directory's own .md children are standalone skills
//   - dot entries and node_modules are skipped, symlinks are followed, and
//     .gitignore / .ignore / .fdignore are honoured
//   - project skills win over user skills, and the first skill with a given
//     name wins while the loser is reported as a collision
//   - an invalid name is a warning, not a rejection; a missing description
//     means the skill is not loaded
//   - disable-model-invocation must be the boolean true
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Spec limits from the Agent Skills specification.
const (
	MaxNameLength        = 64
	MaxDescriptionLength = 1024
)

// configDirName is nulo's project configuration directory, the counterpart of
// Pi's ".pi".
const configDirName = ".nulo"

// Skill is one discoverable skill. Only the name, description and location are
// advertised to the model; the body is read on demand.
type Skill struct {
	Name                   string
	Description            string
	FilePath               string
	BaseDir                string
	Source                 string // "user", "project", or "path"
	DisableModelInvocation bool
}

// Collision reports a name clash: the skill that won and the one that lost.
type Collision struct {
	Name       string
	WinnerPath string
	LoserPath  string
}

// Diagnostic is a load-time warning. Pi reports these at startup and keeps
// going; nulo keeps them on the session for the host to surface.
type Diagnostic struct {
	Type      string // "warning" or "collision"
	Message   string
	Path      string
	Collision *Collision
}

// LoadResult is the outcome of a discovery pass.
type LoadResult struct {
	Skills      []Skill
	Diagnostics []Diagnostic
}

// LoadOptions describes where to look. IncludeDefaults turns on the four
// automatic locations; SkillPaths adds explicit files or directories.
type LoadOptions struct {
	Cwd             string
	AgentDir        string
	ProjectTrusted  bool
	IncludeDefaults bool
	SkillPaths      []string
}

// LoadSkills discovers skills from every configured location in Pi's precedence
// order: project .nulo/skills, then project .agents/skills directories from the
// working directory up to the repository root, then the user agent directory,
// then ~/.agents/skills, and finally any explicit paths.
func LoadSkills(opts LoadOptions) LoadResult {
	var res LoadResult
	seenPath := map[string]bool{}
	indexByName := map[string]int{}

	add := func(r LoadResult) {
		res.Diagnostics = append(res.Diagnostics, r.Diagnostics...)
		for _, sk := range r.Skills {
			real := canonicalPath(sk.FilePath)
			// The same file reached twice (usually through a symlink) is not a
			// collision; Pi skips it silently.
			if seenPath[real] {
				continue
			}
			if i, ok := indexByName[sk.Name]; ok {
				res.Diagnostics = append(res.Diagnostics, Diagnostic{
					Type:    "collision",
					Message: fmt.Sprintf("name %q collision", sk.Name),
					Path:    sk.FilePath,
					Collision: &Collision{
						Name:       sk.Name,
						WinnerPath: res.Skills[i].FilePath,
						LoserPath:  sk.FilePath,
					},
				})
				continue
			}
			indexByName[sk.Name] = len(res.Skills)
			seenPath[real] = true
			res.Skills = append(res.Skills, sk)
		}
	}

	if opts.IncludeDefaults {
		if opts.Cwd != "" && opts.ProjectTrusted {
			add(loadSkillsFromDir(filepath.Join(opts.Cwd, configDirName, "skills"), "project", true, nil, ""))
			userAgents := userAgentsSkillsDir()
			for _, dir := range ancestorAgentsSkillDirs(opts.Cwd) {
				if userAgents != "" && samePath(dir, userAgents) {
					continue
				}
				add(loadSkillsFromDir(dir, "project", true, nil, ""))
			}
		}
		if opts.AgentDir != "" {
			add(loadSkillsFromDir(filepath.Join(opts.AgentDir, "skills"), "user", true, nil, ""))
		}
		if dir := userAgentsSkillsDir(); dir != "" {
			add(loadSkillsFromDir(dir, "user", true, nil, ""))
		}
	}

	for _, raw := range opts.SkillPaths {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		path := resolveSkillPath(raw, opts.Cwd)
		// Pi labels an explicit path as user/project when it happens to live
		// under one of those roots, but only when the defaults are not already
		// being loaded.
		source := "path"
		if !opts.IncludeDefaults {
			switch {
			case opts.AgentDir != "" && isUnderPath(path, filepath.Join(opts.AgentDir, "skills")):
				source = "user"
			case opts.Cwd != "" && isUnderPath(path, filepath.Join(opts.Cwd, configDirName, "skills")):
				source = "project"
			}
		}
		info, err := os.Stat(path)
		if err != nil {
			res.Diagnostics = append(res.Diagnostics, Diagnostic{
				Type: "warning", Message: "skill path does not exist", Path: path,
			})
			continue
		}
		switch {
		case info.IsDir():
			add(loadSkillsFromDir(path, source, true, nil, ""))
		case strings.HasSuffix(strings.ToLower(path), ".md"):
			add(loadSkillFromFile(path, source))
		default:
			res.Diagnostics = append(res.Diagnostics, Diagnostic{
				Type: "warning", Message: "skill path is not a markdown file", Path: path,
			})
		}
	}
	return res
}

// LoadSkillsFromDir loads one directory, following the same recursion rules Pi
// uses. It is exported because hosts may point nulo at extra skill roots.
func LoadSkillsFromDir(dir, source string) LoadResult {
	return loadSkillsFromDir(dir, source, true, nil, "")
}

func loadSkillsFromDir(dir, source string, includeRootFiles bool, ig *ignoreMatcher, root string) LoadResult {
	var res LoadResult
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return res
	}
	if root == "" {
		root = dir
	}
	if ig == nil {
		ig = &ignoreMatcher{}
	}
	// Pi shares one matcher down the walk, so a pattern declared in one
	// directory also applies to the siblings visited after it.
	ig.addFromDir(dir, root)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return res
	}

	// A SKILL.md here makes this directory a skill root. Do not descend: a
	// skill's own references/ and scripts/ must not be scanned for skills.
	for _, entry := range entries {
		if entry.Name() != "SKILL.md" {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		if !isRegularFile(full) {
			continue
		}
		if ig.ignores(relPosix(root, full), false) {
			continue
		}
		return loadSkillFromFile(full, source)
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		full := filepath.Join(dir, name)
		// Stat follows symlinks; a broken link is skipped.
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		isDir := info.IsDir()
		if ig.ignores(relPosix(root, full), isDir) {
			continue
		}
		if isDir {
			sub := loadSkillsFromDir(full, source, false, ig, root)
			res.Skills = append(res.Skills, sub.Skills...)
			res.Diagnostics = append(res.Diagnostics, sub.Diagnostics...)
			continue
		}
		if !includeRootFiles || !strings.HasSuffix(name, ".md") {
			continue
		}
		sub := loadSkillFromFile(full, source)
		res.Skills = append(res.Skills, sub.Skills...)
		res.Diagnostics = append(res.Diagnostics, sub.Diagnostics...)
	}
	return res
}

func loadSkillFromFile(path, source string) LoadResult {
	var res LoadResult
	declared := filepath.Base(path) == "SKILL.md"

	raw, err := os.ReadFile(path)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, Diagnostic{
			Type: "warning", Message: err.Error(), Path: path,
		})
		return res
	}

	fm, _, err := ParseFrontmatter(string(raw))
	if err != nil {
		// An unparseable standalone markdown file is simply not a skill; a
		// broken SKILL.md is worth reporting.
		if declared {
			res.Diagnostics = append(res.Diagnostics, Diagnostic{
				Type: "warning", Message: err.Error(), Path: path,
			})
		}
		return res
	}

	description, _ := fm["description"].(string)
	hasDescription := strings.TrimSpace(description) != ""
	if !declared && !hasDescription {
		return res
	}

	dir := filepath.Dir(path)
	if hasDescription {
		if n := utf8.RuneCountInString(description); n > MaxDescriptionLength {
			res.Diagnostics = append(res.Diagnostics, Diagnostic{
				Type:    "warning",
				Message: fmt.Sprintf("description exceeds %d characters (%d)", MaxDescriptionLength, n),
				Path:    path,
			})
		}
	} else {
		res.Diagnostics = append(res.Diagnostics, Diagnostic{
			Type: "warning", Message: "description is required", Path: path,
		})
	}

	// Pi uses the declared name verbatim — no lowercasing — and falls back to
	// the containing directory. Validation only warns.
	name, _ := fm["name"].(string)
	if name == "" {
		name = filepath.Base(dir)
	}
	for _, msg := range validateName(name) {
		res.Diagnostics = append(res.Diagnostics, Diagnostic{
			Type: "warning", Message: msg, Path: path,
		})
	}

	if !hasDescription {
		return res
	}

	res.Skills = append(res.Skills, Skill{
		Name:                   name,
		Description:            description,
		FilePath:               path,
		BaseDir:                dir,
		Source:                 source,
		DisableModelInvocation: fm["disable-model-invocation"] == true,
	})
	return res
}

var nameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

func validateName(name string) []string {
	var errs []string
	if n := utf8.RuneCountInString(name); n > MaxNameLength {
		errs = append(errs, fmt.Sprintf("name exceeds %d characters (%d)", MaxNameLength, n))
	}
	if !nameRe.MatchString(name) {
		errs = append(errs, "name contains invalid characters (must be lowercase a-z, 0-9, hyphens only)")
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		errs = append(errs, "name must not start or end with a hyphen")
	}
	if strings.Contains(name, "--") {
		errs = append(errs, "name must not contain consecutive hyphens")
	}
	return errs
}

// FormatForPrompt renders the skill cards. The body is deliberately omitted:
// the model reads SKILL.md only when the task matches, which is what keeps
// instructions out of context until they are needed.
//
// fileReadTool names the tool that can load a skill file ("read" or "bash");
// "indirect" means it is only reachable through another tool.
func FormatForPrompt(list []Skill, fileReadTool string) string {
	visible := make([]Skill, 0, len(list))
	for _, s := range list {
		if !s.DisableModelInvocation {
			visible = append(visible, s)
		}
	}
	if len(visible) == 0 {
		return ""
	}

	loadLine := "Load a skill's file when the task matches its description."
	switch fileReadTool {
	case "read":
		loadLine = "Use the read tool to load a skill's file when the task matches its description."
	case "bash":
		loadLine = "Use bash to load a skill's file when the task matches its description."
	}

	var b strings.Builder
	b.WriteString("\n\nThe following skills provide specialized instructions for specific tasks.\n")
	b.WriteString(loadLine + "\n")
	b.WriteString("When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.\n")
	b.WriteString("\n<available_skills>\n")
	for _, s := range visible {
		b.WriteString("  <skill>\n")
		b.WriteString("    <name>" + escapeXML(s.Name) + "</name>\n")
		b.WriteString("    <description>" + escapeXML(s.Description) + "</description>\n")
		b.WriteString("    <location>" + escapeXML(s.FilePath) + "</location>\n")
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// ExpandCommand turns "/skill:name args" into the skill body plus the request.
// It matches the name exactly, so a skill named "PDF-Helper" is invoked as
// /skill:PDF-Helper. Unknown names and unreadable files pass through untouched.
func ExpandCommand(text string, list []Skill) (string, bool) {
	const prefix = "/skill:"
	if !strings.HasPrefix(text, prefix) {
		return text, false
	}
	rest := text[len(prefix):]
	name, args := rest, ""
	if i := strings.Index(rest, " "); i >= 0 {
		name = rest[:i]
		args = strings.TrimSpace(rest[i+1:])
	}

	var found *Skill
	for i := range list {
		if list[i].Name == name {
			found = &list[i]
			break
		}
	}
	if found == nil {
		return text, false
	}

	raw, err := os.ReadFile(found.FilePath)
	if err != nil {
		return text, false
	}
	body := strings.TrimSpace(StripFrontmatter(string(raw)))
	block := "<skill name=\"" + found.Name + "\" location=\"" + found.FilePath + "\">\n" +
		"References are relative to " + found.BaseDir + ".\n\n" +
		body + "\n</skill>"
	if args != "" {
		return block + "\n\n" + args, true
	}
	return block, true
}

func userAgentsSkillsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".agents", "skills")
}

// ancestorAgentsSkillDirs lists <dir>/.agents/skills from cwd upward, stopping
// at the repository root when there is one — the root itself is included.
func ancestorAgentsSkillDirs(cwd string) []string {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		dir = cwd
	}
	gitRoot := findGitRepoRoot(dir)
	var dirs []string
	for {
		dirs = append(dirs, filepath.Join(dir, ".agents", "skills"))
		if gitRoot != "" && samePath(dir, gitRoot) {
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

func findGitRepoRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func resolveSkillPath(raw, cwd string) string {
	p := os.ExpandEnv(strings.TrimSpace(raw))
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), string(os.PathSeparator)))
		}
	}
	if !filepath.IsAbs(p) && cwd != "" {
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(p)
}

// isUnderPath reports whether target is root or lives beneath it, mirroring
// Pi's helper of the same name.
func isUnderPath(target, root string) bool {
	target, root = canonicalPath(target), canonicalPath(root)
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

func canonicalPath(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func samePath(a, b string) bool {
	if strings.EqualFold(canonicalPath(a), canonicalPath(b)) {
		return true
	}
	// EvalSymlinks fails for paths that do not exist yet; fall back to a plain
	// case-insensitive comparison of the cleaned absolute paths.
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func relPosix(root, full string) string {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return filepath.ToSlash(full)
	}
	return filepath.ToSlash(rel)
}

// isRegularFile follows symlinks, because Pi loads a SKILL.md symlink.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
