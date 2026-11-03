# nulo

Pi-compatible coding-agent Session SDK in Go.

Embed with `github.com/xiaokuang95/nulo/sdk`. TUI / Web / Desktop hosts import that package only.

```go
svcs, err := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
    CWD: cwd, AgentDir: sdk.DefaultAgentDir(),
})
```

Config root: `~/.nulo/agent` (`NULO_HOME` / `NULO_CODING_AGENT_DIR`).

## Native plugins

Plugins are shared libraries, not Node/TypeScript extensions. A plugin can use
any language that exports the three C symbols, or Go with `plugin/sdk`:

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
`.dylib` on macOS). Load it from code with `PluginPaths`, from
`settings.json` `extensions`, `~/.nulo/agent/plugins`, `~/.nulo/agent/extensions`,
or the trusted project's `.nulo/plugins`. The loader discovers `plugin.json`
manifests and raw `.dll`/`.so`/`.dylib` files.

`HostInfo`, `HostCall`, tool definitions, provider hooks, session/tree/compaction
events, UI dialogs, MCP registration and flags use JSON across the C ABI. The
plugin runtime never starts Node. See `plugin/doc.go`, `plugin/sdk`, and
`examples/plugin`.

Build and run the end-to-end fixture:

```sh
go test -tags plugin_integration ./sdk -run Plugin -v
```
