# nulo

Coding-agent Session SDK in Go. Nulo owns its session format; Pi sessions can be
imported one-way, but the runtime is not a Pi compatibility layer.

Embed with `github.com/xiaokuang95/nulo/sdk`. TUI / Web / Desktop hosts import that package only.

```go
svcs, err := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
    CWD: cwd, AgentDir: sdk.DefaultAgentDir(),
})
```

Config root: `~/.nulo/agent` (`NULO_HOME` / `NULO_CODING_AGENT_DIR`).

## Host surface

Besides the session lifecycle, the public SDK exposes what a desktop or TUI host
needs to render around a session:

- `ListAllSessions(agentDir)` lists session files across every working directory,
  newest first, so a host can build a project switcher before choosing a CWD.
- `ListProviders(agentDir)` resolves `models.json` into provider names, endpoints,
  context windows, output limits, image/reasoning flags and the API-key source
  (`auth`, `models`, `env`). `AgentSessionServices.ListProviders` additionally
  includes models declared by provider plugins. `SetProviderAPIKey` /
  `ClearProviderAPIKey` write the agent's 0600 `auth.json` without exposing the
  key to a host UI.
- `ListMCPServers` exposes merged global/project MCP entries, including disabled
  servers, without leaking commands or credentials. `ListSkills` discovers the
  skills a workspace would load. `CreateAgentSessionServicesOptions.DisabledSkills`
  filters skills by name, so a host can offer persisted enable switches.
- `PromptOptions.Images` attaches base64 image content blocks to a user message;
  `ImageAttachment` carries the data and MIME type. `TestProviderModel` sends a
  tiny probe through a configured provider, so a host can check its key and
  endpoint without creating a session.
- `Session.SetPlan` / `Session.Plan` expose a structured task plan persisted as a
  custom session entry. `SetPlan` emits `plan_update` with an `Event.Plan`, and
  the default toolset includes a `plan` tool that calls it. An explicit
  `defaultTools` setting must include `"plan"` to keep it.
- `RuntimeOptions.ApproveTool` installs a default approval hook for sessions
  created, resumed or forked by that runtime. `SessionStartOptions.ApproveTool`
  overrides it per session. A denial becomes a tool error; returning an error
  aborts the turn.
- `Session.Branch` / `LeafID`, `NavigateTree`, `Compact`, `File` and the existing
  event subscription give a host enough state for rewind, fork and usage views.
- `Runtime.Close` disposes the current session; `AgentSessionServices.Close`
  releases MCP connections, plugins and other service resources. A runtime is not
  safe for concurrent use.

## SDK plugins

The primary extension API is a Go interface. A plugin registers any combination
of tools, providers, hooks, commands, prompt sections, MCP servers and UI
declarations through one registry:

```go
type MyPlugin struct{}

func (MyPlugin) Name() string { return "my-plugin" }

func (MyPlugin) Register(r *sdk.Registry) error {
    if err := r.AddTool(sdk.ToolFunc{
        Def: sdk.ToolDefinition{Name: "hello", Description: "say hi"},
        Run: func(ctx context.Context, call sdk.ToolCall) (sdk.ToolResult, error) {
            return sdk.ToolResult{Content: []sdk.ContentBlock{{Type: "text", Text: "hi"}}}, nil
        },
    }); err != nil {
        return err
    }
    r.AddHooks(sdk.Hooks{
        BeforeSend: func(ctx context.Context, sess *sdk.Session, in *sdk.PromptInput) error {
            return nil
        },
    })
    return nil
}

svcs, err := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
    CWD: cwd, AgentDir: sdk.DefaultAgentDir(),
    Plugins: []sdk.Plugin{MyPlugin{}},
})
```

Tools can set `ToolDefinition.Exposure` to `direct` (default), `codemode`,
`deferred` or `hidden`; codemode/deferred tools stay in the catalogue without
being declared on every request.

Built-in file/shell tools are registered through the same `Tool` interface. The
three wire protocols are provider plugins too: `sdk.ChatStream`,
`sdk.ResponsesStream` and `sdk.MessagesStream` can back a custom supplier:

```go
r.AddProvider(sdk.ProviderFunc{
    ProviderID: "my-supplier",
    ModelList:  []sdk.Model{{ID: "m", Provider: "my-supplier", API: "chat"}},
    Fn:         sdk.ChatStream,
})
```

`Registry` also carries plugin-provided MCP server declarations and host-facing
renderer/shortcut declarations. `Session.Commands`, `Session.RunCommand` and
`Session.UI` expose the declarative surface to a host.

## Pi session import

Nulo writes its own JSONL format (`format: "nulo"`, schema version 1) and keeps
the session tree/branch/rewind model. Existing Pi logs are read by
`Runtime.ImportPi(path)`, which converts messages, tree links, model/thinking
changes, compaction, branch summaries and labels into a new nulo session. Pi
logs are never written or upgraded in place.

## Session storage

New sessions live under:

```text
<agentDir>/sessions/<project-slug>-<short-hash>/<timestamp>_<session-id>.jsonl
```

The project directory is a readable slug of the working-directory name plus a
short SHA-256 of the resolved CWD, not an encoded absolute path. It is short,
stable for a project, and disambiguates projects with the same folder name.

`NULO_SESSION_DIR` overrides the sessions root. The old
`NULO_CODING_AGENT_SESSION_DIR` name is still accepted as a fallback. Old
Pi-encoded `--...--` project directories are still read and listed so existing
sessions are not lost, but new sessions are never written there.

## Legacy plugin adapter

`sdk.LoadLegacyPlugin` loads an existing native-library or stdio plugin and
exposes its static tools, commands, providers, MCP declarations and UI
declarations through `sdk.Registry`:

