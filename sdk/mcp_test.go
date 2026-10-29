package sdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestMCPToolInLoop(t *testing.T) {
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
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}})
		case "tools/list":
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"tools": []map[string]any{{"name": "ping", "description": "ping"}}},
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
	agentDir := t.TempDir()
	os.WriteFile(filepath.Join(agentDir, "mcp.json"), []byte(`{"mcpServers":{"demo":{"url":"`+srv.URL+`"}}}`), 0o644)
	p := ai.NewFauxProvider()
	p.SetResponses(
		ai.FauxAssistantBlocks([]ai.ContentBlock{ai.ToolCallBlock("c1", "mcp__demo__ping", map[string]any{})}, ai.StopToolUse),
		ai.FauxAssistant("done"),
	)
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: agentDir, InMemory: true, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []Tool{}})
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := sess.Send(context.Background(), "ping")
	if err != nil {
		t.Fatal(err)
	}
	var tr *ai.Message
	for i := range msgs {
		if msgs[i].Role == "toolResult" {
			tr = &msgs[i]
		}
	}
	if tr == nil || tr.ContentString() != "pong" {
		t.Fatalf("toolResult %+v msgs=%+v", tr, msgs)
	}
}
