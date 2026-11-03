package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
	mu               sync.Mutex
	services         *AgentSessionServices
	manager          *session.Manager
	settings         *SettingsManager
	model            ai.Model
	thinking         ai.ThinkingLevel
	warmMode         string
	warmRequest      *warmRequest
	warm             *warmState
	catalog          []agent.AgentTool
	tools            []agent.AgentTool
	listeners        []*subscription
	cancel           context.CancelFunc
	runDone          chan struct{}
	steering         []ai.Message
	followup         []ai.Message
	persistErr       error
	streaming        bool
	disposed         bool
	system           string
	skills           []skills.Skill
	skillDiagnostics []skills.Diagnostic
	mcp              []*mcp.Client
	servers          []mcp.Connected

	// Per-tool exposure from mcp.json; a server's tools can be split across
	// direct, codemode, deferred, and hidden.
	toolExposure   map[string]mcp.Exposure
	toolNamespace  map[string]string
	pendingTools   []agent.AgentTool
	mcpDiagnostics []string

	// Native plugins. The host is owned by Services; these maps bind its
	// registrations to this session.
	pluginToolByName map[string]pluginToolBinding
	pluginSystem     string
	runCtx           context.Context
	turnIndex        int
	shutdownSent     bool
}

func newSession(svcs *AgentSessionServices, mgr *session.Manager, opt SessionStartOptions) (*Session, error) {
	s := &Session{
		services:         svcs,
		manager:          mgr,
		settings:         svcs.SettingsManager,
		model:            opt.Model,
		thinking:         opt.ThinkingLevel,
		system:           opt.SystemPrompt,
		pluginToolByName: map[string]pluginToolBinding{},
	}
	var configuredTools []string
	if opt.Tools == nil {
		s.tools = tools.Default(svcs.CWD)
		if s.settings != nil {
			if configured := s.settings.GetDefaultTools(); configured != nil {
				configuredTools = configured
				s.tools = filterToolsByName(s.tools, configured)
			}
		}
	} else {
		s.tools = opt.Tools
	}
	if s.model.ID == "" {
		s.model = ai.DefaultFauxModel()
	}
	if s.thinking == "" {
		// Resuming a session restores the level it recorded, as Pi does.
		s.thinking = ai.ThinkingLevel(mgr.Project().ThinkingLevel)
	}
	if s.thinking != "" {
		s.thinking = ai.ClampThinkingLevel(s.model, s.thinking)
	}
	if opt.CacheWarming == "" && s.settings != nil {
		opt.CacheWarming = s.settings.GetCacheWarmingMode()
	}
	warmMode, err := cacheWarmingMode(opt.CacheWarming)
	if err != nil {
		return nil, err
	}
	s.warmMode = warmMode
	if len(mgr.Project().Messages) == 0 {
		// Pi records the initial model and thinking level on a session with
		// no messages, so resuming restores both.
		if s.model.Provider != "" || s.model.ID != "" {
			if _, err := mgr.AppendModelChange(s.model.Provider, s.model.ID); err != nil {
				return nil, err
			}
		}
		if s.thinking != "" {
			if _, err := mgr.AppendThinkingLevel(string(s.thinking)); err != nil {
				return nil, err
			}
		}
	}
	var skillPaths []string
	if s.settings != nil {
		skillPaths = s.settings.GetSkillPaths()
	}
	if svcs.plugins != nil {
		s.emitPlugin(context.Background(), "project_trust", map[string]any{
			"type": "project_trust", "cwd": svcs.CWD, "trusted": svcs.ProjectTrusted,
		})
		discovered := s.emitPlugin(context.Background(), "resources_discover", map[string]any{
			"type": "resources_discover", "cwd": svcs.CWD, "reason": "startup",
		})
		for _, path := range stringSliceArg(discovered, "skillPaths") {
			skillPaths = append(skillPaths, path)
		}
	}
	loaded := skills.LoadSkills(skills.LoadOptions{
		Cwd:             svcs.CWD,
		AgentDir:        svcs.AgentDir,
		ProjectTrusted:  svcs.ProjectTrusted,
		IncludeDefaults: true,
		SkillPaths:      skillPaths,
	})
	s.skills = loaded.Skills
	s.skillDiagnostics = loaded.Diagnostics
	cfg := mcp.Load(svcs.AgentDir, svcs.CWD, svcs.ProjectTrusted)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The catalogue starts as the built-in tools; a host may have narrowed the
	// active set, but codemode can still call everything the catalogue holds.
	s.catalog = append([]agent.AgentTool(nil), s.tools...)

	// Exposure decides where each tool lands. codemode keeps them out of the
	// model's tool list so a large server does not cost every request; deferred
	// waits for tool_search; direct declares them like built-in tools; hidden
	// drops them.
	s.toolExposure = map[string]mcp.Exposure{}
	s.toolNamespace = map[string]string{}
	s.mcpDiagnostics = append(s.mcpDiagnostics, cfg.Errors...)
	// Extension-registered servers join the configured set before connecting.
	// A file entry of the same name wins, as Pi's mcp.json precedence does.
	if svcs.plugins != nil {
		svcs.pluginMu.Lock()
		for name, server := range svcs.pluginMCP {
			if _, exists := cfg.Servers[name]; !exists {
				cfg.Servers[name] = server
			}
		}
		svcs.pluginMu.Unlock()
	}

	s.servers = mcp.ConnectAll(ctx, cfg.Servers)
	autoCodemode := false
	deferred := false
	for _, srv := range s.servers {
		s.mcp = append(s.mcp, srv.Client)
		// Pi decides from the configuration, not from the tools that connected,
		// so an exposure with no matching tool still turns its mechanism on.
		if srv.Exposures[mcp.ExposureCodemode] {
			autoCodemode = true
		}
		if srv.Exposures[mcp.ExposureDeferred] {
			deferred = true
		}
		for _, entry := range srv.Tools {
			s.toolExposure[entry.Tool.Name] = entry.Exposure
			s.toolNamespace[entry.Tool.Name] = mcp.Namespace(srv.Name)
			switch entry.Exposure {
			case mcp.ExposureHidden:
				// Not reachable at all.
			case mcp.ExposureDirect:
				s.tools = append(s.tools, entry.Tool)
				s.catalog = append(s.catalog, entry.Tool)
			case mcp.ExposureDeferred, mcp.ExposureCodemode:
				s.catalog = append(s.catalog, entry.Tool)
			}
		}
	}
	// Native plugin tools join the catalogue with Pi's exposure rules.
	s.bindPluginTools()
	for _, binding := range s.pluginToolByName {
		switch binding.spec.ToolExposure() {
		case "codemode":
			autoCodemode = true
		case "deferred":
			deferred = true
		}
	}
	// Deferred tools are only reachable once tool_search loads them.
	if deferred && !hasTool(s.tools, "tool_search") {
		s.tools = append(s.tools, s.toolSearchTool())
	}
	// Pi turns codemode on so a codemode-exposed server is actually reachable,
	// or when defaultTools selects it explicitly.
	if autoCodemode && cfg.AutoEnableCodemode {
		s.ensureCodemodeLocked(true)
	}
	if containsString(configuredTools, "codemode") {
		s.ensureCodemodeLocked(true)
	}
	s.ensureSystem()
	svcs.bindPluginSession(s)
	reason := opt.startReason
	if reason == "" {
		reason = "startup"
	}
	startData := map[string]any{"type": "session_start", "reason": reason}
	if opt.previousSessionFile != "" {
		startData["previousSessionFile"] = opt.previousSessionFile
	}
	s.emitPlugin(context.Background(), "session_start", startData)
	return s, nil
}