```go
legacy, err := sdk.LoadLegacyPlugin(sdk.LegacyPluginOptions{
    Path: pluginPath,
    CWD: cwd,
    AgentDir: sdk.DefaultAgentDir(),
})
if err != nil {
    return err
}
svcs, err := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
    CWD: cwd, AgentDir: sdk.DefaultAgentDir(),
    Plugins: []sdk.Plugin{legacy},
    // Use this when the same path is also configured for automatic loading.
    NoPlugins: true,
})
```

The adapter is deliberately load-time oriented. Dynamic `registerTool` calls
and session-bound events still need the classic `PluginPaths` host; the adapter
reports those limitations through `OnDiagnostic`. Tools with `codemode` and
`deferred` exposure are preserved. Direct tools marked inactive, and hidden
tools, are not added to the registry. Providers and MCP declarations registered
during plugin initialization are bridged; dynamic registrations after startup
are not. Set `ReplaceExisting` when a plugin must override a built-in tool or
provider with the same name.

## Legacy native/stdio plugins

The older dynamic-library and subprocess plugin host remains available for
session-bound events and dynamic registration, and `LoadLegacyPlugin` bridges
its load-time capabilities into the SDK registry above. It has two host modes,
neither of which starts Node:

1. **Shared library** — an in-process `.dll`/`.so`/`.dylib` exporting the C ABI.
2. **Protocol** — a separate executable speaking newline-delimited JSON over
   stdin/stdout (the `plugin.json` manifest selects `"protocol": "stdio"`).

The Go SDK is the same for both. A shared-library plugin uses the `cabi`
package and is built with `-buildmode=c-shared`; a protocol plugin calls
`sdk.ServeProtocol()` and is built as a normal executable.

Shared-library example:

```go
import (
    "github.com/xiaokuang95/nulo/sdk/plugin"
    _ "github.com/xiaokuang95/nulo/sdk/plugin/cabi"
)

func main() {}

func init() {
    sdk.ServeNamed("my-plugin", "1.0.0", func(api *sdk.API) {
        api.RegisterTool(sdk.ToolDefinition{Name: "hello", Description: "say hi",
            Execute: func(ctx *sdk.Context, id string, args map[string]any) (sdk.ToolResult, error) {
                return sdk.ToolResult{Content: []sdk.ContentBlock{{Type: "text", Text: "hi"}}}, nil
            }})
        api.OnSessionStart(func(ctx *sdk.Context, event sdk.Event) (any, error) {
            return nil, ctx.API().SetSessionName("from plugin")
        })
    })
}
```

Build with `go build -buildmode=c-shared -o my-plugin.dll .` (`.so` on Linux,
`.dylib` on macOS).

Protocol example:

```go
func main() {
    if err := sdk.ServeProtocol(); err != nil { log.Fatal(err) }
}
```

with a manifest beside it:

```json
{
  "name": "my-protocol-plugin",
  "version": "1.0.0",
  "protocol": "stdio",
  "command": "./my-protocol-plugin",
  "args": [],
  "timeout": 30
}
```

Build the protocol executable normally (`go build -o my-protocol-plugin .`).
Load either mode from code with `PluginPaths`, from `settings.json`
`extensions`, `~/.nulo/agent/plugins`, `~/.nulo/agent/extensions`, or the
trusted project's `.nulo/plugins`. The loader discovers `plugin.json` manifests,
raw `.dll`/`.so`/`.dylib` files, and protocol executables declared by manifests.

`HostInfo`, `HostCall`, tool definitions, provider hooks, session/tree/compaction
events, UI dialogs, MCP registration and flags use the same JSON shapes in both
modes. See `sdk/internal/plugin/doc.go`, `sdk/plugin`, and `examples/plugin`.

### Custom subagents

The host does not define an `Agent` tool, agent types, prompts, routing, or
concurrency policy. A plugin can implement any of those on top of generic child
sessions:

```go
api.RegisterTool(sdk.ToolDefinition{
    Name: "Agent",
    Parameters: []byte(`{"type":"object","properties":{
        "prompt":{"type":"string"},
        "systemPrompt":{"type":"string"}
    },"required":["prompt"]}`),
    Execute: func(ctx *sdk.Context, id string, args map[string]any) (sdk.ToolResult, error) {
        systemPrompt, _ := args["systemPrompt"].(string)
        child, err := ctx.API().CreateSession(sdk.SessionOptions{
            SystemPrompt: systemPrompt,
            Tools:        []string{"read"},
            // NoTools: true starts with no built-in tools instead.
        })
        if err != nil {
            return sdk.ToolResult{}, err
        }
        defer child.Dispose()

        prompt, _ := args["prompt"].(string)
        result, err := child.Send(prompt)
        if err != nil {
            return sdk.ToolResult{}, err
        }
        return sdk.ToolResult{
            Content: []sdk.ContentBlock{{Type: "text", Text: result.Text}},
        }, nil
    },
})
```

`CreateSession` returns an independent `*sdk.Session`; it does not switch the
parent session. The plugin controls the agent definitions, model/prompt/tool
selection, background goroutines, result storage, completion messages and UI.
The child session API also exposes `Send`, `Prompt`, `Steer`, `Abort`, `Wait`,
`Result`, `Messages`, `Status`, `File`, `ListSessions` and `Dispose`. Child
sessions are in-memory by default; set `Persist: true` to write them to the normal session directory and
`ResumeFrom` to reopen one later. `Plugins` is off by default to avoid recursive
orchestration and duplicated plugin side effects. See
`examples/plugin-subagent` for a protocol plugin that defines its own `scout`
and `worker` types and turns them into an `Agent` tool.

Build and run the end-to-end fixture:

```sh
go test -tags plugin_integration ./sdk -run Plugin -v
```
