package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

var skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func (a *app) skillFolder(s *session, target string) string {
	if target == "project" {
		return filepath.Join(s.CWD, ".nulo", "skills")
	}
	return filepath.Join(a.host.agentDir, "skills")
}

func (a *app) skillFormSession() *session {
	for _, s := range a.sessions {
		if s.ID == a.skillFormSessionID {
			return s
		}
	}
	return nil
}

func (a *app) openSkillForm(s *session) {
	if a == nil || a.host == nil || s == nil {
		return
	}
	a.skillFormTarget = "user"
	if a.host.sessionTrusted(s) {
		a.skillFormTarget = "project"
	}
	a.skillFormEdit = false
	a.skillFormPath = ""
	a.skillFormName = ""
	a.skillFormDescription = ""
	a.skillFormBody = ""
	a.skillFormError = ""
	a.skillFormSessionID = s.ID
	a.skillFormOpen = true
}

func (a *app) skillFormDialog(c *ui.Context) {
	p := a.pal()
	s := a.skillFormSession()
	if s == nil {
		a.skillFormOpen = false
		return
	}
	title, subtitle, actionLabel := "New skill", "Create a SKILL.md with frontmatter and a short instruction body.", "Create skill"
	if a.skillFormEdit {
		title, subtitle, actionLabel = "Edit skill", "Update the description and instruction body. Other frontmatter is preserved.", "Save changes"
	}
	ui.Modal(c, &a.skillFormOpen, func() {
		ui.Text(c, title).FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, subtitle).FontSize(11).TextColor(p.text4)
		ui.Scroll(c).FillWidth().MaxHeight(520).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				if a.skillFormEdit {
					a.formFieldLabel(c, "Skill", "name and path stay unchanged")
					ui.Text(c, a.skillFormName).FontSize(13).TextColor(p.text2)
					ui.Text(c, a.skillFormPath).Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
				} else {
					a.formFieldLabel(c, "Name", "letters, digits, _ and -")
					a.formInput(c, &a.skillFormName, "Skill name", "release-checklist")
				}
				a.formFieldLabel(c, "Description", "when the skill should be loaded")
				a.formInput(c, &a.skillFormDescription, "Description", "Check the release before publishing")
				a.formFieldLabel(c, "Body", "Markdown instructions")
				area := ui.TextAreaBase(c, &a.skillFormBody).FillWidth().Height(150).Padding(6, 9).Radius(6).
					Border(1, p.lineStrong).Background(p.surface2).FontSize(11).TextColor(p.text).
					Label("Skill body")
				_ = area
				if !a.skillFormEdit {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "Save to").FontSize(11).TextColor(p.text4)
						if a.smallButton(c, chooser(a.skillFormTarget == "user", "● User", "User")).Clicked() {
							a.skillFormTarget = "user"
						}
						if a.smallButton(c, chooser(a.skillFormTarget == "project", "● Project", "Project")).
							Disabled(!a.host.sessionTrusted(s)).Clicked() && a.host.sessionTrusted(s) {
							a.skillFormTarget = "project"
						}
					})
				}
				if a.skillFormError != "" {
					ui.Text(c, a.skillFormError).FontSize(11).TextColor(p.bad)
				}
			})
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if a.skillFormEdit {
				if a.smallButton(c, "Raw editor").Clicked() {
					skill := skillInfo{Name: a.skillFormName, Path: a.skillFormPath}
					a.skillFormOpen = false
					a.skillFormEdit = false
					a.editSkill(s, skill)
				}
			}
			ui.Spacer(c)
			if a.button(c, "Cancel").Clicked() {
				a.skillFormOpen = false
				a.skillFormEdit = false
			}
			if a.primaryButton(c, actionLabel, "check").Clicked() {
				a.finishSkillForm()
			}
		})
	})
}

func (a *app) finishSkillForm() {
	if a.skillFormEdit {
		a.finishSkillEdit()
		return
	}
	s := a.skillFormSession()
	if s == nil {
		a.skillFormError = "session is no longer available"
		return
	}
	name := strings.TrimSpace(a.skillFormName)
	if !skillNamePattern.MatchString(name) {
		a.skillFormError = "name must use letters, digits, _ and -"
		return
	}
	description := strings.TrimSpace(a.skillFormDescription)
	if description == "" {
		a.skillFormError = "description is required"
		return
	}
	if len([]rune(description)) > 1024 {
		a.skillFormError = "description is longer than 1024 characters"
		return
	}
	if a.skillFormTarget == "project" && !a.host.sessionTrusted(s) {
		a.skillFormError = "trust this project before adding a project skill"
		return
	}
	for _, skill := range s.SkillAll {
		if skill.Name == name {
			a.skillFormError = "a skill with this name is already loaded"
			return
		}
	}
	if err := a.host.validateConfigReload(a); err != nil {
		a.skillFormError = err.Error()
		return
	}
	root := a.skillFolder(s, a.skillFormTarget)
	dir := filepath.Join(root, name)
	path := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(dir); err == nil {
		a.skillFormError = "skill directory already exists"
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		a.skillFormError = err.Error()
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.skillFormError = err.Error()
		return
	}
	body := strings.TrimSpace(a.skillFormBody)
	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", name, strconv.Quote(description), body)
	if body == "" {
		content = fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n", name, strconv.Quote(description))
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		_ = os.RemoveAll(dir)
		a.skillFormError = err.Error()
		return
	}
	if err := a.reloadConfig("Skills reloaded"); err != nil {
		_ = os.RemoveAll(dir)
		a.skillFormError = "reload failed: " + err.Error()
		return
	}
	a.skillFormOpen = false
	a.refreshCapabilities(s)
	a.pendingToast = "Added skill · " + name
}

func skillRootFor(s *session, skill skillInfo, agentDir string) (string, bool) {
	path := filepath.Clean(skill.Path)
	roots := []string{filepath.Join(agentDir, "skills")}
	if s != nil {
		roots = append(roots, filepath.Join(s.CWD, ".nulo", "skills"))
	}
	for _, root := range roots {
		root = filepath.Clean(root)
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return root, true
		}
	}
	return "", false
}

func (a *app) editSkill(s *session, skill skillInfo) {
	if a == nil || a.host == nil || s == nil || skill.Path == "" {
		return
	}
	a.openConfigEditor("Skill · "+skill.Name, skill.Path, "Edit SKILL.md frontmatter and body. Saving reloads skills for this session.", func() error {
		return a.reloadConfig("Skills reloaded")
	})
}

func (a *app) deleteSkill(s *session, skill skillInfo) error {
	if a == nil || a.host == nil || s == nil || skill.Path == "" {
		return errors.New("skill is not editable")
	}
	root, ok := skillRootFor(s, skill, a.host.agentDir)
	if !ok {
		return errors.New("only user and project skills can be deleted")
	}
	if err := a.host.validateConfigReload(a); err != nil {
		return err
	}
	path := filepath.Clean(skill.Path)
	var err error
	if filepath.Base(path) == "SKILL.md" {
		dir := filepath.Dir(path)
		if dir == filepath.Clean(root) {
			return errors.New("refusing to delete the skills root")
		}
		err = os.RemoveAll(dir)
	} else {
		err = os.Remove(path)
	}
	if err != nil {
		return err
	}
	if err := a.reloadConfig("Skills reloaded"); err != nil {
		return err
	}
	a.refreshCapabilities(s)
	a.pendingToast = "Deleted skill · " + skill.Name
	return nil
}
