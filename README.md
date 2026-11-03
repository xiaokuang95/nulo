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
    "github.com/xiaokuang95/nulo/plugin/sdk"
    _ "github.com/xiaokuang95/nulo/plugin/sdk/cabi"
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
modes. See `plugin/doc.go`, `plugin/sdk`, and `examples/plugin`.

Build and run the end-to-end fixture:

```sh
go test -tags plugin_integration ./sdk -run Plugin -v
```
