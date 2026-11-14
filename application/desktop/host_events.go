package main

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xiaokuang95/nulo/sdk"
)

func (a *app) handleSDKEvent(s *session, e *sessionEngine, ev sdk.Event) {
	if s == nil || e == nil || a.sessionByID(s.ID) == nil {
		return
	}
	switch ev.Type {
	case "agent_start":
		s.Status = "running"
		s.Updated = "now"
		s.Modified = time.Now().UnixMilli()
	case "message_start", "message_update":
		if ev.Message != nil && ev.Message.Role == "assistant" {
			s.Status = "running"
			a.syncSessionMeta(s, e.sess)
			a.renderSDKMessages(s, e.sess, ev.Message)
			a.scrollToEnd = true
		}
	case "message_end":
		if ev.Message == nil {
			return
		}
		switch ev.Message.Role {
		case "assistant":
			a.syncSessionMeta(s, e.sess)
			a.renderSDKMessages(s, e.sess, nil)
		case "toolResult":
			a.renderSDKMessages(s, e.sess, nil)
			a.appendTerminalResult(s, *ev.Message)
			if ev.Message.ToolName == "edit" || ev.Message.ToolName == "write" || ev.Message.ToolName == "bash" || ev.Message.ToolName == "powershell" {
				a.invalidateFileIndex(s.CWD)
				a.requestChanges(s)
			}
		case "user":
			a.renderSDKMessages(s, e.sess, nil)
		}
	case "tool_execution_start":
		a.updateToolBlock(s, ev.ToolCallID, "running", "", false)
		a.appendTerminalStart(s, ev.ToolName, ev.Args)
		a.scrollToEnd = true
	case "tool_execution_end":
		status := "done"
		if ev.IsError {
			status = "fail"
			if ev.ToolName == "plan" {
				a.pendingToast = "Plan update failed"
			}
		}
		a.updateToolBlock(s, ev.ToolCallID, status, "", ev.IsError)
		a.scrollToEnd = true
	case "agent_settled":
		a.syncSessionMeta(s, e.sess)
		a.updateRealUsage(s, e.sess)
		if s.pendingApproval() != nil {
			s.Status = "awaiting"
		} else if reason := lastStopReason(e.sess.Messages()); reason == sdk.StopError {
			s.Status = "error"
		} else if reason == sdk.StopAborted {
			s.Status = "paused"
		} else {
			s.Status = "done"
		}
		a.renderSDKMessages(s, e.sess, nil)
		s.Updated = "now"
		s.Modified = time.Now().UnixMilli()
		s.Note = messageNote(s.Updated, countRealMessages(e.sess.Messages()))
		a.scrollToEnd = true
		a.requestChanges(s)
	case "plan_update":
		if ev.Plan != nil {
			a.applySDKPlan(s, *ev.Plan)
		}
	case "compaction_start":
		s.Status = "running"
	case "compaction_end":
		a.renderSDKMessages(s, e.sess, nil)
		a.updateRealUsage(s, e.sess)
	case "host_compact_done":
		outcome, _ := ev.Result.(compactOutcome)
		s.Status = "done"
		if outcome.err != nil {
			s.Status = "error"
			s.Notices = append(s.Notices, message{Role: "notice", Time: "now", Text: "Compact failed · " + outcome.err.Error()})
		} else if outcome.result != nil {
			after := s.Context.Used
			a.updateRealUsage(s, e.sess)
			s.Notices = append(s.Notices, message{Role: "notice", Time: "now", Text: compactNotice(outcome.result.TokensBefore, after)})
		}
		a.renderSDKMessages(s, e.sess, nil)
		a.scrollToEnd = true
	case "host_error":
		s.Status = "error"
		text, _ := ev.Result.(string)
		if text == "" {
			text = "The session stopped unexpectedly."
		}
		s.Notices = append(s.Notices, message{Role: "notice", Time: "now", Text: "Request failed · " + text})
		a.renderSDKMessages(s, e.sess, nil)
		a.scrollToEnd = true
		a.notifyCompletion(s)
	}
}

