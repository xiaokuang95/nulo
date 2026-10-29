package ai

import (
	"context"
	"sync"
	"time"
)

type AssistantMessageEvent struct {
	Type         string        `json:"type"`
	Partial      *Message      `json:"partial,omitempty"`
	ContentIndex int           `json:"contentIndex,omitempty"`
	Delta        string        `json:"delta,omitempty"`
	Content      string        `json:"content,omitempty"`
	ToolCall     *ContentBlock `json:"toolCall,omitempty"`
	Reason       StopReason    `json:"reason,omitempty"`
	Message      *Message      `json:"message,omitempty"`
	Error        *Message      `json:"error,omitempty"`
}

type EventStream struct {
	ch        chan AssistantMessageEvent
	resultCh  chan Message
	startedAt time.Time
	monoStart time.Time

	mu        sync.Mutex
	closed    bool
	resultSet bool
}

func NewEventStream() *EventStream {
	now := time.Now()
	return &EventStream{
		ch:        make(chan AssistantMessageEvent, 1024),
		resultCh:  make(chan Message, 1),
		startedAt: now,
		monoStart: now,
	}
}

func (s *EventStream) Events() <-chan AssistantMessageEvent { return s.ch }

func (s *EventStream) Result() Message { return <-s.resultCh }

func (s *EventStream) Push(ev AssistantMessageEvent) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if ev.Type == "done" && ev.Message != nil {
		s.time(ev.Message)
		s.finish(*ev.Message)
	}
	if ev.Type == "error" && ev.Error != nil {
		s.time(ev.Error)
		s.finish(*ev.Error)
	}
	s.mu.Unlock()
	s.ch <- ev
}

func (s *EventStream) End(msg *Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if msg != nil {
		s.time(msg)
		s.finish(*msg)
	}
	s.closed = true
	close(s.ch)
}

func (s *EventStream) time(msg *Message) {
	if msg.DurationMs != nil {
		return
	}
	if msg.Timestamp != 0 && time.UnixMilli(msg.Timestamp).Before(s.startedAt.Add(-time.Second)) {
		return
	}
	d := int(time.Since(s.monoStart).Milliseconds())
	if d < 0 {
		d = 0
	}
	msg.DurationMs = &d
}

func (s *EventStream) finish(msg Message) {
	if s.resultSet {
		return
	}
	s.resultSet = true
	s.resultCh <- msg
}

type StreamOptions struct {
	APIKey        string
	Headers       map[string]string
	Env           map[string]string
	MaxTokens     int
	ThinkingLevel ThinkingLevel
	ToolChoice    string
}

type StreamFn func(ctx context.Context, model Model, transcript Transcript, opts StreamOptions) *EventStream

type Transcript struct {
	Messages []Message
}
