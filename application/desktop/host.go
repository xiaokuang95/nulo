package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/xiaokuang95/nulo/sdk"
)

type hostOptions struct {
	Workspace string
	AgentDir  string
	Trusted   bool
	// Sync runs event callbacks on the SDK goroutine. Tests use it so they can
	// wait for Send to return without racing the window loop; the app uses the
	// normal main-thread dispatch.
	Sync bool
}

type desktopHost struct {
	mu              sync.Mutex
	app             *app
	agentDir        string
	initialCWD      string
	trusted         bool
	sync            bool
	closed          bool
	services        map[string]*sdk.AgentSessionServices
	engines         map[string]*sessionEngine
	pending         map[string]*pendingApproval
	changesPending  map[string]bool
	worktrees       map[string]worktreeRecord
	trustedProjects map[string]bool
	disabledSkills  map[string]bool
	defaultMdl      string
	thinking        string
	defaultApproval string
}

type sessionEngine struct {
	id    string
	path  string
	rt    *sdk.Runtime
	sess  *sdk.Session
	unsub func()
}

type pendingApproval struct {
	ch chan string
}

func newDesktopHost(opts hostOptions) *desktopHost {
	if opts.Workspace != "" {
		if abs, err := filepath.Abs(opts.Workspace); err == nil {
			opts.Workspace = abs
		}
	}
	if opts.AgentDir != "" {
		if abs, err := filepath.Abs(opts.AgentDir); err == nil {
			opts.AgentDir = abs
		}
	}
	return &desktopHost{
		agentDir:        opts.AgentDir,
		initialCWD:      opts.Workspace,
		trusted:         opts.Trusted,
		sync:            opts.Sync,
		services:        make(map[string]*sdk.AgentSessionServices),
		engines:         make(map[string]*sessionEngine),
		pending:         make(map[string]*pendingApproval),
		worktrees:       make(map[string]worktreeRecord),
		trustedProjects: make(map[string]bool),
		disabledSkills:  make(map[string]bool),
		defaultApproval: "auto-edits",
	}
}

func (a *app) startSDKHost(opts hostOptions) {
	h := newDesktopHost(opts)
	if h.agentDir == "" {
		h.agentDir = sdk.DefaultAgentDir()
	}
	if h.initialCWD == "" {
		h.initialCWD = defaultWorkspace()
	}
	h.app = a
	a.host = h
	if err := h.bootstrap(a); err != nil {
		a.hostErr = err.Error()
		a.pendingToast = "SDK: " + err.Error()
		if len(a.sessions) == 0 {
			a.projects = []projectGroup{h.projectFor(h.initialCWD)}
			s := &session{
				ID: "unavailable", Project: a.projects[0].Key, CWD: h.initialCWD,
				Title: "SDK unavailable", Status: "error", Branch: "main", Env: "Local",
				Model: "faux/faux-1", ModelName: "Faux Model", ModelLimit: 128000,
				Thinking: "high", Approval: "auto-edits", Started: "now", Updated: "now",
				Welcome: true, Real: true,
			}
			a.sessions = []*session{s}
			a.activeID = s.ID
		}
	}
}

func defaultWorkspace() string {
	if value := strings.TrimSpace(os.Getenv("NULO_WORKSPACE")); value != "" {
		if abs, err := filepath.Abs(value); err == nil {
			return abs
		}
	}
	if saved := loadSavedWorkspace(); saved != "" {
		return saved
	}
	cwd, _ := os.Getwd()
	if root := gitRoot(cwd); root != "" {
		return root
	}
	if cwd != "" && cwd != "/" {
		return cwd
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

func savedWorkspacePath() string {
	return filepath.Join(sdk.DefaultHomeDir(), "desktop-workspace")
}

func loadSavedWorkspace() string {
	data, err := os.ReadFile(savedWorkspacePath())
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(data))
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return ""
	}
	return path
}

func saveWorkspace(path string) {
	if path == "" {
		return
	}
	config := savedWorkspacePath()
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(config, []byte(path+"\n"), 0o644)
}

