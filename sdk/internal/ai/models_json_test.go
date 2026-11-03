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

func TestFindModelDefaultsToOpenAI(t *testing.T) {
	p := writeModels(t, `{"providers": {
	  "openai": {"api": "openai-completions", "models": [{"id": "gpt"}]},
	  "b": {"api": "openai-completions", "models": [{"id": "m"}]}}}`)
	m, err := p.FindModel("gpt")
	if err != nil || m.Provider != "openai" {
		t.Fatalf("%+v %v", m, err)
	}
	// Pi reads a bare id as an openai model, not as a search across providers.
	if _, err := p.FindModel("m"); err == nil || err.Error() != "model not found: openai/m" {
		t.Fatalf("bare unknown: %v", err)
	}
	if _, err := p.FindModel("b/nope"); err == nil || err.Error() != "model not found: b/nope" {
		t.Fatalf("unknown model: %v", err)
	}
}

func TestDuplicateModelIDLastWins(t *testing.T) {
	p := writeModels(t, `{"providers": {"b": {"api": "openai-completions",
	  "models": [{"id": "m", "name": "first"}, {"id": "m", "name": "second"}]}}}`)
	m, err := p.FindModel("b/m")
	if err != nil || m.Name != "second" {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestModelsJSONCommentsAndTrailingCommas(t *testing.T) {
	p := writeModels(t, `{
	  // a provider comment
	  "providers": {
	    "b": {
	      "api": "openai-completions", // inline comment
	      "models": [
	        {"id": "m", "name": "M // not a comment",},
	      ],
	    },
	  },
	}`)
	m, err := p.FindModel("b/m")
	if err != nil || m.Name != "M // not a comment" {
		t.Fatalf("%q %v", m.Name, err)
	}
}

func TestFindModelInputDefaultsToText(t *testing.T) {
	p := writeModels(t, `{"providers": {"b": {"api": "openai-completions", "models": [{"id": "m"}]}}}`)
	m, err := p.FindModel("b/m")
	if err != nil || len(m.Input) != 1 || m.Input[0] != "text" {
		t.Fatalf("%+v %v", m.Input, err)
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
		// One unset variable makes the whole value undefined, as Pi's resolver.
		{"$NULO_TEST_MISSING.", ""},
		{"prefix-$NULO_TEST_MISSING", ""},
		// A braced name that is not an identifier stays literal.
		{"${not a name}", "${not a name}"},
		{"$", "$"},
		{"tail$", "tail$"},
		{"$1abc", "$1abc"},
	}
	for _, tc := range cases {
		if got := resolveConfigValue(tc.in, nil); got != tc.want {
			t.Fatalf("resolveConfigValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	// A leading "!" runs a command; "!" elsewhere stays literal.
	if got := resolveConfigValue("!echo from-command", nil); got != "from-command" {
		t.Fatalf("command %q", got)
	}
	if got := resolveConfigValue("not!a!command", nil); got != "not!a!command" {
		t.Fatalf("literal ! %q", got)
	}
}

func TestResolveConfigValueErrors(t *testing.T) {
	t.Setenv("NULO_TEST_KEY", "sekret")
	// Commands run at request time, so a failure is an error there, not "".
	if value := resolveConfigValue("!exit 3", nil); value != "" {
		t.Fatalf("a failing command resolves to the undefined value, got %q", value)
	}
	if _, err := resolveConfigValueE("!exit 3", "API key for provider \"p\"", nil); err == nil ||
		!strings.Contains(err.Error(), "shell command") {
		t.Fatalf("command failure: %v", err)
	}
	// An empty result is a successful but empty value.
	silent := "true"
	if runtime.GOOS == "windows" {
		silent = "rem"
	}
	if _, err := resolveConfigValueE("!"+silent, "API key for provider \"p\"", nil); err != nil {
		t.Fatalf("empty command output: %v", err)
	}
	_, err := resolveConfigValueE("$NULO_TEST_MISSING", "API key for provider \"p\"", nil)
	if err == nil || !strings.Contains(err.Error(), "NULO_TEST_MISSING") {
		t.Fatalf("missing environment variable: %v", err)
	}
	// Provider-scoped overrides win over the process environment.
	if value := resolveConfigValue("$NULO_TEST_KEY", map[string]string{"NULO_TEST_KEY": "override"}); value != "override" {
		t.Fatalf("env override %q", value)
	}
}

func TestResolveHeaders(t *testing.T) {
	t.Setenv("NULO_TEST_HDR", "hv")
	got, err := resolveHeaders(map[string]string{"A": "$NULO_TEST_HDR", "Empty": ""}, `model "b/m"`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got["A"] != "hv" {
		t.Fatalf("%+v", got)
	}
	if _, ok := got["Empty"]; ok {
		t.Fatalf("an empty header should be dropped: %+v", got)
	}
	// A missing variable is a resolution failure for a configured header.
	if _, err := resolveHeaders(map[string]string{"B": "$NULO_TEST_ABSENT"}, `model "b/m"`, nil); err == nil ||
		!strings.Contains(err.Error(), "NULO_TEST_ABSENT") {
		t.Fatalf("missing header env: %v", err)
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
	model := Model{Provider: "p", ID: "m", Headers: map[string]string{"X-A": "m", "X-B": "m"}}
	got, err := RequestHeaders(model, StreamOptions{Headers: map[string]string{"x-b": "o", "X-C": "o"}})
	if err != nil {
		t.Fatal(err)
	}
	if got["X-A"] != "m" || got["x-b"] != "o" { // case-insensitive override
		t.Fatalf("%+v", got)
	}
	if _, ok := got["X-B"]; ok {
		t.Fatalf("the overridden spelling must not survive: %+v", got)
	}
	if got["X-C"] != "o" {
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
