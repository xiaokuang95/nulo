package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// desktopPreferences holds desktop-only choices that the SDK settings file does
// not model. Missing booleans keep the defaults set before decoding.
type desktopPreferences struct {
	DefaultApproval  string   `json:"defaultApproval,omitempty"`
	ShowTimestamps   bool     `json:"showTimestamps"`
	CollapseThinking bool     `json:"collapseThinking"`
	CompactDensity   bool     `json:"compactDensity,omitempty"`
	Notifications    bool     `json:"notifications"`
	TrustedProjects  []string `json:"trustedProjects,omitempty"`
	DisabledSkills   []string `json:"disabledSkills,omitempty"`
}

func defaultDesktopPreferences() desktopPreferences {
	return desktopPreferences{
		DefaultApproval:  "auto-edits",
		ShowTimestamps:   true,
		CollapseThinking: true,
		Notifications:    true,
	}
}

func desktopPreferencesPath(agentDir string) string {
	return filepath.Join(agentDir, "desktop-settings.json")
}

func loadDesktopPreferences(agentDir string) (desktopPreferences, error) {
	prefs := defaultDesktopPreferences()
	data, err := os.ReadFile(desktopPreferencesPath(agentDir))
	if errors.Is(err, os.ErrNotExist) {
		return prefs, nil
	}
	if err != nil {
		return prefs, err
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return defaultDesktopPreferences(), fmt.Errorf("decode %s: %w", desktopPreferencesPath(agentDir), err)
	}
	if prefs.DefaultApproval == "" {
		prefs.DefaultApproval = "auto-edits"
	}
	return prefs, nil
}

func saveDesktopPreferences(agentDir string, prefs desktopPreferences) error {
	if agentDir == "" {
		return errors.New("agent directory is required")
	}
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(agentDir, ".desktop-settings-*.json")
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
	if err := os.Rename(name, desktopPreferencesPath(agentDir)); err != nil {
		if runtime.GOOS == "windows" {
			_ = os.Remove(desktopPreferencesPath(agentDir))
			return os.Rename(name, desktopPreferencesPath(agentDir))
		}
		return err
	}
	return nil
}

func (h *desktopHost) loadPreferences(app *app) error {
	prefs, err := loadDesktopPreferences(h.agentDir)
	h.defaultApproval = prefs.DefaultApproval
	h.trustedProjects = make(map[string]bool, len(prefs.TrustedProjects))
	for _, root := range prefs.TrustedProjects {
		if root != "" {
			h.trustedProjects[root] = true
		}
	}
	h.disabledSkills = make(map[string]bool, len(prefs.DisabledSkills))
	for _, name := range prefs.DisabledSkills {
		if name != "" {
			h.disabledSkills[name] = true
		}
	}
	if app != nil {
		app.showTimestamps = prefs.ShowTimestamps
		app.collapseThinking = prefs.CollapseThinking
		app.compactDensity = prefs.CompactDensity
		app.notifyDone = prefs.Notifications
	}
	return err
}

func (h *desktopHost) savePreferences(app *app) error {
	prefs := defaultDesktopPreferences()
	if h.defaultApproval != "" {
		prefs.DefaultApproval = h.defaultApproval
	}
	if app != nil {
		prefs.ShowTimestamps = app.showTimestamps
		prefs.CollapseThinking = app.collapseThinking
		prefs.CompactDensity = app.compactDensity
		prefs.Notifications = app.notifyDone
	}
	prefs.TrustedProjects = make([]string, 0, len(h.trustedProjects))
	for root, trusted := range h.trustedProjects {
		if trusted && root != "" {
			prefs.TrustedProjects = append(prefs.TrustedProjects, root)
		}
	}
	sortStrings(prefs.TrustedProjects)
	prefs.DisabledSkills = make([]string, 0, len(h.disabledSkills))
	for name, disabled := range h.disabledSkills {
		if disabled && name != "" {
			prefs.DisabledSkills = append(prefs.DisabledSkills, name)
		}
	}
	sortStrings(prefs.DisabledSkills)
	return saveDesktopPreferences(h.agentDir, prefs)
}

func (h *desktopHost) projectTrusted(cwd string) bool {
	if h.trusted {
		return true
	}
	root := projectTrustRoot(cwd)
	return root != "" && h.trustedProjects[root]
}

func projectTrustRoot(cwd string) string {
	if cwd == "" {
		return ""
	}
	if out, err := gitOutput(cwd, "rev-parse", "--git-common-dir"); err == nil {
		common := strings.TrimSpace(string(out))
		if common != "" {
			if !filepath.IsAbs(common) {
				common = filepath.Join(cwd, common)
			}
			common = filepath.Clean(common)
			if filepath.Base(common) == ".git" {
				return filepath.Dir(common)
			}
		}
	}
	if root := gitRoot(cwd); root != "" {
		return root
	}
	return filepath.Clean(cwd)
}

func (h *desktopHost) sessionTrusted(s *session) bool {
	if h == nil || s == nil {
		return false
	}
	if h.trusted {
		return true
	}
	root := s.TrustRoot
	if root == "" {
		root = projectTrustRoot(s.BaseCWD)
	}
	return root != "" && h.trustedProjects[root]
}