func gitRoot(cwd string) string {
	if cwd == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitBranch(cwd string) string {
	if cwd == "" {
		return "main"
	}
	out, err := exec.Command("git", "-C", cwd, "symbolic-ref", "--quiet", "--short", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return "main"
	}
	return strings.TrimSpace(string(out))
}

func (h *desktopHost) bootstrap(a *app) error {
	a.hostErr = ""
	if err := h.loadPreferences(a); err != nil {
		a.pendingToast = "Desktop settings: " + err.Error()
	}
	if err := h.loadProviderCatalog(a); err != nil {
		a.pendingToast = "Models: " + err.Error()
	}
	if err := h.loadWorktrees(); err != nil {
		a.pendingToast = "Worktrees: " + err.Error()
	}

	settings := sdk.NewSettingsManager(h.initialCWD, h.agentDir)
	if provider, id := settings.GetDefaultProvider(), settings.GetDefaultModel(); id != "" {
		spec := id
		if provider != "" {
			spec = provider + "/" + id
		}
		h.defaultMdl = spec
	} else {
		h.defaultMdl = "faux/faux-1"
	}
	if level := string(settings.GetDefaultThinkingLevel()); level != "" {
		h.thinking = level
	} else {
		h.thinking = "high"
	}
	initialProject := ""
	if h.initialCWD != "" {
		initialProject = h.ensureProject(a, h.initialCWD, true).Key
	}

	infos, err := sdk.ListAllSessions(h.agentDir)
	if err != nil {
		return err
	}
	paths := make(map[string]*session, len(infos))
	staleWorktrees := false
	for _, info := range infos {
		if info.CWD == "" {
			continue
		}
		baseCWD, cwd := info.CWD, info.CWD
		env, worktreeName, branch := "Local", "", gitBranch(info.CWD)
		if record, ok := h.worktreeFor(info.ID); ok {
			if record.Root != "" {
				baseCWD = record.Root
			}
			cwd = baseCWD
			branch = gitBranch(baseCWD)
			if record.Active {
				if st, err := os.Stat(record.Path); err == nil && st.IsDir() {
					cwd = record.Path
					env = "Worktree"
					worktreeName = filepath.Base(record.Path)
					if record.Branch != "" {
						branch = record.Branch
					}
				} else {
					record.Active = false
					h.worktrees[info.ID] = record
					staleWorktrees = true
				}
			}
		}
		g := h.ensureProject(a, baseCWD, false)
		name := strings.TrimSpace(info.Name)
		if name == "" {
			name = titleFor(info.FirstMessage)
		}
		if name == "" {
			name = "Untitled session"
		}
		updated := relativeTime(info.Modified)
		s := &session{
			ID: info.ID, Path: info.Path, Project: g.Key, CWD: cwd, BaseCWD: baseCWD, OriginCWD: info.CWD,
			TrustRoot: projectTrustRoot(baseCWD), Title: name, Status: "done", Branch: branch, Env: env, Worktree: worktreeName,
			Model: h.defaultMdl, Thinking: h.thinking, Approval: h.defaultApproval,
			Started: relativeTime(info.Created), Updated: updated, Modified: info.Modified.UnixMilli(),
			Note: messageNote(updated, info.MessageCount), Welcome: info.MessageCount == 0, Real: true,
		}
		a.applyModelFields(s)
		s.Context.Limit = s.modelContext()
		a.sessions = append(a.sessions, s)
		paths[info.Path] = s
	}
	if staleWorktrees {
		_ = h.saveWorktrees()
	}
	parents := make(map[string]string, len(infos))
	for _, info := range infos {
		if info.ParentSessionPath != "" {
			parents[info.Path] = info.ParentSessionPath
		}
	}
	for _, s := range a.sessions {
		if parentPath := parents[s.Path]; parentPath != "" {
			if parent := paths[parentPath]; parent != nil {
				s.ForkOf = parent.ID
			}
		}
	}

	if len(a.sessions) == 0 {
		g := h.projectFor(h.initialCWD)
		s, err := h.newSession(a, g)
		if err != nil {
			return err
		}
		a.activeID = s.ID
		return nil
	}
	active := a.sessions[0]
	if initialProject != "" {
		for _, s := range a.sessions {
			if s.Project == initialProject {
				active = s
				break
			}
		}
	}
	a.activeID = active.ID
	if _, err := h.ensureEngine(a, active); err != nil {
		active.Status = "error"
		active.Note = "could not load · " + err.Error()
	}
	return nil
}

func (h *desktopHost) loadProviderCatalog(a *app) error {
	catalog, err := sdk.ListProviders(h.agentDir)
	if err != nil {
		a.providers = defaultProviders()
		return err
	}
	out := make([]provider, 0, len(catalog)+1)
	for _, p := range catalog {
		item := provider{ID: p.ID, Name: p.Name, BaseURL: p.BaseURL, API: p.API, APIKeySource: p.APIKeySource}
		for _, m := range p.Models {
			item.Models = append(item.Models, model{
				ID: m.ID, Name: m.Name, Context: m.ContextWindow, MaxOut: m.MaxTokens,
				Reasoning: m.Reasoning, Image: hasString(m.Input, "image"),
			})
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		out = append(out, provider{
			ID: "faux", Name: "Faux", API: "faux",
			Models: []model{{ID: "faux-1", Name: "Faux Model", Context: 128000, MaxOut: 16384}},
		})
	}
	a.providers = out
	return nil
}

func defaultProviders() []provider {
	out := make([]provider, len(providers))
	copy(out, providers)
	return out
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func messageNote(updated string, count int) string {
	if count <= 0 {
		return updated + " · no messages"
	}
	if count == 1 {
		return updated + " · 1 message"
	}
	return fmt.Sprintf("%s · %d messages", updated, count)
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "now"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 48*time.Hour:
		return "Yesterday"
	default:
		return t.Format("Jan 2")
	}
}

func (h *desktopHost) projectFor(cwd string) projectGroup {
	base := filepath.Base(cwd)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "workspace"
	}
	name := strings.ToUpper(base)
	letter := "W"
	for _, r := range name {
		letter = string(r)
		break
	}
	return projectGroup{
		Key: projectKey(base), Name: name, Letter: letter, Tint: "green",
		Branch: gitBranch(cwd), CWD: cwd,
	}
}

func projectKey(base string) string {
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" {
		return "workspace"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range base {
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
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "workspace"
	}
	return out
}

func (h *desktopHost) ensureProject(a *app, cwd string, makeDefault bool) projectGroup {
	cwd = filepath.Clean(cwd)
	for _, g := range a.projects {
		if g.CWD == cwd {
			return g
		}
	}
	g := h.projectFor(cwd)
	key := g.Key
	for n := 2; ; n++ {
		used := false
		for _, other := range a.projects {
			if other.Key == key {
				used = true
				break
			}
		}
		if !used {
			break
		}
		key = fmt.Sprintf("%s-%d", g.Key, n)
	}
	g.Key = key
	if makeDefault || len(a.projects) == 0 {
		a.projects = append([]projectGroup{g}, a.projects...)
	} else {
		a.projects = append(a.projects, g)
	}
	if makeDefault {
		h.initialCWD = cwd
	}
	return g
}

func (h *desktopHost) servicesFor(cwd string) (*sdk.AgentSessionServices, error) {
	cwd = filepath.Clean(cwd)
	h.mu.Lock()
	if svcs := h.services[cwd]; svcs != nil {
		h.mu.Unlock()
		return svcs, nil
	}
	disabledSkills := make([]string, 0, len(h.disabledSkills))
	for name, disabled := range h.disabledSkills {
		if disabled {
			disabledSkills = append(disabledSkills, name)
		}
	}
	h.mu.Unlock()
	svcs, err := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: h.agentDir, ProjectTrusted: h.projectTrusted(cwd),
		DisabledSkills: disabledSkills,
	})
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	if existing := h.services[cwd]; existing != nil {
		h.mu.Unlock()
		_ = svcs.Close()
		return existing, nil
	}
	h.services[cwd] = svcs
	h.mu.Unlock()
	return svcs, nil
}

