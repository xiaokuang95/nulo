package ai

import (
	"context"
	"testing"
)

func collect(s *EventStream) (types []string, result Message) {
	for ev := range s.Events() {
		types = append(types, ev.Type)
	}
	return types, s.Result()
}

func TestFauxTextStream(t *testing.T) {
	p := NewFauxProvider()
	p.SetResponses(FauxAssistant("Hello"))
	s := p.StreamSimple(context.Background(), DefaultFauxModel(), Transcript{}, StreamOptions{})
	types, result := collect(s)
	if result.StopReason != StopStop {
		t.Fatalf("stop=%s err=%s", result.StopReason, result.ErrorMessage)
	}
	if result.ContentString() != "Hello" && MessageText(result) != "Hello" {
		t.Fatalf("text=%q", MessageText(result))
	}
	wantPrefix := []string{"start", "text_start", "text_delta"}
	for i, w := range wantPrefix {
		if i >= len(types) || types[i] != w {
			t.Fatalf("events %v, want prefix %v", types, wantPrefix)
		}
	}
	if types[len(types)-2] != "text_end" || types[len(types)-1] != "done" {
		t.Fatalf("events %v", types)
	}
}

func TestFauxEmptyQueueIsMessageNotError(t *testing.T) {
	p := NewFauxProvider()
	s := p.StreamSimple(context.Background(), DefaultFauxModel(), Transcript{}, StreamOptions{})
	_, result := collect(s)
	if result.StopReason != StopError {
		t.Fatalf("stop=%s", result.StopReason)
	}
	if result.ErrorMessage == "" {
		t.Fatal("expected errorMessage")
	}
}

func TestFauxAbort(t *testing.T) {
	p := NewFauxProvider()
	p.SetResponses(FauxAssistant("0123456789abcdef"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := p.StreamSimple(ctx, DefaultFauxModel(), Transcript{}, StreamOptions{})
	_, result := collect(s)
	if result.StopReason != StopAborted {
		t.Fatalf("stop=%s msg=%s", result.StopReason, result.ErrorMessage)
	}
}

func TestFauxToolCall(t *testing.T) {
	p := NewFauxProvider()
	p.SetResponses(FauxAssistantBlocks([]ContentBlock{
		ToolCallBlock("c1", "read", map[string]any{"path": "a.go"}),
	}, StopToolUse))
	s := p.StreamSimple(context.Background(), DefaultFauxModel(), Transcript{}, StreamOptions{})
	types, result := collect(s)
	if result.StopReason != StopToolUse {
		t.Fatalf("stop=%s", result.StopReason)
	}
	found := false
	for _, ty := range types {
		if ty == "toolcall_end" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events %v", types)
	}
}

func TestNormalizeAndTools(t *testing.T) {
	tr := NormalizeContext("hi", []Tool{{Name: "read", Description: "r", Parameters: []byte(`{"type":"object"}`)}}, nil)
	if len(tr.Messages) != 1 || tr.Messages[0].Role != "system" {
		t.Fatalf("%+v", tr.Messages)
	}
	tools := GetCurrentTools(tr.Messages)
	if len(tools) != 1 || tools[0].Name != "read" {
		t.Fatalf("%+v", tools)
	}
	null := (*string)(nil)
	skills := "s"
	patch := Message{Role: "system", Timestamp: 1, Sections: map[string]*string{"skills": &skills, "docs": null}}
	tr.Messages[0].Sections = map[string]*string{"docs": ptr("old"), "preamble": ptr("p")}
	tr.Messages = append(tr.Messages, patch)
	cur := GetCurrentSystemMessage(tr.Messages)
	if cur == nil || cur.Sections["docs"] != nil {
		t.Fatalf("docs should be removed: %+v", cur)
	}
	if cur.Sections["skills"] == nil || *cur.Sections["skills"] != "s" {
		t.Fatalf("skills: %+v", cur.Sections)
	}
}

func ptr(s string) *string { return &s }
