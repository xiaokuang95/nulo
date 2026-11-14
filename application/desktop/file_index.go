package main

import (
	"context"
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	fileIndexTTL   = 5 * time.Second
	fileIndexLimit = 8000
)

type workspaceFileIndex struct {
	files  []string
	loaded time.Time
}

func (a *app) workspaceFiles(cwd string) []string {
	if cwd == "" {
		return nil
	}
	cwd = filepath.Clean(cwd)
	if cached, ok := a.fileIndices[cwd]; ok && time.Since(cached.loaded) < fileIndexTTL {
		return cached.files
	}
	files := scanWorkspaceFiles(cwd)
	if a.fileIndices == nil {
		a.fileIndices = make(map[string]workspaceFileIndex)
	}
	a.fileIndices[cwd] = workspaceFileIndex{files: files, loaded: time.Now()}
	return files
}

func scanWorkspaceFiles(cwd string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", cwd, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err == nil {
		parts := strings.Split(string(out), "\x00")
		files := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(filepath.ToSlash(part))
			if part != "" {
				files = append(files, part)
			}
			if len(files) >= fileIndexLimit {
				break
			}
		}
		sort.Strings(files)
		return files
	}
	return walkWorkspaceFiles(cwd)
}

func walkWorkspaceFiles(cwd string) []string {
	skip := map[string]bool{
		".git": true, ".hg": true, ".svn": true, "node_modules": true,
		"vendor": true, "dist": true, "build": true, "target": true,
		".next": true, ".cache": true, "Pods": true,
	}
	var files []string
	_ = filepath.WalkDir(cwd, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == cwd {
			return nil
		}
		if entry.IsDir() {
			if skip[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(cwd, path)
		if relErr == nil && rel != "" {
			files = append(files, filepath.ToSlash(rel))
		}
		if len(files) >= fileIndexLimit {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func (a *app) filteredMentions(cwd string) []string {
	q, _ := a.mentionQuery()
	q = strings.ToLower(strings.TrimSpace(q))
	files := a.workspaceFiles(cwd)
	if q == "" {
		if len(files) > 80 {
			return files[:80]
		}
		return files
	}
	prefix := make([]string, 0, 32)
	contains := make([]string, 0, 32)
	for _, file := range files {
		lower := strings.ToLower(file)
		switch {
		case strings.HasPrefix(filepath.Base(lower), q):
			prefix = append(prefix, file)
		case strings.HasPrefix(lower, q):
			prefix = append(prefix, file)
		case strings.Contains(lower, q):
			contains = append(contains, file)
		}
		if len(prefix) >= 80 {
			break
		}
	}
	out := append(prefix, contains...)
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}
