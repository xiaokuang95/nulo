package ai

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Headers merge case-insensitively and a null model value deletes the
// inherited header, as Pi's models.json does.
func TestHeadersInheritAndNullDeletes(t *testing.T) {
	p := writeModels(t, `{"providers": {"b": {
	  "api": "openai-completions",
	  "headers": {"X-Keep": "provider", "X-Drop": "provider", "X-Case": "provider"},
	  "models": [{"id": "m", "headers": {"x-drop": null, "X-CASE": "model"}}]
	}}}`)
	m, err := p.FindModel("b/m")
	if err != nil {
		t.Fatal(err)
	}
	if m.Headers["X-Keep"] != "provider" {
		t.Fatalf("inherited header %+v", m.Headers)
	}
	if _, ok := m.Headers["X-Drop"]; ok {
		t.Fatalf("a null header must delete the inherited one: %+v", m.Headers)
	}
	if len(m.Headers) != 2 || m.Headers["X-CASE"] != "model" {
		t.Fatalf("case-insensitive override: %+v", m.Headers)
	}
}

// Pi's order: request key, auth.json, models.json, then the provider's own
// environment variable. An unknown provider has no ambient key.
func TestLookupAPIKeyPrecedence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"),
		[]byte(`{"p": {"type": "api_key", "key": "from-auth"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "from-env")
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-env")

	model := Model{Provider: "p", APIKey: "from-models"}
	if key, err := LookupAPIKey(model, dir, StreamOptions{APIKey: "explicit"}); err != nil || key != "explicit" {
		t.Fatalf("request key: %q %v", key, err)
	}
	if key, err := LookupAPIKey(model, dir, StreamOptions{}); err != nil || key != "from-auth" {
		t.Fatalf("auth.json must beat models.json: %q %v", key, err)
	}
	if key, err := LookupAPIKey(model, "", StreamOptions{}); err != nil || key != "from-models" {
		t.Fatalf("models.json key: %q %v", key, err)
	}
	if key, err := LookupAPIKey(Model{Provider: "gateway"}, "", StreamOptions{}); err != nil || key != "" {
		t.Fatalf("an unknown provider must not borrow OPENAI_API_KEY: %q %v", key, err)
	}
	if key, err := LookupAPIKey(Model{Provider: "deepseek"}, "", StreamOptions{}); err != nil || key != "deepseek-env" {
		t.Fatalf("provider env key: %q %v", key, err)
	}
}

// The key template resolves per request, so an environment change or a
// "!command" takes effect on the next call.
func TestLookupAPIKeyResolvesPerRequest(t *testing.T) {
	t.Setenv("NULO_ROTATE", "one")
	model := Model{Provider: "p", APIKey: "$NULO_ROTATE"}
	if key, err := LookupAPIKey(model, "", StreamOptions{}); err != nil || key != "one" {
		t.Fatalf("first %q %v", key, err)
	}
	t.Setenv("NULO_ROTATE", "two")
	if key, err := LookupAPIKey(model, "", StreamOptions{}); err != nil || key != "two" {
		t.Fatalf("second %q %v", key, err)
	}
}

// A failing command does not break loading; it fails the request that needs it.
func TestModelBuildDoesNotResolveCommands(t *testing.T) {
	p := writeModels(t, `{"providers": {"b": {
	  "api": "openai-completions", "apiKey": "!exit 3",
	  "models": [{"id": "m"}]
	}}}`)
	m, err := p.FindModel("b/m")
	if err != nil {
		t.Fatalf("loading must not run the command: %v", err)
	}
	if _, err := LookupAPIKey(m, "", StreamOptions{}); err == nil || !strings.Contains(err.Error(), "shell command") {
		t.Fatalf("request must report the command failure: %v", err)
	}
}

// authHeader sends the key as Authorization: Bearer on top of the provider's
// own auth header.
func TestAuthHeaderAddsBearer(t *testing.T) {
	var cap capturedRequest
	srv := newSSEServer(t, &cap, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
	model := Model{ID: "m", Provider: "p", API: "anthropic-messages", BaseURL: srv.URL, AuthHeader: true}
	tr := Transcript{Messages: []Message{UserMessage("hi", 1)}}
	collectTypes(StreamAnthropicMessages(context.Background(), model, tr, StreamOptions{APIKey: "k"}))
	if got := cap.Headers.Get("x-api-key"); got != "k" {
		t.Fatalf("x-api-key %q", got)
	}
	if got := cap.Headers.Get("Authorization"); got != "Bearer k" {
		t.Fatalf("Authorization %q", got)
	}
}

// The highest matching tier replaces every base rate for the whole request.
func TestCostTiers(t *testing.T) {
	model := Model{Cost: Cost{
		Input: 1, Output: 2, CacheRead: 0.1, CacheWrite: 0.2,
		Tiers: []CostTier{{InputTokensAbove: 1000, Input: 10, Output: 20, CacheRead: 1, CacheWrite: 2}},
	}}
	below := Usage{Input: 500}
	calculateCost(model, &below)
	if got := below.Cost.Input; got != 0.0005 {
		t.Fatalf("below tier input cost %v", got)
	}
	above := Usage{Input: 2000, Output: 1000}
	calculateCost(model, &above)
	wantInput := 10.0 / 1e6 * 2000
	wantOutput := 20.0 / 1e6 * 1000
	if above.Cost.Input != wantInput || above.Cost.Output != wantOutput {
		t.Fatalf("tier cost %+v, want input %v output %v", above.Cost, wantInput, wantOutput)
	}
	if above.Cost.Total != wantInput+wantOutput {
		t.Fatalf("total %v", above.Cost.Total)
	}
}

// Windows uses cmd for "!command" values, so the empty-output case needs a
// silent command per platform.
func TestRunConfigCommandEmptyOutput(t *testing.T) {
	silent := "true"
	if runtime.GOOS == "windows" {
		silent = "rem"
	}
	value, ok := runConfigCommand("!" + silent)
	if !ok || value != "" {
		t.Fatalf("value %q ok %v", value, ok)
	}
	if _, ok := runConfigCommand("!exit 3"); ok {
		t.Fatal("a failing command must report failure")
	}
}
