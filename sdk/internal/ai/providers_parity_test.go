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

// Responses API: assistant tool calls and tool results are their own item types,
// and the result names the call by call_id.
func TestResponsesRequestItems(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.created\"}\n\n")
		io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
	}))
	defer srv.Close()

	tools := []Tool{{Name: "read", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)}}
	tr := NormalizeContext("sys", tools, []Message{
		UserMessage("hi", 1),
		{
			Role: "assistant",
			Content: EncodeContentBlocks([]ContentBlock{
				TextBlock("let me read"),
				ToolCallBlock("call_1|fc_1", "read", map[string]any{"path": "a.go"}),
			}),
		},
		ToolResultMessage("call_1|fc_1", "read", []ContentBlock{TextBlock("file body")}, false, 3),
		UserMessage("thanks", 4),
	})
	model := Model{ID: "gpt", Provider: "openai", API: "openai-responses", BaseURL: srv.URL}
	collectTypes(StreamOpenAIResponses(context.Background(), model, tr, StreamOptions{APIKey: "k"}))

	input, _ := body["input"].([]any)
	var kinds []string
	for _, raw := range input {
		item, _ := raw.(map[string]any)
		kind, _ := item["type"].(string)
		if kind == "" || kind == "message" {
			kind = "message:" + item["role"].(string)
		}
		kinds = append(kinds, kind)
	}
	want := []string{"message:developer", "message:user", "message:assistant", "function_call", "function_call_output", "message:user"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("input items %v, want %v", kinds, want)
	}
	call := input[3].(map[string]any)
	if call["call_id"] != "call_1" || call["name"] != "read" || call["arguments"] != `{"path":"a.go"}` {
		t.Fatalf("function_call %v", call)
	}
	result := input[4].(map[string]any)
	if result["call_id"] != "call_1" || result["output"] != "file body" {
		t.Fatalf("function_call_output %v", result)
	}
	toolsOut, _ := body["tools"].([]any)
	tool, _ := toolsOut[0].(map[string]any)
	if tool["name"] != "read" || tool["parameters"] == nil {
		t.Fatalf("responses tools are flat, got %v", tool)
	}
	if _, nested := tool["function"]; nested {
		t.Fatalf("responses tools must not nest under function: %v", tool)
	}
}

