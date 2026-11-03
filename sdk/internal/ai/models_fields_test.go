package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The full models.json schema parses and the object fields merge as Pi's do.
func TestModelsJSONFieldsAndOverrides(t *testing.T) {
	p := writeModels(t, `{
	  "providers": {
	    "gw": {
	      "baseUrl": "https://gw.example/v1",
	      "api": "openai-completions",
	      "oauth": "radius",
	      "models": [{
	        "id": "m",
	        "reasoning": true,
	        "thinkingLevelMap": {"off": null, "high": "HIGH"},
	        "samplingParams": {"temperature": 0.2},
	        "samplingParamsByThinkingLevel": {"high": {"top_p": 0.9}},
	        "inputLimits": {"images": {"resize": {"maxWidth": 1000}}},
	        "promptCache": {"short": 300, "long": 3600},
	        "cost": {"input": 1, "output": 2, "cacheRead": 0.1, "cacheWrite": 0.2}
	      }],
	      "modelOverrides": {
	        "m": {
	          "name": "Overridden",
	          "contextWindow": 200000,
	          "cost": {"input": 3},
	          "headers": {"X-Override": "yes"},
	          "compat": {"supportsReasoningEffort": false},
	          "thinkingLevelMap": {"low": "LOW"},
	          "samplingParams": {"top_k": 5}
	        }
	      }
	    }
	  }
	}`)
	model, err := p.FindModel("gw/m")
	if err != nil {
		t.Fatal(err)
	}
	if model.Name != "Overridden" || model.ContextWindow != 200000 {
		t.Fatalf("override %+v", model)
	}
	if model.Cost.Input != 3 || model.Cost.Output != 2 {
		t.Fatalf("cost override must merge field by field: %+v", model.Cost)
	}
	if model.Headers["X-Override"] != "yes" {
		t.Fatalf("headers %+v", model.Headers)
	}
	if model.Compat.ReasoningEffort() {
		t.Fatal("compat override should disable reasoning effort")
	}
	if model.OAuth != "radius" {
		t.Fatalf("oauth %q", model.OAuth)
	}
	if value, ok := model.ThinkingLevelMap[ThinkingHigh]; !ok || value == nil || *value != "HIGH" {
		t.Fatalf("thinking map %+v", model.ThinkingLevelMap)
	}
	if value, ok := model.ThinkingLevelMap[ThinkingLow]; !ok || value == nil || *value != "LOW" {
		t.Fatalf("thinking override %+v", model.ThinkingLevelMap)
	}
	if value, ok := model.ThinkingLevelMap[ThinkingOff]; !ok || value != nil {
		t.Fatalf("an explicit null must survive: %+v", model.ThinkingLevelMap)
	}
	if model.SamplingParams["temperature"] != 0.2 || model.SamplingParams["top_k"] != float64(5) {
		t.Fatalf("sampling params %+v", model.SamplingParams)
	}
	if model.SamplingParamsByThinkingLevel[ThinkingHigh]["top_p"] != 0.9 {
		t.Fatalf("level sampling %+v", model.SamplingParamsByThinkingLevel)
	}
	if model.InputLimits == nil || model.InputLimits.Images == nil || model.InputLimits.Images.Resize.MaxWidth != 1000 {
		t.Fatalf("input limits %+v", model.InputLimits)
	}
	if model.PromptCache["short"] != 300 || model.PromptCache["long"] != 3600 {
		t.Fatalf("prompt cache %+v", model.PromptCache)
	}
}

func TestThinkingLevelsAndClamp(t *testing.T) {
	model := Model{Reasoning: true, ThinkingLevelMap: map[ThinkingLevel]*string{
		ThinkingOff:  nil,
		ThinkingHigh: stringPtr("HIGH"),
	}}
	levels := SupportedThinkingLevels(model)
	if len(levels) != 4 || levels[0] != ThinkingMinimal || levels[3] != ThinkingHigh {
		t.Fatalf("levels %v", levels)
	}
	if got := ClampThinkingLevel(model, ThinkingOff); got != ThinkingMinimal {
		t.Fatalf("off clamps up: %v", got)
	}
	if got := ClampThinkingLevel(model, ThinkingMax); got != ThinkingHigh {
		t.Fatalf("max clamps down: %v", got)
	}
	if got := ThinkingLevelEffort(model, ThinkingHigh); got != "HIGH" {
		t.Fatalf("mapped effort %q", got)
	}
	if got := ThinkingLevelEffort(model, ThinkingLow); got != "low" {
		t.Fatalf("unmapped effort %q", got)
	}
	plain := Model{}
	if levels := SupportedThinkingLevels(plain); len(levels) != 1 || levels[0] != ThinkingOff {
		t.Fatalf("a non-reasoning model only supports off: %v", levels)
	}
}

