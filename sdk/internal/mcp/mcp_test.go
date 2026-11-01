package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPToolsCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "t"}},
			})
		case "tools/list":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"tools": []map[string]any{{"name": "echo", "description": "echo", "inputSchema": map[string]any{"type": "object"}}}},
			})
		case "tools/call":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"content": []map[string]any{{"type": "text", "text": "pong"}}},
			})
		default:
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}})
		}
	}))
	defer srv.Close()
	cl, tools, err := Connect(context.Background(), "demo", ServerConfig{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if len(tools) != 1 || tools[0].Name != "mcp__demo__echo" || tools[0].OriginalName != "echo" {
		t.Fatalf("%+v", tools)
	}
	blocks, isErr, err := tools[0].call(context.Background(), map[string]any{"x": 1})
	if err != nil || isErr {
		t.Fatalf("%v %v", isErr, err)
	}
	if len(blocks) != 1 || blocks[0].Text != "pong" {
		t.Fatalf("%+v", blocks)
	}
}

func TestConnectStdioTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _, err := Connect(ctx, "hang", ServerConfig{Command: "sleep", Args: []string{"30"}})
	if err == nil {
		t.Fatal("expected timeout")
	}
}

func TestExposureResolution(t *testing.T) {
	cases := map[string]Exposure{
		"codemode":          ExposureCodemode,
		"codemode-deferred": ExposureCodemode, // alias
		"deferred":          ExposureDeferred,
		"direct":            ExposureDirect,
		"hidden":            ExposureHidden,
	}
	for raw, want := range cases {
		got, ok := parseExposure(raw)
		if !ok || got != want {
			t.Fatalf("exposure %q -> %q %v, want %q", raw, got, ok, want)
		}
	}
	if _, ok := parseExposure("nope"); ok {
		t.Fatal("an unknown exposure must not resolve")
	}
	// Pi is strict here: no case folding, no trimming.
	if _, ok := parseExposure("DIRECT"); ok {
		t.Fatal("exposure is case-sensitive, like Pi")
	}
	if !ExposureCodemode.Indirect() || !ExposureDeferred.Indirect() ||
		ExposureDirect.Indirect() || ExposureHidden.Indirect() {
		t.Fatal("codemode and deferred are the indirect exposures")
	}
}

func TestToolExposureOverride(t *testing.T) {
	cfg := ServerConfig{
		Exposure: "codemode",
		ToolExposure: map[string]string{
			"exact_tool": "direct",
			"search_*":   "hidden",
			"read_*":     "deferred",
			"codemode_*": "codemode-deferred",
		},
	}
	cases := map[string]Exposure{
		"exact_tool":     ExposureDirect,
		"search_users":   ExposureHidden,
		"read_issue":     ExposureDeferred,
		"codemode_thing": ExposureCodemode,
		"other":          ExposureCodemode,
	}
	for tool, want := range cases {
		if got := cfg.ExposureFor(tool); got != want {
			t.Fatalf("%s -> %q, want %q", tool, got, want)
		}
	}
	if got := (ServerConfig{}).ExposureFor("x"); got != ExposureCodemode {
		t.Fatalf("default %q", got)
	}
}

func TestToolPattern(t *testing.T) {
	if !matchToolPattern("a*b", "axxb") || matchToolPattern("a*b", "axxc") {
		t.Fatal("literal with a star")
	}
	if !matchToolPattern("*", "anything") || !matchToolPattern("a.b", "a.b") || matchToolPattern("a.b", "axb") {
		t.Fatal("dots are literal, not regexp wildcards")
	}
}

// Tool names are sanitized to [A-Za-z0-9_], capped at 64, and disambiguated with
// a hash suffix when sanitizing makes two names collide.
func TestToolName(t *testing.T) {
	if got := createMcpToolName("dev-radius", "search", func(string) bool { return false }); got != "mcp__dev_radius__search" {
		t.Fatalf("%q", got)
	}
	long := strings.Repeat("x", 80)
	got := createMcpToolName("s", long, func(string) bool { return false })
	if len(got) > maxToolNameLength || !strings.HasSuffix(got, "_"+got[len(got)-8:]) {
		t.Fatalf("long name %q (%d)", got, len(got))
	}
	// A name already taken by another MCP tool gets the suffix even when short.
	taken := createMcpToolName("a-b", "x", func(string) bool { return false })
	got2 := createMcpToolName("a_b", "x", func(candidate string) bool { return candidate == taken })
	if got2 == taken || len(got2) > maxToolNameLength {
		t.Fatalf("%q vs %q", got2, taken)
	}
}