func (a *app) syncSessionMeta(s *session, sess *sdk.Session) {
	if sess == nil {
		return
	}
	model := sess.Model()
	if model.ID != "" {
		s.Model = model.Provider + "/" + model.ID
		s.ModelName = model.Name
		s.ModelLimit = model.ContextWindow
		s.Context.Limit = model.ContextWindow
	}
	s.Thinking = string(sess.ThinkingLevel())
	a.applyModelFields(s)
	s.SkillNames = sess.SkillNames()
	s.MCPServers = sess.MCPServerNames()
	s.PluginNames = sess.PluginNames()
	s.PluginDiags = nil
	for _, diag := range sess.PluginDiagnostics() {
		s.PluginDiags = append(s.PluginDiags, diag.Error())
	}
	s.Context.Skills = len(s.SkillNames)
	s.Context.MCP = len(s.MCPServers)
	s.Context.Plugins = len(s.PluginNames)
	applyMCPHealth(s)
}

func (a *app) renderSDKMessages(s *session, sess *sdk.Session, partial *sdk.Message) {
	if sess == nil {
		return
	}
	msgs := sess.Messages()
	if partial != nil {
		msgs = append(msgs, *partial)
	}
	out := make([]message, 0, len(msgs))
	toolBlocks := make(map[string]*block)
	approvals := make(map[string]block, len(s.Approvals))
	for _, approval := range s.Approvals {
		approvals[approval.CallID] = approval
	}
	placedApprovals := make(map[string]bool, len(approvals))
	var firstUser string
	hasConversation := false
	for _, m := range msgs {
		switch m.Role {
		case "user":
			text := m.ContentString()
			if strings.TrimSpace(text) == "" {
				continue
			}
			if firstUser == "" {
				firstUser = text
			}
			hasConversation = true
			out = append(out, message{Role: "user", Time: messageTime(m.Timestamp), RawTime: m.Timestamp, Text: text})
		case "assistant":
			um := message{Role: "assistant", Time: messageTime(m.Timestamp), RawTime: m.Timestamp}
			hasConversation = true
			var toolIDs []string
			for _, cb := range m.ContentBlocks() {
				switch cb.Type {
				case "thinking":
					if strings.TrimSpace(cb.Thinking) == "" {
						continue
					}
					um.Blocks = append(um.Blocks, block{Type: "thinking", Text: cb.Thinking, Duration: durationLabel(m.DurationMs)})
				case "text":
					if strings.TrimSpace(cb.Text) == "" {
						continue
					}
					um.Blocks = append(um.Blocks, block{Type: "text", Text: cb.Text})
				case "toolCall":
					if cb.Name == "plan" {
						continue
					}
					b := block{
						Type: "tool", Tool: cb.Name, CallID: cb.ID, Status: "running",
						Title: toolTitle(cb.Name, cb.Arguments), Meta: toolMeta(cb.Name, cb.Arguments),
						Output: "",
					}
					um.Blocks = append(um.Blocks, b)
					toolIDs = append(toolIDs, cb.ID)
					if approval, ok := approvals[cb.ID]; ok {
						um.Blocks = append(um.Blocks, approval)
						placedApprovals[cb.ID] = true
					}
				}
			}
			if m.StopReason == sdk.StopError {
				text := strings.TrimSpace(m.ErrorMessage)
				if text == "" {
					text = "The provider returned an error."
				}
				um.Blocks = append(um.Blocks, block{Type: "text", Text: "**Request failed**\n\n" + text})
			} else if m.StopReason == sdk.StopAborted {
				um.Blocks = append(um.Blocks, block{Type: "text", Text: "_Stopped._"})
			}
			if len(um.Blocks) > 0 {
				out = append(out, um)
				// Resolve pointers only after the assistant message is in its
				// final backing array; appending several tool calls can grow
				// the slice and invalidate earlier pointers.
				last := &out[len(out)-1]
				for _, id := range toolIDs {
					for i := range last.Blocks {
						if last.Blocks[i].CallID == id {
							toolBlocks[id] = &last.Blocks[i]
							break
						}
					}
				}
			}
		case "toolResult":
			b := toolBlocks[m.ToolCallID]
			if b == nil {
				continue
			}
			b.Status = "done"
			if m.IsError {
				b.Status = "fail"
			}
			b.Output = clipText(m.ContentString(), 12000)
			b.Duration = durationLabel(m.DurationMs)
		}
	}
	var unplaced []block
	for _, approval := range s.Approvals {
		if !placedApprovals[approval.CallID] {
			unplaced = append(unplaced, approval)
		}
	}
	if len(unplaced) > 0 {
		approvalMessage := message{Role: "assistant", Time: "now"}
		approvalMessage.Blocks = unplaced
		out = append(out, approvalMessage)
	}
	for _, notice := range s.Notices {
		out = append(out, notice)
	}
	assignMessageEntryIDs(sess, out)
	s.Welcome = !hasConversation
	if firstUser != "" && (s.Title == "" || s.Title == "Untitled session" || strings.HasPrefix(s.Title, "Untitled")) {
		s.Title = titleFor(firstUser)
	}
	for _, msg := range out {
		for i := range msg.Blocks {
			b := &msg.Blocks[i]
			if b.Type == "tool" && b.Status == "running" && s.Status != "running" && s.Status != "awaiting" {
				b.Status = "fail"
			}
		}
	}
	s.Messages = out
}

