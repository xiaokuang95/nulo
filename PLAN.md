# nulo SDK 地基

## Context

nulo 是 **Pi 语义的 Go Session SDK**。TUI / Web / Desktop 只消费这份 SDK，不各自写循环。

仓库现状：`github.com/xiaokuang95/nulo`（公开）、Go 1.27.3、`go.mod` 已有、无 commit、无源码。

对照：

| 材料 | 角色 |
|---|---|
| 本机 Pi `/Users/crazy/Downloads/pi` **1.4.1** | 行为 oracle |
| [PiG](https://github.com/MichaelKinsy/PiG) | Go 表达参考；**包名不跟它 1:1** |
| `/Users/crazy/Downloads/pi-go-rewrite.md` | 事件、循环、JSONL；公开面包名以本文为准 |

## 已拍板

- **自己写**，不 embed、不 copy PiG 源码。语义以 Pi 1.4.1 为准。
- **host 只 import `sdk`**。内部包名不必叫 `coding` / 不必和 Pi、PiG 同名。
- **方法名跟 PiG**：`Send` 等到 settled 并返回消息；`Prompt` 只返 error。
- **配置根 `~/.nulo/agent`**：`NULO_HOME` / `NULO_CODING_AGENT_DIR`。不读 `~/.pi`。
- **三条流协议**：`openai-completions`、`openai-responses`、`anthropic-messages`。鉴权只 API key，不做 OAuth。
- **压缩只手动**：`Session.Compact`。阈值自动压、overflow 后再 `continue` 第一期不做。
- **第一期还要**：磁盘 JSONL v3、默认工具、Skills、SystemPrompt（含 AGENTS.md）、MCP、codemode、`/tree` 离枝 LLM 摘要。

## Approach

依赖只许朝下。公开面只有 `sdk`。

```
github.com/xiaokuang95/nulo
  sdk/         唯一公开嵌入面（Services / Runtime / Session）
  ai/          消息、流、三条 API、faux
  agent/       循环、AgentTool、AgentEvent
  session/     JSONL 树、投影、手动压缩、离枝摘要
  tools/       read / bash / edit / write
  prompt/      SystemPrompt sections、AGENTS.md
  skills/      SKILL.md 发现与 /skill:name
  mcp/         MCP 客户端（stdio + streamable HTTP）
  codemode/    QuickJS-WASM 宿主 + codemode 工具
  examples/sdk/
```

`tui` / `web` / 以后的 CLI **只准 import `sdk`**（类型可从 `ai` 再导出，避免 host 摸循环）。`sdk` 可引用内部包；`ai` 不引用其它 nulo 包；`agent` 只引用 `ai`。

SDK **禁止** import 终端 UI、禁止自带 HTTP 服务。事件可 `json.Marshal`。LLM HTTP 用标准库，不引官方 SDK。

### 公开嵌入面（冻）

```go
svcs, err := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
    CWD: cwd, AgentDir: sdk.DefaultAgentDir(), // ~/.nulo/agent
})
rt, err := sdk.NewRuntime(sdk.RuntimeOptions{Services: svcs})
defer rt.Close()
model, err := sdk.BuildModel("provider/id", svcs)
sess, err := rt.New(sdk.SessionStartOptions{Model: model})
defer sess.Dispose()

unsub := sess.Subscribe(func(ev agent.AgentEvent) { /* 短回调 */ })
msgs, err := sess.Send(ctx, "hello")
err = sess.Prompt(ctx, "hello", opts)
text := sess.LastAssistantText()
res, err := sess.Compact(ctx, instructions)
_, err = sess.NavigateTree(ctx, entryID, sdk.NavigateTreeOptions{Summarize: true})
```

换会话（Subscribe 必须重绑）：`Runtime.NewSession` / `SwitchSession` / `Fork` / `ImportFromJsonl`。另提供 `CreateAgentSession(...)`。

`error == nil` 不表示模型成功。失败在助手消息的 `StopReason` / `ErrorMessage`。流式再 `Prompt` 必须带 `steer|followUp`。导出 `Steer` / `FollowUp` / `Abort` / `WaitIdle`。

### 循环（`agent`）

与 Pi `agent-loop.ts` 1:1。`StreamFn` 失败写进流，不 return error。工具 prepare 串行、execute 默认可并行、toolResult 按源顺序。`length` 全失败。前端解锁用 **`agent_settled`**。第一期循环外 **没有** 自动压缩；overflow 就是助手 error。

### JSONL（`session`，v3）

权威是树。finalized 消息立刻 append；`prepareRequest` 用投影覆盖。

- 路径：`~/.nulo/agent/sessions/--<cwd 把 / \ : 换成 ->--/<timestamp>_<id>.jsonl`
- 第一条 user/assistant 才建文件。`InMemory()` 不落盘。
- header 不是树节点。fork 带 `parentSession`。
- **写且读**：`session`、`message`、`model_change`、`thinking_level_change`、`session_info`、`compaction`、`branch_summary`、codemode 用的 `custom`（`codemode-store`）。
- **能读、第一期不生成**：`context_edit`、`usage`、其它 `custom`、`custom_message`、`label`。

### 手动压缩

`Session.Compact` 先 `Abort`。无可压 → `"Nothing to compact (session too small)"`；叶已是 compaction → `"Already compacted"`。切点、`firstKeptEntryId`、摘要前后缀、`SUMMARIZATION_SYSTEM_PROMPT`、`serializeConversation` 与 Pi 一字不差。事件 `compaction_start` / `compaction_end`。

**不做**：阈值自动压、overflow 后再 continue。设置里可先读 `compaction.enabled` 但不挂钩子。

### 离枝摘要（`NavigateTree`）

同文件改叶，不新建 jsonl。`Summarize: true` 时对「旧叶 → 公共祖先」做 LLM 摘要，写入 `branch_summary`。convertToLlm 前后缀：

```
The following is a summary of a branch that this conversation came back from:

<summary>
…summary…
</summary>
```

流式中或正在 Compact 时拒绝。无模型且要求摘要 → error。`Summarize: false` 只改叶、不写摘要。

### SystemPrompt（`prompt`）

按 Pi `buildSystemPromptSections` 做 **named sections**，第一条 system 完整写入，之后只存 patch。默认：

- `preamble`：nulo 编码助手（不写 Pi 的 docs 路径；没有随包文档就不要假装有）
- `tools` / `rules` / `cwd`
- `project_context`：发现 `AGENTS.override.md` > `AGENTS.md` > `CLAUDE.md`（agent 目录 + cwd 向上到 repo 根）
- `skills`：仅 name / description / location
- `addendum`：`Options.AppendSystemPrompt`
- `customPrompt` 替换 preamble 默认；`forceSystemPrompt` 整段不透明（无 sections）

`Options.NoContextFiles` 跳过 AGENTS。项目 `.nulo/` 资源默认要 `ProjectTrusted`（测试可直接 true）；`AGENTS.md` 仍按 Pi 默认加载。

### Skills（`skills`）

Agent Skills 规范。启动只把名片放进 prompt；全文等模型 `read`/`bash` 或 `/skill:name`。

发现（递归 `SKILL.md`）：

- `~/.nulo/agent/skills`
- `<cwd>/.nulo/skills`
- `~/.agents/skills`（用户级，不需要 project trust）
- cwd 向上的 `.agents/skills`（停在 repo 根；项目级，要 trust）

frontmatter：`name`、`description`、`disable-model-invocation`。无效字段警告不阻断。重名：先加载的赢。`/skill:name args` 把全文 + 用户参数编进 prompt，即使 `disable-model-invocation`。

### MCP（`mcp`）

自研 JSON-RPC 客户端，不用官方 MCP Go SDK。配置：

- 用户：`~/.nulo/agent/mcp.json`
- 项目：`<cwd>/.nulo/mcp.json`（要 trust；同名覆盖用户项；只有 `enabled`/`exposure` 的条目做 overlay）

传输：stdio（`command`+`args`）和 streamable HTTP（`url`+`headers`）。拒绝 `sse`。工具名 `mcp__<server>__<tool>`。`enabled: false` 不连。headers/env 支持 `${ENV}`。

第一期不做：MCP OAuth 登录流、CLI `nulo mcp add`、资源订阅 UI。Bearer 靠配置里的 env 头。

会话 `session_start` 时连接；`Dispose` 断开。工具失败变成 `isError` 消息。

### Codemode（`codemode`）

模型写 JS 调其它工具，只有脚本输出回模型。沙箱语义跟 Pi：无文件系统/网络/定时器，只有 `tools` / `text` / `image` / `console` / `store`/`load` / `ALL_TOOLS` / `searchTools` / `describeTool` / `describeNamespace`。

实现：把 Pi 同款 **QuickJS WASM** 当 asset，用 wazero 跑；**不**用 goja 冒充。`models` 全局第一期只列出 chat 模型、不能从脚本跑 classifier/image。

激活方式对齐 Pi SDK：默认 **不** 进工具表；`tools: ["+codemode"]` 或 `SetActiveTools` 打开。成功脚本的 `store` 落 `custom` 条目 `codemode-store`，resume 按枝恢复。

嵌套 `executeTool` 走同一套 before/after 钩子（没有扩展宿主也要能调 read/bash/mcp）。

### 三条 API（`ai`）

| `model.api` | 协议 |
|---|---|
| `openai-completions` | Chat Completions SSE |
| `openai-responses` | Responses SSE |
| `anthropic-messages` | Messages SSE |
| `faux` | 测试队列 |

`BuildModel("provider/id")` 读 `models.json`。凭证 `auth.json` 否则 `OPENAI_API_KEY` / `ANTHROPIC_API_KEY`。无 key → `stopReason=error`，函数 error 仍 nil。不做 OAuth、官方 SDK、其它云厂商 API。

### 工具（`tools`）

默认 `read` / `bash` / `edit` / `write`。`noTools` / `tools` / `excludeTools` / `customTools` 与 Pi SDK 同语义。`grep`/`find`/`ls`/`powershell` 第一期不做。同文件 mutation 队列。

### 配置

| | nulo |
|---|---|
| 根 | `$NULO_HOME` 否则 `~/.nulo` |
| agent | `$NULO_CODING_AGENT_DIR` 否则 `<根>/agent` |
| 会话 | `$NULO_CODING_AGENT_SESSION_DIR` 否则 `<agent>/sessions` |
| 项目配置目录 | `<cwd>/.nulo/` |
| auth / models / mcp | `<agent>/auth.json`、`models.json`、`mcp.json` |

测试一律传入临时 `AgentDir`。

### 明确砍掉（第一期）

TUI、CLI、RPC、HTTP 服务、TS/Node 扩展、Durable/Chord、virtual model、cache warming、OAuth（provider 与 MCP）、项目 trust **UI**（只留 Options 布尔）、自动压缩、classifier/image 模型、prompt templates、themes。

## Files to modify / 新建

```
LICENSE NOTICE README.md
sdk/*.go
ai/{types,stream,faux,openai_completions,openai_responses,anthropic_messages,overflow}.go
agent/{types,agent,loop}.go
session/{manager,compaction,branch_summary,messages}.go
tools/{read,bash,edit,write,queue}.go
prompt/{prompt,context_files}.go
skills/skills.go
mcp/{client,stdio,http,jsonrpc}.go
codemode/{sandbox,tool,store}.go
examples/sdk/main.go
```

测试与实现同包。MCP / 三条 API 用 `httptest` 或假 stdio；codemode 用内置脚本调 `read`；压缩和离枝摘要用 faux。

## Reuse（对照，不 copy 文件）

- Pi `packages/ai` 三条 API + faux + overflow
- Pi `packages/agent` 循环
- Pi `packages/coding-agent/src/core/{sdk,agent-session,session-manager,system-prompt,skills,messages}.ts`
- Pi `packages/coding-agent/src/core/compaction/{compaction,branch-summarization,utils}.ts`
- Pi `packages/mcp` 协议与 stdio / streamable-http（自研，不用官方 SDK）
- Pi `packages/codemode` 沙箱合同 + `extensions/codemode` 工具面；WASM 二进制可作 asset
- Pi `docs/{session-format,skills,mcp,codemode}.md`
- PiG 仅调用顺序：Services → Runtime → New → Send/Prompt/Subscribe

## Steps

每片有测试再进下一片。

- [x] **1. 骨架**：MIT LICENSE、NOTICE、各包 `doc.go`、import 方向测试（host 路径只允许 `sdk`）
- [x] **2. `ai` + faux**
- [x] **3. `agent` 循环**（无工具 / 有工具 / steer / abort / length）
- [x] **4. `session` JSONL**（内存+磁盘、resume、fork）
- [x] **5. `sdk` 工厂**：Subscribe / Send / Prompt / Abort / Dispose；投影进 prepareRequest
- [x] **6. `tools` 默认四件套**
- [x] **7. 三条 API** + auth/models + BuildModel
- [x] **8. `prompt` + `skills`**：sections、AGENTS.md、名片进 prompt、`/skill:name`
- [x] **9. 手动 Compact**
- [x] **10. `NavigateTree` + 离枝摘要**
- [x] **11. MCP**：mcp.json、stdio、HTTP、工具注册
- [x] **12. codemode**：WASM 沙箱、store 落盘、`+codemode` 激活
- [x] **13. Runtime 换会话** + `examples/sdk`

## Verification

- `go test ./...`
- faux `Send`：Start → … → Settled；`LastAssistantText` 有值
- 缺凭证：`err==nil` 且 `StopReason=error`
- JSONL 重启后投影一致；fork header 有 `parentSession`
- Compact：投影以摘要 user 打头；**不**在超阈值时自动出现 compaction 条目
- NavigateTree+Summarize：写出 `branch_summary`，换叶后投影含离枝摘要 user
- Skills：目录里的 SKILL.md 出现在 system `skills` 节；`/skill:name` 把全文送进本轮
- AGENTS.md 出现在 `project_context`
- MCP httptest：列出的工具可被循环调用
- codemode：脚本 `return await tools.read({path})` 的结果到助手，不把中间过程当模型可见工具表（未 +codemode 时无此工具）
- `go list`：`ai` 不 import 其它 nulo 包；除 `sdk` 与测试外没有包 import `sdk`
- 不验收：TUI、真网手工以外、OAuth、TS 扩展、自动压缩、与 `~/.pi` 互通

## 风险

- JSONL 投影、压缩切点、离枝公共祖先是最容易写错的三块。权威在 `session`，禁止循环另存一份会分叉的 messages。
- Codemode 绑 WASM：要钉 Pi 的 wasm 文件和宿主协议；沙箱逃逸面等于 Pi。goja 看起来更省，但语义对不上，不要用。
- MCP 进程/HTTP 是信任边界。项目 `mcp.json` 无 trust 不加载。没有 OAuth 时，只能打公开或 header 已配好的服务器。
- 三条 SSE 第一期只保证：文本流、一轮 tool call、错误进消息。
- 手动压缩测试用 faux 脚本化摘要，不要依赖真网。
- Pi 1.4.1 vs PiG 1.4.0：行为跟 Pi，方法名跟 PiG，包名跟本文。
