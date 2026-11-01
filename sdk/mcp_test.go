package sdk

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

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// newMCPDemoServer is a fake MCP server offering one "ping" tool that answers
// "pong", plus server instructions for the prompt-summary fallback.
func newMCPDemoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"instructions": "Demo server instructions\nsecond line"}})
		case "tools/list":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"tools": []map[string]any{{"name": "ping", "description": "Ping the demo"}}},
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
	t.Cleanup(srv.Close)
	return srv
}

// mcpSession builds an in-memory session with no built-in tools, so the declared
// tool list contains only what MCP exposure and codemode put there. $URL in either
// config file is replaced with the fake server's address.
func mcpSession(t *testing.T, userJSON, projectJSON string, trusted bool, responses ...ai.Message) (*Session, error) {
	t.Helper()
	srv := newMCPDemoServer(t)
	agentDir := t.TempDir()
	cwd := t.TempDir()
	sub := func(s string) string { return strings.ReplaceAll(s, "$URL", srv.URL) }
	if userJSON != "" {
		if err := os.WriteFile(filepath.Join(agentDir, "mcp.json"), []byte(sub(userJSON)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if projectJSON != "" {
		if err := os.MkdirAll(filepath.Join(cwd, ".nulo"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cwd, ".nulo", "mcp.json"), []byte(sub(projectJSON)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := ai.NewFauxProvider()
	p.SetResponses(responses...)
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: agentDir, InMemory: true, ProjectTrusted: trusted, StreamFn: p.StreamSimple,
	})
	if err != nil {
		return nil, err
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { rt.Close() })
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []Tool{}})
	if err != nil {
		return nil, err
	}
	t.Cleanup(sess.Dispose)
	return sess, nil
}

// declaredTools is the tool list the model is told about. The system prompt's
// tools section is built from the same active set the providers declare, and
// unlike the transcript's tool records it exists from session start.
func declaredTools(sess *Session) []string {
	var names []string
	for _, line := range strings.Split(systemSection(sess, "tools"), "\n") {
		entry, ok := strings.CutPrefix(line, "- ")
		if !ok {
			continue
		}
		if name, _, ok := strings.Cut(entry, ": "); ok {
			names = append(names, name)
		}
	}
	return names
}

func systemSection(sess *Session, name string) string {
	for _, m := range sess.Manager().Project().Messages {
		if m.Role == "system" {
			if v, ok := m.Sections[name]; ok && v != nil {
				return *v
			}
		}
	}
	return ""
}

func lastToolResult(msgs []Message) *Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "toolResult" {
			return &msgs[i]
		}
	}
	return nil
}

func TestMCPDefaultExposureIsCodemode(t *testing.T) {
	sess, err := mcpSession(t,
		`{"mcpServers":{"demo":{"url":"$URL"}}}`,
		"", false,
		ai.FauxAssistantBlocks([]ai.ContentBlock{
			ai.ToolCallBlock("c1", "codemode", map[string]any{"source": "return await tools.mcp__demo__ping({})"}),
		}, ai.StopToolUse),
		ai.FauxAssistant("done"),
	)
	if err != nil {
		t.Fatal(err)
	}
	// The tool is not declared, so it costs nothing per request; codemode is
	// switched on automatically so the tool is still reachable.
	if got := declaredTools(sess); len(got) != 1 || got[0] != "codemode" {
		t.Fatalf("declared tools %v, want [codemode] only", got)
	}
	if section := systemSection(sess, "mcp_servers"); !strings.Contains(section, "mcp__demo") ||
		!strings.Contains(section, "codemode") {
		t.Fatalf("mcp_servers section %q", section)
	}
	if !strings.Contains(systemSection(sess, "mcp_servers"), "Demo server instructions") {
		t.Fatalf("summary should fall back to server instructions: %q", systemSection(sess, "mcp_servers"))
	}

	msgs, err := sess.Send(context.Background(), "ping")
	if err != nil {
		t.Fatal(err)
	}
	tr := lastToolResult(msgs)
	if tr == nil || !strings.Contains(tr.ContentString(), "pong") {
		t.Fatalf("codemode should reach the MCP tool: %+v", tr)
	}
}

