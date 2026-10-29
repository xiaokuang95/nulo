# nulo

Pi-compatible coding-agent Session SDK in Go.

Embed with `github.com/xiaokuang95/nulo/sdk`. TUI / Web / Desktop hosts import that package only.

```go
svcs, err := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
    CWD: cwd, AgentDir: sdk.DefaultAgentDir(),
})
```

Config root: `~/.nulo/agent` (`NULO_HOME` / `NULO_CODING_AGENT_DIR`).
