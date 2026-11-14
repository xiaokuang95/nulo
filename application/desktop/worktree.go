package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/xiaokuang95/nulo/sdk"
)

// worktreeRecord is the desktop host's memory of a session's isolated git
// worktree. It lives beside the agent config because a session's JSONL header
// keeps the repository it started in, not the worktree it may have moved to.
type worktreeRecord struct {
	SessionID string `json:"sessionId"`
	Root      string `json:"root"`
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	Active    bool   `json:"active"`
	Created   string `json:"created,omitempty"`
}

type worktreeIndex struct {
	Worktrees map[string]worktreeRecord `json:"worktrees"`
}

func worktreeIndexPath(agentDir string) string {
	return filepath.Join(agentDir, "desktop-worktrees.json")
}

func loadWorktreeIndex(agentDir string) (map[string]worktreeRecord, error) {
	data, err := os.ReadFile(worktreeIndexPath(agentDir))
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]worktreeRecord), nil
	}
	if err != nil {
		return nil, err
	}
	var index worktreeIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("decode %s: %w", worktreeIndexPath(agentDir), err)
	}
	if index.Worktrees == nil {
		index.Worktrees = make(map[string]worktreeRecord)
	}
	return index.Worktrees, nil
}

func saveWorktreeIndex(agentDir string, records map[string]worktreeRecord) error {
	if agentDir == "" {
		return errors.New("agent directory is required")
	}
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(worktreeIndex{Worktrees: records}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(agentDir, ".desktop-worktrees-*.json")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, worktreeIndexPath(agentDir)); err != nil {
		if runtime.GOOS == "windows" {
			_ = os.Remove(worktreeIndexPath(agentDir))
			return os.Rename(name, worktreeIndexPath(agentDir))
		}
		return err
	}
	return nil
}

func (h *desktopHost) loadWorktrees() error {
	records, err := loadWorktreeIndex(h.agentDir)
	if err != nil {
		return err
	}
	h.worktrees = records
	return nil
}

func (h *desktopHost) saveWorktrees() error {
	return saveWorktreeIndex(h.agentDir, h.worktrees)
}

func (h *desktopHost) worktreeFor(sessionID string) (worktreeRecord, bool) {
	record, ok := h.worktrees[sessionID]
	return record, ok
}

func (h *desktopHost) rememberWorktree(record worktreeRecord) {
	if h.worktrees == nil {
		h.worktrees = make(map[string]worktreeRecord)
	}
	h.worktrees[record.SessionID] = record
}

func (h *desktopHost) forgetWorktree(sessionID string) {
	delete(h.worktrees, sessionID)
}

func worktreeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func shortSessionID(id string) string {
	id = worktreeSlug(id)
	if len(id) > 10 {
		id = id[:10]
	}
	if id == "" {
		id = "session"
	}
	return id
}

func (h *desktopHost) worktreePath(root, sessionID string) string {
	repo := worktreeSlug(filepath.Base(root))
	if repo == "" {
		repo = "repo"
	}
	return filepath.Join(h.agentDir, "worktrees", repo+"-"+shortSessionID(sessionID))
}

