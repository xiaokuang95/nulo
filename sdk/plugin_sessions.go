package sdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/plugin"
	"github.com/xiaokuang95/nulo/sdk/internal/tools"
)

// pluginSessionOptions is the host-side shape of sdk/plugin.SessionOptions.
// It is intentionally independent of any agent type or prompt convention: a
// plugin decides what a subagent is, the host only runs the session.
type pluginSessionOptions struct {
	CWD            string   `json:"cwd"`
	Model          string   `json:"model"`
	ThinkingLevel  string   `json:"thinkingLevel"`
	SystemPrompt   string   `json:"systemPrompt"`
	Tools          []string `json:"tools"`
	NoTools        bool     `json:"noTools"`
	Persist        bool     `json:"persist"`
	Plugins        bool     `json:"plugins"`
	NoContextFiles bool     `json:"noContextFiles"`
	ResumeFrom     string   `json:"resumeFrom"`
}

// pluginChildSession is one independent session owned by a plugin. It has its
// own services, runtime, tools, model and history; it is not a branch of the
// parent session.
type pluginChildSession struct {
	id      string
	owner   string
	cwd     string
	runtime *Runtime
	session *Session

	mu     sync.Mutex
	closed bool
	last   *pluginChildResult
}

// pluginChildResult is the JSON shape returned to a plugin for send/result/
// wait. It mirrors sdk/plugin.SessionResult.
type pluginChildResult struct {
	ID           string           `json:"id,omitempty"`
	Text         string           `json:"text,omitempty"`
	Messages     []map[string]any `json:"messages,omitempty"`
	Usage        map[string]any   `json:"usage,omitempty"`
	StopReason   string           `json:"stopReason,omitempty"`
	ErrorMessage string           `json:"error,omitempty"`
}

// pluginChildStatus is the JSON shape returned by session.status.
type pluginChildStatus struct {
	ID            string         `json:"id"`
	CWD           string         `json:"cwd,omitempty"`
	File          string         `json:"file,omitempty"`
	Running       bool           `json:"running"`
	Disposed      bool           `json:"disposed"`
	Messages      int            `json:"messages"`
	Model         map[string]any `json:"model,omitempty"`
	ThinkingLevel string         `json:"thinkingLevel,omitempty"`
}

func newPluginChildID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "child-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("child-%d", time.Now().UnixNano())
}

func (s *AgentSessionServices) createPluginSession(ctx context.Context, arg map[string]any) (any, error) {
	var opts pluginSessionOptions
	raw, err := json.Marshal(arg)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, fmt.Errorf("session.create: %w", err)
	}

	cwd := strings.TrimSpace(opts.CWD)
	if cwd == "" {
		cwd = s.CWD
	} else if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(s.CWD, cwd)
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("session.create cwd: %w", err)
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return nil, fmt.Errorf("session.create cwd: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("session.create cwd is not a directory: %s", cwd)
	}

	parent := s.boundPluginSession()
	var model Model
	if strings.TrimSpace(opts.Model) != "" {
		model, err = BuildModel(opts.Model, s)
		if err != nil {
			return nil, fmt.Errorf("session.create model: %w", err)
		}
	} else if parent != nil {
		model = parent.Model()
	} else {
		model, err = BuildModel("", s)
		if err != nil {
			return nil, fmt.Errorf("session.create model: %w", err)
		}
	}

	thinking := ai.ThinkingLevel(opts.ThinkingLevel)
	if thinking == "" && parent != nil {
		thinking = parent.ThinkingLevel()
	}
	thinking = ai.ClampThinkingLevel(model, thinking)

	selectedTools, err := childToolsForOptions(cwd, opts)
	if err != nil {
		return nil, fmt.Errorf("session.create tools: %w", err)
	}

	projectTrusted := s.ProjectTrusted && samePath(cwd, s.CWD)
	childServices, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD:            cwd,
		AgentDir:       s.AgentDir,
		InMemory:       !opts.Persist,
		ProjectTrusted: projectTrusted,
		NoContextFiles: s.NoContextFiles || opts.NoContextFiles,
		StreamFn:       s.StreamFn,
		PluginPaths:    append([]string(nil), s.PluginPaths...),
		NoPlugins:      !opts.Plugins,
		PluginMode:     s.pluginMode,
	})
	if err != nil {
		return nil, fmt.Errorf("session.create services: %w", err)
	}
	// Providers registered by plugins are not in models.json, so carry the
	// parent's snapshot into the child. This keeps resume and any child model
	// lookup working without hardcoding provider names.
	s.providersMu.Lock()
	for name, provider := range s.dynamicProviders {
		childServices.dynamicProviders[name] = provider
	}
	s.providersMu.Unlock()

	runtime, err := NewRuntime(RuntimeOptions{Services: childServices})
	if err != nil {
		_ = childServices.Close()
		return nil, fmt.Errorf("session.create runtime: %w", err)
	}

	var session *Session
	if opts.ResumeFrom != "" {
		// ResumeSession rebuilds the session from the saved history; seed the
		// runtime with the plugin's tool choices so a resume does not silently
		// regain the default tool set.
		runtime.last = SessionStartOptions{
			SystemPrompt: opts.SystemPrompt,
			Tools:        selectedTools,
		}
		session, err = runtime.ResumeSession(opts.ResumeFrom)
	} else {
		session, err = runtime.New(SessionStartOptions{
			Model:         model,
			ThinkingLevel: thinking,
			SystemPrompt:  opts.SystemPrompt,
			Tools:         selectedTools,
			InMemory:      !opts.Persist,
		})
	}
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("session.create session: %w", err)
	}

	record := &pluginChildSession{
		id:      newPluginChildID(),
		owner:   plugin.NameFromContext(ctx),
		cwd:     cwd,
		runtime: runtime,
		session: session,
	}
	if err := s.storePluginChild(record); err != nil {
		_ = runtime.Close()
		return nil, err
	}

	return map[string]any{
		"id":            record.id,
		"cwd":           cwd,
		"model":         modelJSON(session.Model()),
		"thinkingLevel": string(session.ThinkingLevel()),
	}, nil
}