func assignMessageEntryIDs(sess *sdk.Session, messages []message) {
	if sess == nil {
		return
	}
	type key struct {
		role string
		ts   int64
	}
	ids := map[key]string{}
	for _, entry := range sess.Branch() {
		if entry.Type != "message" || entry.Message == nil {
			continue
		}
		role := entry.Message.Role
		if role != "user" && role != "assistant" {
			continue
		}
		ids[key{role: role, ts: entry.Message.Timestamp}] = entry.ID
	}
	for i := range messages {
		m := &messages[i]
		if m.RawTime == 0 || (m.Role != "user" && m.Role != "assistant") {
			continue
		}
		m.EntryID = ids[key{role: m.Role, ts: m.RawTime}]
	}
}

func clipText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "\n… output truncated by the desktop host"
}

func messageTime(ms int64) string {
	if ms <= 0 {
		return "now"
	}
	return time.UnixMilli(ms).Format("15:04")
}

func durationLabel(ms *int) string {
	if ms == nil || *ms <= 0 {
		return ""
	}
	d := time.Duration(*ms) * time.Millisecond
	if d < time.Second {
		return "<1s"
	}
	return fmt.Sprintf("%.0fs", d.Seconds())
}

func toolTitle(name string, args map[string]any) string {
	path := stringArg(args, "path")
	command := stringArg(args, "command")
	switch name {
	case "read":
		return "Read " + shortPath(path)
	case "edit":
		return "Edit " + shortPath(path)
	case "write":
		return "Write " + shortPath(path)
	case "bash", "powershell":
		if command != "" {
			return "$ " + clipLine(command, 94)
		}
		return name
	case "tool_search":
		return "Search tools"
	default:
		return name
	}
}

func toolMeta(name string, args map[string]any) string {
	switch name {
	case "read":
		if offset, ok := numberArg(args, "offset"); ok {
			return fmt.Sprintf("from line %d", int(offset))
		}
	case "edit", "write":
		return stringArg(args, "mode")
	}
	return ""
}

func shortPath(path string) string {
	if path == "" {
		return "file"
	}
	path = strings.TrimPrefix(path, "./")
	parts := strings.Split(path, "/")
	if len(parts) > 3 {
		return strings.Join(parts[len(parts)-3:], "/")
	}
	return path
}

func clipLine(text string, max int) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
	r := []rune(text)
	if len(r) <= max {
		return text
	}
	return string(r[:max-1]) + "…"
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, _ := args[key].(string)
	return value
}

func numberArg(args map[string]any, key string) (float64, bool) {
	if args == nil {
		return 0, false
	}
	switch value := args[key].(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	}
	return 0, false
}

func (a *app) updateToolBlock(s *session, callID, status, output string, isError bool) {
	for mi := range s.Messages {
		for bi := range s.Messages[mi].Blocks {
			b := &s.Messages[mi].Blocks[bi]
			if b.Type != "tool" || b.CallID != callID {
				continue
			}
			b.Status = status
			if output != "" {
				b.Output = clipText(output, 12000)
			}
			if isError {
				b.Status = "fail"
			}
			return
		}
	}
}