func (h *desktopHost) uniqueWorktreeBranch(root, sessionID string) string {
	base := "nulo/wt-" + shortSessionID(sessionID)
	for n := 1; ; n++ {
		branch := base
		if n > 1 {
			branch = fmt.Sprintf("%s-%d", base, n)
		}
		cmd := exec.Command("git", "-C", root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
		if err := cmd.Run(); err != nil {
			return branch
		}
	}
}

// ensureWorktree reuses the session's existing worktree or creates one from
// the repository's current HEAD.
func (h *desktopHost) ensureWorktree(s *session) (worktreeRecord, error) {
	if s == nil {
		return worktreeRecord{}, errors.New("no session")
	}
	if record, ok := h.worktreeFor(s.ID); ok && record.Path != "" {
		if info, err := os.Stat(record.Path); err == nil && info.IsDir() {
			return record, nil
		}
		h.forgetWorktree(s.ID)
	}
	root := strings.TrimSpace(s.BaseCWD)
	if root == "" {
		root = strings.TrimSpace(s.CWD)
	}
	if root == "" {
		return worktreeRecord{}, errors.New("session has no workspace")
	}
	repoRoot := gitRoot(root)
	if repoRoot == "" {
		return worktreeRecord{}, fmt.Errorf("%s is not a git repository", root)
	}
	path := h.worktreePath(root, s.ID)
	if _, err := os.Stat(path); err == nil {
		return worktreeRecord{}, fmt.Errorf("worktree path already exists: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return worktreeRecord{}, err
	}
	branch := h.uniqueWorktreeBranch(repoRoot, s.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "worktree", "add", "-b", branch, path, "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return worktreeRecord{}, fmt.Errorf("git worktree add: %s", message)
	}
	record := worktreeRecord{
		SessionID: s.ID,
		Root:      root,
		Path:      path,
		Branch:    branch,
		Active:    false,
		Created:   time.Now().UTC().Format(time.RFC3339),
	}
	h.rememberWorktree(record)
	if err := h.saveWorktrees(); err != nil {
		return worktreeRecord{}, fmt.Errorf("save worktree state: %w", err)
	}
	return record, nil
}

func (h *desktopHost) mergeWorktree(a *app, s *session) error {
	if s == nil {
		return errors.New("no session")
	}
	record, ok := h.worktreeFor(s.ID)
	if !ok || record.Path == "" {
		return errors.New("session has no worktree")
	}
	if s.Status == "running" || s.Status == "awaiting" {
		return errors.New("stop the current turn first")
	}
	root := record.Root
	if root == "" {
		root = s.BaseCWD
	}
	if root == "" {
		return errors.New("worktree has no repository root")
	}
	if out, err := gitOutput(root, "status", "--porcelain"); err != nil {
		return err
	} else if strings.TrimSpace(string(out)) != "" {
		return errors.New("local repository has uncommitted changes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "merge", "--no-ff", record.Branch, "-m", "Merge "+record.Branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if files := mergeConflictFiles(root); len(files) > 0 {
			if a != nil {
				a.openMergeConflicts(s, root, record.Branch)
			}
			return fmt.Errorf("merge stopped with %d conflict(s)", len(files))
		}
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("git merge: %s", message)
	}
	if a != nil {
		a.addHostNotice(s, "Merged "+record.Branch+" into "+gitBranch(root))
		a.pendingToast = "Merged worktree into " + gitBranch(root)
		a.requestChanges(s)
	}
	return nil
}

func (h *desktopHost) removeWorktree(a *app, s *session) error {
	if s == nil {
		return errors.New("no session")
	}
	record, ok := h.worktreeFor(s.ID)
	if !ok {
		return errors.New("session has no worktree")
	}
	if s.Status == "running" || s.Status == "awaiting" {
		return errors.New("stop the current turn first")
	}
	if s.Env == "Worktree" {
		if err := h.switchEnvironment(a, s, "Local"); err != nil {
			return err
		}
	}
	root := record.Root
	if root == "" {
		root = s.BaseCWD
	}
	if root == "" {
		return errors.New("worktree has no repository root")
	}
	if record.Path != "" && fileExists(record.Path) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", "-C", root, "worktree", "remove", "--force", record.Path)
		if out, err := cmd.CombinedOutput(); err != nil {
			message := strings.TrimSpace(string(out))
			if message == "" {
				message = err.Error()
			}
			return fmt.Errorf("git worktree remove: %s", message)
		}
	}
	if record.Branch != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", "-C", root, "branch", "-D", record.Branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			message := strings.TrimSpace(string(out))
			if message == "" {
				message = err.Error()
			}
			return fmt.Errorf("git branch -D: %s", message)
		}
	}
	h.forgetWorktree(s.ID)
	if err := h.saveWorktrees(); err != nil {
		return fmt.Errorf("save worktree state: %w", err)
	}
	if a != nil {
		a.addHostNotice(s, "Removed worktree "+record.Path)
		a.pendingToast = "Worktree removed"
		a.requestChanges(s)
	}
	return nil
}

