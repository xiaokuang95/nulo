package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/xiaokuang95/nulo/sdk"
)

func (a *app) setSessionModel(s *session, spec string) {
	if s == nil {
		return
	}
	if a.host != nil {
		if err := a.host.setModel(a, s, spec); err != nil {
			a.pendingToast = "Model: " + err.Error()
			return
		}
	}
	s.Model = spec
	a.applyModelFields(s)
	if s.ModelLimit > 0 {
		s.Context.Limit = s.ModelLimit
	}
}

func (h *desktopHost) setModel(a *app, s *session, spec string) error {
	e, err := h.ensureEngine(a, s)
	if err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("session is not backed by the SDK")
	}
	svcs, err := h.servicesFor(s.CWD)
	if err != nil {
		return err
	}
	model, err := sdk.BuildModel(spec, svcs)
	if err != nil {
		return err
	}
	if err := e.sess.SetModel(model); err != nil {
		return err
	}
	a.syncSessionMeta(s, e.sess)
	return nil
}

func (a *app) setDefaultModel(s *session, spec string) {
	a.setSessionModel(s, spec)
	if a.host == nil {
		return
	}
	provider, id, ok := strings.Cut(spec, "/")
	if !ok {
		provider, id = "", spec
	}
	a.host.defaultMdl = spec
	if s != nil {
		if svcs, err := a.host.servicesFor(s.CWD); err == nil && svcs.SettingsManager != nil {
			_ = svcs.SettingsManager.SetDefaultModelAndProvider(provider, id)
		}
	}
}

func (a *app) setSessionThinking(s *session, level string) {
	if s == nil {
		return
	}
	if a.host != nil {
		if err := a.host.setThinking(a, s, level); err != nil {
			a.pendingToast = "Thinking: " + err.Error()
			return
		}
		return
	}
	s.Thinking = level
}

func (h *desktopHost) setThinking(a *app, s *session, level string) error {
	e, err := h.ensureEngine(a, s)
	if err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("session is not backed by the SDK")
	}
	if err := e.sess.SetThinkingLevel(sdk.ThinkingLevel(level)); err != nil {
		return err
	}
	a.syncSessionMeta(s, e.sess)
	return nil
}

func (a *app) testProvider(p provider, spec string) {
	if a.host == nil || spec == "" {
		a.pendingToast = "No model to test for " + p.Name
		return
	}
	if a.providerTests == nil {
		a.providerTests = make(map[string]string)
	}
	if a.providerTests[p.ID] == "testing" {
		return
	}
	agentDir := a.host.agentDir
	a.providerTests[p.ID] = "testing"
	a.pendingToast = "Testing " + p.Name + "…"
	if a.win != nil {
		a.win.Invalidate()
	}
	go func() {
		err := sdk.TestProviderModel(agentDir, spec)
		apply := func() {
			if a.shuttingDown || a.providerTests == nil {
				return
			}
			if err != nil {
				message := strings.TrimSpace(err.Error())
				if message == "" {
					message = "connection failed"
				}
				a.providerTests[p.ID] = "error: " + message
				a.pendingToast = p.Name + ": " + message
			} else {
				a.providerTests[p.ID] = "ok"
				a.pendingToast = p.Name + " connected"
			}
			if a.win != nil {
				a.win.Invalidate()
			}
		}
		if a.win != nil {
			a.win.Update(apply)
			return
		}
		apply()
	}()
}

func (h *desktopHost) thinkingLevels(s *session) []string {
	if s == nil {
		return nil
	}
	svcs, err := h.servicesFor(sessionOriginCWD(s))
	if err != nil {
		return nil
	}
	model, err := sdk.BuildModel(s.Model, svcs)
	if err != nil {
		return nil
	}
	levels := sdk.SupportedThinkingLevels(model)
	out := make([]string, 0, len(levels))
	for _, level := range levels {
		out = append(out, string(level))
	}
	return out
}

func (h *desktopHost) setDefaultThinking(a *app, s *session, level string) error {
	if err := h.setThinking(a, s, level); err != nil {
		return err
	}
	h.thinking = level
	svcs, err := h.servicesFor(sessionOriginCWD(s))
	if err != nil {
		return err
	}
	if svcs.SettingsManager != nil {
		return svcs.SettingsManager.SetDefaultThinkingLevel(sdk.ThinkingLevel(level))
	}
	return nil
}

