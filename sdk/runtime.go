package sdk

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/session"
)

// Runtime owns at most one Session at a time: New, NewSession, SwitchSession,
// ResumeSession, Fork and ImportFromJsonl dispose the previous one. Concurrent
// conversations need one Runtime each; Services may be shared and a Runtime is
// not safe for concurrent use.
type Runtime struct {
	Services    *AgentSessionServices
	session     *Session
	last        SessionStartOptions
	approveTool ToolApprovalFunc
}

type RuntimeOptions struct {
	Services *AgentSessionServices
	// ApproveTool applies to sessions created or resumed by this runtime
	// unless SessionStartOptions sets its own.
	ApproveTool ToolApprovalFunc
}

func NewRuntime(opt RuntimeOptions) (*Runtime, error) {
	if opt.Services == nil {
		return nil, fmt.Errorf("runtime: Services required")
	}
	opt.Services.initPluginMaps()
	opt.Services.ensurePlugins()
	opt.Services.pluginRuntime = &Runtime{Services: opt.Services, approveTool: opt.ApproveTool}
	return opt.Services.pluginRuntime, nil
}

func (r *Runtime) Session() *Session { return r.session }

// SettingsManager returns the runtime's settings view.
func (r *Runtime) SettingsManager() *SettingsManager {
	if r.Services == nil {
		return nil
	}
	return r.Services.SettingsManager
}

func (r *Runtime) Close() error {
	if r.session != nil {
		r.session.Dispose()
	}
	return r.Services.Close()
}

type SessionStartOptions struct {
	Model Model
	// ThinkingLevel is the initial thinking level. Empty restores the level a
	// resumed session recorded, or leaves the request without one.
	ThinkingLevel ThinkingLevel
	// CacheWarming keeps a model's declared prompt cache warm: "off",
	// "streaming" (default, while a run is active) or "idle" (also between runs).
	CacheWarming string
	SystemPrompt string
	Tools        []AgentTool
	InMemory     bool
	// ApproveTool is consulted before each tool call. A nil function allows
	// every call, as the SDK does without a host.
	ApproveTool ToolApprovalFunc

	startReason         string
	previousSessionFile string
}

func (r *Runtime) New(opt SessionStartOptions) (*Session, error) {
	opt = r.withSettingsDefaults(opt)
	if opt.startReason == "" {
		opt.startReason = "startup"
	}
	return r.replace(r.newManager(opt), opt)
}

func (r *Runtime) NewSession() (*Session, error) {
	opt := r.switchOptions()
	opt.startReason = "new"
	if r.session != nil {
		opt.previousSessionFile = r.session.manager.File()
		if err := r.beforePluginSession("session_before_switch", map[string]any{"type": "session_before_switch", "reason": "new"}); err != nil {
			return nil, err
		}
	}
	return r.replace(r.newManager(opt), opt)
}

// SwitchSession opens path and carries the current session's model and
// thinking level over, as Pi's switchSession does.
func (r *Runtime) SwitchSession(path string) (*Session, error) {
	mgr, err := session.OpenOrCreate(path, r.Services.CWD)
	if err != nil {
		return nil, err
	}
	r.adoptLegacySessionDir(mgr)
	opt := r.switchOptions()
	// The opened session lives on disk, so later NewSession calls must not
	// inherit an earlier in-memory choice.
	opt.InMemory = false
	opt.startReason = "resume"
	opt.previousSessionFile = path
	if err := r.beforePluginSession("session_before_switch", map[string]any{"type": "session_before_switch", "reason": "resume", "targetSessionFile": path}); err != nil {
		return nil, err
	}
	return r.replace(mgr, opt)
}

