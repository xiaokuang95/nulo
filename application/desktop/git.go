package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (a *app) requestChanges(s *session) {
	if s == nil || !s.Real || s.CWD == "" || a.host == nil {
		return
	}
	h := a.host
	h.mu.Lock()
	if h.changesPending == nil {
		h.changesPending = map[string]bool{}
	}
	if h.changesPending[s.ID] {
		h.mu.Unlock()
		return
	}
	h.changesPending[s.ID] = true
	h.mu.Unlock()
	id := s.ID
	cwd := s.CWD
	load := func() {
		changes, err := gitChanges(cwd)
		h.mu.Lock()
		delete(h.changesPending, id)
		h.mu.Unlock()
		if err != nil {
			return
		}
		apply := func() {
			cur := a.sessionByID(id)
			if cur == nil {
				return
			}
			reviewed := map[string]bool{}
			if cur.Changes != nil {
				for _, file := range cur.Changes.Files {
					reviewed[file.Path] = file.Reviewed
				}
			}
			if changes != nil {
				for i := range changes.Files {
					changes.Files[i].Reviewed = reviewed[changes.Files[i].Path]
				}
			}
			cur.Changes = changes
		}
		if h.sync || a.win == nil {
			apply()
			return
		}
		a.win.Update(apply)
	}
	if h.sync {
		load()
		return
	}
	go load()
}

func gitChanges(cwd string) (*changeSet, error) {
	if cwd == "" || !isGitRepo(cwd) {
		return nil, nil
	}
	raw, err := gitOutput(cwd, "status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, err
	}
	statuses := parseGitStatus(raw)
	if len(statuses) == 0 {
		return nil, nil
	}
	numstat := map[string][2]int{}
	if out, err := gitOutput(cwd, "diff", "HEAD", "--numstat"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			fields := strings.SplitN(line, "\t", 3)
			if len(fields) != 3 {
				continue
			}
			add, _ := strconv.Atoi(fields[0])
			del, _ := strconv.Atoi(fields[1])
			numstat[fields[2]] = [2]int{add, del}
		}
	}
	paths := make([]string, 0, len(statuses))
	for path := range statuses {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	set := &changeSet{}
	for _, path := range paths {
		if len(set.Files) >= 60 {
			break
		}
		status := statuses[path]
		item := changeFile{Path: path, Status: status, Language: languageFor(path)}
		if stat, ok := numstat[path]; ok {
			item.Add, item.Del = stat[0], stat[1]
		} else if status == "A" {
			item.Add = countLines(filepath.Join(cwd, path))
		}
		diff, _ := fileDiff(cwd, path, status)
		item.Diff = diff
		set.Files = append(set.Files, item)
	}
	if len(set.Files) == 0 {
		return nil, nil
	}
	cached, _ := gitOutput(cwd, "diff", "--cached", "--name-only")
	unstaged, _ := gitOutput(cwd, "diff", "--name-only")
	set.Staged = len(bytes.TrimSpace(cached)) > 0 && len(bytes.TrimSpace(unstaged)) == 0
	return set, nil
}

func isGitRepo(cwd string) bool {
	out, err := gitOutput(cwd, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func gitOutput(cwd string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func parseGitStatus(raw []byte) map[string]string {
	parts := bytes.Split(raw, []byte{0})
	out := map[string]string{}
	for i := 0; i < len(parts); i++ {
		entry := string(parts[i])
		if len(entry) < 4 {
			continue
		}
		code := entry[:2]
		path := strings.TrimSpace(entry[3:])
		if path == "" {
			continue
		}
		status := "M"
		switch {
		case code == "??":
			status = "A"
		case strings.Contains(code, "A"):
			status = "A"
		case strings.Contains(code, "D"):
			status = "D"
		case strings.Contains(code, "R"), strings.Contains(code, "C"):
			if i+1 < len(parts) {
				i++
			}
		}
		out[path] = status
	}
	return out
}

func fileDiff(cwd, path, status string) ([]string, error) {
	var out []byte
	var err error
	if status == "A" && !tracked(cwd, path) {
		out, err = gitOutputAllowOne(cwd, "diff", "--no-index", "--", os.DevNull, path)
	} else {
		out, err = gitOutput(cwd, "diff", "HEAD", "--", path)
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	if len(lines) > 400 {
		lines = append(lines[:400], "… diff truncated")
	}
	return lines, nil
}

func tracked(cwd, path string) bool {
	_, err := gitOutput(cwd, "ls-files", "--error-unmatch", "--", path)
	return err == nil
}

func gitOutputAllowOne(cwd string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return out, nil
		}
		return out, fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func countLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return 0
	}
	lines := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}

func languageFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs":
		return "js"
	case ".ts", ".tsx":
		return "ts"
	case ".css":
		return "css"
	case ".html", ".htm":
		return "html"
	case ".json":
		return "json"
	case ".md":
		return "md"
	case ".sh":
		return "sh"
	case ".py":
		return "py"
	case ".rs":
		return "rs"
	default:
		return "text"
	}
}

func (a *app) stageAll(s *session) {
	if s == nil || a.host == nil || !s.Real {
		return
	}
	if _, err := gitOutput(s.CWD, "add", "-A"); err != nil {
		a.pendingToast = "Stage: " + err.Error()
		return
	}
	if s.Changes != nil {
		s.Changes.Staged = true
	}
	a.pendingToast = "Staged all changes"
	a.requestChanges(s)
}

func (a *app) commitChanges(s *session, msg string) error {
	if s == nil || a.host == nil || !s.Real {
		return nil
	}
	if _, err := gitOutput(s.CWD, "commit", "-m", msg); err != nil {
		return err
	}
	s.Notices = append(s.Notices, message{Role: "notice", Time: "now", Text: "Committed · " + msg})
	s.Status = "done"
	if a.host != nil {
		if e := a.host.engine(s); e != nil {
			a.renderSDKMessages(s, e.sess, nil)
		}
	}
	a.requestChanges(s)
	return nil
}