func (h *desktopHost) newSession(a *app, g projectGroup) (*session, error) {
	cwd := g.CWD
	if cwd == "" {
		cwd = h.initialCWD
	}
	if cwd == "" {
		cwd = defaultWorkspace()
	}
	g = h.ensureProject(a, cwd, false)
	svcs, err := h.servicesFor(cwd)
	if err != nil {
		return nil, err
	}
	rt, err := sdk.NewRuntime(sdk.RuntimeOptions{Services: svcs})
	if err != nil {
		return nil, err
	}
	spec := h.defaultMdl
	level := h.thinking
	if manager := svcs.SettingsManager; manager != nil {
		if provider, id := manager.GetDefaultProvider(), manager.GetDefaultModel(); id != "" {
			spec = id
			if provider != "" {
				spec = provider + "/" + id
			}
		}
		if configured := string(manager.GetDefaultThinkingLevel()); configured != "" {
			level = configured
		}
	}
	model, err := sdk.BuildModel(spec, svcs)
	if err != nil {
		model = sdk.DefaultFauxModel()
	}
	var sid string
	approve := h.approvalFunc(&sid)
	sess, err := rt.New(sdk.SessionStartOptions{
		Model: model, ThinkingLevel: sdk.ThinkingLevel(level),
		ApproveTool: approve,
	})
	if err != nil {
		return nil, err
	}
	sid = sess.ID()
	s := &session{
		ID: sess.ID(), Path: sess.File(), Project: g.Key, CWD: cwd, BaseCWD: cwd, OriginCWD: cwd,
		TrustRoot: projectTrustRoot(cwd), Title: "Untitled session", Status: "idle", Branch: gitBranch(cwd), Env: "Local",
		Model: model.Provider + "/" + model.ID, ModelName: model.Name, ModelLimit: model.ContextWindow,
		Thinking: string(sess.ThinkingLevel()), Approval: h.defaultApproval,
		Started: "now", Updated: "now", Modified: time.Now().UnixMilli(), Welcome: true, Real: true,
	}
	a.applyModelFields(s)
	a.sessions = append([]*session{s}, a.sessions...)
	engine := &sessionEngine{id: sess.ID(), path: sess.File(), rt: rt, sess: sess}
	h.addEngine(a, s, engine)
	a.refreshCapabilities(s)
	return s, nil
}

