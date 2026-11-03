package sdk

import (
	"fmt"
	"sync/atomic"
)

// SessionOptions configures an independent child session created by a plugin.
//
// The host does not impose an agent type, prompt template, routing rule, or
// result format. A plugin is free to implement its own subagent convention,
// agent files, concurrency and notification policy on top of these options.
type SessionOptions struct {
	// CWD is the child session's working directory. Empty means the parent
	// session's CWD.
	CWD string `json:"cwd,omitempty"`

	// Model is a provider/model id or a fuzzy model name. Empty inherits the
	// parent session's model.
	Model string `json:"model,omitempty"`

	// ThinkingLevel is one of off, minimal, low, medium, high, xhigh, max.
	// Empty inherits the parent session's level.
	ThinkingLevel string `json:"thinkingLevel,omitempty"`

	// SystemPrompt replaces the child's default preamble. Empty lets the host
	// build its normal prompt. Context files still follow unless NoContextFiles
	// is set.
	SystemPrompt string `json:"systemPrompt,omitempty"`

	// Tools selects built-in tools by name. Empty means the host's default
	// tool set; use NoTools to explicitly start with no tools.
	Tools []string `json:"tools,omitempty"`

	// NoTools starts the child session without tools.
	NoTools bool `json:"noTools,omitempty"`

	// Persist writes the child session to the normal session directory. The
	// default is false, so child conversations stay in memory unless the
	// plugin chooses otherwise.
	Persist bool `json:"persist,omitempty"`

	// Plugins loads the host's normal plugins into the child session. The
	// default is false to avoid recursive orchestration and duplicated plugin
	// side effects.
	Plugins bool `json:"plugins,omitempty"`

	// NoContextFiles skips AGENTS.md/CLAUDE.md discovery for the child.
	NoContextFiles bool `json:"noContextFiles,omitempty"`

	// ResumeFrom reopens a persisted child session file instead of starting a
	// new conversation. Use Status().File or Session.File to get the path.
	// Model, thinking level and the saved system message come from the session
	// file; pass Tools again because the active tool list is rebuilt.
	ResumeFrom string `json:"resumeFrom,omitempty"`
}

// SessionResult is the result of one child prompt.
type SessionResult struct {
	ID           string           `json:"id,omitempty"`
	Text         string           `json:"text,omitempty"`
	Messages     []map[string]any `json:"messages,omitempty"`
	Usage        map[string]any   `json:"usage,omitempty"`
	StopReason   string           `json:"stopReason,omitempty"`
	ErrorMessage string           `json:"error,omitempty"`
}

// SessionStatus describes a child session without running it.
type SessionStatus struct {
	ID            string `json:"id"`
	CWD           string `json:"cwd,omitempty"`
	File          string `json:"file,omitempty"`
	Running       bool   `json:"running"`
	Disposed      bool   `json:"disposed"`
	Messages      int    `json:"messages"`
	Model         Model  `json:"model,omitempty"`
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
}

// Session is an independent child session. It does not replace the host's
// current session and is not visible in the parent conversation unless the
// plugin chooses to send a message there.
type Session struct {
	api      *API
	id       string
	cwd      string
	disposed atomic.Bool
}

// ListSessions lists the child sessions created by this plugin.
func (a *API) ListSessions() ([]SessionStatus, error) {
	if err := a.requireReady(); err != nil {
		return nil, err
	}
	var out []SessionStatus
	if err := a.server.hostCall("session.list", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSession creates an independent child session. It is generic on
// purpose: the plugin owns the subagent semantics, while the host owns the
// session lifecycle.
func (a *API) CreateSession(options SessionOptions) (*Session, error) {
	if err := a.requireReady(); err != nil {
		return nil, err
	}
	var out struct {
		ID            string `json:"id"`
		CWD           string `json:"cwd"`
		Model         Model  `json:"model"`
		ThinkingLevel string `json:"thinkingLevel"`
	}
	if err := a.server.hostCall("session.create", options, &out); err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, fmt.Errorf("session.create returned no id")
	}
	return &Session{api: a, id: out.ID, cwd: out.CWD}, nil
}

func (a *API) requireReady() error {
	a.server.mu.Lock()
	ready := a.server.ready
	a.server.mu.Unlock()
	if !ready {
		return fmt.Errorf("plugin API is not ready yet; create child sessions from a tool, command or session event, not from the plugin factory")
	}
	return nil
}

// ID returns the host-assigned child session id.
func (s *Session) ID() string { return s.id }

// CWD returns the child session's working directory.
func (s *Session) CWD() string { return s.cwd }

// Send runs one prompt to completion and returns the final result.
func (s *Session) Send(prompt string) (SessionResult, error) {
	var result SessionResult
	if err := s.api.server.hostCall("session.send", map[string]any{
		"sessionId": s.id,
		"prompt":    prompt,
	}, &result); err != nil {
		return result, err
	}
	if result.ErrorMessage != "" {
		return result, fmt.Errorf("%s", result.ErrorMessage)
	}
	return result, nil
}

// Prompt runs one prompt and discards the result payload.
func (s *Session) Prompt(prompt string) error {
	_, err := s.Send(prompt)
	return err
}

// Steer injects a message into a running child session.
func (s *Session) Steer(message string) error {
	return s.api.server.hostCallVoid("session.steer", map[string]any{
		"sessionId": s.id,
		"message":   message,
	})
}

// Abort cancels the child session's active run.
func (s *Session) Abort() error {
	return s.api.server.hostCallVoid("session.abort", map[string]any{"sessionId": s.id})
}

// Wait waits for the current run to settle and returns the last result.
func (s *Session) Wait() (SessionResult, error) {
	var result SessionResult
	if err := s.api.server.hostCall("session.wait", map[string]any{"sessionId": s.id}, &result); err != nil {
		return result, err
	}
	if result.ErrorMessage != "" {
		return result, fmt.Errorf("%s", result.ErrorMessage)
	}
	return result, nil
}

// Result returns the last completed result without waiting.
func (s *Session) Result() (SessionResult, error) {
	var result SessionResult
	if err := s.api.server.hostCall("session.result", map[string]any{"sessionId": s.id}, &result); err != nil {
		return result, err
	}
	if result.ErrorMessage != "" {
		return result, fmt.Errorf("%s", result.ErrorMessage)
	}
	return result, nil
}

// Messages returns the child session's current projected conversation.
func (s *Session) Messages() ([]map[string]any, error) {
	var messages []map[string]any
	if err := s.api.server.hostCall("session.messages", map[string]any{"sessionId": s.id}, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// Status returns the child session's running state and model information.
func (s *Session) Status() (SessionStatus, error) {
	var status SessionStatus
	if err := s.api.server.hostCall("session.status", map[string]any{"sessionId": s.id}, &status); err != nil {
		return status, err
	}
	return status, nil
}

// File returns the persisted session file, or "" for an in-memory session.
func (s *Session) File() (string, error) {
	status, err := s.Status()
	if err != nil {
		return "", err
	}
	return status.File, nil
}

// Dispose releases the child session and its MCP connections. It is safe to
// call more than once.
func (s *Session) Dispose() error {
	if s.disposed.Swap(true) {
		return nil
	}
	return s.api.server.hostCallVoid("session.dispose", map[string]any{"sessionId": s.id})
}