func (h *desktopHost) compact(a *app, s *session) {
	e, err := h.ensureEngine(a, s)
	if err != nil {
		a.pendingToast = "Compact: " + err.Error()
		return
	}
	if e == nil {
		a.pendingToast = "Compact: session is not backed by the SDK"
		return
	}
	s.Status = "running"
	a.scrollToEnd = true
	go func() {
		result, err := e.sess.Compact(context.Background(), "")
		h.dispatch(a, s, e, sdk.Event{Type: "host_compact_done", Result: compactOutcome{result: result, err: err}})
	}()
}

type compactOutcome struct {
	result *sdk.CompactionResult
	err    error
}

func (h *desktopHost) rewind(a *app, s *session) error {
	e, err := h.ensureEngine(a, s)
	if err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("session is not backed by the SDK")
	}
	branch := e.sess.Branch()
	lastUser := -1
	for i := len(branch) - 1; i >= 0; i-- {
		if branch[i].Type == "message" && branch[i].Message != nil && branch[i].Message.Role == "user" {
			lastUser = i
			break
		}
	}
	if lastUser <= 0 {
		return fmt.Errorf("nothing to rewind")
	}
	target := ""
	if branch[lastUser].ParentID != nil {
		target = *branch[lastUser].ParentID
	}
	return h.rewindTo(a, s, target, "Before your last message", false, "")
}

func (h *desktopHost) rewindTo(a *app, s *session, entryID, label string, restoreFiles bool, snapshotKey string) error {
	e, err := h.ensureEngine(a, s)
	if err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("session is not backed by the SDK")
	}
	if err := e.sess.NavigateTree(context.Background(), entryID, sdk.NavigateTreeOptions{}); err != nil {
		return err
	}
	if label == "" {
		label = "checkpoint"
	}
	s.Notices = append(s.Notices, message{Role: "notice", Time: "now", Text: "Rewound to " + label + "."})
	if restoreFiles {
		restored, err := restoreCheckpoint(h.agentDir, s, snapshotKey)
		if err != nil {
			s.Notices = append(s.Notices, message{Role: "notice", Time: "now", Text: "File restore failed · " + err.Error()})
		} else {
			s.Notices = append(s.Notices, message{Role: "notice", Time: "now", Text: fmt.Sprintf("Restored %d file(s) from checkpoint.", restored)})
		}
		a.invalidateFileIndex(s.CWD)
	}
	a.applySDKPlan(s, e.sess.Plan())
	a.renderSDKMessages(s, e.sess, nil)
	a.updateRealUsage(s, e.sess)
	s.Approvals = nil
	s.Status = "done"
	if s.Welcome {
		s.Status = "idle"
	}
	s.Updated = "now"
	s.Modified = time.Now().UnixMilli()
	a.requestChanges(s)
	return nil
}

func (h *desktopHost) navigateTo(a *app, s *session, targetID string) error {
	e, err := h.ensureEngine(a, s)
	if err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("session is not backed by the SDK")
	}
	if err := e.sess.NavigateTree(context.Background(), targetID, sdk.NavigateTreeOptions{}); err != nil {
		return err
	}
	a.applySDKPlan(s, e.sess.Plan())
	a.renderSDKMessages(s, e.sess, nil)
	a.updateRealUsage(s, e.sess)
	s.Approvals = nil
	s.Status = "done"
	if s.Welcome {
		s.Status = "idle"
	}
	s.Updated = "now"
	s.Modified = time.Now().UnixMilli()
	a.requestChanges(s)
	return nil
}

func (h *desktopHost) messageEntry(a *app, s *session, entryID string) (sdk.SessionEntry, error) {
	e, err := h.ensureEngine(a, s)
	if err != nil {
		return sdk.SessionEntry{}, err
	}
	if e == nil {
		return sdk.SessionEntry{}, fmt.Errorf("session is not backed by the SDK")
	}
	for _, entry := range e.sess.Branch() {
		if entry.ID == entryID {
			return entry, nil
		}
	}
	return sdk.SessionEntry{}, fmt.Errorf("message not found")
}

func parentEntryID(entry sdk.SessionEntry) string {
	if entry.ParentID == nil {
		return ""
	}
	return *entry.ParentID
}

func (h *desktopHost) editMessage(a *app, s *session, entryID, text string, attachments []string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("message is empty")
	}
	entry, err := h.messageEntry(a, s, entryID)
	if err != nil {
		return err
	}
	if entry.Message == nil || entry.Message.Role != "user" {
		return fmt.Errorf("only user messages can be edited")
	}
	if err := h.navigateTo(a, s, parentEntryID(entry)); err != nil {
		return err
	}
	return h.send(a, s, text, attachments)
}

