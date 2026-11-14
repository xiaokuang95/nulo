package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type skillDocument struct {
	Frontmatter []string
	Body        string
	Name        string
	Description string
	DescLine    int
	Unsupported string
}

func readSkillDocument(path string) (skillDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skillDocument{}, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return skillDocument{}, errors.New("SKILL.md has no frontmatter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return skillDocument{}, errors.New("SKILL.md frontmatter is not closed")
	}
	doc := skillDocument{
		Frontmatter: append([]string(nil), lines[1:end]...),
		Body:        strings.TrimSpace(strings.Join(lines[end+1:], "\n")),
		Name:        filepath.Base(filepath.Dir(path)),
		DescLine:    -1,
	}
	for i, line := range doc.Frontmatter {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "description:") {
			if line != trimmed {
				doc.Unsupported = "indented description"
				continue
			}
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
			if value == "" || strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">") {
				doc.Unsupported = "multiline description"
				continue
			}
			doc.DescLine = i
			doc.Description = unquoteSkillValue(value)
		}
		if strings.HasPrefix(trimmed, "name:") && line == trimmed {
			doc.Name = unquoteSkillValue(strings.TrimSpace(strings.TrimPrefix(trimmed, "name:")))
		}
	}
	if doc.Unsupported != "" {
		return doc, errors.New(doc.Unsupported + " cannot be edited in the form")
	}
	return doc, nil
}

func unquoteSkillValue(value string) string {
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
	}
	return value
}

func renderSkillDocument(doc skillDocument, description, body string) []byte {
	front := append([]string(nil), doc.Frontmatter...)
	line := "description: " + strconv.Quote(strings.TrimSpace(description))
	if doc.DescLine >= 0 && doc.DescLine < len(front) {
		front[doc.DescLine] = line
	} else {
		front = append(front, line)
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(strings.Join(front, "\n"))
	b.WriteString("\n---\n")
	if body = strings.TrimSpace(body); body != "" {
		b.WriteString("\n")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return []byte(b.String())
}

func (a *app) openSkillEditForm(s *session, skill skillInfo) {
	if a == nil || a.host == nil || s == nil || skill.Path == "" {
		return
	}
	doc, err := readSkillDocument(skill.Path)
	if err != nil {
		a.pendingToast = "Skill form: " + err.Error() + " · opened raw editor"
		a.editSkill(s, skill)
		return
	}
	a.skillFormEdit = true
	a.skillFormPath = skill.Path
	a.skillFormSessionID = s.ID
	a.skillFormName = doc.Name
	a.skillFormDescription = doc.Description
	a.skillFormBody = doc.Body
	a.skillFormError = ""
	a.skillFormOpen = true
}

func (a *app) finishSkillEdit() {
	s := a.skillFormSession()
	if s == nil {
		a.skillFormError = "session is no longer available"
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
	doc, err := readSkillDocument(a.skillFormPath)
	if err != nil {
		a.skillFormError = err.Error()
		return
	}
	if err := a.host.validateConfigReload(a); err != nil {
		a.skillFormError = err.Error()
		return
	}
	previous, err := os.ReadFile(a.skillFormPath)
	if err != nil {
		a.skillFormError = err.Error()
		return
	}
	info, _ := os.Stat(a.skillFormPath)
	mode := os.FileMode(0o644)
	if info != nil {
		mode = info.Mode().Perm()
	}
	data := renderSkillDocument(doc, description, a.skillFormBody)
	if err := os.WriteFile(a.skillFormPath, data, mode); err != nil {
		a.skillFormError = err.Error()
		return
	}
	if err := a.reloadConfig("Skills reloaded"); err != nil {
		_ = os.WriteFile(a.skillFormPath, previous, mode)
		_ = a.reloadConfig("Skills reloaded")
		a.skillFormError = "reload failed: " + err.Error()
		return
	}
	a.skillFormOpen = false
	a.skillFormEdit = false
	a.refreshCapabilities(s)
	a.pendingToast = "Saved skill · " + doc.Name
}