func (h *desktopHost) addEngine(a *app, s *session, e *sessionEngine) {
	h.mu.Lock()
	h.engines[e.id] = e
	h.mu.Unlock()
	e.unsub = e.sess.Subscribe(func(ev sdk.Event) {
		h.dispatch(a, s, e, ev)
	})
}

func (h *desktopHost) dispatch(a *app, s *session, e *sessionEngine, ev sdk.Event) {
	if h.sync || a.win == nil {
		a.handleSDKEvent(s, e, ev)
		return
	}
	a.win.Update(func() {
		if a.host == nil || a.host != h {
			return
		}
		a.handleSDKEvent(s, e, ev)
	})
}

func (h *desktopHost) ensureEngine(a *app, s *session) (*sessionEngine, error) {
	if s == nil || !s.Real {
		return nil, nil
	}
	h.mu.Lock()
	e := h.engines[s.ID]
	h.mu.Unlock()
	if e != nil {
		return e, nil
	}
	if s.Path == "" {
		return nil, errors.New("session has no file")
	}
	svcs, err := h.servicesFor(s.CWD)
	if err != nil {
		return nil, err
	}
	sid := s.ID
	rt, err := sdk.NewRuntime(sdk.RuntimeOptions{Services: svcs, ApproveTool: h.approvalFunc(&sid)})
	if err != nil {
		return nil, err
	}
	sess, err := rt.ResumeSession(s.Path)
	if err != nil {
		return nil, err
	}
	e = &sessionEngine{id: sess.ID(), path: sess.File(), rt: rt, sess: sess}
	h.addEngine(a, s, e)
	h.hydrate(a, s, sess)
	return e, nil
}

func (h *desktopHost) hydrate(a *app, s *session, sess *sdk.Session) {
	model := sess.Model()
	if model.ID != "" {
		s.Model = model.Provider + "/" + model.ID
		s.ModelName = model.Name
		s.ModelLimit = model.ContextWindow
		s.Context.Limit = model.ContextWindow
	}
	s.Thinking = string(sess.ThinkingLevel())
	a.applySDKPlan(s, sess.Plan())
	if s.Title == "" || s.Title == "Untitled session" {
		if name := strings.TrimSpace(sess.Name()); name != "" {
			s.Title = name
		}
	}
	s.SkillNames = sess.SkillNames()
	s.MCPServers = sess.MCPServerNames()
	s.PluginNames = sess.PluginNames()
	s.PluginDiags = nil
	for _, diag := range sess.PluginDiagnostics() {
		s.PluginDiags = append(s.PluginDiags, diag.Error())
	}
	s.Context.Skills = len(s.SkillNames)
	s.Context.MCP = len(s.MCPServers)
	s.Context.Plugins = len(s.PluginNames)
	a.refreshCapabilities(s)
	a.applyModelFields(s)
	a.renderSDKMessages(s, sess, nil)
	a.updateRealUsage(s, sess)
	switch reason := lastStopReason(sess.Messages()); reason {
	case sdk.StopError:
		s.Status = "error"
	case sdk.StopAborted:
		s.Status = "paused"
	case "":
		if len(s.Messages) > 0 {
			s.Status = "done"
		}
	default:
		s.Status = "done"
	}
}

