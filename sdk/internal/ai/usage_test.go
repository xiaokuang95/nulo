package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rates mirrors the shape of a models.json cost block: per-million-token prices.
var usageTestRates = Cost{Input: 0.3, Output: 1.2, CacheRead: 0.006, CacheWrite: 0.375}

func TestParseChunkUsageCacheNames(t *testing.T) {
	// The three spellings providers use for cache reads, plus the cache-write
	// field, all collapse onto the same accounting.
	cases := []struct {
		name string
		raw  string
	}{
		{"openai prompt_tokens_details", `{"prompt_tokens":1219,"completion_tokens":75,"prompt_tokens_details":{"cached_tokens":1152}}`},
		{"deepseek prompt_cache_hit_tokens", `{"prompt_tokens":1219,"completion_tokens":75,"prompt_cache_hit_tokens":1152}`},
		{"kimi top-level cached_tokens", `{"prompt_tokens":1219,"completion_tokens":75,"cached_tokens":1152}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw oaiUsageFields
			if err := json.Unmarshal([]byte(tc.raw), &raw); err != nil {
				t.Fatal(err)
			}
			u := parseChunkUsage(raw, Model{Cost: usageTestRates})
			if u.Input != 67 || u.Output != 75 || u.CacheRead != 1152 || u.CacheWrite != 0 {
				t.Fatalf("counts %+v", u)
			}
			if u.TotalTokens != 1294 {
				t.Fatalf("totalTokens %d, want 1294", u.TotalTokens)
			}
		})
	}
}

func TestParseChunkUsageSubtractsCacheFromPrompt(t *testing.T) {
	var raw oaiUsageFields
	_ = json.Unmarshal([]byte(`{"prompt_tokens":100,"completion_tokens":10,"prompt_tokens_details":{"cached_tokens":40,"cache_write_tokens":10},"completion_tokens_details":{"reasoning_tokens":7}}`), &raw)
	u := parseChunkUsage(raw, Model{Cost: usageTestRates})
	if u.Input != 50 || u.CacheRead != 40 || u.CacheWrite != 10 {
		t.Fatalf("counts %+v", u)
	}
	if u.Reasoning == nil || *u.Reasoning != 7 {
		t.Fatalf("reasoning %+v", u.Reasoning)
	}
	if u.TotalTokens != 110 {
		t.Fatalf("totalTokens %d, want 110", u.TotalTokens)
	}

	// A provider that counts nothing but the prompt must not go negative.
	_ = json.Unmarshal([]byte(`{"prompt_tokens":10,"prompt_tokens_details":{"cached_tokens":99}}`), &raw)
	if u := parseChunkUsage(raw, Model{}); u.Input != 0 {
		t.Fatalf("input %d, want 0", u.Input)
	}
}

func TestCalculateCost(t *testing.T) {
	model := Model{Cost: Cost{Input: 0.3, Output: 1.2, CacheRead: 0.006, CacheWrite: 0.375}}
	oneh := 500
	u := &Usage{Input: 1000, Output: 2000, CacheRead: 10000, CacheWrite: 1500, CacheWrite1h: &oneh}
	calculateCost(model, u)

	// Cache writes bill the 1h portion at twice the input rate.
	want := Cost{
		Input:      0.3 / 1e6 * 1000,
		Output:     1.2 / 1e6 * 2000,
		CacheRead:  0.006 / 1e6 * 10000,
		CacheWrite: (0.375*1000 + 0.3*2*500) / 1e6,
	}
	want.Total = want.Input + want.Output + want.CacheRead + want.CacheWrite
	if !closeEnough(u.Cost.Input, want.Input) || !closeEnough(u.Cost.Output, want.Output) ||
		!closeEnough(u.Cost.CacheRead, want.CacheRead) || !closeEnough(u.Cost.CacheWrite, want.CacheWrite) ||
		!closeEnough(u.Cost.Total, want.Total) {
		t.Fatalf("cost %+v, want %+v", u.Cost, want)
	}
}

func closeEnough(a, b float64) bool {
	d := a - b
	return d < 1e-12 && d > -1e-12
}

func TestAnthropicUsageMergesMessageDelta(t *testing.T) {
	var start, delta anthropicUsageFields
	_ = json.Unmarshal([]byte(`{"input_tokens":100,"output_tokens":1,"cache_read_input_tokens":900,"cache_creation_input_tokens":50}`), &start)
	// message_delta revises only the output side; the prompt counters must survive.
	_ = json.Unmarshal([]byte(`{"output_tokens":250,"output_tokens_details":{"thinking_tokens":40}}`), &delta)

	u := ZeroUsage()
	start.apply(&u, Model{Cost: usageTestRates})
	delta.apply(&u, Model{Cost: usageTestRates})

	if u.Input != 100 || u.Output != 250 || u.CacheRead != 900 || u.CacheWrite != 50 {
		t.Fatalf("counts %+v", u)
	}
	if u.Reasoning == nil || *u.Reasoning != 40 {
		t.Fatalf("reasoning %+v", u.Reasoning)
	}
	if u.TotalTokens != 1300 {
		t.Fatalf("totalTokens %d, want 1300", u.TotalTokens)
	}
	if u.Cost.Total == 0 {
		t.Fatal("cost was not computed")
	}
}

func TestStreamCompletionsReportsUsage(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		// The final usage chunk carries no choices.
		io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1219,\"completion_tokens\":75,\"prompt_tokens_details\":{\"cached_tokens\":1152}}}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	model := Model{ID: "gpt", Provider: "openai", API: "openai-completions", BaseURL: srv.URL, Cost: usageTestRates}
	_, result := collectTypes(StreamOpenAICompletions(context.Background(), model,
		Transcript{Messages: []Message{UserMessage("hello", 1)}}, StreamOptions{APIKey: "k"}))

	if opts := gotBody["stream_options"]; opts == nil {
		t.Fatal("stream_options must be requested, otherwise streaming carries no usage")
	}
	if result.Usage == nil {
		t.Fatal("no usage on the assistant message")
	}
	if result.Usage.Input != 67 || result.Usage.Output != 75 || result.Usage.CacheRead != 1152 {
		t.Fatalf("usage %+v", result.Usage)
	}
	if result.Usage.Cost.Total <= 0 {
		t.Fatalf("cost %+v", result.Usage.Cost)
	}
}

func TestStreamCompletionsAcceptsChoiceUsage(t *testing.T) {
	// Moonshot reports usage on the choice rather than on the chunk.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"},\"usage\":{\"prompt_tokens\":30,\"completion_tokens\":5}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	model := Model{ID: "m", Provider: "p", API: "openai-completions", BaseURL: srv.URL}
	_, result := collectTypes(StreamOpenAICompletions(context.Background(), model,
		Transcript{Messages: []Message{UserMessage("hello", 1)}}, StreamOptions{APIKey: "k"}))
	if result.Usage == nil || result.Usage.Input != 30 || result.Usage.Output != 5 {
		t.Fatalf("usage %+v", result.Usage)
	}
}

func TestStreamAnthropicReportsUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":1,\"cache_read_input_tokens\":900}}}\n\n")
		io.WriteString(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\"}}\n\n")
		io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n")
		io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":250}}\n\n")
	}))
	defer srv.Close()

	model := Model{ID: "c", Provider: "anthropic", API: "anthropic-messages", BaseURL: srv.URL, Cost: usageTestRates}
	_, result := collectTypes(StreamAnthropicMessages(context.Background(), model,
		Transcript{Messages: []Message{UserMessage("hello", 1)}}, StreamOptions{APIKey: "k"}))
	if result.Usage == nil {
		t.Fatal("no usage")
	}
	if result.Usage.Input != 100 || result.Usage.CacheRead != 900 || result.Usage.Output != 250 {
		t.Fatalf("usage %+v", result.Usage)
	}
	if result.Usage.TotalTokens != 1250 || result.Usage.Cost.Total <= 0 {
		t.Fatalf("usage %+v", result.Usage)
	}
}

func TestStreamResponsesReportsUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\n")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1219,\"output_tokens\":75,\"total_tokens\":1294,\"input_tokens_details\":{\"cached_tokens\":1152},\"output_tokens_details\":{\"reasoning_tokens\":9}}}}\n\n")
	}))
	defer srv.Close()

	model := Model{ID: "g", Provider: "openai", API: "openai-responses", BaseURL: srv.URL, Cost: usageTestRates}
	_, result := collectTypes(StreamOpenAIResponses(context.Background(), model,
		Transcript{Messages: []Message{UserMessage("hello", 1)}}, StreamOptions{APIKey: "k"}))
	if result.Usage == nil {
		t.Fatal("no usage")
	}
	// input_tokens includes the cached tokens, so they are subtracted out.
	if result.Usage.Input != 67 || result.Usage.CacheRead != 1152 || result.Usage.Output != 75 {
		t.Fatalf("usage %+v", result.Usage)
	}
	if result.Usage.Reasoning == nil || *result.Usage.Reasoning != 9 {
		t.Fatalf("reasoning %+v", result.Usage.Reasoning)
	}
	if result.Usage.TotalTokens != 1294 || result.Usage.Cost.Total <= 0 {
		t.Fatalf("usage %+v", result.Usage)
	}
}