func TestMCPDirectExposureIsDeclared(t *testing.T) {
	sess, err := mcpSession(t,
		`{"mcpServers":{"demo":{"url":"$URL","exposure":"direct"}}}`,
		"", false,
		ai.FauxAssistantBlocks([]ai.ContentBlock{ai.ToolCallBlock("c1", "mcp__demo__ping", map[string]any{})}, ai.StopToolUse),
		ai.FauxAssistant("done"),
	)
	if err != nil {
		t.Fatal(err)
	}
	got := declaredTools(sess)
	if len(got) != 1 || got[0] != "mcp__demo__ping" {
		t.Fatalf("declared tools %v, want [mcp__demo__ping]", got)
	}
	// Nothing is indirect, so codemode stays off.
	for _, n := range got {
		if n == "codemode" {
			t.Fatalf("codemode should not be auto-enabled: %v", got)
		}
	}
	// A direct server's tools are declared, so the section does not list them.
	if section := systemSection(sess, "mcp_servers"); section != "" {
		t.Fatalf("direct servers belong nowhere in mcp_servers: %q", section)
	}

	msgs, err := sess.Send(context.Background(), "ping")
	if err != nil {
		t.Fatal(err)
	}
	tr := lastToolResult(msgs)
	if tr == nil || !strings.Contains(tr.ContentString(), "pong") {
		t.Fatalf("the model should call the tool directly: %+v", tr)
	}
}