// ResumeSession opens path and restores the model and thinking level the
// session recorded, as resuming in Pi does.
func (r *Runtime) ResumeSession(path string) (*Session, error) {
	mgr, err := session.OpenOrCreate(path, r.Services.CWD)
	if err != nil {
		return nil, err
	}
	r.adoptLegacySessionDir(mgr)
	opt := r.last
	opt.InMemory = false
	projection := mgr.Project()
	if projection.ModelProvider != "" && projection.ModelID != "" {
		if model, err := BuildModel(projection.ModelProvider+"/"+projection.ModelID, r.Services); err == nil {
			opt.Model = model
		}
	}
	opt.ThinkingLevel = ThinkingLevel(projection.ThinkingLevel)
	opt = r.withSettingsDefaults(opt)
	opt.startReason = "resume"
	opt.previousSessionFile = path
	if err := r.beforePluginSession("session_before_switch", map[string]any{"type": "session_before_switch", "reason": "resume", "targetSessionFile": path}); err != nil {
		return nil, err
	}
	return r.replace(mgr, opt)
}

func (r *Runtime) Fork() (*Session, error) {
	if r.session == nil {
		return nil, fmt.Errorf("runtime: no session")
	}
	if !r.session.manager.Persisted() {
		return nil, fmt.Errorf("this session has not been saved yet; send a message before forking it")
	}
	if _, err := os.Stat(r.session.manager.File()); err != nil {
		return nil, fmt.Errorf("this session has not been saved yet; send a message before forking it")
	}
	if err := r.beforePluginSession("session_before_fork", map[string]any{"type": "session_before_fork", "entryId": r.session.manager.LeafID(), "position": "at"}); err != nil {
		return nil, err
	}
	child, err := r.session.manager.Fork()
	if err != nil {
		return nil, err
	}
	opt := r.switchOptions()
	opt.startReason = "fork"
	opt.previousSessionFile = r.session.manager.File()
	return r.replace(child, opt)
}

// ImportPi reads a Pi JSONL session, converts it to the nulo format in this
// runtime's session directory, and switches to the imported session. Pi import
// is one-way: nulo does not write Pi-compatible logs.
func (r *Runtime) ImportPi(path string) (*Session, error) {
	mgr, err := session.ImportPi(path, r.sessionDir(), r.Services.CWD)
	if err != nil {
		return nil, err
	}
	return r.ResumeSession(mgr.File())
}

// ImportFromJsonl copies a session log into this runtime's session directory
// (never overwriting an existing file) and switches to it. This is a one-way
// import; nulo does not write Pi-compatible logs.
func (r *Runtime) ImportFromJsonl(path string) (*Session, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, err
	}
	dir := r.sessionDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	destination := filepath.Join(dir, filepath.Base(abs))
	if filepath.Clean(destination) != filepath.Clean(abs) {
		destination = uniqueSessionPath(destination)
		if err := copyFile(abs, destination); err != nil {
			return nil, err
		}
	}
	return r.ResumeSession(destination)
}

// ListSessions returns the sessions stored for this runtime's working
// directory, newest activity first.
func (r *Runtime) ListSessions() ([]SessionInfo, error) {
	seen := map[string]bool{}
	var out []SessionInfo
	for _, dir := range r.sessionDirs() {
		infos, err := session.List(dir)
		if err != nil {
			return nil, err
		}
		for _, info := range infos {
			if seen[info.Path] {
				continue
			}
			seen[info.Path] = true
			out = append(out, info)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Modified.After(out[j].Modified)
	})
	return out, nil
}

// RenameSession records a new name in a stored session.
func (r *Runtime) RenameSession(path, name string) error {
	if err := r.checkSessionPath(path); err != nil {
		return err
	}
	return session.Rename(path, name)
}

// DeleteSession removes a stored session file.
func (r *Runtime) DeleteSession(path string) error {
	if err := r.checkSessionPath(path); err != nil {
		return err
	}
	return session.Delete(path)
}

func (r *Runtime) checkSessionPath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for _, dir := range r.sessionDirs() {
		absRoot, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absRoot, absPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("session path is outside the session directory: %s", path)
}

func uniqueSessionPath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)
	dir := filepath.Dir(path)
	for suffix := 1; ; suffix++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", name, suffix, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(destination)
		return err
	}
	return out.Close()
}

