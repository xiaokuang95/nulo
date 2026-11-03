package sdk

import (
	"context"
	"fmt"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// Cache warming keeps a provider's prompt cache warm between requests. Models
// declare the cache lifetime in models.json's promptCache; a request is only
// repeated when the expected savings exceed five cents, as Pi's cache warmer
// does.
const (
	cacheWarmingMinimumSavings  = 0.05
	idleContinuationProbability = 0.15
)

// warmRequest is the last request the agent sent: the cacheable prefix, the
// model and the options a replay needs.
type warmRequest struct {
	model    ai.Model
	messages []ai.Message
}

// warmState is the scheduled or running refresh.
type warmState struct {
	request *warmRequest
	timer   *time.Timer
	cancel  context.CancelFunc
}

// newCacheWarmer validates a session's cacheWarming option.
func cacheWarmingMode(mode string) (string, error) {
	switch mode {
	case "":
		// Pi's default: warm while the agent is running.
		return "streaming", nil
	case "off", "streaming", "idle":
		return mode, nil
	default:
		return "", fmt.Errorf(`unknown cacheWarming mode %q; use "off", "streaming" or "idle"`, mode)
	}
}

// captureWarmRequest records the request the agent is about to send.
func (s *Session) captureWarmRequest(model ai.Model, messages []ai.Message) {
	if s.warmMode == "off" {
		return
	}
	s.mu.Lock()
	s.warmRequest = &warmRequest{model: model, messages: append([]ai.Message(nil), messages...)}
	s.mu.Unlock()
}

// cancelWarm stops a scheduled refresh.
func (s *Session) cancelWarm() {
	s.mu.Lock()
	state := s.warm
	s.warm = nil
	s.mu.Unlock()
	stopWarm(state)
}

func stopWarm(state *warmState) {
	if state == nil {
		return
	}
	if state.timer != nil {
		state.timer.Stop()
	}
	if state.cancel != nil {
		state.cancel()
	}
}

// scheduleWarm arms the refresh timer for the last request. phase is
// "streaming" while the agent runs or "idle" between runs.
func (s *Session) scheduleWarm(phase string) {
	s.mu.Lock()
	mode := s.warmMode
	request := s.warmRequest
	previous := s.warm
	s.warm = nil
	s.mu.Unlock()
	stopWarm(previous)
	if mode == "off" || request == nil {
		return
	}
	if mode == "streaming" && phase == "idle" {
		return
	}
	delay, ok := warmDecision(request, phase)
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &warmState{request: request, cancel: cancel}
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		cancel()
		return
	}
	s.warm = state
	s.mu.Unlock()
	state.timer = time.AfterFunc(delay, func() {
		s.runWarm(ctx, state)
	})
}

// runWarm replays the last request with a one-token answer, then schedules the
// next refresh while the session stays idle.
func (s *Session) runWarm(ctx context.Context, state *warmState) {
	s.mu.Lock()
	current := s.warm == state && !s.disposed
	mode := s.warmMode
	request := state.request
	s.mu.Unlock()
	if !current || request == nil {
		return
	}
	fn := s.services.StreamFn
	if fn == nil {
		fn = ai.StreamForDir(s.services.AgentDir)
	}
	opts := ai.StreamOptions{MaxTokens: 1}
	stream := fn(ctx, request.model, ai.Transcript{Messages: request.messages}, opts)
	for range stream.Events() {
	}
	result := stream.Result()
	// Pi records warming usage as its own entry; it does not enter context.
	if result.Usage != nil && result.Usage.TotalTokens > 0 {
		_, _ = s.manager.AppendUsage("cache_warm", request.model.Provider, request.model.ID, *result.Usage, "")
	}
	s.mu.Lock()
	active := s.warm == state && !s.disposed && mode == "idle"
	s.mu.Unlock()
	if active {
		s.scheduleWarm("idle")
	}
}

// warmDecision reports the refresh delay and whether warming pays off.
func warmDecision(request *warmRequest, phase string) (time.Duration, bool) {
	ttl, ok := ai.PromptCacheTTL(request.model, "short")
	if !ok {
		return 0, false
	}
	delay, ok := ai.CacheWarmingDelay(ttl)
	if !ok {
		return 0, false
	}
	promptTokens := lastPromptTokens(request.messages)
	if promptTokens == 0 {
		return 0, false
	}
	cacheHit := ai.EstimateCost(request.model, ai.Usage{CacheRead: promptTokens})
	missUsage := ai.Usage{Input: promptTokens}
	if request.model.Cost.CacheWrite > 0 {
		missUsage = ai.Usage{CacheWrite: promptTokens}
	}
	cacheMiss := ai.EstimateCost(request.model, missUsage)
	warmCost := ai.EstimateCost(request.model, ai.Usage{CacheRead: promptTokens, Output: 1})
	missCost := max(0, cacheMiss.Total-cacheHit.Total)
	probability := 1.0
	if phase == "idle" {
		probability = idleContinuationProbability
	}
	if probability*missCost-warmCost.Total < cacheWarmingMinimumSavings {
		return 0, false
	}
	return delay, true
}

// hasToolCalls reports whether an assistant message asks for tool work.
func hasToolCalls(message ai.Message) bool {
	for _, block := range message.ContentBlocks() {
		if block.Type == "toolCall" {
			return true
		}
	}
	return false
}

// lastPromptTokens is the prompt size of the most recent priced reply in the
// captured prefix.
func lastPromptTokens(messages []ai.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Usage == nil {
			continue
		}
		usage := messages[i].Usage
		if total := usage.Input + usage.CacheRead + usage.CacheWrite; total > 0 {
			return total
		}
	}
	return 0
}