func (h *desktopHost) send(a *app, s *session, text string, attachments []string) error {
	loaded, err := loadHostAttachments(attachments)
	if err != nil {
		return err
	}
	e, err := h.ensureEngine(a, s)
	if err != nil {
		return err
	}
	if e == nil {
		return errors.New("session is not backed by the SDK")
	}
	s.Welcome = false
	if strings.HasPrefix(s.Title, "Untitled") {
		s.Title = titleFor(text)
	}
	_ = h.captureCheckpoint(s, e.sess.LeafID())
	prompt := text
	var images []sdk.ImageAttachment
	if len(loaded) > 0 {
		var lines []string
		for _, item := range loaded {
			lines = append(lines, "- "+item.Path)
			if item.Image != nil {
				images = append(images, *item.Image)
			}
		}
		prompt += "\n\nAttached files:\n" + strings.Join(lines, "\n")
	}
	s.Messages = append(s.Messages, message{Role: "user", Time: "now", Text: text, Attachments: append([]string(nil), attachments...)})
	s.Status = "running"
	s.Updated = "now"
	s.Modified = time.Now().UnixMilli()
	a.scrollToEnd = true
	go func() {
		if err := e.sess.Prompt(context.Background(), prompt, sdk.PromptOptions{Images: images}); err != nil {
			h.dispatch(a, s, e, sdk.Event{Type: "host_error", Result: err.Error()})
		}
	}()
	return nil
}

func (h *desktopHost) abort(s *session) {
	e := h.engine(s)
	if e == nil {
		return
	}
	_ = e.sess.Abort(context.Background())
}

func (h *desktopHost) engine(s *session) *sessionEngine {
	if s == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.engines[s.ID]
}

func (h *desktopHost) resolveApproval(a *app, s *session, b *block, decision string) {
	h.mu.Lock()
	p := h.pending[b.CallID]
	delete(h.pending, b.CallID)
	h.mu.Unlock()
	if decision == "always" {
		s.Approval = "auto"
		decision = "approved"
	}
	b.Approval = decision
	for i := range s.Approvals {
		if s.Approvals[i].CallID == b.CallID {
			s.Approvals[i].Approval = b.Approval
		}
	}
	if p != nil {
		select {
		case p.ch <- decision:
		default:
		}
	}
	if decision == "denied" {
		s.Status = "paused"
	} else {
		s.Status = "running"
	}
	a.scrollToEnd = true
}

func (h *desktopHost) approvalFunc(sessionID *string) sdk.ToolApprovalFunc {
	return func(ctx context.Context, req sdk.ToolApprovalRequest) (sdk.ToolApprovalDecision, error) {
		var mode string
		h.onMain(func() {
			if s := h.app.sessionByID(*sessionID); s != nil {
				mode = s.Approval
			}
		})
		allow, block := approvalPolicy(mode, req.ToolName)
		if block != "" {
			return sdk.ToolApprovalDecision{Allow: false, Reason: block}, nil
		}
		if allow {
			return sdk.ToolApprovalDecision{Allow: true}, nil
		}
		ch := make(chan string, 1)
		h.mu.Lock()
		h.pending[req.ToolCallID] = &pendingApproval{ch: ch}
		h.mu.Unlock()
		h.onMain(func() {
			if s := h.app.sessionByID(*sessionID); s != nil {
				h.app.addHostApproval(s, req)
			}
		})
		select {
		case decision := <-ch:
			return sdk.ToolApprovalDecision{Allow: decision == "approved", Reason: "Denied in the desktop host"}, nil
		case <-ctx.Done():
			h.mu.Lock()
			delete(h.pending, req.ToolCallID)
			h.mu.Unlock()
			h.onMain(func() {
				if s := h.app.sessionByID(*sessionID); s != nil {
					for i := range s.Approvals {
						if s.Approvals[i].CallID == req.ToolCallID && s.Approvals[i].Approval == "pending" {
							s.Approvals[i].Approval = "denied"
						}
					}
				}
			})
			return sdk.ToolApprovalDecision{Allow: false, Reason: ctx.Err().Error()}, ctx.Err()
		}
	}
}

