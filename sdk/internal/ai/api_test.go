package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func collectTypes(s *EventStream) (types []string, result Message) {
	for ev := range s.Events() {
		types = append(types, ev.Type)
	}
	return types, s.Result()
}

func TestCompletionsTextAndTool(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(string(b), `"hello"`) {
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
			io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"a.go\\\"}\"}}]}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	model := Model{ID: "gpt", Provider: "openai", API: "openai-completions", BaseURL: srv.URL, MaxTokens: 128}
	tr := Transcript{Messages: []Message{UserMessage("hello", 1)}}
	s := StreamOpenAICompletions(context.Background(), model, tr, StreamOptions{APIKey: "k"})
	_, result := collectTypes(s)
	if result.StopReason != StopStop || MessageText(result) != "Hi" {
		t.Fatalf("text %+v %q", result.StopReason, MessageText(result))
	}
	s = StreamOpenAICompletions(context.Background(), model, Transcript{Messages: []Message{UserMessage("tool", 1)}}, StreamOptions{APIKey: "k"})
	_, result = collectTypes(s)
	if result.StopReason != StopToolUse {
		t.Fatalf("tool stop %s", result.StopReason)
	}
	calls := result.ContentBlocks()
	if len(calls) == 0 || calls[0].Name != "read" {
		t.Fatalf("blocks %+v", calls)
	}
}

func TestAnthropicTextAndTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(string(b), "hello") {
			io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
			io.WriteString(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\"}}\n\n")
			io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hey\"}}\n\n")
			io.WriteString(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
			io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
			return
		}
		io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		io.WriteString(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"t1\",\"name\":\"read\"}}\n\n")
		io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\\\"x\\\"}\"}}\n\n")
		io.WriteString(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\n")
	}))
	defer srv.Close()
	model := Model{ID: "claude", Provider: "anthropic", API: "anthropic-messages", BaseURL: srv.URL, MaxTokens: 128}
	s := StreamAnthropicMessages(context.Background(), model, Transcript{Messages: []Message{UserMessage("hello", 1)}}, StreamOptions{APIKey: "k"})
	_, result := collectTypes(s)
	if MessageText(result) != "Hey" {
		t.Fatalf("%q stop=%s err=%s", MessageText(result), result.StopReason, result.ErrorMessage)
	}
	s = StreamAnthropicMessages(context.Background(), model, Transcript{Messages: []Message{UserMessage("tool", 1)}}, StreamOptions{APIKey: "k"})
	_, result = collectTypes(s)
	if result.StopReason != StopToolUse {
		t.Fatalf("stop %s", result.StopReason)
	}
}

func TestResponsesText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.created\"}\n\n")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Yo\"}\n\n")
		io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
	}))
	defer srv.Close()
	model := Model{ID: "gpt", Provider: "openai", API: "openai-responses", BaseURL: srv.URL}
	s := StreamOpenAIResponses(context.Background(), model, Transcript{Messages: []Message{UserMessage("hi", 1)}}, StreamOptions{APIKey: "k"})
	_, result := collectTypes(s)
	if MessageText(result) != "Yo" {
		t.Fatalf("%q %s %s", MessageText(result), result.StopReason, result.ErrorMessage)
	}
}

func TestMissingKey(t *testing.T) {
	model := Model{ID: "gpt", Provider: "openai", API: "openai-completions", BaseURL: "http://127.0.0.1:1"}
	s := StreamOpenAICompletions(context.Background(), model, Transcript{}, StreamOptions{})
	_, result := collectTypes(s)
	if result.StopReason != StopError || result.ErrorMessage == "" {
		t.Fatalf("%+v", result)
	}
}
