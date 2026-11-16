package sdk

import (
	"context"
	"reflect"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestPluginRegistryCapabilities(t *testing.T) {
	var started bool
	var sent string
	var commandArgs string
	p := PluginFunc{
		PluginName: "test-features",
		Func: func(r *Registry) error {
			if err := r.AddTool(ToolFunc{
				Def: ToolDefinition{Name: "echo", Description: "echo", Parameters: []byte(`{"type":"object"}`)},
				Run: func(_ context.Context, call ToolCall) (ToolResult, error) {
					return ToolResult{Content: []ContentBlock{ai.TextBlock(call.Arguments["text"].(string))}}, nil
				},
			}); err != nil {
				return err
			}
			if err := r.AddProvider(ProviderFunc{
				ProviderID: "test-provider",
				Fn: func(_ context.Context, _ Model, _ Transcript, _ StreamOptions) *EventStream {
					return NewEventStream()
				},
			}); err != nil {
				return err
			}
			r.AddHooks(Hooks{
				SessionStart: func(_ context.Context, sess *Session) error {
					started = true
					return sess.AddTool(ToolFunc{
						Def: ToolDefinition{Name: "dynamic", Description: "added at session start"},
						Run: func(context.Context, ToolCall) (ToolResult, error) { return ToolResult{}, nil },
					})
				},
				BeforeSend: func(_ context.Context, _ *Session, in *PromptInput) error {
					sent = in.Text
					in.Text = in.Text + "!"
					return nil
				},
			})
			if err := r.AddCommand(Command{
				Name: "hello",
				Run: func(_ context.Context, _ *Session, args string) error {
					commandArgs = args
					return nil
				},
			}); err != nil {
				return err
			}
			if err := r.AddPromptSection(PromptSection{
				Name:  "test_section",
				Build: func(context.Context, *Session) (string, error) { return "plugin context", nil },
			}); err != nil {
				return err
			}
			r.AddUI(UIDeclaration{
				Renderers: []Renderer{{Plugin: "test-features", Kind: "tool", Name: "echo"}},
				Shortcuts: []Shortcut{{Plugin: "test-features", Key: "ctrl-x", Description: "test"}},
			})
			return nil
		},
	}

	faux := ai.NewFauxProvider()
	faux.SetResponses(ai.FauxAssistant("ok"))
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true,
		StreamFn: faux.StreamSimple, Plugins: []Plugin{p},
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Dispose()
	if !started {
		t.Fatal("SessionStart hook did not run")
	}
	if got := toolNames(sess); !containsTool(got, "echo") || !containsTool(got, "dynamic") {
		t.Fatalf("tools = %v, want echo and dynamic", got)
	}
	if _, err := sess.Send(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	var user string
	for _, m := range sess.Messages() {
		if m.Role == "user" {
			user = m.ContentString()
		}
	}
	if user != "hi!" || sent != "hi" {
		t.Fatalf("user = %q, beforeSend saw %q", user, sent)
	}
	if err := sess.RunCommand(context.Background(), "hello", "world"); err != nil {
		t.Fatal(err)
	}
	if commandArgs != "world" {
		t.Fatalf("command args = %q", commandArgs)
	}
	if got := sess.UI(); len(got.Renderers) != 1 || len(got.Shortcuts) != 1 {
		t.Fatalf("UI = %+v", got)
	}
	if got := sess.Plugins().StreamFn(Model{Provider: "test-provider"}); got == nil {
		t.Fatal("provider hook lookup failed")
	}
	system := sess.Messages()[0]
	if system.Sections["test_section"] == nil || *system.Sections["test_section"] != "<test_section>\nplugin context\n</test_section>" {
		t.Fatalf("prompt section = %+v", system.Sections)
	}
}

func TestRegistryMCPDeclaration(t *testing.T) {
	r := NewRegistry()
	if err := r.AddMCPServer(MCPServer{Name: "test-mcp", Transport: "stdio", Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	if got := r.MCPServers(); len(got) != 1 || got[0].Name != "test-mcp" {
		t.Fatalf("MCP = %+v", got)
	}
}

func TestPluginRegistryRejectsDuplicates(t *testing.T) {
	r := NewRegistry()
	tool := ToolFunc{Def: ToolDefinition{Name: "same"}}
	if err := r.AddTool(tool); err != nil {
		t.Fatal(err)
	}
	if err := r.AddTool(tool); err == nil {
		t.Fatal("expected duplicate tool error")
	}
	if err := r.AddProvider(ProviderFunc{ProviderID: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := r.AddProvider(ProviderFunc{ProviderID: "p"}); err == nil {
		t.Fatal("expected duplicate provider error")
	}
	if err := r.AddCommand(Command{Name: "c", Run: func(context.Context, *Session, string) error { return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := r.AddCommand(Command{Name: "c", Run: func(context.Context, *Session, string) error { return nil }}); err == nil {
		t.Fatal("expected duplicate command error")
	}
}

func TestRegistryToolAdapter(t *testing.T) {
	r := NewRegistry()
	if err := r.AddTool(ToolFunc{
		Def: ToolDefinition{Name: "echo", Label: "Echo", ExecutionMode: "sequential"},
		Run: func(_ context.Context, call ToolCall) (ToolResult, error) {
			return ToolResult{Content: []ContentBlock{ai.TextBlock(call.ID)}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	tools := r.AgentTools()
	if len(tools) != 1 {
		t.Fatalf("tools = %d", len(tools))
	}
	res, err := tools[0].Execute(context.Background(), "call-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "call-1" {
		t.Fatalf("result = %+v", res)
	}
	if !reflect.DeepEqual(toolNamesFromAgent(tools), []string{"echo"}) {
		t.Fatalf("names = %v", toolNamesFromAgent(tools))
	}
}

func containsTool(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

func toolNamesFromAgent(tools []AgentTool) []string {
	out := make([]string, len(tools))
	for i, tool := range tools {
		out[i] = tool.Name
	}
	return out
}

func TestProtocolStreamFunctions(t *testing.T) {
	fns := map[string]StreamFn{
		"chat":      ChatStream,
		"responses": ResponsesStream,
		"messages":  MessagesStream,
	}
	for name, fn := range fns {
		if fn == nil {
			t.Fatalf("%s stream is nil", name)
		}
	}
}

func TestPluginProviderModels(t *testing.T) {
	model := Model{ID: "m", Name: "M", Provider: "acme", API: "chat", ContextWindow: 1000}
	p := PluginFunc{
		PluginName: "acme",
		Func: func(r *Registry) error {
			return r.AddProvider(ProviderFunc{
				ProviderID: "acme",
				ModelList:  []Model{model},
				Fn:         ChatStream,
			})
		},
	}
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true,
		NoBuiltinPlugins: true, Plugins: []Plugin{p},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := BuildModel("acme/m", svcs); err != nil || got.ID != "m" || got.Provider != "acme" {
		t.Fatalf("BuildModel = %+v, %v", got, err)
	}
	providers, err := svcs.ListProviders()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, provider := range providers {
		if provider.ID == "acme" && len(provider.Models) == 1 && provider.Models[0].ID == "m" {
			found = true
		}
	}
	if !found {
		t.Fatalf("plugin provider missing: %+v", providers)
	}
}

func TestBuiltinProtocolAliases(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(builtinProvidersPlugin(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, api := range []string{"chat", "responses", "messages", "openai-completions", "openai-responses", "anthropic-messages"} {
		if fn := r.StreamFn(Model{API: api}); fn == nil {
			t.Fatalf("no provider for API %q", api)
		}
	}
}

func TestRegistryReplaceBuiltin(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(builtinToolsPlugin(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := r.ReplaceTool(ToolFunc{Def: ToolDefinition{Name: "read", Description: "custom read"}}); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tool := range r.Tools() {
		if tool.Definition().Name == "read" && tool.Definition().Description == "custom read" {
			found = true
		}
	}
	if !found {
		t.Fatal("read was not replaced")
	}
	if err := r.Register(builtinProvidersPlugin(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := r.ReplaceProvider(ProviderFunc{ProviderID: "chat"}); err != nil {
		t.Fatal(err)
	}
	if fn := r.StreamFn(Model{API: "chat"}); fn == nil {
		t.Fatal("replaced provider not found")
	}
}

func TestPluginToolExposure(t *testing.T) {
	p := PluginFunc{
		PluginName: "exposure",
		Func: func(r *Registry) error {
			for _, name := range []string{"direct", "cm", "df", "hidden"} {
				exposure := map[string]string{"direct": "direct", "cm": "codemode", "df": "deferred", "hidden": "hidden"}[name]
				if err := r.AddTool(ToolFunc{
					Def: ToolDefinition{Name: name, Exposure: exposure},
					Run: func(context.Context, ToolCall) (ToolResult, error) { return ToolResult{}, nil },
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true,
		NoBuiltinPlugins: true, Plugins: []Plugin{p},
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Dispose()
	if !hasTool(sess.tools, "direct") || hasTool(sess.tools, "cm") || hasTool(sess.tools, "df") || hasTool(sess.tools, "hidden") {
		t.Fatalf("active tools = %v", toolNames(sess))
	}
	if !hasTool(sess.catalog, "cm") || !hasTool(sess.catalog, "df") || hasTool(sess.catalog, "hidden") {
		t.Fatalf("catalog = %v", toolNamesFromAgent(sess.catalog))
	}
	if !hasTool(sess.tools, "codemode") || !hasTool(sess.tools, "tool_search") {
		t.Fatalf("codemode/tool_search missing: %v", toolNames(sess))
	}
	var found bool
	for _, info := range sess.toolInfos() {
		if info["name"] == "cm" && info["source"] == "plugin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("plugin source missing: %+v", sess.toolInfos())
	}
}
