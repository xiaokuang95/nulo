package ai

import (
	"time"
)

func errorStream(model Model, msg string, reason StopReason) *EventStream {
	if reason == "" {
		reason = StopError
	}
	s := NewEventStream()
	go func() {
		m := AssistantMessage(nil, model, reason, time.Now().UnixMilli())
		m.ErrorMessage = msg
		s.Push(AssistantMessageEvent{Type: "error", Reason: reason, Error: &m})
		s.End(nil)
	}()
	return s
}