func stringPtr(value string) *string { return &value }

func TestCompletionsThinkingFormats(t *testing.T) {
	tr := Transcript{Messages: []Message{UserMessage("hi", 1)}}
	cases := []struct {
		name   string
		model  Model
		level  ThinkingLevel
		field  string
		value  any
		absent string
	}{
		{
			name:  "openai",
			model: Model{Reasoning: true},
			level: ThinkingHigh,
			field: "reasoning_effort",
			value: "high",
		},
		{
			name:  "mapped",
			model: Model{Reasoning: true, ThinkingLevelMap: map[ThinkingLevel]*string{ThinkingHigh: stringPtr("HIGH")}},
			level: ThinkingHigh,
			field: "reasoning_effort",
			value: "HIGH",
		},
		{
			name:  "deepseek",
			model: Model{Reasoning: true, Compat: Compat{ThinkingFormat: "deepseek"}},
			level: ThinkingHigh,
			field: "thinking",
			value: map[string]any{"type": "enabled"},
		},
		{
			name:  "openrouter",
			model: Model{Reasoning: true, Compat: Compat{ThinkingFormat: "openrouter"}},
			level: ThinkingHigh,
			field: "reasoning",
			value: map[string]any{"effort": "high"},
		},
		{
			name:  "qwen",
			model: Model{Reasoning: true, Compat: Compat{ThinkingFormat: "qwen"}},
			level: ThinkingHigh,
			field: "enable_thinking",
			value: true,
		},
		{
			name:  "zai off",
			model: Model{Reasoning: true, Compat: Compat{ThinkingFormat: "zai"}},
			level: ThinkingOff,
			field: "thinking",
			value: map[string]any{"type": "disabled"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cap capturedRequest
			srv := newSSEServer(t, &cap, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n", "data: [DONE]\n\n")
			model := tc.model
			model.ID, model.Provider, model.API, model.BaseURL = "m", "p", "openai-completions", srv.URL
			collectTypes(StreamOpenAICompletions(context.Background(), model, tr, StreamOptions{APIKey: "k", ThinkingLevel: tc.level}))
			got, ok := cap.Body[tc.field]
			if !ok {
				t.Fatalf("missing %s in %v", tc.field, cap.Body)
			}
			left, _ := json.Marshal(got)
			right, _ := json.Marshal(tc.value)
			if !bytes.Equal(left, right) {
				t.Fatalf("%s = %s, want %s", tc.field, left, right)
			}
		})
	}
}

func TestCompletionsThinkingBudgetField(t *testing.T) {
	var cap capturedRequest
	srv := newSSEServer(t, &cap, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n", "data: [DONE]\n\n")
	model := Model{ID: "m", Provider: "p", API: "openai-completions", BaseURL: srv.URL,
		Reasoning: true, MaxTokens: 32000,
		Compat: Compat{ThinkingFormat: "openai", ThinkingTokenBudgetField: "thinking_budget"}}
	collectTypes(StreamOpenAICompletions(context.Background(), model, Transcript{Messages: []Message{UserMessage("hi", 1)}},
		StreamOptions{APIKey: "k", ThinkingLevel: ThinkingHigh}))
	// The high budget (16384) leaves at least 1024 answer tokens under the 32000 cap.
	if got := cap.Body["thinking_budget"]; got != float64(16384) {
		t.Fatalf("thinking_budget %v", got)
	}
}

func TestResponsesThinking(t *testing.T) {
	var cap capturedRequest
	srv := newSSEServer(t, &cap, `data: {"type":"response.completed"}`+"\n\n")
	model := Model{ID: "m", Provider: "p", API: "openai-responses", BaseURL: srv.URL, Reasoning: true}
	collectTypes(StreamOpenAIResponses(context.Background(), model, Transcript{Messages: []Message{UserMessage("hi", 1)}},
		StreamOptions{APIKey: "k", ThinkingLevel: ThinkingMedium}))
	reasoning, _ := cap.Body["reasoning"].(map[string]any)
	if reasoning["effort"] != "medium" || reasoning["summary"] != "auto" {
		t.Fatalf("reasoning %v", cap.Body["reasoning"])
	}
	include, _ := cap.Body["include"].([]any)
	if len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("include %v", cap.Body["include"])
	}
}

func TestAnthropicThinkingBudget(t *testing.T) {
	var cap capturedRequest
	srv := newSSEServer(t, &cap, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
	model := Model{ID: "m", Provider: "p", API: "anthropic-messages", BaseURL: srv.URL, Reasoning: true, MaxTokens: 8192}
	collectTypes(StreamAnthropicMessages(context.Background(), model, Transcript{Messages: []Message{UserMessage("hi", 1)}},
		StreamOptions{APIKey: "k", ThinkingLevel: ThinkingMedium}))
	thinking, _ := cap.Body["thinking"].(map[string]any)
	// The 8192-token budget is capped so 1024 tokens remain for the answer.
	if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(7168) {
		t.Fatalf("thinking %v", cap.Body["thinking"])
	}
	if cap.Body["max_tokens"] != float64(8192) {
		t.Fatalf("max_tokens %v", cap.Body["max_tokens"])
	}
}

func TestAnthropicAdaptiveThinking(t *testing.T) {
	var cap capturedRequest
	srv := newSSEServer(t, &cap, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
	trueVal := true
	model := Model{ID: "m", Provider: "p", API: "anthropic-messages", BaseURL: srv.URL, Reasoning: true, MaxTokens: 8192,
		Compat: Compat{ForceAdaptiveThinking: &trueVal}}
	collectTypes(StreamAnthropicMessages(context.Background(), model, Transcript{Messages: []Message{UserMessage("hi", 1)}},
		StreamOptions{APIKey: "k", ThinkingLevel: ThinkingHigh}))
	thinking, _ := cap.Body["thinking"].(map[string]any)
	output, _ := cap.Body["output_config"].(map[string]any)
	if thinking["type"] != "adaptive" || output["effort"] != "high" {
		t.Fatalf("adaptive %v %v", cap.Body["thinking"], cap.Body["output_config"])
	}
}

func TestSamplingParamsOrder(t *testing.T) {
	var cap capturedRequest
	srv := newSSEServer(t, &cap, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n", "data: [DONE]\n\n")
	model := Model{ID: "m", Provider: "p", API: "openai-completions", BaseURL: srv.URL,
		SamplingParams: map[string]any{"temperature": 0.2, "top_p": 0.5},
		SamplingParamsByThinkingLevel: map[ThinkingLevel]map[string]any{
			ThinkingHigh: {"top_p": 0.9},
		}}
	collectTypes(StreamOpenAICompletions(context.Background(), model, Transcript{Messages: []Message{UserMessage("hi", 1)}},
		StreamOptions{APIKey: "k", ThinkingLevel: ThinkingHigh, SamplingParams: map[string]any{"top_p": 0.7}}))
	if cap.Body["temperature"] != 0.2 || cap.Body["top_p"] != 0.7 {
		t.Fatalf("sampling %v", cap.Body)
	}
}

func TestResizeImages(t *testing.T) {
	// A 300x100 PNG is wider than the 100px profile.
	source := image.NewRGBA(image.Rect(0, 0, 300, 100))
	for x := 0; x < 300; x++ {
		for y := 0; y < 100; y++ {
			source.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	message := UserMessage("look", 1)
	message.Content = EncodeContentBlocks([]ContentBlock{TextBlock("look"), {
		Type: "image", Data: base64.StdEncoding.EncodeToString(encoded.Bytes()), MIMEType: "image/png",
	}})
	limits := &InputLimits{Images: &ImageInputLimits{Resize: &ImageResizeOptions{MaxWidth: 100, MaxHeight: 100, MaxBytes: 1 << 20, JPEGQuality: 70}}}
	tr := withResizedImages(Transcript{Messages: []Message{message}}, limits)
	blocks := tr.Messages[0].ContentBlocks()
	if len(blocks) != 2 || blocks[1].Type != "image" {
		t.Fatalf("blocks %+v", blocks)
	}
	raw, err := base64.StdEncoding.DecodeString(blocks[1].Data)
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 100 || config.Height != 33 {
		t.Fatalf("resized to %dx%d", config.Width, config.Height)
	}
	// The original message must not be mutated.
	original := message.ContentBlocks()
	if original[1].Data == blocks[1].Data {
		t.Fatal("the transcript copy shares the resized data")
	}
}

func TestPromptCacheTiming(t *testing.T) {
	model := Model{PromptCache: map[string]float64{"short": 300, "long": 3600}}
	ttl, ok := PromptCacheTTL(model, "short")
	if !ok || ttl != 300*time.Second {
		t.Fatalf("ttl %v %v", ttl, ok)
	}
	delay, ok := CacheWarmingDelay(ttl)
	if !ok || delay != 270*time.Second {
		t.Fatalf("delay %v %v", delay, ok)
	}
	if _, ok := PromptCacheTTL(model, "none"); ok {
		t.Fatal("an unknown tier has no lifetime")
	}
}

func TestOAuthCredentialRefresh(t *testing.T) {
	var refreshed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/oauth/token" {
			t.Errorf("path %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.PostForm.Get("grant_type") != "refresh_token" || r.PostForm.Get("refresh_token") != "old-refresh" {
			t.Errorf("form %v", r.PostForm)
		}
		refreshed = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"scope":"gateway"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	expired := time.Now().Add(-time.Minute).UnixMilli()
	auth := `{"gw": {"type": "oauth", "access": "old-access", "refresh": "old-refresh", "expires": ` +
		jsonNumber(expired) + `}}`
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}
	model := Model{Provider: "gw", BaseURL: srv.URL, OAuth: "radius"}
	key, err := LookupAPIKey(model, dir, StreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if key != "new-access" || !refreshed {
		t.Fatalf("key %q refreshed %v", key, refreshed)
	}
	credential, ok := credentialFromFile("gw", dir)
	if !ok || credential.Access != "new-access" || credential.Refresh != "new-refresh" {
		t.Fatalf("persisted credential %+v", credential)
	}

	// A fresh token is used without a request.
	refreshed = false
	model.Provider = "fresh"
	body := `{"fresh": {"type": "oauth", "access": "still-good", "expires": 9999999999999}}`
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err = LookupAPIKey(model, dir, StreamOptions{})
	if err != nil || key != "still-good" || refreshed {
		t.Fatalf("fresh key %q err %v refreshed %v", key, err, refreshed)
	}
}

func jsonNumber(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// The unsupported oauth flow is a configuration error, not a silent fallback.
func TestOAuthFlowValidation(t *testing.T) {
	p := writeModels(t, `{"providers": {"gw": {"api": "openai-completions", "oauth": "other", "models": [{"id": "m"}]}}}`)
	if _, err := p.FindModel("gw/m"); err == nil || !strings.Contains(err.Error(), "unsupported oauth") {
		t.Fatalf("err %v", err)
	}
}

func TestThinkingFormatDetection(t *testing.T) {
	cases := []struct{ provider, base, want string }{
		{"deepseek", "", "deepseek"},
		{"custom", "https://api.deepseek.com/v1", "deepseek"},
		{"openrouter", "", "openrouter"},
		{"zai", "", "zai"},
		{"together", "", "together"},
		{"ant-ling", "", "ant-ling"},
		{"openai", "", "openai"},
	}
	for _, tc := range cases {
		if got := thinkingFormatFor(Model{Provider: tc.provider, BaseURL: tc.base}); got != tc.want {
			t.Fatalf("%s/%s = %q, want %q", tc.provider, tc.base, got, tc.want)
		}
	}
	if !supportsReasoningEffort(Model{Provider: "deepseek"}) {
		t.Fatal("deepseek supports reasoning_effort")
	}
	if supportsReasoningEffort(Model{Provider: "zai"}) || supportsReasoningEffort(Model{Provider: "together"}) {
		t.Fatal("these gateways reject reasoning_effort")
	}
	enabled := true
	if !supportsReasoningEffort(Model{Provider: "zai", Compat: Compat{SupportsReasoningEffort: &enabled}}) {
		t.Fatal("an explicit compat flag wins")
	}
}
