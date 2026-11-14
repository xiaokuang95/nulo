package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiaokuang95/nulo/sdk"
)

const (
	maxCheckpointBytes = 16 << 20
	maxCheckpointFiles = 256
)

type checkpointFile struct {
	Path       string `json:"path"`
	Existed    bool   `json:"existed"`
	Data       string `json:"data,omitempty"`
	Mode       uint32 `json:"mode,omitempty"`
	Symlink    bool   `json:"symlink,omitempty"`
	LinkTarget string `json:"linkTarget,omitempty"`
}

type checkpointManifest struct {
	Key         string           `json:"key"`
	Workspace   string           `json:"workspace,omitempty"`
	Head        string           `json:"head,omitempty"`
	CreatedAt   int64            `json:"createdAt"`
	Files       []checkpointFile `json:"files,omitempty"`
	Unavailable string           `json:"unavailable,omitempty"`
}

func checkpointRoot(agentDir, sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return filepath.Join(agentDir, "desktop-checkpoints", hex.EncodeToString(sum[:8]))
}

func checkpointKey(key string) string {
	if strings.TrimSpace(key) == "" {
		return "root"
	}
	return key
}

func checkpointDir(agentDir, sessionID, key string) string {
	sum := sha256.Sum256([]byte(checkpointKey(key)))
	return filepath.Join(checkpointRoot(agentDir, sessionID), hex.EncodeToString(sum[:12]))
}

func loadCheckpointManifest(agentDir, sessionID, key string) (*checkpointManifest, error) {
	data, err := os.ReadFile(filepath.Join(checkpointDir(agentDir, sessionID, key), "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var manifest checkpointManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode checkpoint: %w", err)
	}
	return &manifest, nil
}

func saveCheckpointManifest(agentDir, sessionID, key string, manifest checkpointManifest) error {
	dir := checkpointDir(agentDir, sessionID, key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600)
}

func (h *desktopHost) captureCheckpoint(s *session, key string) error {
	if h == nil || s == nil || !s.Real || s.CWD == "" {
		return nil
	}
	key = checkpointKey(key)
	if manifest, err := loadCheckpointManifest(h.agentDir, s.ID, key); err == nil && manifest != nil {
		return nil
	}
	if !isGitRepo(s.CWD) {
		return saveCheckpointManifest(h.agentDir, s.ID, key, checkpointManifest{
			Key: key, CreatedAt: time.Now().UnixMilli(), Unavailable: "workspace is not a git repository",
		})
	}
	head, _ := gitOutput(s.CWD, "rev-parse", "--verify", "HEAD")
	paths, err := checkpointPaths(s.CWD)
	if h.agentDir != "" && checkpointInside(s.CWD, h.agentDir) {
		filtered := paths[:0]
		for _, path := range paths {
			full, ok := checkpointPath(s.CWD, path)
			if ok && checkpointInside(h.agentDir, full) {
				continue
			}
			filtered = append(filtered, path)
		}
		paths = filtered
	}
	if err != nil {
		return saveCheckpointManifest(h.agentDir, s.ID, key, checkpointManifest{
			Key: key, Head: strings.TrimSpace(string(head)), CreatedAt: time.Now().UnixMilli(),
			Unavailable: "could not read git status: " + err.Error(),
		})
	}
	manifest := checkpointManifest{Key: key, Workspace: checkpointRealPath(s.CWD), Head: strings.TrimSpace(string(head)), CreatedAt: time.Now().UnixMilli()}
	if len(paths) > maxCheckpointFiles {
		manifest.Unavailable = fmt.Sprintf("%d changed files exceed the snapshot limit of %d", len(paths), maxCheckpointFiles)
		return saveCheckpointManifest(h.agentDir, s.ID, key, manifest)
	}
	dir := checkpointDir(h.agentDir, s.ID, key)
	filesDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return err
	}
	var total int64
	for i, path := range paths {
		full, ok := checkpointPath(s.CWD, path)
		if !ok {
			continue
		}
		entry := checkpointFile{Path: path}
		info, statErr := os.Lstat(full)
		switch {
		case errors.Is(statErr, os.ErrNotExist):
			manifest.Files = append(manifest.Files, entry)
			continue
		case statErr != nil:
			manifest.Unavailable = "could not stat " + path + ": " + statErr.Error()
			return saveCheckpointManifest(h.agentDir, s.ID, key, manifest)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(full)
			if err != nil {
				manifest.Unavailable = "could not read symlink " + path + ": " + err.Error()
				return saveCheckpointManifest(h.agentDir, s.ID, key, manifest)
			}
			entry.Existed = true
			entry.Symlink = true
			entry.LinkTarget = target
		case info.Mode().IsRegular():
			total += info.Size()
			if total > maxCheckpointBytes {
				manifest.Unavailable = fmt.Sprintf("changed files exceed the %s snapshot limit", formatBytesShort(maxCheckpointBytes))
				return saveCheckpointManifest(h.agentDir, s.ID, key, manifest)
			}
			data, err := os.ReadFile(full)
			if err != nil {
				manifest.Unavailable = "could not read " + path + ": " + err.Error()
				return saveCheckpointManifest(h.agentDir, s.ID, key, manifest)
			}
			name := fmt.Sprintf("%04d", i)
			if err := os.WriteFile(filepath.Join(filesDir, name), data, 0o600); err != nil {
				return err
			}
			entry.Existed = true
			entry.Mode = uint32(info.Mode().Perm())
			entry.Data = name
		default:
			manifest.Unavailable = "unsupported file type: " + path
			return saveCheckpointManifest(h.agentDir, s.ID, key, manifest)
		}
		manifest.Files = append(manifest.Files, entry)
	}
	return saveCheckpointManifest(h.agentDir, s.ID, key, manifest)
}

