package sdk

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func providerTestDir(t *testing.T, baseURL string) string {
	t.Helper()
	dir := t.TempDir()
	models := fmt.Sprintf(`{"providers":{"local":{
		"name":"Local Test","api":"openai-completions","baseUrl":%q,"apiKey":"sk-test",
		"models":[{"id":"test-model","name":"Test Model","contextWindow":64000,"maxTokens":4096}]}}}`, baseURL)
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(models), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestProviderModelConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"OK\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	dir := providerTestDir(t, server.URL)
	if err := TestProviderModel(dir, "local/test-model"); err != nil {
		t.Fatal(err)
	}
}

func TestProviderModelConnectionRejectsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid api key", http.StatusUnauthorized)
	}))
	defer server.Close()
	dir := providerTestDir(t, server.URL)
	err := TestProviderModel(dir, "local/test-model")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid api key") {
		t.Fatalf("error = %v", err)
	}
}
