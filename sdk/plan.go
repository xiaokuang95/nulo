package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// Plan returns the latest plan entry on the active branch. A session without a
// plan returns the zero value. The plan is persisted in the session log as a
// custom entry, so it survives resume, fork, and rewind.
func (s *Session) Plan() Plan {
	if s == nil {
		return Plan{}
	}
	entries := s.manager.Branch()
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Type != "custom" || entry.CustomType != "plan" || len(entry.Data) == 0 {
			continue
		}
		var plan Plan
		if json.Unmarshal(entry.Data, &plan) == nil {
			return plan
		}
	}
	return Plan{}
}

// SetPlan replaces the visible task plan and emits a plan_update event. Hosts
// can also call it directly; the built-in plan tool calls it on behalf of the
// model.
func (s *Session) SetPlan(plan Plan) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	plan = normalizePlan(plan)
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	s.mu.Lock()
	disposed := s.disposed
	s.mu.Unlock()
	if disposed {
		return fmt.Errorf("session disposed")
	}
	if _, err := s.manager.AppendCustom("plan", data); err != nil {
		return err
	}
	s.fire(Event{Type: "plan_update", Plan: &plan})
	return nil
}

func normalizePlan(plan Plan) Plan {
	plan.Title = strings.TrimSpace(plan.Title)
	if plan.Title == "" {
		plan.Title = "Plan"
	}
	plan.Next = strings.TrimSpace(plan.Next)
	if len(plan.Steps) > 100 {
		plan.Steps = plan.Steps[:100]
	}
	out := make([]PlanStep, 0, len(plan.Steps))
	active := false
	for _, step := range plan.Steps {
		step.Title = strings.TrimSpace(step.Title)
		if step.Title == "" {
			continue
		}
		step.Detail = strings.TrimSpace(step.Detail)
		step.Status = normalizePlanStatus(step.Status)
		if step.Status == "active" {
			if active {
				step.Status = "pending"
			} else {
				active = true
			}
		}
		out = append(out, step)
	}
	plan.Steps = out
	return plan
}

func normalizePlanStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "complete", "completed", "finished", "success":
		return "done"
	case "active", "in_progress", "in-progress", "running", "current":
		return "active"
	case "blocked", "failed", "error":
		return "blocked"
	default:
		return "pending"
	}
}

func (s *Session) planTool() agent.AgentTool {
	return agent.AgentTool{
		Tool: ai.Tool{
			Name: "plan",
			Description: "Create or update the visible task plan. Use this at the start of multi-step work and after each completed step. " +
				"Keep one step active while working. Statuses are pending, active, done, and blocked.",
			Parameters: json.RawMessage(`{
				"type":"object",
				"properties":{
					"title":{"type":"string","description":"Plan title"},
					"next":{"type":"string","description":"One-line description of the next action"},
					"steps":{"type":"array","items":{
						"type":"object",
						"properties":{
							"title":{"type":"string"},
							"detail":{"type":"string"},
							"status":{"type":"string","enum":["pending","active","done","blocked"]}
						},
						"required":["title"]
					}}
				},
				"required":["steps"]
			}`),
		},
		Label: "Plan",
		Execute: func(_ context.Context, _ string, args map[string]any) (agent.ToolResult, error) {
			plan := Plan{Title: stringArg(args, "title"), Next: stringArg(args, "next")}
			rawSteps, _ := args["steps"].([]any)
			for _, raw := range rawSteps {
				stepMap, _ := raw.(map[string]any)
				if stepMap == nil {
					continue
				}
				plan.Steps = append(plan.Steps, PlanStep{
					Title:  stringArg(stepMap, "title"),
					Detail: stringArg(stepMap, "detail"),
					Status: stringArg(stepMap, "status"),
				})
			}
			if err := s.SetPlan(plan); err != nil {
				return agent.ToolResult{}, err
			}
			plan = normalizePlan(plan)
			return agent.ToolResult{Content: []ai.ContentBlock{
				ai.TextBlock(fmt.Sprintf("Plan updated: %d step(s).", len(plan.Steps))),
			}}, nil
		},
	}
}
