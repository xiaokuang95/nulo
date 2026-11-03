package sdk

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func reasoningModel() ai.Model {
	return ai.Model{ID: "m", Provider: "p", API: "faux", Reasoning: true}
}

func TestThinkingLevelPersistsAndRestores(t *testing.T) {
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD:      t.TempDir(),
		AgentDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: reasoningModel()})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.SetThinkingLevel(ThinkingHigh); err != nil {
		t.Fatal(err)
	}
	// A session file appears with the first conversation entry.
	if _, err := sess.Manager().AppendMessage(ai.UserMessage("hi", 1)); err != nil {
		t.Fatal(err)
	}
	if got := sess.ThinkingLevel(); got != ThinkingHigh {
		t.Fatalf("level %q", got)
	}
	if err := sess.SetThinkingLevel("nonsense"); err == nil {
		t.Fatal("an unknown level must be rejected")
	}
	path := sess.Manager().File()
	switched, err := rt.SwitchSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := switched.ThinkingLevel(); got != ThinkingHigh {
		t.Fatalf("restored level %q", got)
	}

	// An explicit start option wins over the recorded level.
	opt := SessionStartOptions{Model: reasoningModel(), ThinkingLevel: ThinkingLow}
	explicit, err := rt.New(opt)
	if err != nil {
		t.Fatal(err)
	}
	if got := explicit.ThinkingLevel(); got != ThinkingLow {
		t.Fatalf("explicit level %q", got)
	}
	if _, err := explicit.Manager().AppendMessage(ai.UserMessage("hi", 1)); err != nil {
		t.Fatal(err)
	}
	restored, err := rt.SwitchSession(explicit.Manager().File())
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.ThinkingLevel(); got != ThinkingLow {
		t.Fatalf("a start level must persist too, got %q", got)
	}
}

func TestCacheWarmingDecision(t *testing.T) {
	model := ai.Model{
		PromptCache: map[string]float64{"short": 300},
		Cost:        ai.Cost{Input: 10, Output: 20, CacheRead: 1},
	}
	request := &warmRequest{model: model, messages: []ai.Message{
		ai.AssistantMessage(nil, model, ai.StopStop, 1),
	}}
	request.messages[0].Usage = &ai.Usage{Input: 100_000}

	if _, ok := warmDecision(request, "streaming"); !ok {
		t.Fatal("a large, expensive prompt should be warmed while the agent runs")
	}
	if _, ok := warmDecision(request, "idle"); ok {
		t.Fatal("idle warming needs a much higher expected saving at 15% continuation")
	}

	// A cheap cache read makes the refresh not worth it.
	cheap := model
	cheap.Cost = ai.Cost{Input: 10, Output: 20, CacheRead: 9.99}
	cheapRequest := &warmRequest{model: cheap, messages: request.messages}
	if _, ok := warmDecision(cheapRequest, "streaming"); ok {
		t.Fatal("nearly-free cache reads leave nothing to save")
	}

	// A model without a prompt-cache lifetime is never warmed.
	if _, ok := warmDecision(&warmRequest{model: ai.Model{}, messages: request.messages}, "streaming"); ok {
		t.Fatal("no lifetime means no warming")
	}
}

func TestCacheWarmingMode(t *testing.T) {
	if mode, err := cacheWarmingMode(""); err != nil || mode != "streaming" {
		t.Fatalf("default %q %v", mode, err)
	}
	if mode, err := cacheWarmingMode("idle"); err != nil || mode != "idle" {
		t.Fatalf("idle %q %v", mode, err)
	}
	if mode, err := cacheWarmingMode("off"); err != nil || mode != "off" {
		t.Fatalf("off %q %v", mode, err)
	}
	if _, err := cacheWarmingMode("sometimes"); err == nil {
		t.Fatal("an unknown mode must be rejected")
	}
}

func TestIdleCacheWarmingReplaysTheLastRequest(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	warmed := make(chan struct{}, 1)
	stream := func(_ context.Context, model ai.Model, _ ai.Transcript, opts ai.StreamOptions) *ai.EventStream {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if opts.MaxTokens == 1 {
			select {
			case warmed <- struct{}{}:
			default:
			}
		}
		message := ai.AssistantMessage([]ai.ContentBlock{ai.TextBlock("ok")}, model, ai.StopStop, 1)
		if n == 1 {
			message.Usage = &ai.Usage{Input: 1_000_000}
		}
		events := ai.NewEventStream()
		go events.End(&message)
		return events
	}
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD:      t.TempDir(),
		AgentDir: t.TempDir(),
		StreamFn: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	model := ai.Model{
		ID: "m", Provider: "p", API: "faux",
		PromptCache: map[string]float64{"short": 11},
		Cost:        ai.Cost{Input: 10, CacheRead: 1, Output: 20},
	}
	sess, err := rt.New(SessionStartOptions{Model: model, InMemory: true, CacheWarming: "idle"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Dispose()
	ctx := context.Background()
	// The first request has no priced reply, so the second one carries it.
	if _, err := sess.Send(ctx, "hi"); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Send(ctx, "again"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-warmed:
	case <-time.After(4 * time.Second):
		t.Fatal("the idle session did not warm its cache")
	}
}
