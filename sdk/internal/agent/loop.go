package agent

import (
	"context"
	"sync"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func Run(ctx context.Context, prompts []ai.Message, agentCtx Context, cfg Config, emit EventSink) []ai.Message {
	initial := declareToolChanges(agentCtx, prompts)
	newMessages := append([]ai.Message(nil), initial...)
	agentCtx.Messages = append(append([]ai.Message(nil), agentCtx.Messages...), initial...)
	emit(Event{Type: "agent_start"})
	emit(Event{Type: "turn_start"})
	for i := range initial {
		m := initial[i]
		emit(Event{Type: "message_start", Message: &m})
		emit(Event{Type: "message_end", Message: &m})
	}
	runLoop(ctx, &agentCtx, &newMessages, cfg, emit)
	return newMessages
}

func runLoop(ctx context.Context, current *Context, newMessages *[]ai.Message, cfg Config, emit EventSink) {
	var lastTurn *TurnContext
	explicitContinuation := false
	pending := drain(cfg.GetSteering)

	for {
		hasMoreTools := true
		for hasMoreTools || len(pending) > 0 {
			var prepared []ai.Message
			if lastTurn != nil {
				if cfg.PrepareNextTurn != nil {
					if u := cfg.PrepareNextTurn(ctx, *lastTurn); u != nil {
						if u.Context != nil {
							*current = *u.Context
						}
						prepared = u.Messages
						if u.Model != nil {
							cfg.Model = *u.Model
						}
						if u.ThinkingLevel != nil {
							cfg.ThinkingLevel = *u.ThinkingLevel
						}
					}
				}
				if len(pending) == 0 {
					pending = drain(cfg.GetSteering)
				}
				emit(Event{Type: "turn_start"})
			}

			for _, message := range declareToolChanges(*current, append(append([]ai.Message{}, prepared...), pending...)) {
				m := message
				emit(Event{Type: "message_start", Message: &m})
				emit(Event{Type: "message_end", Message: &m})
				current.Messages = append(current.Messages, message)
				*newMessages = append(*newMessages, message)
			}
			pending = nil

			if cfg.PrepareRequest != nil {
				*current = cfg.PrepareRequest(ctx, *current, cfg.Model)
			}

			msg := streamAssistant(ctx, current, cfg, emit)
			*newMessages = append(*newMessages, msg)

			if msg.StopReason == ai.StopError || msg.StopReason == ai.StopAborted {
				lastTurn = &TurnContext{Message: msg, Context: *current, NewMessages: append([]ai.Message{}, *newMessages...)}
				if cfg.FinishTurn != nil {
					cfg.FinishTurn(ctx, *lastTurn)
				}
				emit(Event{Type: "turn_end", Message: &msg, ToolResults: []ai.Message{}})
				emit(Event{Type: "agent_end", Messages: append([]ai.Message{}, *newMessages...)})
				return
			}

			calls := toolCalls(msg)
			var results []ai.Message
			hasMoreTools = false
			if len(calls) > 0 {
				var batch toolBatch
				if msg.StopReason == ai.StopLength {
					batch = failTruncated(calls, emit)
				} else {
					batch = executeToolCalls(ctx, *current, msg, calls, cfg, emit)
				}
				results = batch.messages
				hasMoreTools = !batch.terminate
				for _, r := range results {
					current.Messages = append(current.Messages, r)
					*newMessages = append(*newMessages, r)
				}
			}

			lastTurn = &TurnContext{Message: msg, ToolResults: results, Context: *current, NewMessages: append([]ai.Message{}, *newMessages...)}
			var decision *TurnDecision
			if cfg.FinishTurn != nil {
				decision = cfg.FinishTurn(ctx, *lastTurn)
			}
			emit(Event{Type: "turn_end", Message: &msg, ToolResults: results})
			if decision != nil && decision.Action == "end" {
				emit(Event{Type: "agent_end", Messages: append([]ai.Message{}, *newMessages...)})
				return
			}
			explicitContinuation = decision != nil && decision.Action == "continue"
			pending = drain(cfg.GetSteering)
			if hasMoreTools || len(pending) > 0 {
				explicitContinuation = false
			}
		}

		follow := drain(cfg.GetFollowUp)
		if len(follow) > 0 {
			explicitContinuation = false
			pending = follow
			continue
		}
		if explicitContinuation {
			explicitContinuation = false
			continue
		}
		break
	}
	emit(Event{Type: "agent_end", Messages: append([]ai.Message{}, *newMessages...)})
}

func drain(fn func() []ai.Message) []ai.Message {
	if fn == nil {
		return nil
	}
	return fn()
}

func toolCalls(msg ai.Message) []ai.ContentBlock {
	var out []ai.ContentBlock
	for _, b := range msg.ContentBlocks() {
		if b.Type == "toolCall" {
			out = append(out, b)
		}
	}
	return out
}

func streamAssistant(ctx context.Context, current *Context, cfg Config, emit EventSink) ai.Message {
	msgs := current.Messages
	if cfg.ConvertToLlm != nil {
		msgs = cfg.ConvertToLlm(msgs)
	}
	transcript := ai.Transcript{Messages: msgs}
	opts := cfg.StreamOptions
	if cfg.GetAPIKey != nil {
		if k := cfg.GetAPIKey(cfg.Model.Provider); k != "" {
			opts.APIKey = k
		}
	}
	opts.ThinkingLevel = cfg.ThinkingLevel
	stream := cfg.StreamFn(ctx, cfg.Model, transcript, opts)

	added := false
	for ev := range stream.Events() {
		switch ev.Type {
		case "start":
			if ev.Partial != nil {
				cp := *ev.Partial
				current.Messages = append(current.Messages, cp)
				added = true
				emit(Event{Type: "message_start", Message: &cp})
			}
		case "text_start", "text_delta", "text_end",
			"thinking_start", "thinking_delta", "thinking_end",
			"toolcall_start", "toolcall_delta", "toolcall_end":
			if ev.Partial != nil {
				cp := *ev.Partial
				current.Messages[len(current.Messages)-1] = cp
				evCopy := ev
				emit(Event{Type: "message_update", Message: &cp, AssistantMessageEvent: &evCopy})
			}
		case "done", "error":
			final := stream.Result()
			if cfg.ThinkingLevel != "" {
				final.ThinkingLevel = string(cfg.ThinkingLevel)
			}
			if added {
				current.Messages[len(current.Messages)-1] = final
			} else {
				current.Messages = append(current.Messages, final)
				emit(Event{Type: "message_start", Message: &final})
			}
			emit(Event{Type: "message_end", Message: &final})
			return final
		}
	}
	final := stream.Result()
	if added {
		current.Messages[len(current.Messages)-1] = final
	} else {
		current.Messages = append(current.Messages, final)
		emit(Event{Type: "message_start", Message: &final})
	}
	emit(Event{Type: "message_end", Message: &final})
	return final
}

type toolBatch struct {
	messages  []ai.Message
	terminate bool
}

func failTruncated(calls []ai.ContentBlock, emit EventSink) toolBatch {
	var messages []ai.Message
	for _, call := range calls {
		emit(Event{Type: "tool_execution_start", ToolCallID: call.ID, ToolName: call.Name, Args: call.Arguments})
		res := errorResult("Tool call \"" + call.Name + "\" was not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the tool call with complete arguments.")
		emit(Event{Type: "tool_execution_end", ToolCallID: call.ID, ToolName: call.Name, Result: res, IsError: true})
		tr := toolResultMessage(call, res, true, nil)
		emit(Event{Type: "message_start", Message: &tr})
		emit(Event{Type: "message_end", Message: &tr})
		messages = append(messages, tr)
	}
	return toolBatch{messages: messages}
}

func executeToolCalls(ctx context.Context, current Context, assistant ai.Message, calls []ai.ContentBlock, cfg Config, emit EventSink) toolBatch {
	if cfg.ToolExecution == "sequential" || hasSequential(current.Tools, calls) {
		return executeSequential(ctx, current, assistant, calls, cfg, emit)
	}
	return executeParallel(ctx, current, assistant, calls, cfg, emit)
}

func hasSequential(tools []AgentTool, calls []ai.ContentBlock) bool {
	for _, c := range calls {
		for _, t := range tools {
			if t.Name == c.Name && t.ExecutionMode == "sequential" {
				return true
			}
		}
	}
	return false
}

type finalized struct {
	call       ai.ContentBlock
	result     ToolResult
	isError    bool
	durationMs *int
}

func executeSequential(ctx context.Context, current Context, assistant ai.Message, calls []ai.ContentBlock, cfg Config, emit EventSink) toolBatch {
	var outs []finalized
	var messages []ai.Message
	for _, call := range calls {
		emit(Event{Type: "tool_execution_start", ToolCallID: call.ID, ToolName: call.Name, Args: call.Arguments})
		f := runOne(ctx, current, assistant, call, cfg, emit)
		emitEnd(f, emit)
		tr := toolResultMessage(f.call, f.result, f.isError, f.durationMs)
		emit(Event{Type: "message_start", Message: &tr})
		emit(Event{Type: "message_end", Message: &tr})
		outs = append(outs, f)
		messages = append(messages, tr)
		if ctx.Err() != nil {
			break
		}
	}
	return toolBatch{messages: messages, terminate: shouldTerminate(outs)}
}

func executeParallel(ctx context.Context, current Context, assistant ai.Message, calls []ai.ContentBlock, cfg Config, emit EventSink) toolBatch {
	type slot struct {
		imm *finalized
		run func() finalized
	}
	slots := make([]slot, 0, len(calls))
	var emitMu sync.Mutex
	safeEmit := func(ev Event) {
		emitMu.Lock()
		defer emitMu.Unlock()
		emit(ev)
	}

	for _, call := range calls {
		emit(Event{Type: "tool_execution_start", ToolCallID: call.ID, ToolName: call.Name, Args: call.Arguments})
		prep, imm := prepareCall(ctx, current, assistant, call, cfg)
		if imm != nil {
			emitEnd(*imm, emit)
			slots = append(slots, slot{imm: imm})
			if ctx.Err() != nil {
				break
			}
			continue
		}
		p := prep
		c := call
		slots = append(slots, slot{run: func() finalized {
			if ctx.Err() != nil {
				f := finalized{call: c, result: errorResult("Operation aborted"), isError: true}
				emitEnd(f, safeEmit)
				return f
			}
			f := executePrepared(ctx, current, assistant, p, cfg, safeEmit)
			emitEnd(f, safeEmit)
			return f
		}})
		if ctx.Err() != nil {
			break
		}
	}

	finals := make([]finalized, len(slots))
	var wg sync.WaitGroup
	for i, s := range slots {
		if s.imm != nil {
			finals[i] = *s.imm
			continue
		}
		wg.Add(1)
		i, s := i, s
		go func() {
			defer wg.Done()
			finals[i] = s.run()
		}()
	}
	wg.Wait()

	var messages []ai.Message
	for _, f := range finals {
		tr := toolResultMessage(f.call, f.result, f.isError, f.durationMs)
		emit(Event{Type: "message_start", Message: &tr})
		emit(Event{Type: "message_end", Message: &tr})
		messages = append(messages, tr)
	}
	return toolBatch{messages: messages, terminate: shouldTerminate(finals)}
}

type prepared struct {
	call ai.ContentBlock
	tool AgentTool
	args map[string]any
}

func prepareCall(ctx context.Context, current Context, assistant ai.Message, call ai.ContentBlock, cfg Config) (prepared, *finalized) {
	var tool *AgentTool
	for i := range current.Tools {
		if current.Tools[i].Name == call.Name {
			tool = &current.Tools[i]
			break
		}
	}
	if tool == nil {
		f := finalized{call: call, result: errorResult("Tool " + call.Name + " not found"), isError: true}
		return prepared{}, &f
	}
	if call.ArgumentsError != "" {
		f := finalized{call: call, result: errorResult(call.ArgumentsError), isError: true}
		return prepared{}, &f
	}
	args := call.Arguments
	if args == nil {
		args = map[string]any{}
	}
	if cfg.BeforeToolCall != nil {
		br, err := cfg.BeforeToolCall(ctx, assistant, call, args)
		if err != nil {
			f := finalized{call: call, result: errorResult(err.Error()), isError: true}
			return prepared{}, &f
		}
		if ctx.Err() != nil {
			f := finalized{call: call, result: errorResult("Operation aborted"), isError: true}
			return prepared{}, &f
		}
		if br != nil && br.Block {
			res := errorResult(br.Reason)
			if res.Details == nil {
				res.Details = map[string]any{}
			}
			if br.Reason == "" {
				res = errorResult("Tool execution was blocked")
			}
			if br.Terminate {
				res.Terminate = true
			}
			f := finalized{call: call, result: res, isError: true}
			return prepared{}, &f
		}
	}
	return prepared{call: call, tool: *tool, args: args}, nil
}

func runOne(ctx context.Context, current Context, assistant ai.Message, call ai.ContentBlock, cfg Config, emit EventSink) finalized {
	prep, imm := prepareCall(ctx, current, assistant, call, cfg)
	if imm != nil {
		return *imm
	}
	return executePrepared(ctx, current, assistant, prep, cfg, emit)
}

func executePrepared(ctx context.Context, current Context, assistant ai.Message, p prepared, cfg Config, emit EventSink) finalized {
	start := time.Now()
	res, err := p.tool.Execute(ctx, p.call.ID, p.args)
	d := int(time.Since(start).Milliseconds())
	isError := err != nil || res.IsError
	if err != nil {
		res = errorResult(err.Error())
		isError = true
	}
	if cfg.AfterToolCall != nil {
		if ar, aerr := cfg.AfterToolCall(ctx, assistant, p.call, p.args, res, isError); aerr != nil {
			res = errorResult(aerr.Error())
			isError = true
		} else if ar != nil {
			if ar.Content != nil {
				res.Content = ar.Content
			}
			if ar.Details != nil {
				res.Details = ar.Details
			}
			if ar.IsError != nil {
				isError = *ar.IsError
			}
			if ar.Terminate != nil {
				res.Terminate = *ar.Terminate
			}
		}
	}
	return finalized{call: p.call, result: res, isError: isError, durationMs: &d}
}

func emitEnd(f finalized, emit EventSink) {
	emit(Event{Type: "tool_execution_end", ToolCallID: f.call.ID, ToolName: f.call.Name, Result: f.result, IsError: f.isError, DurationMs: f.durationMs})
}

func shouldTerminate(outs []finalized) bool {
	if len(outs) == 0 {
		return false
	}
	for _, o := range outs {
		if !o.result.Terminate {
			return false
		}
	}
	return true
}

func errorResult(msg string) ToolResult {
	return ToolResult{Content: []ai.ContentBlock{ai.TextBlock(msg)}, Details: map[string]any{}}
}

func toolResultMessage(call ai.ContentBlock, res ToolResult, isError bool, duration *int) ai.Message {
	m := ai.ToolResultMessage(call.ID, call.Name, res.Content, isError, time.Now().UnixMilli())
	m.DurationMs = duration
	return m
}

func declareToolChanges(ctx Context, pending []ai.Message) []ai.Message {
	systemIndex := -1
	for i := len(pending) - 1; i >= 0; i-- {
		if pending[i].Role == "system" {
			systemIndex = i
			break
		}
	}
	baseline := pending
	if systemIndex >= 0 {
		baseline = append([]ai.Message{}, pending...)
		baseline[systemIndex] = withToolChanges(pending[systemIndex], nil, nil)
	}
	currentTools := make([]ai.Tool, 0, len(ctx.Tools))
	for _, t := range ctx.Tools {
		currentTools = append(currentTools, ai.Tool{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
	}
	added, removed := ai.ToolStateChanges(ai.GetCurrentTools(append(append([]ai.Message{}, ctx.Messages...), baseline...)), currentTools)
	unchanged := len(added) == 0 && len(removed) == 0
	if systemIndex >= 0 {
		pendingSys := pending[systemIndex]
		if unchanged && len(pendingSys.ToolsAdded) == 0 && len(pendingSys.ToolsRemoved) == 0 {
			return pending
		}
		out := append([]ai.Message{}, baseline...)
		out[systemIndex] = withToolChanges(pending[systemIndex], added, removed)
		return out
	}
	if unchanged {
		return pending
	}
	update := withToolChanges(ai.Message{Role: "system", Timestamp: time.Now().UnixMilli(), Content: ai.EncodeContentString("")}, added, removed)
	insert := len(pending)
	for i, m := range pending {
		if m.Role != "system" {
			insert = i
			break
		}
	}
	out := make([]ai.Message, 0, len(pending)+1)
	out = append(out, pending[:insert]...)
	out = append(out, update)
	out = append(out, pending[insert:]...)
	return out
}

func withToolChanges(m ai.Message, added []ai.Tool, removed []ai.ToolRef) ai.Message {
	m.ToolsAdded = nil
	m.ToolsRemoved = nil
	if len(added) > 0 {
		m.ToolsAdded = added
	}
	if len(removed) > 0 {
		m.ToolsRemoved = removed
	}
	return m
}

func DefaultConvert(msgs []ai.Message) []ai.Message {
	var out []ai.Message
	for _, m := range msgs {
		switch m.Role {
		case "system", "user", "assistant", "toolResult":
			out = append(out, m)
		}
	}
	return out
}