// A bad entry is reported and skipped; the other servers still load.
func TestLoadCollectsErrorsAndKeepsGoing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{"mcpServers":{
		"good":{"url":"https://example.com/mcp"},
		"bad":{"url":"http://x","exposure":"bogus"},
		"noname":{"args":["y"]},
		"sse":{"type":"sse","url":"http://x"}
	}}`), 0o600)

	cfg := Load(dir, "", false)
	if _, ok := cfg.Servers["good"]; !ok {
		t.Fatalf("the good server should survive: %+v", cfg.Servers)
	}
	if _, ok := cfg.Servers["bad"]; ok {
		t.Fatal("a bad exposure should drop only that server")
	}
	if len(cfg.Errors) != 3 {
		t.Fatalf("expected three problems, got %q", cfg.Errors)
	}
}

func TestLoadRejectsInvalidServerName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mcp.json"),
		[]byte(`{"mcpServers":{"bad name":{"url":"https://x"}}}`), 0o600)
	cfg := Load(dir, "", false)
	if len(cfg.Servers) != 0 || len(cfg.Errors) != 1 {
		t.Fatalf("%+v %q", cfg.Servers, cfg.Errors)
	}
}

// Two names that sanitize to one namespace would silently shadow each other.
func TestLoadRejectsNamespaceClash(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mcp.json"),
		[]byte(`{"mcpServers":{"a-b":{"url":"https://x"},"a_b":{"url":"https://y"}}}`), 0o600)
	cfg := Load(dir, "", false)
	if len(cfg.Servers) != 1 {
		t.Fatalf("only one should survive: %+v", cfg.Servers)
	}
	if len(cfg.Errors) != 1 || !strings.Contains(cfg.Errors[0], "conflicts") {
		t.Fatalf("%q", cfg.Errors)
	}
}

func TestLoadValidatesTransport(t *testing.T) {
	cases := map[string]string{
		"neither":     `{"args":["x"]}`,
		"badurl":      `{"url":"ftp://x"}`,
		"httpsse":     `{"type":"sse","url":"http://x"}`,
		"urlwithcmd":  `{"url":"https://x","type":"stdio"}`,
		"badtimeout":  `{"url":"https://x","timeout":-1}`,
		"badexposure": `{"url":"https://x","toolExposure":{"a":"nope"}}`,
		"badheaders":  `{"url":"https://x","headers":{"a":1}}`,
	}
	for name, body := range cases {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "mcp.json"),
			[]byte(`{"mcpServers":{"`+name+`":`+body+`}}`), 0o600)
		cfg := Load(dir, "", false)
		if len(cfg.Errors) != 1 {
			t.Fatalf("%s should be rejected: %+v %q", name, cfg.Servers, cfg.Errors)
		}
	}
}

func TestLoadProjectOverlayKeepsCredentials(t *testing.T) {
	user, project := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(user, "mcp.json"), []byte(`{"mcpServers":{
		"a":{"url":"https://x","exposure":"codemode","description":"user desc","headers":{"X-K":"v"},"env":{"E":"1"}}}}`), 0o600)
	os.MkdirAll(filepath.Join(project, ".nulo"), 0o755)
	// No url, so this entry overrides only the exposure-related fields.
	os.WriteFile(filepath.Join(project, ".nulo", "mcp.json"),
		[]byte(`{"mcpServers":{"a":{"exposure":"direct","enabled":false}}}`), 0o600)

	cfg := Load(user, project, true)
	if len(cfg.Errors) != 0 {
		t.Fatalf("unexpected errors %q", cfg.Errors)
	}
	got := cfg.Servers["a"]
	if got.URL != "https://x" || got.Headers["X-K"] != "v" || got.Env["E"] != "1" {
		t.Fatalf("credentials should survive the overlay: %+v", got)
	}
	if got.Exposure != "direct" || got.on() {
		t.Fatalf("overlayed fields %+v", got)
	}
	if got.Description != "user desc" {
		t.Fatalf("description is not an override key: %+v", got)
	}

	// A full entry replaces the user one instead.
	os.WriteFile(filepath.Join(project, ".nulo", "mcp.json"),
		[]byte(`{"mcpServers":{"a":{"url":"https://y"}}}`), 0o600)
	cfg = Load(user, project, true)
	if got := cfg.Servers["a"]; got.URL != "https://y" || got.Headers != nil {
		t.Fatalf("a full entry should replace: %+v", got)
	}
}

// An override may not smuggle in anything but enabled/exposure/toolExposure, and
// it needs a global server to build on.
func TestLoadOverrideIsStrict(t *testing.T) {
	user, project := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(user, "mcp.json"),
		[]byte(`{"mcpServers":{"a":{"url":"https://x"}}}`), 0o600)
	os.MkdirAll(filepath.Join(project, ".nulo"), 0o755)

	os.WriteFile(filepath.Join(project, ".nulo", "mcp.json"),
		[]byte(`{"mcpServers":{"a":{"exposure":"direct","description":"nope"}}}`), 0o600)
	cfg := Load(user, project, true)
	if len(cfg.Errors) != 1 || !strings.Contains(cfg.Errors[0], "an override can only set") {
		t.Fatalf("%q", cfg.Errors)
	}

	os.WriteFile(filepath.Join(project, ".nulo", "mcp.json"),
		[]byte(`{"mcpServers":{"missing":{"enabled":false}}}`), 0o600)
	cfg = Load(user, project, true)
	if len(cfg.Errors) != 1 || !strings.Contains(cfg.Errors[0], "or a global server to override") {
		t.Fatalf("%q", cfg.Errors)
	}
}

func TestLoadAutoEnableCodemodeDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := Load(dir, "", false)
	if !cfg.AutoEnableCodemode {
		t.Fatalf("default should be on: %+v", cfg)
	}
	os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{"autoEnableCodemode":false}`), 0o600)
	if cfg = Load(dir, "", false); cfg.AutoEnableCodemode {
		t.Fatalf("explicit false should win: %+v", cfg)
	}
}

func TestLoadReportsUnparseableFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{`), 0o600)
	cfg := Load(dir, "", false)
	if len(cfg.Errors) != 1 {
		t.Fatalf("%q", cfg.Errors)
	}
}

func TestLoadStripsBOM(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mcp.json"),
		append([]byte("\xef\xbb\xbf"), []byte(`{"mcpServers":{"a":{"url":"https://x"}}}`)...), 0o600)
	cfg := Load(dir, "", false)
	if _, ok := cfg.Servers["a"]; !ok {
		t.Fatalf("a BOM should not break the file: %q", cfg.Errors)
	}
}

func TestConvertResult(t *testing.T) {
	// Text blocks are joined, images survive, and a resource link is readable.
	raw := json.RawMessage(`{"content":[
		{"type":"text","text":"one"},
		{"type":"text","text":"two"},
		{"type":"image","data":"AAAA","mimeType":"image/png"},
		{"type":"resource_link","uri":"file:///a.md","name":"a","mimeType":"text/markdown"}
	]}`)
	blocks, isErr := convertResult(raw)
	if isErr {
		t.Fatal("not an error result")
	}
	// Blocks stay separate; Pi only joins them when truncating.
	if len(blocks) != 4 || blocks[0].Text != "one" || blocks[1].Text != "two" {
		t.Fatalf("%+v", blocks)
	}
	if blocks[2].Type != "image" || blocks[2].MIMEType != "image/png" {
		t.Fatalf("image %+v", blocks[2])
	}
	if !strings.Contains(blocks[3].Text, "file:///a.md") {
		t.Fatalf("resource link %q", blocks[3].Text)
	}
}

func TestConvertResultTruncatesMiddle(t *testing.T) {
	big := strings.Repeat("ab", modelFacingLimit)
	raw, _ := json.Marshal(map[string]any{
		"content": []map[string]any{{"type": "text", "text": big}},
	})
	blocks, _ := convertResult(raw)
	if len(blocks) != 1 {
		t.Fatalf("%+v", blocks)
	}
	text := blocks[0].Text
	if !strings.HasPrefix(text, "Warning: truncated output") {
		t.Fatalf("%q", text[:80])
	}
	if !strings.Contains(text, "chars truncated") || !strings.Contains(text, "[Full output: ") {
		t.Fatalf("missing marker: %q", text[:200])
	}
	if len(text) > modelFacingLimit*2 {
		t.Fatalf("still too big: %d", len(text))
	}
}

func TestConvertResultErrorFlag(t *testing.T) {
	blocks, isErr := convertResult(json.RawMessage(`{"isError":true,"content":[{"type":"text","text":"boom"}]}`))
	if !isErr || len(blocks) != 1 || blocks[0].Text != "boom" {
		t.Fatalf("%+v %v", blocks, isErr)
	}
}

func TestConvertResultNonObject(t *testing.T) {
	blocks, isErr := convertResult(json.RawMessage(`"plain"`))
	if isErr || len(blocks) != 1 || blocks[0].Text != `"plain"` {
		t.Fatalf("%+v %v", blocks, isErr)
	}
}
