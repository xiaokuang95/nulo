package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateHome points the user-level locations at a temp dir, because LoadSkills
// always consults ~/.agents/skills.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeSkill(t *testing.T, root, name, content string) string {
	t.Helper()
	return writeFile(t, filepath.Join(root, name, "SKILL.md"), content)
}

func frontmatter(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\nBody text.\n"
}

func names(list []Skill) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.Name)
	}
	return out
}

func hasName(list []Skill, name string) bool {
	for _, s := range list {
		if s.Name == name {
			return true
		}
	}
	return false
}

func find(list []Skill, name string) *Skill {
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return nil
}

func warnings(res LoadResult) []string {
	var out []string
	for _, d := range res.Diagnostics {
		if d.Type == "warning" {
			out = append(out, d.Message)
		}
	}
	return out
}

// Project skills take precedence over user skills, which is Pi's ordering: the
// resource loader sorts project-scoped entries ahead of user-scoped ones.
func TestLoadSkillsPrecedence(t *testing.T) {
	home := isolateHome(t)
	agentDir, cwd := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(cwd, ".git"), 0o755)

	writeSkill(t, filepath.Join(agentDir, "skills"), "shared", frontmatter("shared", "user copy"))
	writeSkill(t, filepath.Join(home, ".agents", "skills"), "only-user", frontmatter("only-user", "user agents"))
	writeSkill(t, filepath.Join(cwd, ".nulo", "skills"), "shared", frontmatter("shared", "project copy"))

	res := LoadSkills(LoadOptions{Cwd: cwd, AgentDir: agentDir, ProjectTrusted: true, IncludeDefaults: true})
	if got := find(res.Skills, "shared"); got == nil || got.Description != "project copy" {
		t.Fatalf("project should win: %+v", res.Skills)
	}
	if !hasName(res.Skills, "only-user") {
		t.Fatalf("user agents dir should load: %v", names(res.Skills))
	}
	var collision *Diagnostic
	for i := range res.Diagnostics {
		if res.Diagnostics[i].Type == "collision" {
			collision = &res.Diagnostics[i]
		}
	}
	if collision == nil || collision.Collision == nil {
		t.Fatalf("the losing skill should be reported: %+v", res.Diagnostics)
	}
	if !strings.HasSuffix(filepath.ToSlash(collision.Collision.WinnerPath), "/.nulo/skills/shared/SKILL.md") {
		t.Fatalf("winner %q", collision.Collision.WinnerPath)
	}
	if !strings.HasSuffix(filepath.ToSlash(collision.Collision.LoserPath), "/skills/shared/SKILL.md") {
		t.Fatalf("loser %q", collision.Collision.LoserPath)
	}
}

func TestProjectSourcesNeedTrust(t *testing.T) {
	isolateHome(t)
	agentDir, cwd := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(cwd, ".git"), 0o755)
	writeSkill(t, filepath.Join(cwd, ".nulo", "skills"), "proj", frontmatter("proj", "d"))
	writeSkill(t, filepath.Join(cwd, ".agents", "skills"), "repo", frontmatter("repo", "d"))
	writeSkill(t, filepath.Join(agentDir, "skills"), "user", frontmatter("user", "d"))

	untrusted := LoadSkills(LoadOptions{Cwd: cwd, AgentDir: agentDir, IncludeDefaults: true})
	if hasName(untrusted.Skills, "proj") || hasName(untrusted.Skills, "repo") {
		t.Fatalf("project sources must require trust: %v", names(untrusted.Skills))
	}
	if !hasName(untrusted.Skills, "user") {
		t.Fatalf("user skills should still load: %v", names(untrusted.Skills))
	}

	trusted := LoadSkills(LoadOptions{Cwd: cwd, AgentDir: agentDir, ProjectTrusted: true, IncludeDefaults: true})
	for _, want := range []string{"proj", "repo", "user"} {
		if !hasName(trusted.Skills, want) {
			t.Fatalf("trusted load missing %q: %v", want, names(trusted.Skills))
		}
	}
}

// The ancestor walk stops at the repository root and includes it.
func TestAncestorAgentsSkillDirs(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	deep := filepath.Join(root, "a", "b")
	os.MkdirAll(deep, 0o755)

	dirs := ancestorAgentsSkillDirs(deep)
	if len(dirs) != 3 {
		t.Fatalf("expected b, a, root: %v", dirs)
	}
	if !strings.HasSuffix(filepath.ToSlash(dirs[0]), "a/b/.agents/skills") {
		t.Fatalf("first %q", dirs[0])
	}
	if !strings.HasSuffix(filepath.ToSlash(dirs[2]), "/.agents/skills") {
		t.Fatalf("last %q", dirs[2])
	}
	if got := filepath.Base(filepath.Dir(filepath.Dir(dirs[2]))); got != filepath.Base(root) {
		t.Fatalf("the repository root should be included, got %q", dirs[2])
	}
}

