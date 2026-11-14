package sdk

import (
	"context"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestPlanToolEmitsAndPersists(t *testing.T) {
	cwd := t.TempDir()
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	p := ai.NewFauxProvider()
	p.SetResponses(
		ai.FauxAssistantBlocks([]ai.ContentBlock{
			ai.ToolCallBlock("plan-1", "plan", map[string]any{
				"title": "Ship the change",
				"next":  "Write the test",
				"steps": []any{
					map[string]any{"title": "Inspect code", "status": "done"},
					map[string]any{"title": "Write the test", "status": "active", "detail": "Cover the happy path"},
					map[string]any{"title": "Verify", "status": "pending"},
				},
			}),
		}, ai.StopToolUse),
		ai.FauxAssistant("Plan is ready."),
	)
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: agentDir, StreamFn: p.StreamSimple,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	var updates []Plan
	sess.Subscribe(func(ev Event) {
		if ev.Type == "plan_update" && ev.Plan != nil {
			updates = append(updates, *ev.Plan)
		}
	})
	if _, err := sess.Send(context.Background(), "make a plan"); err != nil {
		t.Fatal(err)
	}
	plan := sess.Plan()
	if plan.Title != "Ship the change" || len(plan.Steps) != 3 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Steps[1].Status != "active" || plan.Steps[1].Detail != "Cover the happy path" {
		t.Fatalf("steps = %+v", plan.Steps)
	}
	if len(updates) != 1 || updates[0].Title != plan.Title {
		t.Fatalf("plan updates = %+v", updates)
	}
	path := sess.File()
	if path == "" {
		t.Fatal("session was not persisted")
	}
	_ = rt.Close()

	svcs2, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt2, err := NewRuntime(RuntimeOptions{Services: svcs2})
	if err != nil {
		t.Fatal(err)
	}
	defer rt2.Close()
	resumed, err := rt2.ResumeSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := resumed.Plan(); got.Title != plan.Title || len(got.Steps) != len(plan.Steps) {
		t.Fatalf("resumed plan = %+v", got)
	}
}

func TestSetPlanNormalizesStatuses(t *testing.T) {
	p := ai.NewFauxProvider()
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
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
	if err := sess.SetPlan(Plan{Steps: []PlanStep{
		{Title: "one", Status: "in_progress"},
		{Title: "two", Status: "completed"},
		{Title: "three", Status: "running"},
	}}); err != nil {
		t.Fatal(err)
	}
	got := sess.Plan()
	if got.Title != "Plan" || got.Steps[0].Status != "active" || got.Steps[1].Status != "done" || got.Steps[2].Status != "pending" {
		t.Fatalf("normalized plan = %+v", got)
	}
}
