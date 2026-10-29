package ai

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const (
	DefaultFauxAPI      = "faux"
	DefaultFauxProvider = "faux"
	DefaultFauxModelID  = "faux-1"
)

type FauxProvider struct {
	API      string
	Provider string
	Models   []Model
	mu       sync.Mutex
	queue    []Message
}

func NewFauxProvider() *FauxProvider {
	return &FauxProvider{
		API:      DefaultFauxAPI,
		Provider: DefaultFauxProvider,
		Models:   []Model{DefaultFauxModel()},
	}
}

func DefaultFauxModel() Model {
	return Model{
		ID:            DefaultFauxModelID,
		Name:          "Faux Model",
		API:           DefaultFauxAPI,
		Provider:      DefaultFauxProvider,
		BaseURL:       "http://localhost:0",
		Input:         []string{"text", "image"},
		ContextWindow: 128000,
		MaxTokens:     16384,
	}
}

func (p *FauxProvider) SetResponses(msgs ...Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queue = append([]Message(nil), msgs...)
}

func (p *FauxProvider) AppendResponses(msgs ...Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queue = append(p.queue, msgs...)
}

func FauxAssistant(text string) Message {
	return AssistantMessage([]ContentBlock{TextBlock(text)}, DefaultFauxModel(), StopStop, time.Now().UnixMilli())
}

func FauxAssistantBlocks(blocks []ContentBlock, reason StopReason) Message {
	if reason == "" {
		reason = StopStop
	}
	return AssistantMessage(blocks, DefaultFauxModel(), reason, time.Now().UnixMilli())
}

func FauxError(errText string) Message {
	msg := AssistantMessage(nil, DefaultFauxModel(), StopError, time.Now().UnixMilli())
	msg.ErrorMessage = errText
	return msg
}

func (p *FauxProvider) StreamSimple(ctx context.Context, model Model, _ Transcript, _ StreamOptions) *EventStream {
	s := NewEventStream()
	go p.run(ctx, model, s)
	return s
}

func (p *FauxProvider) run(ctx context.Context, model Model, s *EventStream) {
	defer s.End(nil)

	p.mu.Lock()
	if len(p.queue) == 0 {
		p.mu.Unlock()
		msg := FauxError("No more faux responses queued")
		msg.API = p.API
		msg.Provider = p.Provider
		msg.Model = model.ID
		s.Push(AssistantMessageEvent{Type: "error", Reason: StopError, Error: &msg})
		return
	}
	msg := p.queue[0]
	p.queue = p.queue[1:]
	p.mu.Unlock()
	msg.API = p.API
	msg.Provider = p.Provider
	if model.ID != "" {
		msg.Model = model.ID
	}
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
	}

	if err := streamWithDeltas(ctx, s, msg); err != nil {
		aborted := msg
		aborted.StopReason = StopAborted
		aborted.ErrorMessage = "Request was aborted"
		s.Push(AssistantMessageEvent{Type: "error", Reason: StopAborted, Error: &aborted})
	}
}

func streamWithDeltas(ctx context.Context, s *EventStream, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	partial := msg
	partial.Content = EncodeContentBlocks(nil)
	partial.StopReason = StopPending
	s.Push(AssistantMessageEvent{Type: "start", Partial: cloneMsg(&partial)})

	blocks := msg.ContentBlocks()
	built := make([]ContentBlock, 0, len(blocks))
	for i, block := range blocks {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch block.Type {
		case "thinking":
			built = append(built, ContentBlock{Type: "thinking"})
			partial.Content = EncodeContentBlocks(built)
			s.Push(AssistantMessageEvent{Type: "thinking_start", ContentIndex: i, Partial: cloneMsg(&partial)})
			for _, chunk := range chunkText(block.Thinking, 4) {
				if err := ctx.Err(); err != nil {
					return err
				}
				built[i].Thinking += chunk
				partial.Content = EncodeContentBlocks(built)
				s.Push(AssistantMessageEvent{Type: "thinking_delta", ContentIndex: i, Delta: chunk, Partial: cloneMsg(&partial)})
			}
			built[i] = block
			partial.Content = EncodeContentBlocks(built)
			s.Push(AssistantMessageEvent{Type: "thinking_end", ContentIndex: i, Content: block.Thinking, Partial: cloneMsg(&partial)})
		case "text":
			built = append(built, ContentBlock{Type: "text"})
			partial.Content = EncodeContentBlocks(built)
			s.Push(AssistantMessageEvent{Type: "text_start", ContentIndex: i, Partial: cloneMsg(&partial)})
			for _, chunk := range chunkText(block.Text, 4) {
				if err := ctx.Err(); err != nil {
					return err
				}
				built[i].Text += chunk
				partial.Content = EncodeContentBlocks(built)
				s.Push(AssistantMessageEvent{Type: "text_delta", ContentIndex: i, Delta: chunk, Partial: cloneMsg(&partial)})
			}
			built[i] = block
			partial.Content = EncodeContentBlocks(built)
			s.Push(AssistantMessageEvent{Type: "text_end", ContentIndex: i, Content: block.Text, Partial: cloneMsg(&partial)})
		case "toolCall":
			built = append(built, ContentBlock{Type: "toolCall", ID: block.ID, Name: block.Name, Arguments: map[string]any{}})
			partial.Content = EncodeContentBlocks(built)
			s.Push(AssistantMessageEvent{Type: "toolcall_start", ContentIndex: i, Partial: cloneMsg(&partial)})
			raw, _ := json.Marshal(block.Arguments)
			if len(raw) == 0 {
				raw = []byte("{}")
			}
			for _, chunk := range chunkText(string(raw), 4) {
				if err := ctx.Err(); err != nil {
					return err
				}
				s.Push(AssistantMessageEvent{Type: "toolcall_delta", ContentIndex: i, Delta: chunk, Partial: cloneMsg(&partial)})
			}
			built[i] = block
			partial.Content = EncodeContentBlocks(built)
			tc := block
			s.Push(AssistantMessageEvent{Type: "toolcall_end", ContentIndex: i, ToolCall: &tc, Partial: cloneMsg(&partial)})
		default:
			built = append(built, block)
			partial.Content = EncodeContentBlocks(built)
		}
	}

	if msg.StopReason == "" || msg.StopReason == StopPending {
		msg.StopReason = StopStop
	}
	if msg.StopReason == StopError || msg.StopReason == StopAborted {
		s.Push(AssistantMessageEvent{Type: "error", Reason: msg.StopReason, Error: cloneMsg(&msg)})
		return nil
	}
	s.Push(AssistantMessageEvent{Type: "done", Reason: msg.StopReason, Message: cloneMsg(&msg)})
	return nil
}

func chunkText(s string, n int) []string {
	if s == "" {
		return []string{""}
	}
	if n < 1 {
		n = 1
	}
	var out []string
	r := []rune(s)
	for i := 0; i < len(r); i += n {
		j := i + n
		if j > len(r) {
			j = len(r)
		}
		out = append(out, string(r[i:j]))
	}
	return out
}

func cloneMsg(m *Message) *Message {
	cp := *m
	if m.Usage != nil {
		u := *m.Usage
		cp.Usage = &u
	}
	if m.Content != nil {
		cp.Content = append(json.RawMessage(nil), m.Content...)
	}
	return &cp
}
