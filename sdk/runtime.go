package sdk

import (
	"fmt"

	"github.com/xiaokuang95/nulo/sdk/internal/session"
)

type Runtime struct {
	Services *AgentSessionServices
	session  *Session
	last     SessionStartOptions
}

type RuntimeOptions struct {
	Services *AgentSessionServices
}

func NewRuntime(opt RuntimeOptions) (*Runtime, error) {
	if opt.Services == nil {
		return nil, fmt.Errorf("runtime: Services required")
	}
	return &Runtime{Services: opt.Services}, nil
}

func (r *Runtime) Session() *Session { return r.session }

func (r *Runtime) Close() error {
	if r.session != nil {
		r.session.Dispose()
	}
	return r.Services.Close()
}

type SessionStartOptions struct {
	Model        Model
	SystemPrompt string
	Tools        []Tool
	InMemory     bool
}

func (r *Runtime) New(opt SessionStartOptions) (*Session, error) {
	return r.replace(r.newManager(opt), opt)
}

func (r *Runtime) NewSession() (*Session, error) {
	opt := r.last
	return r.replace(r.newManager(opt), opt)
}

func (r *Runtime) SwitchSession(path string) (*Session, error) {
	mgr, err := session.Open(path)
	if err != nil {
		return nil, err
	}
	return r.replace(mgr, r.last)
}

func (r *Runtime) Fork() (*Session, error) {
	if r.session == nil {
		return nil, fmt.Errorf("runtime: no session")
	}
	child, err := r.session.manager.Fork()
	if err != nil {
		return nil, err
	}
	return r.replace(child, r.last)
}

func (r *Runtime) ImportFromJsonl(path string) (*Session, error) {
	return r.SwitchSession(path)
}

func (r *Runtime) newManager(opt SessionStartOptions) *session.Manager {
	inMemory := opt.InMemory || r.Services.InMemory
	if inMemory {
		return session.InMemory(r.Services.CWD)
	}
	return session.Create(r.Services.CWD, session.SessionDir(r.Services.AgentDir, r.Services.CWD))
}

func (r *Runtime) replace(mgr *session.Manager, opt SessionStartOptions) (*Session, error) {
	sess, err := newSession(r.Services, mgr, opt)
	if err != nil {
		return nil, err
	}
	if r.session != nil {
		r.session.Dispose()
	}
	r.session = sess
	r.last = opt
	return sess, nil
}
