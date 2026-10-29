package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
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
	if len(tools) != 1 || tools[0].Name != "mcp__demo__echo" {
		t.Fatalf("%+v", tools)
	}
	text, isErr, err := tools[0].call(context.Background(), map[string]any{"x": 1})
	if err != nil || isErr || text != "pong" {
		t.Fatalf("%q %v %v", text, isErr, err)
	}
	_ = ai.DefaultFauxModel
}

func TestConnectStdioTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _, err := Connect(ctx, "hang", ServerConfig{Command: "sleep", Args: []string{"30"}})
	if err == nil {
		t.Fatal("expected timeout")
	}
}