// A SKILL.md makes its directory a skill root: references/ and scripts/ must not
// be scanned for further skills.
func TestRecursionRules(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "outer", frontmatter("outer", "d"))
	// Nested SKILL.md inside a skill root is a supporting file, not a skill.
	writeSkill(t, filepath.Join(root, "outer"), "references/inner", frontmatter("inner", "d"))
	// A root .md file is a standalone skill.
	writeFile(t, filepath.Join(root, "loose.md"), frontmatter("loose", "d"))
	// A nested directory without SKILL.md is walked, and its .md files are not
	// standalone skills because only the root keeps root files.
	writeFile(t, filepath.Join(root, "nested", "notaskill.md"), frontmatter("nested-loose", "d"))
	// node_modules and dot directories are skipped.
	writeSkill(t, filepath.Join(root, "node_modules", "pkg"), "nm", frontmatter("nm", "d"))
	writeSkill(t, filepath.Join(root, ".hidden"), "hid", frontmatter("hid", "d"))

	res := LoadSkillsFromDir(root, "path")
	got := names(res.Skills)
	if len(got) != 2 || !hasName(res.Skills, "outer") || !hasName(res.Skills, "loose") {
		t.Fatalf("discovered %v", got)
	}
}

func TestSymlinkedSkillFileLoads(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	target := writeFile(t, filepath.Join(root, "real.md"), frontmatter("linked", "d"))
	link := filepath.Join(root, "linked", "SKILL.md")
	os.MkdirAll(filepath.Dir(link), 0o755)
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	res := LoadSkillsFromDir(root, "path")
	if !hasName(res.Skills, "linked") {
		t.Fatalf("a symlinked SKILL.md should load: %v", names(res.Skills))
	}
}

func TestIgnoreFiles(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "keep", frontmatter("keep", "d"))
	writeSkill(t, root, "skipme", frontmatter("skipme", "d"))
	writeFile(t, filepath.Join(root, ".gitignore"), "skipme/\n")

	res := LoadSkillsFromDir(root, "path")
	if !hasName(res.Skills, "keep") || hasName(res.Skills, "skipme") {
		t.Fatalf("discovered %v", names(res.Skills))
	}
}

// An invalid name is a warning; the skill still loads. A missing description
// means the skill is not loaded at all.
func TestValidationWarnsButLoads(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "Upper-Case", "---\nname: Upper-Case\ndescription: d\n---\n")
	writeSkill(t, root, "badname", "---\nname: under_score\ndescription: d\n---\n")
	writeSkill(t, root, "nodesc", "---\nname: nodesc\n---\n")
	writeSkill(t, root, "noname", "---\ndescription: falls back to the directory\n---\n")

	res := LoadSkillsFromDir(root, "path")

	// Pi does not lowercase the declared name.
	if s := find(res.Skills, "Upper-Case"); s == nil {
		t.Fatalf("an invalid name should still load: %v", names(res.Skills))
	}
	if s := find(res.Skills, "under_score"); s == nil {
		t.Fatalf("an invalid name should still load: %v", names(res.Skills))
	}
	if hasName(res.Skills, "nodesc") {
		t.Fatalf("a skill without a description must not load: %v", names(res.Skills))
	}
	if s := find(res.Skills, "noname"); s == nil || s.Description != "falls back to the directory" {
		t.Fatalf("name should fall back to the directory: %+v", res.Skills)
	}

	joined := strings.Join(warnings(res), "\n")
	if !strings.Contains(joined, "invalid characters") {
		t.Fatalf("expected a name warning, got %q", joined)
	}
	if !strings.Contains(joined, "description is required") {
		t.Fatalf("expected a description warning, got %q", joined)
	}
}

func TestDescriptionLengthWarning(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	long := strings.Repeat("x", MaxDescriptionLength+1)
	writeSkill(t, root, "long", "---\nname: long\ndescription: "+long+"\n---\n")

	res := LoadSkillsFromDir(root, "path")
	if !hasName(res.Skills, "long") {
		t.Fatal("an over-long description warns but still loads")
	}
	if joined := strings.Join(warnings(res), "\n"); !strings.Contains(joined, "description exceeds 1024") {
		t.Fatalf("warnings %q", joined)
	}
}

// Only the boolean true hides a skill; the YAML 1.2 parser treats "yes" as a
// string, so Pi does too.
func TestDisableModelInvocationStrict(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	writeSkill(t, root, "hidden", "---\nname: hidden\ndescription: d\ndisable-model-invocation: true\n---\n")
	writeSkill(t, root, "yes", "---\nname: yes-skill\ndescription: d\ndisable-model-invocation: yes\n---\n")

	res := LoadSkillsFromDir(root, "path")
	if s := find(res.Skills, "hidden"); s == nil || !s.DisableModelInvocation {
		t.Fatalf("true should hide: %+v", res.Skills)
	}
	if s := find(res.Skills, "yes-skill"); s == nil || s.DisableModelInvocation {
		t.Fatalf("yes is a string, not true: %+v", res.Skills)
	}
}