func checkpointPaths(cwd string) ([]string, error) {
	rootOut, err := gitOutput(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	root := checkpointRealPath(strings.TrimSpace(string(rootOut)))
	relCwd, _ := filepath.Rel(root, checkpointRealPath(cwd))
	if relCwd == "." {
		relCwd = ""
	}
	prefix := ""
	if relCwd != "" {
		prefix = filepath.ToSlash(relCwd) + "/"
	}
	seen := map[string]bool{}
	add := func(raw []byte) {
		for _, item := range strings.Split(string(raw), "\x00") {
			if item == "" {
				continue
			}
			if prefix != "" {
				if !strings.HasPrefix(item, prefix) {
					continue
				}
				item = strings.TrimPrefix(item, prefix)
			}
			seen[item] = true
		}
	}
	if _, err := gitOutput(root, "rev-parse", "--verify", "HEAD"); err == nil {
		out, err := gitOutput(root, "diff", "--name-only", "-z", "HEAD")
		if err != nil {
			return nil, err
		}
		add(out)
	} else {
		out, err := gitOutput(root, "ls-files", "-z")
		if err != nil {
			return nil, err
		}
		add(out)
	}
	out, err := gitOutput(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	add(out)
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sortStrings(paths)
	return paths, nil
}

func checkpointInside(base, path string) bool {
	rel, err := filepath.Rel(checkpointRealPath(base), checkpointRealPath(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func checkpointRealPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

func checkpointPath(cwd, rel string) (string, bool) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	full := filepath.Join(cwd, clean)
	inside, err := filepath.Rel(cwd, full)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}

func restoreCheckpoint(agentDir string, s *session, key string) (int, error) {
	if s == nil || s.CWD == "" {
		return 0, errors.New("session has no workspace")
	}
	manifest, err := loadCheckpointManifest(agentDir, s.ID, checkpointKey(key))
	if err != nil {
		return 0, err
	}
	if manifest == nil {
		return 0, errors.New("no file snapshot for this checkpoint")
	}
	if manifest.Unavailable != "" {
		return 0, errors.New(manifest.Unavailable)
	}
	if manifest.Workspace != "" && manifest.Workspace != checkpointRealPath(s.CWD) {
		return 0, errors.New("this checkpoint belongs to another workspace")
	}
	if isGitRepo(s.CWD) {
		head, _ := gitOutput(s.CWD, "rev-parse", "--verify", "HEAD")
		currentHead := strings.TrimSpace(string(head))
		if currentHead != manifest.Head {
			return 0, errors.New("HEAD moved since this checkpoint; restore files manually")
		}
	}
	dir := checkpointDir(agentDir, s.ID, checkpointKey(key))
	restored := 0
	for _, file := range manifest.Files {
		full, ok := checkpointPath(s.CWD, file.Path)
		if !ok {
			continue
		}
		if !file.Existed {
			if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
				return restored, err
			}
			restored++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return restored, err
		}
		_ = os.Remove(full)
		if file.Symlink {
			if err := os.Symlink(file.LinkTarget, full); err != nil {
				return restored, err
			}
			restored++
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "files", file.Data))
		if err != nil {
			return restored, err
		}
		mode := os.FileMode(file.Mode & 0o777)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(full, data, mode); err != nil {
			return restored, err
		}
		_ = os.Chmod(full, mode)
		restored++
	}
	return restored, nil
}

func copyCheckpointSnapshots(agentDir, fromSession, toSession string) error {
	if agentDir == "" || fromSession == "" || toSession == "" || fromSession == toSession {
		return nil
	}
	src := checkpointRoot(agentDir, fromSession)
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	dst := checkpointRoot(agentDir, toSession)
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
}

func deleteCheckpointSnapshots(agentDir, sessionID string) error {
	if agentDir == "" || sessionID == "" {
		return nil
	}
	return os.RemoveAll(checkpointRoot(agentDir, sessionID))
}

func findCheckpointSnapshot(agentDir, sessionID string, branch []sdk.SessionEntry, target string) (key string, files int, note string, ok bool) {
	parents := map[string]string{}
	roles := map[string]string{}
	for _, entry := range branch {
		parent := ""
		if entry.ParentID != nil {
			parent = *entry.ParentID
		}
		parents[entry.ID] = parent
		if entry.Message != nil {
			roles[entry.ID] = entry.Message.Role
		}
	}
	current := target
	for {
		files, note, ok = checkpointInfo(agentDir, sessionID, current)
		if ok || note != "" {
			return current, files, note, ok
		}
		if roles[current] != "system" {
			return "", 0, "", false
		}
		parent, exists := parents[current]
		if !exists || parent == current {
			return "", 0, "", false
		}
		current = parent
	}
}

func checkpointInfo(agentDir, sessionID, key string) (files int, note string, ok bool) {
	manifest, err := loadCheckpointManifest(agentDir, sessionID, key)
	if err != nil || manifest == nil {
		return 0, "", false
	}
	if manifest.Unavailable != "" {
		return 0, manifest.Unavailable, false
	}
	return len(manifest.Files), "", true
}

func formatBytesShort(n int64) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	if n >= 1<<10 {
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}
