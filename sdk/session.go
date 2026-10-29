package sdk

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/codemode"
	"github.com/xiaokuang95/nulo/sdk/internal/mcp"
	"github.com/xiaokuang95/nulo/sdk/internal/prompt"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
	"github.com/xiaokuang95/nulo/sdk/internal/skills"
	"github.com/xiaokuang95/nulo/sdk/internal/tools"
)

type PromptOptions struct {
	StreamingBehavior string
	Source            string
}

type CompactionResult struct {
	Summary          string
	FirstKeptEntryID string
	TokensBefore     int
}

type NavigateTreeOptions struct {
	Summarize          bool
	CustomInstructions string
}

type Session struct {
	mu         sync.Mutex
	services   *AgentSessionServices
	manager    *session.Manager
	model      ai.Model
	catalog    []agent.AgentTool
	tools      []agent.AgentTool
	listeners  []func(Event)
	cancel     context.CancelFunc
	runDone    chan struct{}
	steering   []ai.Message
	followup   []ai.Message
	persistErr error
	streaming  bool
	disposed   bool
	system     string
	skills     []skills.Skill
	mcp        []*mcp.Client
}

func newSession(svcs *AgentSessionServices, mgr *session.Manager, opt SessionStartOptions) (*Session, error) {
	s := &Session{
		services: svcs,
		manager:  mgr,
		model:    opt.Model,
		system:   opt.SystemPrompt,
	}
	if opt.Tools == nil {
		s.tools = tools.Default(svcs.CWD)
	} else {
		s.tools = opt.Tools
	}
	if s.model.ID == "" {
		s.model = ai.DefaultFauxModel()
	}
	s.skills = skills.Discover(svcs.CWD, svcs.AgentDir, svcs.ProjectTrusted)
	cfgs := mcp.Load(svcs.AgentDir, svcs.CWD, svcs.ProjectTrusted)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	clients, mcpTools := mcp.ConnectAll(ctx, cfgs)
	s.mcp = clients
	s.tools = append(s.tools, mcpTools...)
	s.catalog = append([]agent.AgentTool(nil), s.tools...)
	s.ensureSystem()
	return s, nil
}

func (s *Session) ensureSystem() {
	sec := prompt.BuildSections(prompt.Options{
		CWD:            s.services.CWD,
		AgentDir:       s.services.AgentDir,
		CustomPrompt:   s.system,
		Tools:          s.tools,
		Skills:         s.skills,
		NoContextFiles: s.services.NoContextFiles,
		ProjectTrusted: s.services.ProjectTrusted,
	})
	wrapped := prompt.WrapSections(sec)
	for _, m := range s.manager.Project().Messages {
		if m.Role == "system" {
			return
		}
	}
	_, _ = s.manager.AppendMessage(ai.Message{
		Role:      "system",
		Timestamp: time.Now().UnixMilli(),
		Content:   ai.EncodeContentString(""),
		Sections:  wrapped,
	})
}

func (s *Session) Subscribe(fn func(Event)) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
	i := len(s.listeners) - 1
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if i < len(s.listeners) {
			s.listeners[i] = func(Event) {}
		}
	}
}

func (s *Session) fire(ev Event) {
	s.mu.Lock()
	ls := append([]func(Event){}, s.listeners...)
	s.mu.Unlock()
	for _, fn := range ls {
		fn(ev)
	}
}

func (s *Session) onAgent(ev agent.Event) {
	if ev.Type == "message_end" && ev.Message != nil {
		if _, err := s.manager.AppendMessage(*ev.Message); err != nil {
			s.mu.Lock()
			if s.persistErr == nil {
				s.persistErr = err
			}
			s.mu.Unlock()
		}
	}
	s.fire(ev)
	if ev.Type == "agent_end" {
		s.fire(agent.Event{Type: "agent_settled", Messages: ev.Messages})
	}
}

func (s *Session) Prompt(ctx context.Context, text string, opts ...PromptOptions) error {
	_, err := s.Send(ctx, text, opts...)
	return err
}

func (s *Session) Send(ctx context.Context, text string, opts ...PromptOptions) ([]Message, error) {
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return nil, fmt.Errorf("session disposed")
	}
	var opt PromptOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if s.streaming && opt.StreamingBehavior == "" {
		s.mu.Unlock()
		return nil, fmt.Errorf("session is streaming; set StreamingBehavior to steer or followUp")
	}
	if s.streaming {
		beh := opt.StreamingBehavior
		s.mu.Unlock()
		user := ai.UserMessage(text, time.Now().UnixMilli())
		if beh == "followUp" {
			return nil, s.FollowUp(ctx, user)
		}
		return nil, s.Steer(ctx, user)
	}
	s.streaming = true
	s.persistErr = nil
	s.runDone = make(chan struct{})
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.streaming = false
		s.cancel = nil
		close(s.runDone)
		s.runDone = nil
		s.mu.Unlock()
	}()

	if expanded, ok := skills.ExpandCommand(text, s.skills); ok {
		text = expanded
	}
	user := ai.UserMessage(text, time.Now().UnixMilli())
	cfg := s.config()
	ac := agent.Context{
		Messages: append([]ai.Message(nil), s.manager.Project().Messages...),
		Tools:    s.tools,
	}
	msgs := agent.Run(ctx, []ai.Message{user}, ac, cfg, s.onAgent)
	s.mu.Lock()
	err := s.persistErr
	s.mu.Unlock()
	return msgs, err
}