func TestMCPHiddenExposureReachesNowhere(t *testing.T) {
	sess, err := mcpSession(t,
		`{"mcpServers":{"demo":{"url":"$URL","exposure":"hidden"}}}`,
		"", false,
		ai.FauxAssistant("done"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := declaredTools(sess); len(got) != 0 {
		t.Fatalf("hidden tools must not be declared: %v", got)
	}
	if section := systemSection(sess, "mcp_servers"); section != "" {
		t.Fatalf("hidden servers must not appear: %q", section)
	}
	// Nor may a script reach them.
	if _, err := sess.Send(context.Background(), "ping"); err != nil {
		t.Fatal(err)
	}
}

// deferred tools are not declared until tool_search loads them; loading is
// recorded in the transcript so a resume or fork keeps the tool.
func TestMCPDeferredLoadsViaToolSearch(t *testing.T) {
	sess, err := mcpSession(t,
		`{"mcpServers":{"demo":{"url":"$URL","exposure":"deferred"}}}`,
		"", false,
		ai.FauxAssistantBlocks([]ai.ContentBlock{
			ai.ToolCallBlock("c1", "tool_search", map[string]any{"query": "ping the demo server"}),
		}, ai.StopToolUse),
		ai.FauxAssistantBlocks([]ai.ContentBlock{
			ai.ToolCallBlock("c2", "mcp__demo__ping", map[string]any{}),
		}, ai.StopToolUse),
		ai.FauxAssistant("done"),
	)
	if err != nil {
		t.Fatal(err)
	}
	// The MCP tool is hidden; only the discovery tool is declared, and codemode
	// stays off because nothing uses it.
	if got := declaredTools(sess); len(got) != 1 || got[0] != "tool_search" {
		t.Fatalf("declared tools %v, want [tool_search]", got)
	}
	if section := systemSection(sess, "mcp_servers"); !strings.Contains(section, "tool_search") ||
		!strings.Contains(section, "Load the tools of `tool_search` servers") {
		t.Fatalf("section %q", section)
	}

	msgs, err := sess.Send(context.Background(), "ping")
	if err != nil {
		t.Fatal(err)
	}
	var loaded bool
	for _, m := range msgs {
		if strings.Contains(m.ContentString(), "Loaded 1 tool") {
			loaded = true
		}
	}
	if !loaded {
		t.Fatalf("tool_search should report what it loaded: %+v", msgs)
	}
	// The loop records the addition, which is what makes it survive /tree.
	var declared bool
	for _, m := range msgs {
		for _, added := range m.ToolsAdded {
			if added.Name == "mcp__demo__ping" {
				declared = true
			}
		}
	}
	if !declared {
		t.Fatalf("the loaded tool should be declared in the transcript: %+v", msgs)
	}
	tr := lastToolResult(msgs)
	if tr == nil || !strings.Contains(tr.ContentString(), "pong") {
		t.Fatalf("the loaded tool should be callable: %+v", tr)
	}
}

// A per-tool override splits one server across exposures.
func TestMCPToolExposureSplitsServer(t *testing.T) {
	sess, err := mcpSession(t,
		`{"mcpServers":{"demo":{"url":"$URL","exposure":"codemode","toolExposure":{"ping":"direct"}}}}`,
		"", false,
		ai.FauxAssistantBlocks([]ai.ContentBlock{
			ai.ToolCallBlock("c1", "mcp__demo__ping", map[string]any{}),
		}, ai.StopToolUse),
		ai.FauxAssistant("done"),
	)
	if err != nil {
		t.Fatal(err)
	}
	got := declaredTools(sess)
	var hasPing, hasCodemode bool
	for _, name := range got {
		switch name {
		case "mcp__demo__ping":
			hasPing = true
		case "codemode":
			hasCodemode = true
		}
	}
	if !hasPing {
		t.Fatalf("the direct override should declare the tool: %v", got)
	}
	if !hasCodemode {
		t.Fatalf("the server-level codemode still turns codemode on: %v", got)
	}

	msgs, err := sess.Send(context.Background(), "ping")
	if err != nil {
		t.Fatal(err)
	}
	if tr := lastToolResult(msgs); tr == nil || !strings.Contains(tr.ContentString(), "pong") {
		t.Fatalf("%+v", tr)
	}
}

func TestMCPAutoEnableCodemodeCanBeDisabled(t *testing.T) {
	sess, err := mcpSession(t,
		`{"autoEnableCodemode":false,"mcpServers":{"demo":{"url":"$URL"}}}`,
		"", false,
		ai.FauxAssistant("done"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := declaredTools(sess); len(got) != 0 {
		t.Fatalf("codemode should stay off: %v", got)
	}
	// The server is still announced, so the model knows the tools exist.
	if section := systemSection(sess, "mcp_servers"); !strings.Contains(section, "mcp__demo") {
		t.Fatalf("section %q", section)
	}
}

// A bad entry is reported and skipped; the good servers still start, the way Pi
// keeps going and prints the problems at startup.
func TestMCPBadExposureIsReportedNotFatal(t *testing.T) {
	sess, err := mcpSession(t,
		`{"mcpServers":{
			"bad":{"url":"$URL","exposure":"bogus"},
			"good":{"url":"$URL","exposure":"direct"}}}`,
		"", false,
		ai.FauxAssistant("done"),
	)
	if err != nil {
		t.Fatalf("a bad entry must not stop the session: %v", err)
	}
	diags := sess.MCPDiagnostics()
	if len(diags) != 1 || !strings.Contains(diags[0], "exposure") {
		t.Fatalf("diagnostics %q", diags)
	}
	var hasGood bool
	for _, name := range declaredTools(sess) {
		if name == "mcp__good__ping" {
			hasGood = true
		}
	}
	if !hasGood {
		t.Fatalf("the good server should still connect: %v", declaredTools(sess))
	}
}

func TestMCPProjectOverlayChangesExposureOnly(t *testing.T) {
	// A project entry without url overrides just exposure and keeps the rest.
	sess, err := mcpSession(t,
		`{"mcpServers":{"demo":{"url":"$URL","exposure":"codemode"}}}`,
		`{"mcpServers":{"demo":{"exposure":"direct"}}}`,
		true,
		ai.FauxAssistant("done"),
	)
	if err != nil {
		t.Fatal(err)
	}
	got := declaredTools(sess)
	if len(got) != 1 || got[0] != "mcp__demo__ping" {
		t.Fatalf("project overlay should switch the server to direct: %v", got)
	}
}

func TestMCPProjectConfigNeedsTrust(t *testing.T) {
	// Without trust the project file is ignored, so the user exposure stands.
	sess, err := mcpSession(t,
		`{"mcpServers":{"demo":{"url":"$URL","exposure":"codemode"}}}`,
		`{"mcpServers":{"demo":{"exposure":"direct"}}}`,
		false,
		ai.FauxAssistant("done"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := declaredTools(sess); len(got) != 1 || got[0] != "codemode" {
		t.Fatalf("untrusted project config must be skipped: %v", got)
	}
}
