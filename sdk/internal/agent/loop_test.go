package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func eventTypes(evs []Event) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Type
	}
	return out
}

func runCollect(t *testing.T, p *ai.FauxProvider, tools []AgentTool, prompts []ai.Message) (evs []Event, msgs []ai.Message) {
	t.Helper()
	cfg := Config{
		Model:        ai.DefaultFauxModel(),
		ConvertToLlm: DefaultConvert,
		StreamFn:     p.StreamSimple,
	}
	msgs = Run(context.Background(), prompts, Context{Tools: tools}, cfg, func(e Event) {
		evs = append(evs, e)
	})
	return evs, msgs
}

func TestLoopNoTools(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("hi"))
	evs, msgs := runCollect(t, p, nil, []ai.Message{ai.UserMessage("hello", 1)})
	types := eventTypes(evs)
	if types[0] != "agent_start" || types[1] != "turn_start" {
		t.Fatalf("%v", types)
	}
	if types[len(types)-1] != "agent_end" {
		t.Fatalf("last %v", types)
	}
	var assistant *ai.Message
	for i := range msgs {
		if msgs[i].Role == "assistant" {
			assistant = &msgs[i]
		}
	}
	if assistant == nil || ai.MessageText(*assistant) != "hi" {
		t.Fatalf("msgs=%+v", msgs)
	}
}

func TestLoopToolThenStop(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(
		ai.FauxAssistantBlocks([]ai.ContentBlock{
			ai.ToolCallBlock("c1", "echo", map[string]any{"x": "1"}),
		}, ai.StopToolUse),
		ai.FauxAssistant("done"),
	)
	echo := AgentTool{
		Tool: ai.Tool{Name: "echo", Description: "echo", Parameters: []byte(`{"type":"object"}`)},
		Execute: func(ctx context.Context, id string, args map[string]any) (ToolResult, error) {
			return ToolResult{Content: []ai.ContentBlock{ai.TextBlock("ok")}}, nil
		},
	}
	evs, msgs := runCollect(t, p, []AgentTool{echo}, []ai.Message{ai.UserMessage("do", 1)})
	var sawExec, sawResult bool
	for _, e := range evs {
		if e.Type == "tool_execution_end" && !e.IsError {
			sawExec = true
		}
		if e.Type == "message_end" && e.Message != nil && e.Message.Role == "toolResult" {
			sawResult = true
		}
	}
	if !sawExec || !sawResult {
		t.Fatalf("events %v", eventTypes(evs))
	}
	last := msgs[len(msgs)-1]
	if last.Role != "assistant" || ai.MessageText(last) != "done" {
		t.Fatalf("last %+v", last)
	}
}

func TestLoopLengthFailsTools(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(
		ai.FauxAssistantBlocks([]ai.ContentBlock{
			ai.ToolCallBlock("c1", "echo", map[string]any{"x": "1"}),
		}, ai.StopLength),
		ai.FauxAssistant("after"),
	)
	ran := false
	echo := AgentTool{
		Tool: ai.Tool{Name: "echo", Description: "echo", Parameters: []byte(`{}`)},
		Execute: func(ctx context.Context, id string, args map[string]any) (ToolResult, error) {
			ran = true
			return ToolResult{}, nil
		},
	}
	_, msgs := runCollect(t, p, []AgentTool{echo}, []ai.Message{ai.UserMessage("do", 1)})
	if ran {
		t.Fatal("tool must not run on length")
	}
	var tr *ai.Message
	for i := range msgs {
		if msgs[i].Role == "toolResult" {
			tr = &msgs[i]
		}
	}
	if tr == nil || !tr.IsError {
		t.Fatalf("expected error toolResult, msgs=%+v", msgs)
	}
	if !strings.Contains(tr.ContentString(), "token limit") {
		t.Fatalf("msg %q", tr.ContentString())
	}
}

func TestLoopAbort(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("0123456789abcdef"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var last ai.Message
	Run(ctx, []ai.Message{ai.UserMessage("x", 1)}, Context{}, Config{
		Model:        ai.DefaultFauxModel(),
		ConvertToLlm: DefaultConvert,
		StreamFn:     p.StreamSimple,
	}, func(e Event) {
		if e.Type == "message_end" && e.Message != nil && e.Message.Role == "assistant" {
			last = *e.Message
		}
	})
	if last.StopReason != ai.StopAborted && last.StopReason != ai.StopError {
		t.Fatalf("stop=%s", last.StopReason)
	}
}

func TestLoopSteer(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("one"), ai.FauxAssistant("two"))
	steerCalls := 0
	cfg := Config{
		Model:        ai.DefaultFauxModel(),
		ConvertToLlm: DefaultConvert,
		StreamFn:     p.StreamSimple,
		GetSteering: func() []ai.Message {
			steerCalls++
			if steerCalls == 2 {
				return []ai.Message{ai.UserMessage("steer", 2)}
			}
			return nil
		},
	}
	msgs := Run(context.Background(), []ai.Message{ai.UserMessage("hi", 1)}, Context{}, cfg, func(Event) {})
	var users int
	var texts []string
	for _, m := range msgs {
		if m.Role == "user" {
			users++
		}
		if m.Role == "assistant" {
			texts = append(texts, ai.MessageText(m))
		}
	}
	if users < 2 {
		t.Fatalf("want steered user, msgs=%+v", msgs)
	}
	if len(texts) < 2 || texts[1] != "two" {
		t.Fatalf("texts %v", texts)
	}
}