func (h *desktopHost) dropEngine(s *session) {
	e := h.engine(s)
	if e == nil {
		return
	}
	if e.unsub != nil {
		e.unsub()
		e.unsub = nil
	}
	if e.sess != nil {
		e.sess.Dispose()
	}
	h.mu.Lock()
	delete(h.engines, e.id)
	h.mu.Unlock()
}

func (h *desktopHost) newEngineFor(cwd, modelSpec, thinking string) (*sessionEngine, error) {
	if cwd == "" {
		return nil, errors.New("session has no workspace")
	}
	svcs, err := h.servicesFor(cwd)
	if err != nil {
		return nil, err
	}
	rt, err := sdk.NewRuntime(sdk.RuntimeOptions{Services: svcs})
	if err != nil {
		return nil, err
	}
	model, err := sdk.BuildModel(modelSpec, svcs)
	if err != nil || model.ID == "" {
		model = sdk.DefaultFauxModel()
	}
	level := thinking
	if level == "" {
		level = h.thinking
	}
	var sid string
	sess, err := rt.New(sdk.SessionStartOptions{
		Model: model, ThinkingLevel: sdk.ThinkingLevel(level),
		ApproveTool: h.approvalFunc(&sid),
	})
	if err != nil {
		return nil, err
	}
	sid = sess.ID()
	return &sessionEngine{id: sid, path: sess.File(), rt: rt, sess: sess}, nil
}