// Responses API streams several calls at once, keys deltas by item_id, and
// reports reasoning, length and failures.
func TestResponsesStreamCallsAndReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"response.created"}`,
			`{"type":"response.output_item.added","item":{"type":"reasoning","id":"rs_1","encrypted_content":"enc"}}`,
			`{"type":"response.reasoning_summary_text.delta","delta":"thinking..."}`,
			`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_a","name":"read","arguments":""}}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"path\":"}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"a\"}"}`,
			`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_2","call_id":"call_b","name":"glob","arguments":""}}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_2","delta":"{\"q\":\"x\"}"}`,
			`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_2","call_id":"call_b","name":"glob","arguments":"{\"q\":\"x\"}"}}`,
			`{"type":"response.completed"}`,
		}
		for _, ev := range events {
			io.WriteString(w, "data: "+ev+"\n\n")
		}
	}))
	defer srv.Close()
	model := Model{ID: "gpt", Provider: "openai", API: "openai-responses", BaseURL: srv.URL}
	types, result := collectTypes(StreamOpenAIResponses(context.Background(), model, Transcript{Messages: []Message{UserMessage("go", 1)}}, StreamOptions{APIKey: "k"}))

	if result.StopReason != StopToolUse {
		t.Fatalf("stop %s (%s)", result.StopReason, result.ErrorMessage)
	}
	blocks := result.ContentBlocks()
	if len(blocks) != 3 {
		t.Fatalf("blocks %+v", blocks)
	}
	if blocks[0].Type != "thinking" || !strings.Contains(blocks[0].Thinking, "thinking...") {
		t.Fatalf("reasoning block %+v", blocks[0])
	}
	if blocks[1].ID != "call_a" || blocks[1].Arguments["path"] != "a" {
		t.Fatalf("call a %+v", blocks[1])
	}
	if blocks[2].ID != "call_b" || blocks[2].Arguments["q"] != "x" {
		t.Fatalf("call b %+v", blocks[2])
	}
	joined := strings.Join(types, ",")
	if !strings.Contains(joined, "thinking_start") || !strings.Contains(joined, "toolcall_end") {
		t.Fatalf("events %s", joined)
	}
}

func TestResponsesIncompleteAndFailed(t *testing.T) {
	stream := func(events ...string) Message {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, ev := range events {
				io.WriteString(w, "data: "+ev+"\n\n")
			}
		}))
		defer srv.Close()
		model := Model{ID: "gpt", Provider: "openai", API: "openai-responses", BaseURL: srv.URL}
		_, res := collectTypes(StreamOpenAIResponses(context.Background(), model, Transcript{Messages: []Message{UserMessage("x", 1)}}, StreamOptions{APIKey: "k"}))
		return res
	}
	if got := stream(`{"type":"response.created"}`, `{"type":"response.incomplete"}`); got.StopReason != StopLength {
		t.Fatalf("incomplete should stop on length: %s", got.StopReason)
	}
	got := stream(`{"type":"response.created"}`, `{"type":"response.failed","response":{"error":{"message":"boom"}}}`)
	if got.StopReason != StopError || !strings.Contains(got.ErrorMessage, "boom") {
		t.Fatalf("failed should surface its message: %+v", got)
	}
}

// Chat completions: reasoning_content becomes a thinking block, and a reasoning
// list works too.
func TestCompletionsReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"step 1 \"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning\":[{\"text\":\"step 2\"}]}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	model := Model{ID: "r1", Provider: "deepseek", API: "openai-completions", BaseURL: srv.URL}
	types, result := collectTypes(StreamOpenAICompletions(context.Background(), model, Transcript{Messages: []Message{UserMessage("x", 1)}}, StreamOptions{APIKey: "k"}))
	blocks := result.ContentBlocks()
	if len(blocks) != 2 || blocks[0].Type != "thinking" || blocks[0].Thinking != "step 1 step 2" || blocks[1].Text != "answer" {
		t.Fatalf("blocks %+v", blocks)
	}
	if !strings.Contains(strings.Join(types, ","), "thinking_end") {
		t.Fatalf("events %v", types)
	}
}

// Anthropic: a thinking block keeps its signature for the next request, the
// redacted form round-trips, and a mid-stream error is reported.
func TestAnthropicThinkingRoundTrip(t *testing.T) {
	var req map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		io.WriteString(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n")
		io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hm\"}}\n\n")
		io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig-1\"}}\n\n")
		io.WriteString(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
	}))
	defer srv.Close()
	model := Model{ID: "claude", Provider: "anthropic", API: "anthropic-messages", BaseURL: srv.URL, MaxTokens: 128}
	_, result := collectTypes(StreamAnthropicMessages(context.Background(), model, Transcript{Messages: []Message{UserMessage("x", 1)}}, StreamOptions{APIKey: "k"}))
	blocks := result.ContentBlocks()
	if len(blocks) != 1 || blocks[0].ThinkingSignature != "sig-1" {
		t.Fatalf("signature lost: %+v", blocks)
	}

	// Replaying the block sends the signature, and a redacted block sends its data.
	_, msgs, _ := anthropicPayload(Transcript{Messages: []Message{result, UserMessage("next", 2)}})
	sent, _ := msgs[0]["content"].([]map[string]any)
	if len(sent) == 0 || sent[0]["signature"] != "sig-1" {
		t.Fatalf("replay %+v", msgs[0])
	}
	redacted := Message{Role: "assistant", Content: EncodeContentBlocks([]ContentBlock{
		{Type: "thinking", Thinking: "[Reasoning redacted]", ThinkingSignature: "opaque", Redacted: true},
	})}
	_, msgs, _ = anthropicPayload(Transcript{Messages: []Message{redacted}})
	sent, _ = msgs[0]["content"].([]map[string]any)
	if len(sent) == 0 || sent[0]["type"] != "redacted_thinking" || sent[0]["data"] != "opaque" {
		t.Fatalf("redacted replay %+v", msgs[0])
	}
}

func TestAnthropicStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		io.WriteString(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"overloaded\"}}\n\n")
	}))
	defer srv.Close()
	model := Model{ID: "claude", Provider: "anthropic", API: "anthropic-messages", BaseURL: srv.URL, MaxTokens: 128}
	_, result := collectTypes(StreamAnthropicMessages(context.Background(), model, Transcript{Messages: []Message{UserMessage("x", 1)}}, StreamOptions{APIKey: "k"}))
	if result.StopReason != StopError || !strings.Contains(result.ErrorMessage, "overloaded") {
		t.Fatalf("stream error %+v", result)
	}
}

// A tool that returns an image keeps it, instead of flattening the result to
// text, on all three APIs.
func TestToolResultImagesReachEveryAPI(t *testing.T) {
	img := ContentBlock{Type: "image", Data: "aGVsbG8=", MIMEType: "image/png"}
	result := ToolResultMessage("call_1", "shot", []ContentBlock{TextBlock("see"), img}, false, 2)
	user := Message{Role: "user", Timestamp: 3, Content: EncodeContentBlocks([]ContentBlock{TextBlock("look"), img})}

	// Anthropic: image blocks with a base64 source, and a captioned result.
	_, msgs, _ := anthropicPayload(Transcript{Messages: []Message{result, user}})
	resultContent, _ := msgs[0]["content"].([]map[string]any)[0]["content"].([]map[string]any)
	if len(resultContent) != 2 || resultContent[1]["type"] != "image" {
		t.Fatalf("anthropic tool result %+v", resultContent)
	}
	source, _ := resultContent[1]["source"].(map[string]any)
	if source["media_type"] != "image/png" || source["data"] != "aGVsbG8=" {
		t.Fatalf("anthropic image source %+v", source)
	}
	userContent, _ := msgs[1]["content"].([]map[string]any)
	if len(userContent) != 2 || userContent[1]["type"] != "image" {
		t.Fatalf("anthropic user image %+v", userContent)
	}

	// Chat completions: image_url parts.
	_, chat := openaiMessages(Transcript{Messages: []Message{result}})
	chatParts, _ := chat[0]["content"].([]map[string]any)
	if len(chatParts) != 2 || chatParts[1]["type"] != "image_url" {
		t.Fatalf("completions tool result %+v", chat[0])
	}

	// Responses: input_image parts inside function_call_output.
	_, input := responsesInput(Transcript{Messages: []Message{result}})
	output, _ := input[0]["output"].([]map[string]any)
	if len(output) != 2 || output[1]["type"] != "input_image" {
		t.Fatalf("responses tool result %+v", input[0])
	}
}