func (a *app) appendTerminalStart(s *session, tool string, args map[string]any) {
	if tool != "bash" && tool != "powershell" {
		return
	}
	command := stringArg(args, "command")
	if command == "" {
		return
	}
	s.Terminal = append(s.Terminal, termLine{Kind: "cmd", Text: clipLine(command, 160)})
	s.Terminal = capTerminal(s.Terminal)
}

func (a *app) appendTerminalResult(s *session, m sdk.Message) {
	if m.ToolName != "bash" && m.ToolName != "powershell" {
		return
	}
	out := strings.TrimRight(m.ContentString(), "\n")
	if out != "" {
		for _, line := range strings.Split(out, "\n") {
			kind := "out"
			s.Terminal = append(s.Terminal, termLine{Kind: kind, Text: clipLine(line, 220)})
		}
	}
	if m.IsError {
		s.Terminal = append(s.Terminal, termLine{Kind: "err", Text: "exit error"})
	} else {
		s.Terminal = append(s.Terminal, termLine{Kind: "ok", Text: "done"})
	}
	s.Terminal = capTerminal(s.Terminal)
}

func capTerminal(lines []termLine) []termLine {
	if len(lines) <= 120 {
		return lines
	}
	return append([]termLine(nil), lines[len(lines)-120:]...)
}

func lastStopReason(messages []sdk.Message) sdk.StopReason {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			return messages[i].StopReason
		}
	}
	return ""
}

func countRealMessages(messages []sdk.Message) int {
	count := 0
	for _, m := range messages {
		if m.Role == "user" || m.Role == "assistant" {
			count++
		}
	}
	return count
}

func (a *app) updateRealUsage(s *session, sess *sdk.Session) {
	msgs := sess.Messages()
	used := 0
	breakdown := []contextSlice{}
	systemTokens, messageTokens, toolTokens := 0, 0, 0
	fileTokens := 0
	seenFiles := map[string]bool{}
	var entries []contextEntry
	for _, m := range msgs {
		switch m.Role {
		case "system":
			systemTokens += estimateTokens(m.ContentString())
			for _, section := range m.Sections {
				if section != nil {
					systemTokens += estimateTokens(*section)
				}
			}
			for _, block := range m.ContentBlocks() {
				systemTokens += estimateTokens(block.Text + block.Thinking)
			}
		case "assistant":
			for _, block := range m.ContentBlocks() {
				switch block.Type {
				case "text", "thinking":
					messageTokens += estimateTokens(block.Text + block.Thinking)
				case "toolCall":
					if path := stringArg(block.Arguments, "path"); path != "" && !seenFiles[path] && len(entries) < 7 {
						seenFiles[path] = true
						tokens := 600
						fileTokens += tokens
						entries = append(entries, contextEntry{Kind: "file", Path: path, Tokens: tokens})
					}
				}
			}
			if m.Usage != nil {
				total := m.Usage.Input + m.Usage.CacheRead + m.Usage.CacheWrite + m.Usage.Output
				if total > used {
					used = total
				}
			}
		case "toolResult":
			toolTokens += estimateTokens(m.ContentString())
		case "user":
			messageTokens += estimateTokens(m.ContentString())
		}
	}
	if used == 0 {
		used = systemTokens + messageTokens + toolTokens + fileTokens
	}
	if used < 1 {
		used = 1
	}
	breakdown = append(breakdown,
		contextSlice{Label: "System prompt", Value: systemTokens, Kind: "accent"},
		contextSlice{Label: "Messages", Value: messageTokens, Kind: "info"},
		contextSlice{Label: "Tool results", Value: toolTokens, Kind: "warn"},
		contextSlice{Label: "Files", Value: fileTokens, Kind: "violet"},
	)
	s.Context.Used = used
	s.Context.Limit = s.modelContext()
	s.Context.Breakdown = breakdown
	s.Context.Entries = entries
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	runes := len([]rune(text))
	return max(1, runes/4)
}

func (a *app) applySDKPlan(s *session, p sdk.Plan) {
	if s == nil {
		return
	}
	next := plan{Title: p.Title, Next: p.Next}
	for _, step := range p.Steps {
		status := step.Status
		switch status {
		case "pending", "active", "done", "blocked":
		default:
			status = "pending"
		}
		next.Steps = append(next.Steps, planStep{
			Title: step.Title, Detail: step.Detail, Status: status,
		})
	}
	s.Plan = next
}