// switchEnvironment moves a live session between its repository and an
// isolated worktree. Messages and the session file stay in place; the SDK
// runtime is rebuilt with the other working directory.
func (h *desktopHost) switchEnvironment(a *app, s *session, env string) error {
	if s == nil || !s.Real {
		return errors.New("session is not backed by the SDK")
	}
	if s.Status == "running" || s.Status == "awaiting" {
		return errors.New("stop the current turn before switching environment")
	}
	env = strings.TrimSpace(env)
	if env != "Worktree" {
		env = "Local"
	}
	if env == "Local" && s.Env == "Worktree" {
		if record, ok := h.worktreeFor(s.ID); ok && mergeInProgress(record.Root) {
			return errors.New("resolve or abort the merge before switching to Local")
		}
	}
	if s.Env == env {
		if env != "Worktree" {
			return nil
		}
		if info, err := os.Stat(s.CWD); err == nil && info.IsDir() {
			return nil
		}
	}
	base := strings.TrimSpace(s.BaseCWD)
	if base == "" {
		base = strings.TrimSpace(s.CWD)
	}
	targetCWD, targetBranch, targetName := base, gitBranch(base), ""
	var record worktreeRecord
	if env == "Worktree" {
		created, err := h.ensureWorktree(s)
		if err != nil {
			return err
		}
		record = created
		targetCWD = created.Path
		targetBranch = created.Branch
		targetName = filepath.Base(created.Path)
	}
	if targetCWD == "" {
		return errors.New("no workspace to switch to")
	}
	if info, err := os.Stat(targetCWD); err != nil || !info.IsDir() {
		return fmt.Errorf("workspace is unavailable: %s", targetCWD)
	}

	oldCWD, oldEnv, oldWorktree, oldBranch, oldBase, oldOrigin := s.CWD, s.Env, s.Worktree, s.Branch, s.BaseCWD, s.OriginCWD
	oldID, oldPath := s.ID, s.Path
	e := h.engine(s)
	hadEngine := e != nil
	unsaved := hadEngine && e.sess != nil && countRealMessages(e.sess.Messages()) == 0 && !fileExists(oldPath)
	if hadEngine {
		h.dropEngine(s)
	}

	s.CWD = targetCWD
	s.BaseCWD = base
	s.Env = env
	s.Worktree = targetName
	s.Branch = targetBranch

	newEngine := (*sessionEngine)(nil)
	var switchErr error
	switch {
	case unsaved:
		newEngine, switchErr = h.newEngineFor(targetCWD, s.Model, s.Thinking)
		if switchErr == nil {
			s.ID = newEngine.id
			s.Path = newEngine.path
			s.OriginCWD = targetCWD
			h.addEngine(a, s, newEngine)
		}
	case hadEngine:
		newEngine, switchErr = h.ensureEngine(a, s)
	case !fileExists(s.Path):
		newEngine, switchErr = h.newEngineFor(targetCWD, s.Model, s.Thinking)
		if switchErr == nil {
			s.ID = newEngine.id
			s.Path = newEngine.path
			s.OriginCWD = targetCWD
			h.addEngine(a, s, newEngine)
		}
	}
	if switchErr != nil {
		s.CWD, s.Env, s.Worktree, s.Branch, s.BaseCWD, s.OriginCWD = oldCWD, oldEnv, oldWorktree, oldBranch, oldBase, oldOrigin
		s.ID, s.Path = oldID, oldPath
		if hadEngine {
			if fileExists(oldPath) {
				_, _ = h.ensureEngine(a, s)
			} else if restored, err := h.newEngineFor(oldCWD, s.Model, s.Thinking); err == nil {
				s.ID, s.Path = restored.id, restored.path
				h.addEngine(a, s, restored)
				if a != nil && a.activeID == oldID {
					a.activeID = s.ID
				}
			}
		}
		if env == "Worktree" && record.Path != "" {
			if oldID != s.ID {
				delete(h.worktrees, oldID)
			}
			record.SessionID = s.ID
			record.Active = false
			h.rememberWorktree(record)
			_ = h.saveWorktrees()
		}
		return fmt.Errorf("switch environment: %w", switchErr)
	}
	if newEngine != nil {
		s.ID = newEngine.id
		s.Path = newEngine.path
		if a != nil {
			a.syncSessionMeta(s, newEngine.sess)
			a.updateRealUsage(s, newEngine.sess)
		}
	}
	s.Updated = "now"
	s.Modified = time.Now().UnixMilli()

	if env == "Worktree" {
		if oldID != s.ID {
			delete(h.worktrees, oldID)
		}
		record.SessionID = s.ID
		record.Active = true
		h.rememberWorktree(record)
	} else {
		if existing, ok := h.worktreeFor(oldID); ok {
			existing.Active = false
			if oldID != s.ID {
				delete(h.worktrees, oldID)
			}
			existing.SessionID = s.ID
			h.worktrees[s.ID] = existing
		}
		if oldID != s.ID {
			if existing, ok := h.worktreeFor(s.ID); ok {
				existing.Active = false
				h.worktrees[s.ID] = existing
			}
		}
	}
	if err := h.saveWorktrees(); err != nil && a != nil {
		a.addHostNotice(s, "Could not save worktree state: "+err.Error())
	}
	if a != nil && s.ID != oldID {
		if a.activeID == oldID {
			a.activeID = s.ID
		}
	}
	if a != nil {
		if env == "Worktree" {
			a.addHostNotice(s, "Worktree ready: "+record.Path)
		} else {
			a.addHostNotice(s, "Switched back to the repository: "+base)
		}
	}
	return nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func (a *app) switchEnvironment(s *session, env string) {
	if s == nil {
		return
	}
	if a.host == nil || !s.Real {
		if env == "Worktree" {
			a.pendingToast = "Worktree needs a real SDK session"
			return
		}
		s.Env = "Local"
		s.Worktree = ""
		a.pendingToast = "Environment: Local"
		return
	}
	if err := a.host.switchEnvironment(a, s, env); err != nil {
		a.pendingToast = "Environment: " + err.Error()
		return
	}
	a.focusTerminalID = s.ID
	a.scrollToEnd = true
	if env == "Worktree" {
		a.pendingToast = "Worktree: " + s.Worktree
	} else {
		a.pendingToast = "Environment: Local"
	}
}
