package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaokuang95/nulo/sdk"
)

func TestSDKHostStreamsAndPersistsSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, chunk := range []string{
			`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}`,
			`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":" from the SDK"}}]}`,
			`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	workspace := t.TempDir()
	agentDir := t.TempDir()
	sessionDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", sessionDir)
	writeFile(t, filepath.Join(agentDir, "models.json"), `{"providers":{"local":{
		"name":"Local Test","api":"openai-completions","baseUrl":"`+server.URL+`","apiKey":"sk-test",
		"models":[{"id":"test-model","name":"Test Model","contextWindow":64000,"maxTokens":4096}]}}}`)
	writeFile(t, filepath.Join(agentDir, "settings.json"), `{"defaultProvider":"local","defaultModel":"test-model","defaultThinkingLevel":"high"}`)

	a := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	if s == nil || !s.Real {
		t.Fatalf("active session = %+v", s)
	}
	if s.Model != "local/test-model" || s.modelName() != "Test Model" {
		t.Fatalf("model = %q / %q", s.Model, s.modelName())
	}
	if len(a.projects) == 0 || a.projects[0].CWD != workspace {
		t.Fatalf("projects = %+v", a.projects)
	}
	if !s.Welcome {
		t.Fatalf("new session is not in welcome state")
	}
	e := a.host.engine(s)
	if e == nil {
		t.Fatal("engine missing")
	}
	settled := make(chan struct{}, 1)
	e.sess.Subscribe(func(ev sdk.Event) {
		if ev.Type == "agent_settled" {
			select {
			case settled <- struct{}{}:
			default:
			}
		}
	})
	a.send("say hello")
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for agent_settled")
	}
	if s.Status != "done" {
		t.Fatalf("status = %q, messages = %+v", s.Status, s.Messages)
	}
	if !strings.Contains(threadText(s), "Hello from the SDK") {
		t.Fatalf("assistant text missing: %+v", s.Messages)
	}
	if s.Path == "" {
		t.Fatal("session path is empty")
	}
	if _, err := os.Stat(s.Path); err != nil {
		t.Fatalf("session was not persisted: %v", err)
	}
	infos, err := sdk.ListAllSessions(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].CWD != workspace {
		t.Fatalf("stored sessions = %+v", infos)
	}
	if s.Title == "Untitled session" || s.Title == "" {
		t.Fatalf("title was not derived from the first message: %q", s.Title)
	}
	parentPath := s.Path
	child, err := a.host.fork(a, s)
	if err != nil {
		t.Fatal(err)
	}
	if child.ForkOf != s.ID || child.Path == parentPath {
		t.Fatalf("fork = %+v (parent %s, %s)", child, s.ID, parentPath)
	}
	if !strings.Contains(threadText(child), "Hello from the SDK") {
		t.Fatalf("fork did not inherit the thread: %s", threadText(child))
	}
	if err := a.host.rewind(a, s); err != nil {
		t.Fatal(err)
	}
	if !s.Welcome || strings.Contains(threadText(s), "Hello from the SDK") {
		t.Fatalf("rewind did not return to the checkpoint: welcome=%v text=%q", s.Welcome, threadText(s))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func threadText(s *session) string {
	var b strings.Builder
	for _, m := range s.Messages {
		b.WriteString(m.Text)
		b.WriteByte('\n')
		for _, block := range m.Blocks {
			b.WriteString(block.Text)
			b.WriteString(block.Output)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func TestSDKHostAsksBeforeToolCall(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		if calls == 0 {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"bash\",\"arguments\":\"{\\\"command\\\":\\\"printf hello-from-tool\\\"}\"}}]}}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
			calls = 1
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Tool finished.\"}}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		}
		if flusher != nil {
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	workspace := t.TempDir()
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	writeFile(t, filepath.Join(agentDir, "models.json"), `{"providers":{"local":{
		"name":"Local Test","api":"openai-completions","baseUrl":"`+server.URL+`","apiKey":"sk-test",
		"models":[{"id":"test-model","name":"Test Model","contextWindow":64000,"maxTokens":4096}]}}}`)
	writeFile(t, filepath.Join(agentDir, "settings.json"), `{"defaultProvider":"local","defaultModel":"test-model"}`)

	a := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	e := a.host.engine(s)
	if e == nil {
		t.Fatal("engine missing")
	}
	settled := make(chan struct{}, 1)
	e.sess.Subscribe(func(ev sdk.Event) {
		if ev.Type == "agent_settled" {
			select {
			case settled <- struct{}{}:
			default:
			}
		}
	})
	if err := a.host.send(a, s, "run a tool", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var approval *block
	for time.Now().Before(deadline) {
		if approval = s.pendingApproval(); approval != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if approval == nil {
		t.Fatalf("approval did not appear; status=%q messages=%+v", s.Status, s.Messages)
	}
	if approval.Command != "printf hello-from-tool" || approval.CWD != workspace {
		t.Fatalf("approval = %+v", approval)
	}
	a.resolveApproval(s, approval, "approved")
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out after approval")
	}
	if s.Status != "done" {
		t.Fatalf("status = %q messages = %+v", s.Status, s.Messages)
	}
	if !strings.Contains(threadText(s), "hello-from-tool") {
		t.Fatalf("tool output missing: %s", threadText(s))
	}
	if len(s.Terminal) == 0 || !strings.Contains(s.Terminal[0].Text, "printf hello-from-tool") {
		t.Fatalf("terminal = %+v", s.Terminal)
	}
	if len(s.Approvals) != 1 || s.Approvals[0].Approval != "approved" {
		t.Fatalf("approval history = %+v", s.Approvals)
	}
}

func TestSDKHostRestoresPersistedSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Persisted reply\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	workspace := t.TempDir()
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	writeFile(t, filepath.Join(agentDir, "models.json"), `{"providers":{"local":{
		"name":"Local Test","api":"openai-completions","baseUrl":"`+server.URL+`","apiKey":"sk-test",
		"models":[{"id":"test-model","name":"Test Model","contextWindow":64000,"maxTokens":4096}]}}}`)
	writeFile(t, filepath.Join(agentDir, "settings.json"), `{"defaultProvider":"local","defaultModel":"test-model"}`)

	a := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	s := a.active()
	e := a.host.engine(s)
	settled := make(chan struct{}, 1)
	e.sess.Subscribe(func(ev sdk.Event) {
		if ev.Type == "agent_settled" {
			settled <- struct{}{}
		}
	})
	if err := a.host.send(a, s, "persist this", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
	path := s.Path
	a.closeHost()

	b := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	defer b.closeHost()
	if len(b.sessions) != 1 || b.active() == nil {
		t.Fatalf("restored sessions = %+v", b.sessions)
	}
	restored := b.active()
	if restored.Path != path {
		t.Fatalf("path = %q, want %q", restored.Path, path)
	}
	if restored.Status != "done" || !strings.Contains(threadText(restored), "Persisted reply") {
		t.Fatalf("restored thread = status %q, %s", restored.Status, threadText(restored))
	}
	if restored.Title == "Untitled session" || restored.modelName() != "Test Model" {
		t.Fatalf("restored meta = title %q model %q", restored.Title, restored.modelName())
	}
}

func TestSDKHostStartsWithoutConfig(t *testing.T) {
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	a := newAppWithHost(hostOptions{Workspace: t.TempDir(), AgentDir: t.TempDir(), Sync: true})
	defer a.closeHost()
	s := a.active()
	if s == nil || !s.Real || !s.Welcome {
		t.Fatalf("active = %+v", s)
	}
	if s.Model != "faux/faux-1" && !strings.Contains(s.Model, "faux") {
		t.Fatalf("model = %q", s.Model)
	}
	if len(a.providers) == 0 || a.providers[0].ID != "faux" {
		t.Fatalf("providers = %+v", a.providers)
	}
}

func TestSDKHostOpensProject(t *testing.T) {
	t.Setenv("NULO_HOME", t.TempDir())
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	first := t.TempDir()
	second := t.TempDir()
	a := newAppWithHost(hostOptions{Workspace: first, AgentDir: t.TempDir(), Sync: true})
	defer a.closeHost()
	a.openWorkspace(second)
	s := a.active()
	if s == nil || s.CWD != second {
		t.Fatalf("active = %+v", s)
	}
	found := false
	for _, g := range a.projects {
		if g.CWD == second {
			found = true
		}
	}
	if !found {
		t.Fatalf("project %q missing from %+v", second, a.projects)
	}
}

func TestSDKHostSendsImageAttachment(t *testing.T) {
	bodyCh := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- string(body):
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"role":"assistant","content":"I can see the image."}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	workspace := t.TempDir()
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	writeFile(t, filepath.Join(agentDir, "models.json"), `{"providers":{"local":{
		"name":"Local Test","api":"openai-completions","baseUrl":"`+server.URL+`","apiKey":"sk-test",
		"models":[{"id":"test-model","name":"Test Model","contextWindow":64000,"maxTokens":4096,"input":["text","image"]}]}}}`)
	writeFile(t, filepath.Join(agentDir, "settings.json"), `{"defaultProvider":"local","defaultModel":"test-model"}`)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(workspace, "pixel.png")
	if err := os.WriteFile(imagePath, png, 0o644); err != nil {
		t.Fatal(err)
	}

	a := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	e := a.host.engine(s)
	settled := make(chan struct{}, 1)
	e.sess.Subscribe(func(ev sdk.Event) {
		if ev.Type == "agent_settled" {
			select {
			case settled <- struct{}{}:
			default:
			}
		}
	})
	if err := a.host.send(a, s, "what is this?", []string{imagePath}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
	select {
	case body := <-bodyCh:
		if !strings.Contains(body, `"type":"image_url"`) || !strings.Contains(body, "data:image/png;base64,") {
			t.Fatalf("request did not include an image block: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request body was not captured")
	}
	if !strings.Contains(threadText(s), "I can see the image.") {
		t.Fatalf("assistant reply missing: %s", threadText(s))
	}
}

func TestProviderAPIKeyEditor(t *testing.T) {
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	writeFile(t, filepath.Join(agentDir, "models.json"), `{"providers":{"local":{
		"name":"Local Test","api":"openai-completions","baseUrl":"https://example.test/v1",
		"models":[{"id":"test-model","name":"Test Model","contextWindow":64000}]}}}`)
	a := newAppWithHost(hostOptions{Workspace: t.TempDir(), AgentDir: agentDir, Sync: true})
	defer a.closeHost()
	prov, ok := a.providerByID("local")
	if !ok || prov.APIKeySource != "" {
		t.Fatalf("provider = %+v", prov)
	}
	a.openProviderKey(prov)
	a.keyValue = "sk-ui"
	a.saveProviderKey()
	prov, _ = a.providerByID("local")
	if prov.APIKeySource != "auth" {
		t.Fatalf("source after save = %q", prov.APIKeySource)
	}
	if _, err := os.Stat(filepath.Join(agentDir, "auth.json")); err != nil {
		t.Fatal(err)
	}
	a.keyProvider, a.keyName = prov.ID, prov.Name
	a.clearProviderKey()
	prov, _ = a.providerByID("local")
	if prov.APIKeySource != "" {
		t.Fatalf("source after clear = %q", prov.APIKeySource)
	}
}

func TestSDKHostPlanToolUpdatesPanel(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if atomic.AddInt32(&calls, 1) == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call-plan\",\"type\":\"function\",\"function\":{\"name\":\"plan\",\"arguments\":\"{\\\"title\\\":\\\"Build the feature\\\",\\\"next\\\":\\\"Run the test\\\",\\\"steps\\\":[{\\\"title\\\":\\\"Inspect code\\\",\\\"status\\\":\\\"done\\\"},{\\\"title\\\":\\\"Write test\\\",\\\"status\\\":\\\"active\\\"}]}\"}}]}}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Plan updated.\"}}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	workspace := t.TempDir()
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	writeFile(t, filepath.Join(agentDir, "models.json"), `{"providers":{"local":{
		"name":"Local Test","api":"openai-completions","baseUrl":"`+server.URL+`","apiKey":"sk-test",
		"models":[{"id":"test-model","name":"Test Model","contextWindow":64000,"maxTokens":4096}]}}}`)
	writeFile(t, filepath.Join(agentDir, "settings.json"), `{"defaultProvider":"local","defaultModel":"test-model"}`)

	a := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	s := a.active()
	e := a.host.engine(s)
	settled := make(chan struct{}, 1)
	e.sess.Subscribe(func(ev sdk.Event) {
		if ev.Type == "agent_settled" {
			select {
			case settled <- struct{}{}:
			default:
			}
		}
	})
	a.send("build it")
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out: status=%q messages=%+v terminal=%+v", s.Status, s.Messages, s.Terminal)
	}
	if s.Plan.Title != "Build the feature" || len(s.Plan.Steps) != 2 {
		t.Fatalf("plan = %+v", s.Plan)
	}
	if s.Plan.Steps[1].Status != "active" || s.Plan.Next != "Run the test" {
		t.Fatalf("plan = %+v", s.Plan)
	}
	if strings.Contains(threadText(s), "Plan updated: 2 step(s).") {
		t.Fatalf("plan tool result should stay out of the thread: %s", threadText(s))
	}
	a.closeHost()

	b := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	defer b.closeHost()
	restored := b.active()
	if restored.Plan.Title != "Build the feature" || len(restored.Plan.Steps) != 2 {
		t.Fatalf("restored plan = %+v", restored.Plan)
	}
}

func TestSDKHostMessageActionsAndRewind(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, chunk := range []string{
			`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"Answer"}}]}`,
			`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	workspace := t.TempDir()
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	writeFile(t, filepath.Join(agentDir, "models.json"), `{"providers":{"local":{
		"name":"Local Test","api":"openai-completions","baseUrl":"`+server.URL+`","apiKey":"sk-test",
		"models":[{"id":"test-model","name":"Test Model","contextWindow":64000,"maxTokens":4096}]}}}`)
	writeFile(t, filepath.Join(agentDir, "settings.json"), `{"defaultProvider":"local","defaultModel":"test-model"}`)

	a := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	e := a.host.engine(s)
	if e == nil {
		t.Fatal("engine missing")
	}
	settled := make(chan struct{}, 8)
	e.sess.Subscribe(func(ev sdk.Event) {
		if ev.Type == "agent_settled" {
			settled <- struct{}{}
		}
	})
	wait := func() {
		t.Helper()
		select {
		case <-settled:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for agent_settled")
		}
	}

	a.send("first prompt")
	wait()
	if len(s.Messages) != 2 || s.Messages[0].EntryID == "" || s.Messages[1].EntryID == "" {
		t.Fatalf("messages = %+v", s.Messages)
	}

	a.beginMessageEdit(s, &s.Messages[0], 0)
	if a.draft != "first prompt" {
		t.Fatalf("edit draft = %q", a.draft)
	}
	a.send("edited prompt")
	wait()
	if len(s.Messages) != 2 || s.Messages[0].Text != "edited prompt" || a.messageEditID != "" {
		t.Fatalf("edited messages = %+v", s.Messages)
	}

	assistant := &s.Messages[1]
	a.retryMessage(s, assistant)
	wait()
	if len(s.Messages) != 2 || s.Messages[0].Text != "edited prompt" {
		t.Fatalf("retried messages = %+v", s.Messages)
	}

	assistant = &s.Messages[len(s.Messages)-1]
	child, err := a.host.branchFromMessage(a, s, assistant.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	if child.ForkOf != s.ID || len(child.Messages) != 2 {
		t.Fatalf("child = %+v", child)
	}

	a.activeID = child.ID
	a.rewindSession(child)
	if len(a.rewindPoints) != 1 {
		t.Fatalf("rewind points = %+v", a.rewindPoints)
	}
	a.confirmRewind()
	real := 0
	for _, m := range child.Messages {
		if m.Role == "user" || m.Role == "assistant" {
			real++
		}
	}
	if !child.Welcome || real != 0 {
		t.Fatalf("rewind left messages = %+v, welcome=%v", child.Messages, child.Welcome)
	}
}

func TestSDKHostRewindRestoresFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, chunk := range []string{
			`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"Done"}}]}`,
			`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	workspace := gitTestRepo(t)
	agentDir := t.TempDir()
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	writeFile(t, filepath.Join(agentDir, "models.json"), `{"providers":{"local":{
		"name":"Local Test","api":"openai-completions","baseUrl":"`+server.URL+`","apiKey":"sk-test",
		"models":[{"id":"test-model","name":"Test Model","contextWindow":64000,"maxTokens":4096}]}}}`)
	writeFile(t, filepath.Join(agentDir, "settings.json"), `{"defaultProvider":"local","defaultModel":"test-model"}`)

	writeFile(t, filepath.Join(workspace, "README.md"), "checkpoint\n")
	writeFile(t, filepath.Join(workspace, "checkpoint.txt"), "before\n")
	a := newAppWithHost(hostOptions{Workspace: workspace, AgentDir: agentDir, Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	e := a.host.engine(s)
	if e == nil {
		t.Fatal("engine missing")
	}
	settled := make(chan struct{}, 1)
	e.sess.Subscribe(func(ev sdk.Event) {
		if ev.Type == "agent_settled" {
			select {
			case settled <- struct{}{}:
			default:
			}
		}
	})
	a.send("change files")
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for agent_settled")
	}
	writeFile(t, filepath.Join(workspace, "README.md"), "later\n")
	if err := os.Remove(filepath.Join(workspace, "checkpoint.txt")); err != nil {
		t.Fatal(err)
	}
	a.rewindSession(s)
	if len(a.rewindPoints) != 1 || !a.rewindPoints[0].Snapshot {
		t.Fatalf("rewind points = %+v", a.rewindPoints)
	}
	a.confirmRewind()
	data, err := os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(data) != "checkpoint\n" {
		t.Fatalf("README = %q, %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(workspace, "checkpoint.txt"))
	if err != nil || string(data) != "before\n" {
		t.Fatalf("checkpoint.txt = %q, %v", data, err)
	}
}