func (h *desktopHost) onMain(fn func()) {
	if h.sync || h.app.win == nil {
		fn()
		return
	}
	mygo.RunOnMain(fn)
}

func approvalPolicy(mode, tool string) (allow bool, block string) {
	switch mode {
	case "auto":
		return true, ""
	case "plan":
		switch tool {
		case "read", "ls", "grep", "find", "plan":
			return true, ""
		default:
			return false, "Plan mode only allows read-only tools"
		}
	case "auto-edits":
		switch tool {
		case "read", "ls", "grep", "find", "plan", "edit", "write":
			return true, ""
		default:
			return false, ""
		}
	case "manual", "":
		switch tool {
		case "read", "ls", "grep", "find", "plan":
			return true, ""
		default:
			return false, ""
		}
	}
	return false, ""
}

func (a *app) addHostApproval(s *session, req sdk.ToolApprovalRequest) {
	b := block{
		Type: "approval", Approval: "pending", CallID: req.ToolCallID, Tool: req.ToolName,
		Title: "Allow this tool?", Command: approvalCommand(req), CWD: req.CWD,
		Reason: "The agent wants to run " + toolLabel(req.ToolName) + " in this workspace.",
		Risk:   approvalRisk(req.ToolName),
	}
	for i := range s.Approvals {
		if s.Approvals[i].CallID == req.ToolCallID {
			return
		}
	}
	s.Approvals = append(s.Approvals, b)
	s.Status = "awaiting"
	if e := a.host.engine(s); e != nil {
		a.renderSDKMessages(s, e.sess, nil)
	}
	a.scrollToEnd = true
	if a.win != nil {
		a.win.Invalidate()
	}
	a.notifyCompletion(s)
}

func approvalCommand(req sdk.ToolApprovalRequest) string {
	if cmd, ok := req.Args["command"].(string); ok && cmd != "" {
		return cmd
	}
	if path, ok := req.Args["path"].(string); ok && path != "" {
		return req.ToolName + " " + path
	}
	return req.ToolName
}

func approvalRisk(tool string) string {
	switch tool {
	case "bash", "powershell":
		return "runs a shell command in this workspace"
	case "edit", "write":
		return "writes to the workspace"
	default:
		return "runs an external tool"
	}
}

func toolLabel(name string) string {
	if name == "" {
		return "a tool"
	}
	return name
}

func (h *desktopHost) closeHost() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	engines := make([]*sessionEngine, 0, len(h.engines))
	for _, e := range h.engines {
		engines = append(engines, e)
	}
	services := make([]*sdk.AgentSessionServices, 0, len(h.services))
	for _, svcs := range h.services {
		services = append(services, svcs)
	}
	pending := make([]*pendingApproval, 0, len(h.pending))
	for _, p := range h.pending {
		pending = append(pending, p)
	}
	h.pending = map[string]*pendingApproval{}
	h.mu.Unlock()
	for _, p := range pending {
		select {
		case p.ch <- "denied":
		default:
		}
	}
	for _, e := range engines {
		if e.unsub != nil {
			e.unsub()
		}
		if e.sess != nil {
			e.sess.Dispose()
		}
	}
	for _, svcs := range services {
		_ = svcs.Close()
	}
}

func (a *app) sessionByID(id string) *session {
	for _, s := range a.sessions {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (a *app) closeHost() {
	a.shuttingDown = true
	a.closeTerminals()
	if a.host != nil {
		a.host.closeHost()
	}
}

func (a *app) hostAvailable() bool { return a.host != nil && a.hostErr == "" }

func (a *app) defaultModelSpec() string {
	if a.host != nil && a.host.defaultMdl != "" {
		return a.host.defaultMdl
	}
	return "faux/faux-1"
}

func (a *app) applyModelFields(s *session) {
	if s == nil {
		return
	}
	for _, provider := range a.providers {
		for _, mdl := range provider.Models {
			if provider.ID+"/"+mdl.ID != s.Model {
				continue
			}
			s.ModelName = mdl.Name
			s.ModelLimit = mdl.Context
			s.Context.Limit = mdl.Context
			return
		}
	}
	if s.Model == "faux/faux-1" || s.Model == "faux" {
		s.ModelName = "Faux Model"
		s.ModelLimit = 128000
	}
}
