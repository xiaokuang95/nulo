# nulo

Pi-compatible coding-agent Session SDK in Go.

Embed with `github.com/xiaokuang95/nulo/sdk`. TUI / Web / Desktop hosts import that package only.

```go
svcs, err := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
    CWD: cwd, AgentDir: sdk.DefaultAgentDir(),
})
```

Config root: `~/.nulo/agent` (`NULO_HOME` / `NULO_CODING_AGENT_DIR`).

## Plugins

Plugins have two host modes, neither of which starts Node:

1. **Shared library** ¡ª an in-process `.dll`/`.so`/`.dylib` exporting the C ABI.
2. **Protocol** ¡ª a separate executable speaking newline-delimited JSON over
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