func (s *Session) config() agent.Config {
	fn := s.services.StreamFn
	if fn == nil {
		fn = ai.StreamForDir(s.services.AgentDir)
	}
	mgr := s.manager
	return agent.Config{
		Model:        s.model,
		ConvertToLlm: session.ConvertToLlm,
		StreamFn:     fn,
		GetSteering:  s.drainSteering,
		GetFollowUp:  s.drainFollowUp,
		PrepareRequest: func(ctx context.Context, c agent.Context, model ai.Model) agent.Context {
			c.Messages = mgr.Project().Messages
			return c
		},
	}
}

func (s *Session) Steer(_ context.Context, msg ai.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disposed {
		return fmt.Errorf("session disposed")
	}
	s.steering = append(s.steering, msg)
	return nil
}

func (s *Session) FollowUp(_ context.Context, msg ai.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disposed {
		return fmt.Errorf("session disposed")
	}
	s.followup = append(s.followup, msg)
	return nil
}

func (s *Session) drainSteering() []ai.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.steering) == 0 {
		return nil
	}
	msg := s.steering[0]
	s.steering = s.steering[1:]
	return []ai.Message{msg}
}

func (s *Session) drainFollowUp() []ai.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.followup) == 0 {
		return nil
	}
	msg := s.followup[0]
	s.followup = s.followup[1:]
	return []ai.Message{msg}
}