func (h *desktopHost) retryMessage(a *app, s *session, entryID string) error {
	e, err := h.ensureEngine(a, s)
	if err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("session is not backed by the SDK")
	}
	branch := e.sess.Branch()
	index := -1
	for i, entry := range branch {
		if entry.ID == entryID {
			index = i
			break
		}
	}
	if index < 0 || branch[index].Message == nil || branch[index].Message.Role != "assistant" {
		return fmt.Errorf("assistant message not found")
	}
	for i := index - 1; i >= 0; i-- {
		entry := branch[i]
		if entry.Type == "message" && entry.Message != nil && entry.Message.Role == "user" {
			text := strings.TrimSpace(entry.Message.ContentString())
			if text == "" {
				return fmt.Errorf("the user message is empty")
			}
			if err := h.navigateTo(a, s, parentEntryID(entry)); err != nil {
				return err
			}
			return h.send(a, s, text, nil)
		}
	}
	return fmt.Errorf("no user message to retry")
}

func (h *desktopHost) deleteMessage(a *app, s *session, entryID string) error {
	entry, err := h.messageEntry(a, s, entryID)
	if err != nil {
		return err
	}
	return h.navigateTo(a, s, parentEntryID(entry))
}

func (h *desktopHost) branchFromMessage(a *app, s *session, entryID string) (*session, error) {
	childUI, err := h.fork(a, s)
	if err != nil {
		return nil, err
	}
	ce := h.engine(childUI)
	if ce == nil {
		return nil, fmt.Errorf("forked session is not backed by the SDK")
	}
	if err := ce.sess.NavigateTree(context.Background(), entryID, sdk.NavigateTreeOptions{}); err != nil {
		return nil, err
	}
	h.hydrate(a, childUI, ce.sess)
	childUI.Status = "done"
	a.scrollToEnd = true
	return childUI, nil
}

func (h *desktopHost) fork(a *app, s *session) (*session, error) {
	e, err := h.ensureEngine(a, s)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("session is not backed by the SDK")
	}
	if e.unsub != nil {
		e.unsub()
		e.unsub = nil
	}
	h.mu.Lock()
	delete(h.engines, e.id)
	h.mu.Unlock()
	child, err := e.rt.Fork()
	if err != nil {
		h.mu.Lock()
		h.engines[e.id] = e
		h.mu.Unlock()
		e.unsub = e.sess.Subscribe(func(ev sdk.Event) { h.dispatch(a, s, e, ev) })
		return nil, err
	}
	e.id = child.ID()
	e.path = child.File()
	e.sess = child
	childUI := &session{
		ID: child.ID(), Path: child.File(), Project: s.Project, CWD: s.CWD, BaseCWD: s.BaseCWD,
		OriginCWD: s.OriginCWD, TrustRoot: s.TrustRoot, ParentPath: s.Path, Worktree: s.Worktree,
		Title: forkTitle(s.Title), Status: "done", Branch: s.Branch, Env: s.Env,
		Model: s.Model, ModelName: s.ModelName, ModelLimit: s.ModelLimit,
		Thinking: s.Thinking, Approval: s.Approval, Started: "now", Updated: "now",
		Modified: time.Now().UnixMilli(), ForkOf: s.ID, Real: true,
	}
	_ = copyCheckpointSnapshots(h.agentDir, s.ID, childUI.ID)
	if record, ok := h.worktreeFor(s.ID); ok && s.Env == "Worktree" {
		record.SessionID = childUI.ID
		record.Active = true
		h.rememberWorktree(record)
		_ = h.saveWorktrees()
	}
	h.addEngine(a, childUI, e)
	a.sessions = append([]*session{childUI}, a.sessions...)
	a.activeID = childUI.ID
	h.hydrate(a, childUI, child)
	a.scrollToEnd = true
	return childUI, nil
}

func (h *desktopHost) rename(a *app, s *session, name string) error {
	if s == nil {
		return fmt.Errorf("no session")
	}
	if e := h.engine(s); e != nil {
		if err := e.sess.SetName(name); err != nil {
			return err
		}
		s.Title = name
		return nil
	}
	if s.Path == "" {
		s.Title = name
		return nil
	}
	svcs, err := h.servicesFor(sessionOriginCWD(s))
	if err != nil {
		return err
	}
	rt, err := sdk.NewRuntime(sdk.RuntimeOptions{Services: svcs})
	if err != nil {
		return err
	}
	if err := rt.RenameSession(s.Path, name); err != nil {
		return err
	}
	s.Title = name
	return nil
}