func childToolsForOptions(cwd string, opts pluginSessionOptions) ([]Tool, error) {
	if opts.NoTools {
		return []Tool{}, nil
	}
	if len(opts.Tools) == 0 {
		// nil tells SessionStartOptions to use the default tool set.
		return nil, nil
	}
	defaults := tools.Default(cwd)
	if len(opts.Tools) == 1 && (opts.Tools[0] == "*" || opts.Tools[0] == "all") {
		return defaults, nil
	}
	byName := make(map[string]Tool, len(defaults))
	for _, tool := range defaults {
		byName[tool.Name] = tool
	}
	out := make([]Tool, 0, len(opts.Tools))
	seen := map[string]bool{}
	for _, raw := range opts.Tools {
		name := strings.TrimSpace(raw)
		if name == "" || name == "none" {
			continue
		}
		if seen[name] {
			continue
		}
		tool, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool %q; available: %s", name, strings.Join(tools.Names(defaults), ", "))
		}
		seen[name] = true
		out = append(out, tool)
	}
	return out, nil
}

func (s *AgentSessionServices) storePluginChild(record *pluginChildSession) error {
	s.pluginMu.Lock()
	defer s.pluginMu.Unlock()
	if s.childSessions == nil {
		s.childSessions = map[string]*pluginChildSession{}
	}
	if s.childSessions[record.id] != nil {
		return fmt.Errorf("session.create: duplicate id %q", record.id)
	}
	s.childSessions[record.id] = record
	return nil
}

func (s *AgentSessionServices) pluginChild(ctx context.Context, id string) (*pluginChildSession, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("session id is required")
	}
	s.pluginMu.Lock()
	record := s.childSessions[id]
	s.pluginMu.Unlock()
	if record == nil {
		return nil, fmt.Errorf("unknown session %q", id)
	}
	owner := plugin.NameFromContext(ctx)
	if owner != "" && record.owner != "" && owner != record.owner {
		return nil, fmt.Errorf("session %q belongs to another plugin", id)
	}
	return record, nil
}

func (s *AgentSessionServices) removePluginChild(id string) {
	s.pluginMu.Lock()
	delete(s.childSessions, id)
	s.pluginMu.Unlock()
}

func (s *AgentSessionServices) closePluginChildren() {
	s.pluginMu.Lock()
	children := make([]*pluginChildSession, 0, len(s.childSessions))
	for _, child := range s.childSessions {
		children = append(children, child)
	}
	s.childSessions = map[string]*pluginChildSession{}
	s.pluginMu.Unlock()
	for _, child := range children {
		child.close()
	}
}

func (s *AgentSessionServices) pluginChildSend(ctx context.Context, arg map[string]any) (any, error) {
	child, err := s.pluginChild(ctx, stringArg(arg, "sessionId"))
	if err != nil {
		return nil, err
	}
	prompt := stringArg(arg, "prompt")
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("session.send: prompt is required")
	}
	if child.isClosed() {
		return nil, fmt.Errorf("session %q is disposed", child.id)
	}
	_, sendErr := child.session.Send(ctx, prompt)
	result := child.snapshot(sendErr)
	child.mu.Lock()
	child.last = &result
	child.mu.Unlock()
	return result, nil
}

func (s *AgentSessionServices) pluginChildSteer(ctx context.Context, arg map[string]any) (any, error) {
	child, err := s.pluginChild(ctx, stringArg(arg, "sessionId"))
	if err != nil {
		return nil, err
	}
	message := stringArg(arg, "message")
	if strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("session.steer: message is required")
	}
	return nil, child.session.Steer(ctx, ai.UserMessage(message, time.Now().UnixMilli()))
}