func (r *Runtime) switchOptions() SessionStartOptions {
	opt := r.last
	if r.session != nil {
		opt.Model = r.session.Model()
		opt.ThinkingLevel = r.session.ThinkingLevel()
	}
	return opt
}

func (r *Runtime) newManager(opt SessionStartOptions) *session.Manager {
	inMemory := opt.InMemory || r.Services.InMemory
	if inMemory {
		return session.InMemory(r.Services.CWD)
	}
	return session.Create(r.Services.CWD, r.sessionDir())
}

// sessionDir is settings.sessionDir when set, otherwise the nulo per-project
// directory under the default sessions root.
func (r *Runtime) sessionDir() string {
	if r.Services.SettingsManager != nil {
		if dir, err := r.Services.SettingsManager.GetSessionDir(); err == nil && dir != "" {
			return dir
		}
	}
	return session.SessionDirIn(DefaultSessionDir(r.Services.AgentDir), r.Services.CWD)
}

// legacySessionDir is the old Pi-encoded directory for the same project. It is
// used only to keep old sessions visible; new sessions are never written there.
func (r *Runtime) legacySessionDir() string {
	if r.Services.SettingsManager != nil {
		if dir, err := r.Services.SettingsManager.GetSessionDir(); err == nil && dir != "" {
			return dir
		}
	}
	return session.LegacySessionDirIn(DefaultSessionDir(r.Services.AgentDir), r.Services.CWD)
}

// adoptLegacySessionDir points future forks of an old Pi-layout session at the
// new nulo project directory. The old file itself remains untouched.
func (r *Runtime) adoptLegacySessionDir(mgr *session.Manager) {
	if mgr == nil || mgr.File() == "" {
		return
	}
	legacy := r.legacySessionDir()
	if legacy == "" || legacy == r.sessionDir() {
		return
	}
	if filepath.Dir(mgr.File()) == legacy {
		mgr.SetSessionDir(r.sessionDir())
	}
}

func (r *Runtime) sessionDirs() []string {
	dir := r.sessionDir()
	legacy := r.legacySessionDir()
	if legacy == "" || legacy == dir {
		return []string{dir}
	}
	return []string{dir, legacy}
}

// withSettingsDefaults fills an omitted model and thinking level from
// settings. An explicit option always wins.
func (r *Runtime) withSettingsDefaults(opt SessionStartOptions) SessionStartOptions {
	manager := r.Services.SettingsManager
	if manager == nil {
		if opt.Model.ID == "" {
			opt.Model = DefaultFauxModel()
		}
		return opt
	}
	if opt.Model.ID == "" {
		opt.Model = r.defaultModel(manager)
	}
	if opt.ThinkingLevel == "" {
		opt.ThinkingLevel = ThinkingLevel(manager.GetDefaultThinkingLevel())
	}
	return opt
}

func (r *Runtime) defaultModel(manager *SettingsManager) Model {
	if manager != nil {
		provider, modelID := manager.GetDefaultProvider(), manager.GetDefaultModel()
		if modelID != "" {
			spec := modelID
			if provider != "" {
				spec = provider + "/" + modelID
			}
			if model, err := BuildModel(spec, r.Services); err == nil {
				return model
			}
		}
	}
	return DefaultFauxModel()
}

func (r *Runtime) beforePluginSession(event string, data map[string]any) error {
	if r.session == nil {
		return nil
	}
	result := r.session.emitPlugin(context.Background(), event, data)
	if cancelled, _ := result["cancel"].(bool); cancelled {
		return fmt.Errorf("%s cancelled by plugin", event)
	}
	return nil
}

func (r *Runtime) replace(mgr *session.Manager, opt SessionStartOptions) (*Session, error) {
	if opt.ApproveTool == nil {
		opt.ApproveTool = r.approveTool
	}
	old := r.session
	sess, err := newSession(r.Services, mgr, opt)
	if err != nil {
		return nil, err
	}
	if old != nil {
		old.shutdown(opt.startReason, mgr.File())
		old.Dispose()
	}
	r.session = sess
	r.last = opt
	return sess, nil
}