func (h *desktopHost) deleteWorktreeFiles(s *session) error {
	record, ok := h.worktreeFor(s.ID)
	if !ok {
		return nil
	}
	root := record.Root
	if root == "" && s != nil {
		root = s.BaseCWD
	}
	var firstErr error
	if record.Path != "" && fileExists(record.Path) && root != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		cmd := exec.CommandContext(ctx, "git", "-C", root, "worktree", "remove", "--force", record.Path)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			message := strings.TrimSpace(string(out))
			if message == "" {
				message = err.Error()
			}
			firstErr = fmt.Errorf("git worktree remove: %s", message)
		}
	}
	if record.Branch != "" && root != "" {
		if _, err := gitOutput(root, "show-ref", "--verify", "--quiet", "refs/heads/"+record.Branch); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			cmd := exec.CommandContext(ctx, "git", "-C", root, "branch", "-D", record.Branch)
			out, err := cmd.CombinedOutput()
			cancel()
			if err != nil && firstErr == nil {
				message := strings.TrimSpace(string(out))
				if message == "" {
					message = err.Error()
				}
				firstErr = fmt.Errorf("git branch -D: %s", message)
			}
		}
	}
	h.forgetWorktree(s.ID)
	if err := h.saveWorktrees(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("save worktree state: %w", err)
	}
	return firstErr
}

func (h *desktopHost) deleteSession(a *app, s *session) error {
	if s == nil {
		return nil
	}
	worktreeErr := h.deleteWorktreeFiles(s)
	_ = deleteCheckpointSnapshots(h.agentDir, s.ID)
	if e := h.engine(s); e != nil {
		if e.unsub != nil {
			e.unsub()
		}
		if e.sess != nil {
			e.sess.Dispose()
		}
		h.mu.Lock()
		delete(h.engines, e.id)
		h.mu.Unlock()
	}
	if s.Path == "" {
		return worktreeErr
	}
	svcs, err := h.servicesFor(sessionOriginCWD(s))
	if err != nil {
		return err
	}
	rt, err := sdk.NewRuntime(sdk.RuntimeOptions{Services: svcs})
	if err != nil {
		return err
	}
	if err := rt.DeleteSession(s.Path); err != nil {
		return err
	}
	return worktreeErr
}

func sessionOriginCWD(s *session) string {
	if s == nil {
		return ""
	}
	if s.OriginCWD != "" {
		return s.OriginCWD
	}
	if s.BaseCWD != "" {
		return s.BaseCWD
	}
	return s.CWD
}

func (a *app) addHostNotice(s *session, text string) {
	if s == nil {
		return
	}
	s.Notices = append(s.Notices, message{Role: "notice", Time: "now", Text: text})
	if a.host != nil {
		if e := a.host.engine(s); e != nil {
			a.renderSDKMessages(s, e.sess, nil)
		}
	}
}

func compactNotice(before, after int) string {
	return fmt.Sprintf("Context compacted · %s → %s tokens", formatTokens(before), formatTokens(after))
}

func forkTitle(title string) string {
	if strings.TrimSpace(title) == "" {
		return "Untitled session · fork"
	}
	return title + " · fork"
}

func (a *app) providerByID(id string) (provider, bool) {
	for _, p := range a.providers {
		if p.ID == id {
			return p, true
		}
	}
	return provider{}, false
}

func (a *app) openProviderKey(prov provider) {
	a.keyProvider = prov.ID
	a.keyName = prov.Name
	a.keyValue = ""
	a.keyOpen = true
}

func (a *app) reloadProviders() {
	if a.host == nil {
		return
	}
	if err := a.host.loadProviderCatalog(a); err != nil {
		a.pendingToast = "Providers: " + err.Error()
	}
}

func (a *app) saveProviderKey() {
	if a.host == nil {
		a.pendingToast = "Provider keys need the SDK host"
		return
	}
	key := strings.TrimSpace(a.keyValue)
	if key == "" {
		a.pendingToast = "Enter an API key"
		return
	}
	if err := sdk.SetProviderAPIKey(a.host.agentDir, a.keyProvider, key); err != nil {
		a.pendingToast = "API key: " + err.Error()
		return
	}
	a.keyOpen = false
	a.keyValue = ""
	a.reloadProviders()
	a.pendingToast = "API key saved for " + a.keyName
}

func (a *app) clearProviderKey() {
	if a.host == nil {
		return
	}
	if err := sdk.ClearProviderAPIKey(a.host.agentDir, a.keyProvider); err != nil {
		a.pendingToast = "Clear API key: " + err.Error()
		return
	}
	a.keyOpen = false
	a.keyValue = ""
	a.reloadProviders()
	a.pendingToast = "Removed stored API key for " + a.keyName
}

func providerKeySourceLabel(source string) string {
	switch source {
	case "auth":
		return "stored in auth.json"
	case "models":
		return "using models.json"
	case "env":
		return "using environment"
	default:
		return "no key configured"
	}
}
