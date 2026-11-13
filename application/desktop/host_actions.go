package main

import (
	"context"
	"fmt"
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
	if err := e.sess.NavigateTree(context.Background(), branch[lastUser-1].ID, sdk.NavigateTreeOptions{}); err != nil {
		return err
	}
	s.Notices = append(s.Notices, message{Role: "notice", Time: "now", Text: "Rewound to the checkpoint before your last message."})
	a.renderSDKMessages(s, e.sess, nil)
	a.updateRealUsage(s, e.sess)
	s.Status = "done"
	s.Updated = "now"
	s.Modified = time.Now().UnixMilli()
	return nil
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
		ID: child.ID(), Path: child.File(), Project: s.Project, CWD: s.CWD, ParentPath: s.Path,
		Title: forkTitle(s.Title), Status: "done", Branch: s.Branch, Env: s.Env,
		Model: s.Model, ModelName: s.ModelName, ModelLimit: s.ModelLimit,
		Thinking: s.Thinking, Approval: s.Approval, Started: "now", Updated: "now",
		Modified: time.Now().UnixMilli(), ForkOf: s.ID, Real: true,
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
	svcs, err := h.servicesFor(s.CWD)
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

func (h *desktopHost) deleteSession(a *app, s *session) error {
	if s == nil {
		return nil
	}
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
		return nil
	}
	svcs, err := h.servicesFor(s.CWD)
	if err != nil {
		return err
	}
	rt, err := sdk.NewRuntime(sdk.RuntimeOptions{Services: svcs})
	if err != nil {
		return err
	}
	return rt.DeleteSession(s.Path)
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