func (s *Session) Abort(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (s *Session) WaitIdle(ctx context.Context) error {
	s.mu.Lock()
	ch := s.runDone
	s.mu.Unlock()
	if ch == nil {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) LastAssistantText() *string {
	proj := s.manager.Project()
	for i := len(proj.Messages) - 1; i >= 0; i-- {
		if proj.Messages[i].Role == "assistant" {
			t := ai.MessageText(proj.Messages[i])
			if t == "" {
				return nil
			}
			return &t
		}
	}
	return nil
}

func (s *Session) Dispose() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disposed = true
	if s.cancel != nil {
		s.cancel()
	}
	for _, c := range s.mcp {
		c.Close()
	}
	s.mcp = nil
	s.listeners = nil
}

func (s *Session) Close() error {
	s.Dispose()
	return nil
}

func (s *Session) Compact(ctx context.Context, instructions string) (*CompactionResult, error) {
	_ = s.Abort(ctx)
	if err := s.WaitIdle(ctx); err != nil {
		return nil, err
	}
	s.fire(Event{Type: "compaction_start"})
	path := s.manager.Branch()
	if err := session.CanCompact(path); err != nil {
		s.fire(Event{Type: "compaction_end"})
		return nil, err
	}
	cut := session.FindCutIndex(path, session.DefaultKeepRecentTokens)
	if cut <= 0 || cut >= len(path) {
		s.fire(Event{Type: "compaction_end"})
		return nil, session.CanCompact(nil)
	}
	firstKept := path[cut].ID
	var toSum []ai.Message
	for i := 0; i < cut; i++ {
		toSum = append(toSum, session.ConvertToLlm(entryMsgs(path[i]))...)
	}
	serialized := session.SerializeConversation(toSum)
	prompt := serialized
	if instructions != "" {
		prompt = serialized + "\n\nAdditional instructions:\n" + instructions
	}
	summary, err := s.summarize(ctx, prompt)
	if err != nil {
		s.fire(Event{Type: "compaction_end"})
		return nil, err
	}
	var tokens int
	for _, m := range toSum {
		tokens += session.EstimateTokens(m)
	}
	sys := sessionSys(s.manager.Project())
	if _, err := s.manager.AppendCompaction(summary, firstKept, tokens, sys); err != nil {
		s.fire(Event{Type: "compaction_end"})
		return nil, err
	}
	s.fire(Event{Type: "compaction_end"})
	return &CompactionResult{Summary: summary, FirstKeptEntryID: firstKept, TokensBefore: tokens}, nil
}

func entryMsgs(e session.Entry) []ai.Message {
	if e.Type == "message" && e.Message != nil {
		return []ai.Message{*e.Message}
	}
	return nil
}

func sessionSys(p session.Projection) *ai.Message {
	for i := range p.Messages {
		if p.Messages[i].Role == "system" {
			m := p.Messages[i]
			return &m
		}
	}
	return nil
}

func (s *Session) summarize(ctx context.Context, serialized string) (string, error) {
	fn := s.services.StreamFn
	if fn == nil {
		fn = ai.StreamForDir(s.services.AgentDir)
	}
	tr := ai.Transcript{Messages: []ai.Message{
		ai.SystemMessage(session.SummarizationSystemPrompt, time.Now().UnixMilli()),
		ai.UserMessage(serialized, time.Now().UnixMilli()),
	}}
	stream := fn(ctx, s.model, tr, ai.StreamOptions{})
	for range stream.Events() {
	}
	msg := stream.Result()
	if msg.StopReason == ai.StopError || msg.StopReason == ai.StopAborted {
		return "", fmt.Errorf("summarize: %s", msg.ErrorMessage)
	}
	return ai.MessageText(msg), nil
}

func (s *Session) NavigateTree(ctx context.Context, entryID string, opt NavigateTreeOptions) error {
	s.mu.Lock()
	busy := s.streaming
	s.mu.Unlock()
	if busy {
		_ = s.Abort(ctx)
		if err := s.WaitIdle(ctx); err != nil {
			return err
		}
	}
	oldLeaf := s.manager.LeafID()
	if entryID == oldLeaf {
		return nil
	}
	if err := s.manager.SetLeaf(entryID); err != nil {
		return err
	}
	if !opt.Summarize {
		return nil
	}
	if s.model.ID == "" {
		_ = s.manager.SetLeaf(oldLeaf)
		return fmt.Errorf("No model available for summarization")
	}
	pathOld := buildPathTo(s.manager, oldLeaf)
	pathNew := s.manager.Branch()
	anc := commonAncestor(pathOld, pathNew)
	var toSum []ai.Message
	collect := false
	for _, e := range pathOld {
		if anc != "" && e.ID == anc {
			collect = true
			continue
		}
		if collect || anc == "" {
			toSum = append(toSum, entryMsgs(e)...)
		}
	}
	serialized := session.SerializeConversation(session.ConvertToLlm(toSum))
	if opt.CustomInstructions != "" {
		serialized += "\n\n" + opt.CustomInstructions
	}
	summary, err := s.summarize(ctx, serialized)
	if err != nil {
		_ = s.manager.SetLeaf(oldLeaf)
		return err
	}
	// leaf is target; append summary as child of target so it becomes new leaf
	_, err = s.manager.AppendBranchSummary(oldLeaf, summary)
	return err
}

func buildPathTo(m *session.Manager, leaf string) []session.Entry {
	cur := m.LeafID()
	_ = m.SetLeaf(leaf)
	p := m.Branch()
	_ = m.SetLeaf(cur)
	return p
}

func commonAncestor(a, b []session.Entry) string {
	seen := map[string]bool{}
	for _, e := range b {
		seen[e.ID] = true
	}
	for i := len(a) - 1; i >= 0; i-- {
		if seen[a[i].ID] {
			return a[i].ID
		}
	}
	return ""
}

func (s *Session) Manager() *session.Manager { return s.manager }

func (s *Session) SetActiveTools(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range names {
		if n == "codemode" || n == "+codemode" {
			s.ensureCodemodeLocked()
		}
	}
	s.tools = selectTools(s.catalog, s.tools, names)
}

func (s *Session) ensureCodemodeLocked() {
	for _, t := range s.catalog {
		if t.Name == "codemode" {
			return
		}
	}
	cm := codemode.Tool(s.catalog, s.loadCMStore, s.saveCMStore)
	s.catalog = append(s.catalog, cm)
}

func selectTools(catalog, current []agent.AgentTool, names []string) []agent.AgentTool {
	byName := map[string]agent.AgentTool{}
	for _, t := range catalog {
		byName[t.Name] = t
	}
	modsOnly := true
	for _, n := range names {
		if n != "" && !strings.HasPrefix(n, "+") && !strings.HasPrefix(n, "-") {
			modsOnly = false
			break
		}
	}
	selected := map[string]bool{}
	if modsOnly {
		for _, t := range current {
			selected[t.Name] = true
		}
		for _, n := range names {
			switch {
			case strings.HasPrefix(n, "+"):
				selected[strings.TrimPrefix(n, "+")] = true
			case strings.HasPrefix(n, "-"):
				delete(selected, strings.TrimPrefix(n, "-"))
			}
		}
	} else {
		for _, n := range names {
			if strings.HasPrefix(n, "-") {
				continue
			}
			selected[strings.TrimPrefix(n, "+")] = true
		}
	}
	var out []agent.AgentTool
	for _, t := range catalog {
		if selected[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

func (s *Session) loadCMStore() map[string]any {
	entries := s.manager.Branch()
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.Type == "custom" && e.CustomType == codemode.StoreType() {
			return codemode.DecodeStore(e.Data)
		}
	}
	return map[string]any{}
}

func (s *Session) saveCMStore(m map[string]any) {
	_, _ = s.manager.AppendCustom(codemode.StoreType(), codemode.EncodeStore(m))
}