func hasTool(tools []agent.AgentTool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func filterToolsByName(all []agent.AgentTool, names []string) []agent.AgentTool {
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	out := make([]agent.AgentTool, 0, len(all))
	for _, tool := range all {
		if wanted[tool.Name] {
			out = append(out, tool)
		}
	}
	return out
}

func (s *Session) ensureSystem() {
	sec := prompt.BuildSections(prompt.Options{
		CWD:            s.services.CWD,
		AgentDir:       s.services.AgentDir,
		CustomPrompt:   s.system,
		Tools:          s.tools,
		Skills:         s.skills,
		MCPServers:     s.mcpPromptServers(),
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

// Subscribe adds an event listener and returns a function that removes it.
//
// Every event goes to each listener registered on this session when it is
// emitted, once per listener, so several listeners on one session all see the
// same events in the same order; a listener removed during a dispatch is not
// called again. A listener runs on the goroutine driving the session's run and
// blocks that run while it executes: keep it short and off the session's own
// Send. Listeners of different sessions may run at the same time, so a listener
// shared by several sessions must guard its own state; a listener of one
// session is never called concurrently with itself.
func (s *Session) Subscribe(fn func(Event)) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := &subscription{fn: fn}
	s.listeners = append(s.listeners, sub)
	// The token is the subscription itself, not its index: an index could name a
	// later listener once the list is reset (Dispose) and refilled.
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		sub.fn = nil
	}
}

// subscription is one listener. Unsubscribing clears fn instead of removing the
// entry, so a dispatch already under way keeps its snapshot.
type subscription struct{ fn func(Event) }

func (s *Session) fire(ev Event) {
	s.mu.Lock()
	ls := append([]*subscription(nil), s.listeners...)
	s.mu.Unlock()
	for _, sub := range ls {
		if sub.fn != nil {
			sub.fn(ev)
		}
	}
}

func (s *Session) onAgent(ev agent.Event) {
	if replacement := s.pluginOnAgent(ev); replacement != nil && ev.Type == "message_end" {
		ev.Message = replacement
	}
	switch {
	case ev.Type == "message_start" && ev.Message != nil && ev.Message.Role == "assistant":
		// The next request refreshes the cache itself.
		s.cancelWarm()
	case ev.Type == "message_end" && ev.Message != nil && ev.Message.Role == "assistant" && hasToolCalls(*ev.Message):
		// Tool execution may outlive the cache lifetime, so warm while it runs.
		s.scheduleWarm("streaming")
	}
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
		s.scheduleWarm("idle")
		s.pluginBeforeSettle(ev.Messages)
		settled := agent.Event{Type: "agent_settled", Messages: ev.Messages}
		s.pluginOnAgent(settled)
		s.fire(settled)
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
	s.cancelWarm()
	defer func() {
		cancel()
		s.mu.Lock()
		s.streaming = false
		s.cancel = nil
		s.runCtx = nil
		s.pluginSystem = ""
		close(s.runDone)
		s.runDone = nil
		s.mu.Unlock()
	}()

	s.mu.Lock()
	s.runCtx = ctx
	s.turnIndex = 0
	s.pluginSystem = ""
	s.mu.Unlock()
	if transformed, ok := s.pluginInput(ctx, text, sourceForPrompt(opt), opt.StreamingBehavior); !ok {
		return nil, nil
	} else {
		text = transformed
	}
	if expanded, ok := skills.ExpandCommand(text, s.skills); ok {
		text = expanded
	}
	s.pluginBeforeAgentStart(ctx, text)
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
		Model:         s.model,
		ThinkingLevel: s.thinking,
		ConvertToLlm:  session.ConvertToLlm,
		StreamFn:      fn,
		GetSteering:   s.drainSteering,
		GetFollowUp:   s.drainFollowUp,
		StreamOptions: s.pluginStreamOptions(),
		AfterToolBatch: func(c *agent.Context) {
			s.mu.Lock()
			pending := s.pendingTools
			s.pendingTools = nil
			s.mu.Unlock()
			if len(pending) > 0 {
				c.Tools = append(c.Tools, pending...)
			}
		},
		BeforeToolCall: func(ctx context.Context, assistant ai.Message, call ai.ContentBlock, args map[string]any) (*agent.BeforeToolCallResult, error) {
			return s.pluginBeforeToolCall(ctx, assistant, call, args)
		},
		AfterToolCall: func(ctx context.Context, assistant ai.Message, call ai.ContentBlock, args map[string]any, result agent.ToolResult, isError bool) (*agent.AfterToolCallResult, error) {
			return s.pluginAfterToolCall(ctx, assistant, call, args, result, isError)
		},
		PrepareRequest: func(ctx context.Context, c agent.Context, model ai.Model) agent.Context {
			c.Messages = s.pluginPrepareRequest(ctx, mgr.Project().Messages)
			s.captureWarmRequest(model, c.Messages)
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

// shutdown emits the session_shutdown event once, with Pi's replacement reason.
func (s *Session) shutdown(reason, target string) {
	s.mu.Lock()
	if s.shutdownSent {
		s.mu.Unlock()
		return
	}
	s.shutdownSent = true
	s.mu.Unlock()
	if reason == "" {
		reason = "quit"
	}
	data := map[string]any{"type": "session_shutdown", "reason": reason}
	if target != "" {
		data["targetSessionFile"] = target
	}
	s.emitPlugin(context.Background(), "session_shutdown", data)
}

func (s *Session) Dispose() {
	s.shutdown("quit", "")
	if s.services != nil {
		s.services.unbindPluginSession(s)
	}
	s.cancelWarm()
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
	completed := false
	var failure string
	defer func() {
		if completed {
			return
		}
		s.emitPlugin(ctx, "session_compact_failed", map[string]any{
			"type": "session_compact_failed", "reason": "manual", "aborted": false,
			"errorMessage": failure, "willRetry": false, "fromExtension": false,
		})
	}()
	path := s.manager.Branch()
	before := s.emitPlugin(ctx, "session_before_compact", map[string]any{
		"type":               "session_before_compact",
		"preparation":        map[string]any{"targetId": s.manager.LeafID()},
		"branchEntries":      sessionEntriesJSON(s),
		"customInstructions": instructions,
		"reason":             "manual",
		"willRetry":          false,
	})
	if cancelled, _ := before["cancel"].(bool); cancelled {
		failure = "compaction cancelled by plugin"
		s.fire(Event{Type: "compaction_end"})
		return nil, fmt.Errorf("%s", failure)
	}
	if supplied, ok := before["compaction"].(map[string]any); ok {
		summary, _ := supplied["summary"].(string)
		firstKept, _ := supplied["firstKeptEntryId"].(string)
		if summary == "" {
			failure = "plugin supplied an empty compaction summary"
			s.fire(Event{Type: "compaction_end"})
			return nil, fmt.Errorf("%s", failure)
		}
		details, _ := json.Marshal(supplied["details"])
		var usage *ai.Usage
		if raw, ok := supplied["usage"]; ok {
			var parsed ai.Usage
			if data, err := json.Marshal(raw); err == nil && json.Unmarshal(data, &parsed) == nil {
				usage = &parsed
			}
		}
		entryID, err := s.manager.AppendCompaction(summary, firstKept, 0, details, true, usage, nil)
		if err != nil {
			failure = err.Error()
			s.fire(Event{Type: "compaction_end"})
			return nil, err
		}
		completed = true
		s.emitPlugin(ctx, "session_compact", map[string]any{
			"type": "session_compact", "compactionEntry": map[string]any{"id": entryID, "summary": summary, "firstKeptEntryId": firstKept},
			"fromExtension": true, "reason": "manual", "willRetry": false,
		})
		s.fire(Event{Type: "compaction_end"})
		return &CompactionResult{Summary: summary, FirstKeptEntryID: firstKept}, nil
	}
	keepRecent := session.DefaultKeepRecentTokens
	if s.settings != nil {
		if value, err := s.settings.GetCompactionKeepRecentTokens(s.model); err == nil {
			keepRecent = value
		}
	}
	if err := session.CanCompactWith(path, keepRecent); err != nil {
		failure = err.Error()
		s.fire(Event{Type: "compaction_end"})
		return nil, err
	}
	cut := session.FindCutIndex(path, keepRecent)
	if cut <= 0 || cut >= len(path) {
		err := session.CanCompact(nil)
		failure = err.Error()
		s.fire(Event{Type: "compaction_end"})
		return nil, err
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
	summary, usage, err := s.summarize(ctx, prompt)
	if err != nil {
		failure = err.Error()
		s.fire(Event{Type: "compaction_end"})
		return nil, err
	}
	var tokens int
	for _, m := range toSum {
		tokens += session.EstimateTokens(m)
	}
	sys := sessionSys(s.manager.Project())
	if sys != nil {
		sys.Timestamp = time.Now().UnixMilli()
	}
	entryID, err := s.manager.AppendCompaction(summary, firstKept, tokens, nil, false, usage, sys)
	if err != nil {
		failure = err.Error()
		s.fire(Event{Type: "compaction_end"})
		return nil, err
	}
	completed = true
	s.emitPlugin(ctx, "session_compact", map[string]any{
		"type": "session_compact", "compactionEntry": map[string]any{"id": entryID, "summary": summary, "firstKeptEntryId": firstKept},
		"fromExtension": false, "reason": "manual", "willRetry": false,
	})
	s.fire(Event{Type: "compaction_end"})
	return &CompactionResult{Summary: summary, FirstKeptEntryID: firstKept, TokensBefore: tokens}, nil
}

func entryMsgs(e session.Entry) []ai.Message {
	if e.Type == "message" && e.Message != nil {
		return []ai.Message{*e.Message}
	}
	return nil
}

// sessionSys is the current prompt state at a compaction boundary: Pi
// replays every system message on the branch into one snapshot.
func sessionSys(p session.Projection) *ai.Message {
	return ai.GetCurrentSystemMessage(p.Messages)
}

func (s *Session) summarize(ctx context.Context, serialized string) (string, *ai.Usage, error) {
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
		return "", nil, fmt.Errorf("summarize: %s", msg.ErrorMessage)
	}
	return ai.MessageText(msg), msg.Usage, nil
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
	before := s.emitPlugin(ctx, "session_before_tree", map[string]any{
		"type": "session_before_tree",
		"preparation": map[string]any{
			"targetId": entryID, "oldLeafId": oldLeaf,
			"userWantsSummary": opt.Summarize, "customInstructions": opt.CustomInstructions,
		},
	})
	if cancelled, _ := before["cancel"].(bool); cancelled {
		return fmt.Errorf("tree navigation cancelled by plugin")
	}
	if custom, ok := before["customInstructions"].(string); ok && custom != "" {
		opt.CustomInstructions = custom
	}
	if err := s.manager.SetLeaf(entryID); err != nil {
		return err
	}
	if summary, ok := before["summary"].(map[string]any); ok {
		summaryText, _ := summary["summary"].(string)
		if summaryText != "" {
			details, _ := json.Marshal(summary["details"])
			var usage *ai.Usage
			if raw, ok := summary["usage"]; ok {
				var parsed ai.Usage
				if data, err := json.Marshal(raw); err == nil && json.Unmarshal(data, &parsed) == nil {
					usage = &parsed
				}
			}
			summaryID, err := s.manager.AppendBranchSummary(oldLeaf, summaryText, details, true, usage)
			if err != nil {
				return err
			}
			s.emitPlugin(ctx, "session_tree", map[string]any{
				"type": "session_tree", "newLeafId": s.manager.LeafID(), "oldLeafId": oldLeaf,
				"summaryEntry": map[string]any{"id": summaryID, "summary": summaryText}, "fromExtension": true,
			})
			return nil
		}
	}
	if !opt.Summarize {
		s.emitPlugin(ctx, "session_tree", map[string]any{"type": "session_tree", "newLeafId": entryID, "oldLeafId": oldLeaf})
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
	summary, usage, err := s.summarize(ctx, serialized)
	if err != nil {
		_ = s.manager.SetLeaf(oldLeaf)
		return err
	}
	if oldLeaf == "" {
		oldLeaf = "root"
	}
	// leaf is target; append summary as child of target so it becomes new leaf
	summaryID, err := s.manager.AppendBranchSummary(oldLeaf, summary, nil, false, usage)
	if err == nil {
		s.emitPlugin(ctx, "session_tree", map[string]any{
			"type": "session_tree", "newLeafId": s.manager.LeafID(), "oldLeafId": oldLeaf,
			"summaryEntry": map[string]any{"id": summaryID, "summary": summary},
		})
	}
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

// Tree returns the session's branch tree with resolved labels.
func (s *Session) Tree() []*SessionTreeNode { return s.manager.Tree() }

// SettingsManager returns the settings view this session was created with.
func (s *Session) SettingsManager() *SettingsManager { return s.settings }

// GetSettingsManager is SettingsManager under an accessor name.
func (s *Session) GetSettingsManager() *SettingsManager { return s.settings }

// SettingsDiagnostics returns and clears settings load/write failures. Pi
// reports these at startup; nulo leaves the reporting to the host.
func (s *Session) SettingsDiagnostics() []SettingsError {
	if s.settings == nil {
		return []SettingsError{}
	}
	return s.settings.DrainErrors()
}

// SkillDiagnostic is a warning produced while discovering skills. Pi reports
// these at startup and carries on; nulo keeps them here for the host to show.
type SkillDiagnostic struct {
	Type      string
	Message   string
	Path      string
	Collision *SkillCollision
}

// MCPDiagnostics returns the problems found while loading mcp.json. Pi reports
// them at startup and keeps the servers that did load; nulo does the same and
// leaves the reporting to the host.
func (s *Session) MCPDiagnostics() []string {
	return append([]string(nil), s.mcpDiagnostics...)
}

// SkillCollision names the winning and losing file of a duplicate skill name.
type SkillCollision struct {
	Name       string
	WinnerPath string
	LoserPath  string
}

// SkillDiagnostics returns the warnings and name collisions produced while
// discovering skills for this session.
func (s *Session) SkillDiagnostics() []SkillDiagnostic {
	out := make([]SkillDiagnostic, 0, len(s.skillDiagnostics))
	for _, d := range s.skillDiagnostics {
		item := SkillDiagnostic{Type: d.Type, Message: d.Message, Path: d.Path}
		if d.Collision != nil {
			item.Collision = &SkillCollision{
				Name:       d.Collision.Name,
				WinnerPath: d.Collision.WinnerPath,
				LoserPath:  d.Collision.LoserPath,
			}
		}
		out = append(out, item)
	}
	return out
}

func (s *Session) SetActiveTools(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range names {
		if n == "codemode" || n == "+codemode" {
			s.ensureCodemodeLocked(false)
		}
	}
	s.tools = selectTools(s.catalog, s.tools, names)
}

// Name is the latest name recorded for the session, or empty when none is set.
func (s *Session) Name() string { return s.manager.Name() }

// SetName records a session name as a session_info entry, as Pi's /name does.
func (s *Session) SetName(name string) error {
	_, err := s.manager.AppendSessionInfo(name)
	if err == nil {
		s.emitPlugin(context.Background(), "session_info_changed", map[string]any{"type": "session_info_changed", "name": name})
	}
	return err
}

// SetLabel names an entry; a nil label clears it.
func (s *Session) SetLabel(targetID string, label *string) error {
	_, err := s.manager.AppendLabel(targetID, label)
	return err
}

// Label returns the latest label for an entry.
func (s *Session) Label(targetID string) (string, bool) { return s.manager.Label(targetID) }

// Model returns the model the next request uses.
func (s *Session) Model() Model {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model
}

// SetModel changes the model for later requests and records a model_change
// entry, so resuming the session restores it.
func (s *Session) SetModel(model Model) error {
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return fmt.Errorf("session disposed")
	}
	previousModel := s.model
	previousThinking := s.thinking
	s.model = model
	// Pi's model switch prefers the target model's modelThinkingLevels entry,
	// then defaultThinkingLevel, then the current level.
	if s.settings != nil {
		level := s.settings.GetModelThinkingLevel(model.Provider, model.ID)
		if level == "" {
			level = s.settings.GetDefaultThinkingLevel()
		}
		if level != "" {
			s.thinking = ai.ClampThinkingLevel(model, level)
		} else if s.thinking != "" {
			s.thinking = ai.ClampThinkingLevel(model, s.thinking)
		}
	}
	thinking := s.thinking
	s.mu.Unlock()
	if _, err := s.manager.AppendModelChange(model.Provider, model.ID); err != nil {
		return err
	}
	if thinking != previousThinking && thinking != "" {
		if _, err := s.manager.AppendThinkingLevel(string(thinking)); err != nil {
			return err
		}
	}
	var previous any
	if previousModel.Provider != "" || previousModel.ID != "" {
		previous = modelJSON(previousModel)
	}
	s.emitPlugin(context.Background(), "model_select", map[string]any{
		"type":          "model_select",
		"model":         modelJSON(model),
		"previousModel": previous,
		"source":        "set",
	})
	return nil
}

// ThinkingLevel returns the level the next request uses; empty means no
// reasoning parameter is sent.
func (s *Session) ThinkingLevel() ThinkingLevel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.thinking
}

// SetThinkingLevel records a new thinking level for later requests, clamped to
// what the model supports. An empty level clears it. The level is persisted as
// a thinking_level_change entry, so resuming the session restores it.
func (s *Session) SetThinkingLevel(level ThinkingLevel) error {
	switch level {
	case "", ThinkingOff, ThinkingMinimal, ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingXHigh, ThinkingMax:
	default:
		return fmt.Errorf("unknown thinking level %q", level)
	}
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return fmt.Errorf("session disposed")
	}
	if level != "" {
		level = ai.ClampThinkingLevel(s.model, level)
	}
	s.thinking = level
	s.mu.Unlock()
	if level == "" {
		return nil
	}
	if _, err := s.manager.AppendThinkingLevel(string(level)); err != nil {
		return err
	}
	s.emitPlugin(context.Background(), "thinking_level_select", map[string]any{
		"type":  "thinking_level_select",
		"level": string(level),
	})
	return nil
}

// ensureCodemodeLocked puts the codemode tool in the catalogue, and in the active
// set when activate is true. It is idempotent, so a second call only ever adds the
// missing piece.
func (s *Session) ensureCodemodeLocked(activate bool) {
	for _, t := range s.catalog {
		if t.Name == "codemode" {
			if activate {
				s.activateTool(t)
			}
			return
		}
	}
	budget := codemode.DefaultInlineBudget
	if s.settings != nil {
		budget = s.settings.GetCodemodeInlineBudget()
	}
	cm := codemode.ToolWithBudget(s.catalog, s.mcpNamespaces(), s.loadCMStore, s.saveCMStore, budget)
	s.catalog = append(s.catalog, cm)
	if activate {
		s.activateTool(cm)
	}
}

func (s *Session) activateTool(t agent.AgentTool) {
	for _, existing := range s.tools {
		if existing.Name == t.Name {
			return
		}
	}
	s.tools = append(s.tools, t)
}

// mcpNamespaces describes every server a script can reach, for
// describeNamespace() and searchTools({namespace}).
func (s *Session) mcpNamespaces() []codemode.Namespace {
	var out []codemode.Namespace
	if len(s.pluginToolByName) > 0 {
		names := make([]string, 0, len(s.pluginToolByName))
		for name, binding := range s.pluginToolByName {
			if binding.spec.ToolExposure() == "codemode" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			sort.Strings(names)
			out = append(out, codemode.Namespace{Name: "plugin", Description: "Native plugin tools", Tools: names})
		}
	}
	for _, srv := range s.servers {
		names := make([]string, 0, len(srv.Tools))
		for _, t := range srv.Tools {
			if t.Exposure == mcp.ExposureHidden {
				continue
			}
			names = append(names, t.Tool.Name)
		}
		if len(names) == 0 {
			continue
		}
		out = append(out, codemode.Namespace{
			Name:         mcp.Namespace(srv.Name),
			Description:  srv.Description,
			Instructions: srv.Instructions,
			Tools:        names,
		})
	}
	return out
}

// mcpPromptServers lists the servers whose tools are not declared to the model.
func (s *Session) mcpPromptServers() []prompt.MCPServer {
	var out []prompt.MCPServer
	for _, srv := range s.servers {
		reach := ""
		switch {
		case srv.Exposures[mcp.ExposureCodemode]:
			reach = "codemode"
		case srv.Exposures[mcp.ExposureDeferred]:
			reach = "tool_search"
		}
		if reach == "" {
			continue
		}
		out = append(out, prompt.MCPServer{
			Namespace: mcp.Namespace(srv.Name),
			Summary:   mcpServerSummary(srv),
			Reach:     reach,
		})
	}
	return out
}

// mcpServerSummary is Pi's serverSummary: the configured description, else the
// server's own instructions, reduced to its first line.
func mcpServerSummary(srv mcp.Connected) string {
	text := strings.TrimSpace(srv.Description)
	if text == "" {
		text = srv.Instructions
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
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

func sourceForPrompt(opt PromptOptions) string {
	if opt.Source != "" {
		return opt.Source
	}
	return "rpc"
}