func TestDuplicateFilesAreNotCollisions(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	target := writeFile(t, filepath.Join(root, "real.md"), frontmatter("dup", "d"))
	linkDir := filepath.Join(root, "linked")
	os.MkdirAll(linkDir, 0o755)
	link := filepath.Join(linkDir, "SKILL.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	res := LoadSkills(LoadOptions{IncludeDefaults: false, SkillPaths: []string{root}})
	if len(res.Skills) != 1 {
		t.Fatalf("the same file reached twice should load once: %v", names(res.Skills))
	}
	for _, d := range res.Diagnostics {
		if d.Type == "collision" {
			t.Fatalf("a duplicate file is not a name collision: %+v", d)
		}
	}
}

func TestExplicitSkillPaths(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	file := writeFile(t, filepath.Join(root, "one.md"), frontmatter("one", "d"))

	res := LoadSkills(LoadOptions{IncludeDefaults: false, SkillPaths: []string{file, filepath.Join(root, "missing")}})
	if len(res.Skills) != 1 || res.Skills[0].Source != "path" {
		t.Fatalf("%+v", res.Skills)
	}
	if joined := strings.Join(warnings(res), "\n"); !strings.Contains(joined, "does not exist") {
		t.Fatalf("warnings %q", joined)
	}
}

func TestFormatForPrompt(t *testing.T) {
	list := []Skill{
		{Name: "visible", Description: "a <b> & \"c\"", FilePath: "/tmp/visible/SKILL.md"},
		{Name: "hidden", Description: "nope", FilePath: "/tmp/hidden/SKILL.md", DisableModelInvocation: true},
	}

	body := FormatForPrompt(list, "read")
	if !strings.HasPrefix(body, "\n\nThe following skills") {
		t.Fatalf("prefix %q", body)
	}
	if !strings.Contains(body, "Use the read tool to load a skill's file") {
		t.Fatalf("read line missing: %q", body)
	}
	if strings.Contains(body, "hidden") {
		t.Fatalf("disable-model-invocation must not be advertised: %q", body)
	}
	if !strings.Contains(body, "<description>a &lt;b&gt; &amp; &quot;c&quot;</description>") {
		t.Fatalf("escaping %q", body)
	}
	if !strings.Contains(body, "<location>/tmp/visible/SKILL.md</location>") {
		t.Fatalf("location %q", body)
	}

	if got := FormatForPrompt(list, "bash"); !strings.Contains(got, "Use bash to load a skill's file") {
		t.Fatalf("bash line %q", got)
	}
	if got := FormatForPrompt(list, "indirect"); !strings.Contains(got, "Load a skill's file when the task matches") {
		t.Fatalf("indirect line %q", got)
	}
	if got := FormatForPrompt([]Skill{{Name: "x", DisableModelInvocation: true}}, "read"); got != "" {
		t.Fatalf("expected no section, got %q", got)
	}
}

func TestExpandCommand(t *testing.T) {
	dir := t.TempDir()
	path := writeSkill(t, dir, "demo", "---\nname: demo\ndescription: d\n---\n\nFull body here.\n")
	manual := writeSkill(t, dir, "manual", "---\nname: manual\ndisable-model-invocation: true\n---\n\nManual body.\n")
	list := []Skill{
		{Name: "demo", FilePath: path, BaseDir: filepath.Dir(path)},
		{Name: "manual", FilePath: manual, BaseDir: filepath.Dir(manual), DisableModelInvocation: true},
	}

	got, ok := ExpandCommand("/skill:demo extract the pdf", list)
	if !ok {
		t.Fatal("expected expansion")
	}
	if !strings.HasPrefix(got, "<skill name=\"demo\" location=\"") {
		t.Fatalf("block header %q", got)
	}
	if !strings.Contains(got, "References are relative to "+filepath.Dir(path)+".") {
		t.Fatalf("base dir line %q", got)
	}
	if !strings.Contains(got, "Full body here.") || !strings.HasSuffix(got, "</skill>\n\nextract the pdf") {
		t.Fatalf("body/args %q", got)
	}
	if strings.Contains(got, "name: demo") {
		t.Fatalf("frontmatter should be stripped: %q", got)
	}

	// Without arguments the block stands alone.
	if got, ok = ExpandCommand("/skill:demo", list); !ok || !strings.HasSuffix(got, "</skill>") {
		t.Fatalf("%v %q", ok, got)
	}
	// A disabled skill is still reachable explicitly.
	if got, ok = ExpandCommand("/skill:manual", list); !ok || !strings.Contains(got, "Manual body.") {
		t.Fatalf("%v %q", ok, got)
	}
	// Name matching is exact and unknown commands pass through.
	for _, text := range []string{"/skill:DEMO", "/skill:nope", "/other:demo", "skill:demo", ""} {
		if got, ok := ExpandCommand(text, list); ok || got != text {
			t.Fatalf("%q should pass through, got %q ok=%v", text, got, ok)
		}
	}
}
