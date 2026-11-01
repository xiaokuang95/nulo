package sdk

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// Sessions that run at the same time keep their events apart: each listener sees
// one session's events in order, and none of another session's.
func TestConcurrentSessionsKeepEventsApart(t *testing.T) {
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true,
		StreamFn: func(ctx context.Context, model ai.Model, tr ai.Transcript, opts ai.StreamOptions) *ai.EventStream {
			// A provider per call: the common provider is not required to be safe
			// for concurrent use, and this test is about the session, not it.
			p := ai.NewFauxProvider()
			p.SetResponses(ai.FauxAssistant("ok"))
			return p.StreamSimple(ctx, model, tr, opts)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	const sessions = 8
	var wg sync.WaitGroup
	for i := 0; i < sessions; i++ {
		rt, err := NewRuntime(RuntimeOptions{Services: svcs})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { rt.Close() })
		sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []Tool{}})
		if err != nil {
			t.Fatal(err)
		}

		var mu sync.Mutex
		var events []string
		settled := make(chan struct{})
		sess.Subscribe(func(ev Event) {
			mu.Lock()
			events = append(events, ev.Type)
			mu.Unlock()
			if ev.Type == "agent_settled" {
				select {
				case <-settled:
				default:
					close(settled)
				}
			}
		})

		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			if _, err := sess.Send(context.Background(), "hi"); err != nil {
				t.Errorf("session %d: %v", index, err)
				return
			}
			select {
			case <-settled:
			case <-time.After(5 * time.Second):
				t.Errorf("session %d: no agent_settled", index)
				return
			}
			mu.Lock()
			got := append([]string(nil), events...)
			mu.Unlock()
			if len(got) == 0 || got[0] != "agent_start" || got[len(got)-1] != "agent_settled" {
				t.Errorf("session %d: events %v", index, got)
			}
		}(i)
	}
	wg.Wait()
}

// Every listener on a session sees the same events, and unsubscribing one does
// not disturb another, even after the list was reset by Dispose.
func TestSubscribeDeliversToEveryListener(t *testing.T) {
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true,
		StreamFn: func(ctx context.Context, model ai.Model, tr ai.Transcript, opts ai.StreamOptions) *ai.EventStream {
			p := ai.NewFauxProvider()
			p.SetResponses(ai.FauxAssistant("ok"))
			return p.StreamSimple(ctx, model, tr, opts)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []Tool{}})
	if err != nil {
		t.Fatal(err)
	}

	var first, second []string
	sess.Subscribe(func(ev Event) { first = append(first, ev.Type) })
	sess.Subscribe(func(ev Event) { second = append(second, ev.Type) })
	if _, err := sess.Send(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || len(first) != len(second) {
		t.Fatalf("both listeners should see every event: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("listeners saw different events at %d: %q vs %q", i, first[i], second[i])
		}
	}

	// An unsubscribe token must not name a listener that was added later.
	live := sess.Subscribe(func(Event) {})
	sess.Dispose()
	var calls int
	sess.Subscribe(func(Event) { calls++ })
	live()
	sess.fire(Event{Type: "probe"})
	if calls != 1 {
		t.Fatalf("unsubscribing an old listener removed a new one: %d calls", calls)
	}
}
