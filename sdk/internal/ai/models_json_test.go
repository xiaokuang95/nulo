package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeModels(t *testing.T, body string) Providers {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProviders(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// capturedRequest holds what the fake provider endpoint received.
type capturedRequest struct {
	Body    map[string]any
	Headers http.Header
}

// newSSEServer answers with the given frames and records the request.
func newSSEServer(t *testing.T, cap *capturedRequest, frames ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		cap.Body = nil
		_ = json.Unmarshal(b, &cap.Body)
		cap.Headers = r.Header.Clone()
		w.Header().Set("Content-Type", "text/event-stream")
		for _, f := range frames {
			io.WriteString(w, f)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLoadProvidersInheritsAndOverrides(t *testing.T) {
	p := writeModels(t, `{
	  "providers": {
	    "tokenharbor": {
	      "name": "Token Harbor",
	      "baseUrl": "https://gw.example.com/v1",
	      "api": "openai-completions",
	      "apiKey": "sk-literal",
	      "headers": {"X-Provider": "p", "X-Both": "provider"},
	      "compat": {"maxTokensField": "max_completion_tokens", "supportsUsageInStreaming": false},
	      "models": [
	        {"id": "plain"},
	        {
	          "id": "special",
	          "api": "anthropic-messages",
	          "baseUrl": "https://other.example.com",
	          "headers": {"X-Both": "model"},
	          "compat": {"supportsStore": true},
	          "maxTokens": 4096
	        }
	      ]
	    }
	  }
	}`)

	m, err := p.FindModel("tokenharbor/plain")
	if err != nil {
		t.Fatal(err)
	}
	if m.Provider != "tokenharbor" || m.BaseURL != "https://gw.example.com/v1" || m.API != "openai-completions" {
		t.Fatalf("inherited model %+v", m)
	}
	if m.APIKey != "sk-literal" {
		t.Fatalf("apiKey %q", m.APIKey)
	}
	if m.MaxTokens != defaultMaxTokens || m.ContextWindow != defaultContextWindow {
		t.Fatalf("defaults %+v", m)
	}
	if m.Headers["X-Provider"] != "p" {
		t.Fatalf("headers %+v", m.Headers)
	}
	if m.Compat.UsageInStreaming() {
		t.Fatal("provider compat should disable streaming usage")
	}
	if m.Compat.TokensField() != "max_completion_tokens" {
		t.Fatalf("tokens field %q", m.Compat.TokensField())
	}

	// A model-level value wins, and compat merges field by field.
	s, err := p.FindModel("tokenharbor/special")
	if err != nil {
		t.Fatal(err)
	}
	if s.API != "anthropic-messages" || s.BaseURL != "https://other.example.com" || s.MaxTokens != 4096 {
		t.Fatalf("overridden model %+v", s)
	}
	if s.Headers["X-Both"] != "model" {
		t.Fatalf("model header should win: %+v", s.Headers)
	}
	if !s.Compat.Store() {
		t.Fatal("model compat should be added")
	}
	if s.Compat.UsageInStreaming() {
		t.Fatal("provider compat field must survive a model-level merge")
	}
}

func TestFindModelBareIDAndUnknown(t *testing.T) {
	p := writeModels(t, `{"providers": {"b": {"api": "openai-completions", "models": [{"id": "m"}]}}}`)
	m, err := p.FindModel("m")
	if err != nil || m.Provider != "b" {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err := p.FindModel("b/nope"); err == nil {
		t.Fatal("unknown model should error")
	}
}

func TestModelWithoutAPIErrors(t *testing.T) {
	p := writeModels(t, `{"providers": {"b": {"baseUrl": "https://x/v1", "models": [{"id": "m"}]}}}`)
	if _, err := p.FindModel("b/m"); err == nil || !strings.Contains(err.Error(), "no \"api\" specified") {
		t.Fatalf("err %v", err)
	}
}

func TestLegacyModelsShapeIsRejected(t *testing.T) {
	for name, body := range map[string]string{
		"object": `{"models": [{"id": "m", "api": "openai-completions", "provider": "p", "baseUrl": "https://x/v1"}]}`,
		"array":  `[{"id": "m"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "models.json"), []byte(body), 0o600)
			_, err := LoadProviders(dir)
			if err == nil || !strings.Contains(err.Error(), "providers") {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestMissingModelsFileIsNotAnError(t *testing.T) {
	p, err := LoadProviders(t.TempDir())
	if err != nil || p != nil {
		t.Fatalf("%v %v", p, err)
	}
}

func TestResolveConfigValue(t *testing.T) {
	t.Setenv("NULO_TEST_KEY", "sekret")
	cases := []struct{ in, want string }{
		{"literal", "literal"},
		{"$NULO_TEST_KEY", "sekret"},
		{"${NULO_TEST_KEY}", "sekret"},
		{"Bearer $NULO_TEST_KEY/x", "Bearer sekret/x"},
		{"$$NULO_TEST_KEY", "$NULO_TEST_KEY"},
		{"$!literal", "!literal"},
		{"a$$b", "a$b"},
		{"$NULO_TEST_MISSING.", "."},
		{"$", "$"},
		{"tail$", "tail$"},
	}
	for _, tc := range cases {
		got, err := ResolveConfigValue(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ResolveConfigValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	// A leading "!" runs a command; "!" elsewhere stays literal.
	if got, err := ResolveConfigValue("!echo from-command"); err != nil || got != "from-command" {
		t.Fatalf("command %q %v", got, err)
	}
	if got, _ := ResolveConfigValue("not!a!command"); got != "not!a!command" {
		t.Fatalf("literal ! %q", got)
	}
}

func TestResolveConfigValueCommandFailure(t *testing.T) {
	if _, err := ResolveConfigValue("!exit 3"); err == nil {
		t.Fatal("a failing command should error")
	}
	// An empty result is a resolution failure, not an empty string. `rem` is silent
	// on Windows where `true` is silent on Unix; `echo` is not, because cmd's echo
	// with no argument reports its own state.
	silent := "true"
	if runtime.GOOS == "windows" {
		silent = "rem"
	}
	if _, err := ResolveConfigValue("!" + silent); err == nil || !strings.Contains(err.Error(), "no output") {
		t.Fatalf("empty command output should error, got %v", err)
	}
}

func TestResolveHeadersDropsEmpty(t *testing.T) {
	t.Setenv("NULO_TEST_HDR", "hv")
	got, err := ResolveHeaders(map[string]string{"A": "$NULO_TEST_HDR", "B": "$NULO_TEST_ABSENT"})
	if err != nil {
		t.Fatal(err)
	}
	if got["A"] != "hv" {
		t.Fatalf("%+v", got)
	}
	if _, ok := got["B"]; ok {
		t.Fatalf("an empty header should be dropped: %+v", got)
	}
}

func TestCompatDefaults(t *testing.T) {
	var c Compat
	if c.TokensField() != "max_tokens" {
		t.Fatalf("tokens field %q", c.TokensField())
	}
	if !c.UsageInStreaming() || !c.MaxOutputTokens() {
		t.Fatal("streaming usage and the responses output cap default on")
	}
	if c.Store() || c.ToolResultName() {
		t.Fatal("store and tool-result name default off")
	}
	if c.DeveloperRole(false) {
		t.Fatal("the completions default role is system")
	}
	if !c.DeveloperRole(true) {
		t.Fatal("the responses default role is developer")
	}
}

func TestRequestHeadersOverride(t *testing.T) {
	model := Model{Headers: map[string]string{"X-A": "m", "X-B": "m"}}
	got := RequestHeaders(model, StreamOptions{Headers: map[string]string{"X-B": "o", "X-C": "o"}})
	if got["X-A"] != "m" || got["X-B"] != "o" || got["X-C"] != "o" {
		t.Fatalf("%+v", got)
	}
}

func TestCompletionsHonoursCompat(t *testing.T) {
	var cap capturedRequest
	srv := newSSEServer(t, &cap,
		`data: {"choices":[{"delta":{"content":"Hi"}}]}`+"\n\n",
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n",
		"data: [DONE]\n\n")

	falseVal, trueVal := false, true
	model := Model{
		ID: "m", Provider: "p", API: "openai-completions", BaseURL: srv.URL,
		Compat: Compat{
			MaxTokensField:           "max_completion_tokens",
			SupportsDeveloperRole:    &trueVal,
			SupportsUsageInStreaming: &falseVal,
			SupportsStore:            &trueVal,
		},
	}
	opts := StreamOptions{APIKey: "k", MaxTokens: 1234, Headers: map[string]string{"X-Req": "1"}}
	tr := Transcript{Messages: []Message{SystemMessage("sys", 1), UserMessage("hi", 2)}}
	if _, result := collectTypes(StreamOpenAICompletions(context.Background(), model, tr, opts)); result.StopReason != StopStop {
		t.Fatalf("stop %v", result.StopReason)
	}

	if _, ok := cap.Body["max_tokens"]; ok {
		t.Fatalf("max_tokens must not be sent when max_completion_tokens is configured: %v", cap.Body["max_tokens"])
	}
	if cap.Body["max_completion_tokens"] != float64(1234) {
		t.Fatalf("max_completion_tokens %v", cap.Body["max_completion_tokens"])
	}
	if v, ok := cap.Body["store"].(bool); !ok || v {
		t.Fatalf("store should be sent as false: %v", cap.Body["store"])
	}
	if _, ok := cap.Body["stream_options"]; ok {
		t.Fatal("stream_options must be omitted when compat disables streaming usage")
	}
	msgs, _ := cap.Body["messages"].([]any)
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "developer" {
		t.Fatalf("leading role %v", first["role"])
	}
	if cap.Headers.Get("X-Req") != "1" {
		t.Fatalf("request headers %+v", cap.Headers)
	}
}

func TestResponsesSendsOutputCap(t *testing.T) {
	var cap capturedRequest
	srv := newSSEServer(t, &cap, `data: {"type":"response.output_text.delta","delta":"Hi"}`+"\n\n")

	model := Model{ID: "m", Provider: "p", API: "openai-responses", BaseURL: srv.URL}
	opts := StreamOptions{APIKey: "k", MaxTokens: 4}
	tr := Transcript{Messages: []Message{UserMessage("hi", 1)}}
	collectTypes(StreamOpenAIResponses(context.Background(), model, tr, opts))
	// OpenAI rejects a cap below 16, so the floor applies.
	if cap.Body["max_output_tokens"] != float64(minResponsesOutputTokens) {
		t.Fatalf("max_output_tokens %v", cap.Body["max_output_tokens"])
	}

	falseVal := false
	model.Compat.SupportsMaxOutputTokens = &falseVal
	collectTypes(StreamOpenAIResponses(context.Background(), model, tr, opts))
	if _, ok := cap.Body["max_output_tokens"]; ok {
		t.Fatal("compat should be able to suppress max_output_tokens")
	}
}

func TestCompletionsToolResultName(t *testing.T) {
	var cap capturedRequest
	srv := newSSEServer(t, &cap,
		`data: {"choices":[{"delta":{"content":"ok"}}]}`+"\n\n", "data: [DONE]\n\n")

	tr := Transcript{Messages: []Message{
		UserMessage("read it", 1),
		AssistantMessage([]ContentBlock{{Type: "toolCall", ID: "c1", Name: "read", Arguments: map[string]any{"path": "a"}}}, Model{}, StopToolUse, 2),
		ToolResultMessage("c1", "read", []ContentBlock{TextBlock("contents")}, false, 3),
	}}
	trueVal := true
	model := Model{ID: "m", Provider: "p", API: "openai-completions", BaseURL: srv.URL,
		Compat: Compat{RequiresToolResultName: &trueVal}}
	collectTypes(StreamOpenAICompletions(context.Background(), model, tr, StreamOptions{APIKey: "k"}))

	msgs, _ := cap.Body["messages"].([]any)
	last, _ := msgs[len(msgs)-1].(map[string]any)
	if last["role"] != "tool" {
		t.Fatalf("last message %+v", last)
	}
	if last["name"] != "read" {
		t.Fatalf("requiresToolResultName should add the tool name: %+v", last)
	}
}

func TestProvidersAPIKeyNeverSerializes(t *testing.T) {
	b, err := json.Marshal(Model{ID: "m", APIKey: "sk-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "sk-secret") {
		t.Fatalf("APIKey leaked into JSON: %s", b)
	}
}