func (a *app) setProjectTrusted(s *session, trusted bool) {
	if a.host == nil || s == nil {
		return
	}
	if s.Status == "running" || s.Status == "awaiting" {
		a.pendingToast = "Stop the current turn before changing project trust"
		return
	}
	root := s.TrustRoot
	if root == "" {
		root = projectTrustRoot(s.BaseCWD)
	}
	if root == "" {
		return
	}
	if err := a.host.setProjectTrusted(a, root, trusted); err != nil {
		a.pendingToast = "Project trust: " + err.Error()
		return
	}
	if trusted {
		a.pendingToast = "Trusted project · " + root
	} else {
		a.pendingToast = "Project trust removed · " + root
	}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func (a *app) anyOverlayOpen() bool {
	return a.paletteOpen || a.settingsOpen || a.keyOpen || a.commitOpen ||
		a.renameOpen || a.deleteOpen || a.confirmOpen || a.imageOpen ||
		a.configOpen || a.mcpFormOpen || a.skillFormOpen || a.providerFormOpen ||
		a.modelFormOpen || a.mergeConflictOpen || a.showPerm || a.showEnv ||
		a.showModel || a.slashOpen || a.mentionOpen
}

func (a *app) confirmDestructive(title, detail, label string, action func()) {
	a.confirmTitle = title
	a.confirmDetail = detail
	a.confirmLabel = label
	a.confirmAction = action
	a.confirmOpen = true
}

func (a *app) setDefaultApproval(s *session, mode string) {
	if s != nil {
		s.Approval = mode
	}
	if a.host != nil {
		a.host.defaultApproval = mode
		if err := a.host.savePreferences(a); err != nil {
			a.pendingToast = "Desktop settings: " + err.Error()
		}
	}
}

func (a *app) setDefaultThinking(s *session, level string) {
	if s == nil {
		return
	}
	if a.host != nil && s.Real {
		if err := a.host.setDefaultThinking(a, s, level); err != nil {
			a.pendingToast = "Thinking: " + err.Error()
		}
		return
	}
	s.Thinking = level
}

func (a *app) toggleShowTimestamps() {
	a.showTimestamps = !a.showTimestamps
	if a.host != nil {
		if err := a.host.savePreferences(a); err != nil {
			a.pendingToast = "Desktop settings: " + err.Error()
		}
	}
}

func (a *app) thinkingLevelOptions(s *session) []string {
	if a.host != nil && s != nil && s.Real {
		if levels := a.host.thinkingLevels(s); len(levels) > 0 {
			return levels
		}
	}
	return []string{"low", "medium", "high"}
}

func (a *app) toggleCompactDensity() {
	a.compactDensity = !a.compactDensity
	if a.host != nil {
		if err := a.host.savePreferences(a); err != nil {
			a.pendingToast = "Desktop settings: " + err.Error()
		}
	}
}

func (a *app) toggleNotifications() {
	a.notifyDone = !a.notifyDone
	if a.host != nil {
		if err := a.host.savePreferences(a); err != nil {
			a.pendingToast = "Desktop settings: " + err.Error()
		}
	}
}

func (a *app) toggleCollapseThinking() {
	a.collapseThinking = !a.collapseThinking
	if a.host != nil {
		if err := a.host.savePreferences(a); err != nil {
			a.pendingToast = "Desktop settings: " + err.Error()
		}
	}
}

func (h *desktopHost) setProjectTrusted(app *app, root string, trusted bool) error {
	if root == "" {
		return errors.New("project root is required")
	}
	if !h.trusted && app != nil {
		for _, s := range app.sessions {
			if s.TrustRoot != root {
				continue
			}
			if s.Status == "running" || s.Status == "awaiting" {
				return errors.New("stop running sessions in this project first")
			}
			if h.engine(s) != nil && !s.Welcome && (s.Path == "" || !fileExists(s.Path)) {
				return errors.New("send or persist the current session before changing project trust")
			}
		}
	}
	if h.trustedProjects == nil {
		h.trustedProjects = make(map[string]bool)
	}
	previous, hadPrevious := h.trustedProjects[root]
	if trusted {
		h.trustedProjects[root] = true
	} else {
		delete(h.trustedProjects, root)
	}
	if err := h.savePreferences(app); err != nil {
		if hadPrevious {
			h.trustedProjects[root] = previous
		} else {
			delete(h.trustedProjects, root)
		}
		return err
	}
	if h.trusted || app == nil {
		return nil
	}
	h.mu.Lock()
	cwds := make([]string, 0, len(h.services))
	for cwd := range h.services {
		cwds = append(cwds, cwd)
	}
	h.mu.Unlock()
	for _, cwd := range cwds {
		if projectTrustRoot(cwd) != root {
			continue
		}
		h.mu.Lock()
		svcs := h.services[cwd]
		delete(h.services, cwd)
		h.mu.Unlock()
		if svcs != nil {
			_ = svcs.Close()
		}
	}
	for _, s := range app.sessions {
		if s.TrustRoot != root {
			continue
		}
		h.dropEngine(s)
		s.SkillNames = nil
		s.SkillAll = nil
		s.MCPServers = nil
		s.MCPAll = nil
		s.Context.Skills = 0
		s.Context.MCP = 0
	}
	if active := app.active(); active != nil && active.TrustRoot == root {
		if _, err := h.ensureEngine(app, active); err != nil {
			active.Status = "error"
			active.Note = "could not reload · " + err.Error()
			return err
		}
	}
	return nil
}