func (s *AgentSessionServices) pluginChildAbort(ctx context.Context, arg map[string]any) (any, error) {
	child, err := s.pluginChild(ctx, stringArg(arg, "sessionId"))
	if err != nil {
		return nil, err
	}
	return nil, child.session.Abort(ctx)
}

func (s *AgentSessionServices) pluginChildWait(ctx context.Context, arg map[string]any) (any, error) {
	child, err := s.pluginChild(ctx, stringArg(arg, "sessionId"))
	if err != nil {
		return nil, err
	}
	if err := child.session.WaitIdle(ctx); err != nil {
		return nil, err
	}
	child.mu.Lock()
	last := child.last
	child.mu.Unlock()
	if last != nil {
		return *last, nil
	}
	return child.snapshot(nil), nil
}

func (s *AgentSessionServices) pluginChildResult(ctx context.Context, arg map[string]any) (any, error) {
	child, err := s.pluginChild(ctx, stringArg(arg, "sessionId"))
	if err != nil {
		return nil, err
	}
	child.mu.Lock()
	last := child.last
	child.mu.Unlock()
	if last != nil {
		return *last, nil
	}
	return nil, fmt.Errorf("session %q has no completed result yet", child.id)
}

func (s *AgentSessionServices) pluginChildMessages(ctx context.Context, arg map[string]any) (any, error) {
	child, err := s.pluginChild(ctx, stringArg(arg, "sessionId"))
	if err != nil {
		return nil, err
	}
	return child.messages(), nil
}

func (s *AgentSessionServices) pluginChildStatus(ctx context.Context, arg map[string]any) (any, error) {
	child, err := s.pluginChild(ctx, stringArg(arg, "sessionId"))
	if err != nil {
		return nil, err
	}
	return child.status(), nil
}

func (s *AgentSessionServices) pluginChildList(ctx context.Context) (any, error) {
	owner := plugin.NameFromContext(ctx)
	s.pluginMu.Lock()
	children := make([]*pluginChildSession, 0, len(s.childSessions))
	for _, child := range s.childSessions {
		if owner == "" || child.owner == "" || child.owner == owner {
			children = append(children, child)
		}
	}
	s.pluginMu.Unlock()
	out := make([]pluginChildStatus, 0, len(children))
	for _, child := range children {
		out = append(out, child.status())
	}
	return out, nil
}

func (s *AgentSessionServices) pluginChildDispose(ctx context.Context, arg map[string]any) (any, error) {
	child, err := s.pluginChild(ctx, stringArg(arg, "sessionId"))
	if err != nil {
		return nil, err
	}
	child.close()
	s.removePluginChild(child.id)
	return nil, nil
}

func (c *pluginChildSession) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *pluginChildSession) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()
	_ = c.runtime.Close()
}

func (c *pluginChildSession) snapshot(err error) pluginChildResult {
	result := pluginChildResult{ID: c.id}
	if c.session != nil {
		if text := c.session.LastAssistantText(); text != nil {
			result.Text = *text
		}
		result.Messages = c.messages()
		result.Usage = childUsage(c.session.manager.Project().Messages)
		messages := c.session.manager.Project().Messages
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role != "assistant" {
				continue
			}
			result.StopReason = string(messages[i].StopReason)
			if messages[i].ErrorMessage != "" {
				result.ErrorMessage = messages[i].ErrorMessage
			}
			break
		}
	}
	if err != nil {
		result.ErrorMessage = err.Error()
	}
	return result
}

func (c *pluginChildSession) messages() []map[string]any {
	if c.session == nil || c.session.manager == nil {
		return []map[string]any{}
	}
	raw, err := json.Marshal(c.session.manager.Project().Messages)
	if err != nil {
		return []map[string]any{}
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return []map[string]any{}
	}
	return out
}

func (c *pluginChildSession) status() pluginChildStatus {
	status := pluginChildStatus{ID: c.id, CWD: c.cwd}
	c.mu.Lock()
	status.Disposed = c.closed
	c.mu.Unlock()
	if c.session != nil {
		status.Running = !c.session.isIdle()
		status.File = c.session.manager.File()
		status.Messages = len(c.session.manager.Project().Messages)
		status.Model = modelJSON(c.session.Model())
		status.ThinkingLevel = string(c.session.ThinkingLevel())
	}
	return status
}

func childUsage(messages []Message) map[string]any {
	var input, output, cacheRead, cacheWrite, total int
	var cost float64
	found := false
	for _, message := range messages {
		if message.Role != "assistant" || message.Usage == nil {
			continue
		}
		found = true
		input += message.Usage.Input
		output += message.Usage.Output
		cacheRead += message.Usage.CacheRead
		cacheWrite += message.Usage.CacheWrite
		total += message.Usage.TotalTokens
		cost += message.Usage.Cost.Total
	}
	if !found {
		return nil
	}
	return map[string]any{
		"input":       input,
		"output":      output,
		"cacheRead":   cacheRead,
		"cacheWrite":  cacheWrite,
		"totalTokens": total,
		"cost":        cost,
	}
}

func samePath(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}
