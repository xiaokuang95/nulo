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
  codemode/    moejs 宿主 + codemode 工具
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
err = sess.SetName("...")
sess.SetModel(model)
tree := sess.Tree()
infos, err := rt.ListSessions()
```

换会话（Subscribe 必须重绑）：`Runtime.NewSession` / `SwitchSession` / `ResumeSession` / `Fork` / `ImportFromJsonl`；`ListSessions` / `RenameSession` / `DeleteSession` 管存储。另提供 `CreateAgentSession(...)`。

`error == nil` 不表示模型成功。失败在助手消息的 `StopReason` / `ErrorMessage`。流式再 `Prompt` 必须带 `steer|followUp`。导出 `Steer` / `FollowUp` / `Abort` / `WaitIdle`。

### 循环（`agent`）

与 Pi `agent-loop.ts` 1:1。`StreamFn` 失败写进流，不 return error。工具 prepare 串行、execute 默认可并行、toolResult 按源顺序。`length` 全失败。前端解锁用 **`agent_settled`**。第一期循环外 **没有** 自动压缩；overflow 就是助手 error。

### JSONL（`session`，v3）

权威是树。finalized 消息立刻 append；`prepareRequest` 用投影覆盖。

- 路径：`~/.nulo/agent/sessions/--<cwd 把 / \ : 换成 ->--/<timestamp>_<uuidv7>.jsonl`；cwd 先解析成绝对路径，和 Pi 的 `encodeCwdForSessionDir` 一致。
- 第一条 user/assistant 才建文件。`InMemory()` 不落盘。header 不是树节点；fork 带绝对 `parentSession`。
- **写且读**：`session`、`message`（含 `bashExecution`）、`model_change`、`thinking_level_change`、`session_info`、`compaction`（`details` / `usage` / `fromHook` / `systemMessage`）、`branch_summary`（`details` / `usage` / `fromHook`）、`custom`、`custom_message`、`label`、`usage`、`context_edit`。助手消息自身的 `usage` 随 `message` 落盘，cache warming 记独立 `usage`（`kind=cache_warm`）。
- **打开语义同 Pi**：坏 JSON 行跳过、行无长度上限、缺尾换行补 `\n`；v1 补 ID/parent 链并把 `firstKeptEntryIndex` 换成 `firstKeptEntryId`，v2 把 `hookMessage` 改名 `custom`，迁移后写回 v3。常规打开不会重写未知 entry。
- **投影**：最新 compaction 的 `firstKeptEntryId` 决定保留范围；`context_edit` 替换/删除目标内容；`custom_message` 以 `custom` 角色进上下文，`bashExecution` 渲染成 `Ran ...` 文本（`excludeFromContext` 跳过）。
- **Session/Runtime**：`Name` / `SetName`、`SetLabel` / `Label`、`Tree`、`SetModel`；新会话写 `model_change`（显式 thinking 档位再写 `thinking_level_change`）；`ResumeSession` 从文件恢复 model/thinking，`SwitchSession` 沿用当前设置；`ListSessions` / `RenameSession` / `DeleteSession`；`ImportFromJsonl` 先复制到 session 目录（重名加 `-N`），不覆盖；`Fork` 只允许已落盘会话。

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

Agent Skills 规范，**逐条对齐 Pi 的 `skills.ts` + `DefaultResourceLoader`**。启动只把名片放进 prompt；全文等模型 `read`/`bash` 或 `/skill:name`。

发现顺序（即优先级，先加载的重名赢）：

1. `<cwd>/.nulo/skills`（项目，要 trust）
2. cwd 向上的 `.agents/skills`（项目，要 trust；停在 repo 根且包含 repo 根，无层数上限）
3. `<agentDir>/skills`（用户，不需要 trust）
4. `~/.agents/skills`（用户，不需要 trust）
5. 显式 `skillPaths`

注意顺序是**项目优先于用户**——Pi 的 `resourcePrecedenceRank` 把 project 排在 user 前面，原来的实现是反的。

递归规则：目录里有 `SKILL.md` 就是技能根，**不再往下走**（`references/`、`scripts/` 不会被当成技能扫）；否则递归子目录找 `SKILL.md`，并且**只有根目录**的 `.md` 子女作为独立技能。跳过点开头的条目和 `node_modules`，跟随符号链接，尊重 `.gitignore` / `.ignore` / `.fdignore`。

frontmatter 按 Pi 的方式提取：CRLF 归一、去 BOM、闭合围栏是 offset 3 之后第一个 `\n---`、YAML 从 offset 4 开始、body 两端 trim。YAML 取子集（纯量、单双引号、`|`/`>` 块、注释、bool/number/null），因为 nulo 不消费任何嵌套字段。

校验语义（这是和旧实现差最大的地方）：`name` **不转小写**、缺失时回落目录名；名字非法（字符、64 长度、首尾/连续连字符）**只告警，照样加载**；`description` 缺失或空白才**不加载**；超过 1024 字符告警。`disable-model-invocation` 必须是布尔 `true`（YAML 1.2 里 `yes` 是字符串，所以不生效）。重名保留先加载的，另一个记为 `collision` 诊断（带 winner/loser 路径）；同一文件经符号链接被扫到两次不算碰撞。

名片是 XML：`name` / `description` / `location`，五个字符转义，`disable-model-invocation` 的不出现。**只有存在 `read` 或 `bash` 工具时才输出这一节**，并点名该工具（`indirect` 表示只能经由别的工具读到）。

`/skill:name args` 展开成 `<skill name="…" location="…">` 块 + 一行 `References are relative to <baseDir>.` + 正文（剥 frontmatter），参数接在后面。名字**精确匹配**，未知命令原样穿过。被 `disable-model-invocation` 禁用的技能仍然可以手动调用。

诊断通过 `Session.SkillDiagnostics()` 暴露（Pi 在启动时打印，nulo 没有启动输出通道，交给宿主）。

未做：Pi 的 `settings.skills` 覆盖项与 `--skill` 过滤、`/reload`、`enableSkillCommands`（TUI 命令发现）。

### MCP（`mcp`）

自研 JSON-RPC 客户端，不用官方 MCP Go SDK。配置：

- 用户：`~/.nulo/agent/mcp.json`
- 项目：`<cwd>/.nulo/mcp.json`（要 trust）

加载语义对齐 Pi 的 `validateMcpServerConfig` / `readConfigFile`：

- **每个条目单独校验，坏条目只记错误并跳过**，其余照常加载（原来任一错误会让整个 MCP 失败）。错误经 `Session.MCPDiagnostics()` 暴露。
- 服务器名 `^[A-Za-z0-9_-]+$`；两个名字规范化后同 namespace（`a-b` 与 `a_b`）算冲突，后一个报错。
- 传输：`url`（`http` / `streamable-http`）或 `command`（`stdio`）；`sse` 明确拒绝；`url` 必须 http(s)；`timeout` 是正数秒。文件 BOM 会剥掉。
- 项目条目若没有 `command` / `url` / `type`，就是同名全局服务器的 overlay，**只能**设 `enabled` / `exposure` / `toolExposure`；多一个字段报错，找不到基座也报错。凭证（`env` / `headers`）不会从项目文件覆盖。

**工具暴露**：服务器级 `exposure`（默认 `codemode`）+ 工具级 `toolExposure`（精确名优先，其次 `*` 通配；nulo 对通配按名字排序而不是 JSON 声明顺序，只在一条工具同时命中两个通配时有差别）。

| 值 | 行为 |
|---|---|
| `codemode`（默认） | 只进 catalog，脚本可调，**不声明给模型** |
| `deferred` | 只进 catalog，等 `tool_search` 加载后**直接调用** |
| `direct` | 像内置工具一样声明，脚本也能调 |
| `hidden` | 哪儿都不给 |

`codemode-deferred` 是 `codemode` 的旧别名；`deferred` 现在是真的另一种暴露，不再降级。

`deferred` 走新加的 `tool_search` 工具（BM25，k1=1.2 / b=0.75，默认 8 条），命中的工具写进激活集，由 agent loop 的 `AfterToolBatch` 钩子在下一次模型调用前生效，并按 `ToolsAdded` 记进 transcript，`/tree` 恢复后仍在。配置里有 `codemode` 时自动开 codemode（`autoEnableCodemode: false` 可关）。`mcp_servers` 提示词节列出所有含 codemode/deferred 的服务器，`Reach` 标 `codemode` 或 `tool_search`。

**工具名**：`mcp__<server>__<tool>`，`[^A-Za-z0-9_]` → `_`；超过 64 字符、或规范化后撞名时，加 8 位 sha256 后缀。

**结果**：文本块逐块保留（正常路径不合并），图片以 image block 透传，`resource_link` 变成一行可读文本，`isError` 变成工具错误标志。文本合计超 20KB 时中间截断，全文落临时文件并在结果里给出路径。

第一期不做：MCP OAuth 登录流、`list_mcp_resources` / `read_mcp_resource` 等资源工具、CLI `nulo mcp add`、资源订阅 UI。Bearer 靠配置里的 env 头。

会话 `session_start` 时连接；`Dispose` 断开。工具失败变成 `isError` 消息。

### Codemode（`codemode`）

模型写 JS 调其它工具，只有脚本输出回模型。沙箱语义跟 Pi：无文件系统/网络/定时器，只有 `tools` / `text` / `image` / `console` / `store`/`load` / `ALL_TOOLS` / `searchTools` / `describeTool` / `describeNamespace`。

实现：纯 Go 的 **moejs**（`github.com/MoeForge/moejs`）当引擎，和 Pi 的 QuickJS-WASM 语义对齐，但不是 goja 冒充。每次 Run 新建 Runtime，prelude 用 `CompileScript` 编译一次复用；`models` 全局第一期只列出 chat 模型、不能从脚本跑 classifier/image。

性能与约束：一次脚本约 0.1ms（旧实现每次 Run 重新编译 623KB QuickJS WASM，约 370ms）。超时走 `Runtime.Interrupt`（另一 goroutine 可中断死循环），内存走 `Options.MemoryLimit`（默认 256MiB），调用方取消会中断脚本。

激活方式对齐 Pi SDK：默认 **不** 进工具表；`tools: ["+codemode"]` 或 `SetActiveTools` 打开。成功脚本的 `store` 落 `custom` 条目 `codemode-store`，resume 按枝恢复。

嵌套 `executeTool` 走同一套 before/after 钩子（没有扩展宿主也要能调 read/bash/mcp）。

### 三条 API（`ai`）

| `model.api` | 协议 |
|---|---|
| `openai-completions` | Chat Completions SSE |
| `openai-responses` | Responses SSE |
| `anthropic-messages` | Messages SSE |
| `faux` | 测试队列 |

`BuildModel("provider/id")` 读 `models.json`;裸 id 按 Pi 当作 `openai/<id>`。凭证顺序与 Pi 一致:请求级 key → `auth.json`(API key 或 OAuth token,过期自动刷新)→ `models.json` 的 `apiKey` → provider 自己的环境变量(未知 provider 不借用 `OPENAI_API_KEY`)。`apiKey` 与 header 的 `$ENV` / `!command` 在每次请求时解析,命令失败报在请求上而不是加载时。无 key → `stopReason=error`,函数 error 仍 nil。不做交互式 OAuth 登录(`auth.json` 里的 OAuth 凭证可直接用并刷新)、官方 SDK、其它云厂商 API。

用量与成本三条 API 都解析,填 `Message.Usage`(input / output / cacheRead / cacheWrite / reasoning / totalTokens),再按 `models.json` 的 `cost` 单价算出 `Usage.Cost`。`openai-completions` 请求带 `stream_options.include_usage`,否则流式响应根本不含用量。缓存读取兼容 `prompt_tokens_details.cached_tokens`(OpenAI)、`prompt_cache_hit_tokens`(DeepSeek)、顶层 `cached_tokens`(Kimi)三种拼写,并从 prompt 计数中扣除。`cost.tiers` 按 Pi:整请求 prompt(input + cacheRead + cacheWrite)超过阈值的最高档替换四个基础单价。

### 工具（`tools`）

默认 `read` / `bash` / `edit` / `write`;**Windows 上以 `powershell` 取代 `bash`**,即 `read` / `powershell` / `edit` / `write`,因为 Windows 不自带 POSIX shell。`powershell` 语义对齐 Pi:仅 Windows 可用、`-NoProfile -NonInteractive -ExecutionPolicy Bypass -Command`、每轮前置 UTF-8 编码前缀、**tail 截断**(留最后 2000 行 / 50KB,全文落临时文件)、超时 `taskkill /F /T` 杀进程树。`noTools` / `tools` / `excludeTools` / `customTools` 与 Pi SDK 同语义。`grep`/`find`/`ls` 第一期不做。同文件 mutation 队列。

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

TUI、CLI、RPC、HTTP 服务、TS/Node 扩展（改用 native plugin ABI + stdio 协议）、Durable/Chord、virtual model、cache warming、OAuth（provider 与 MCP）、项目 trust **UI**（只留 Options 布尔）、自动压缩、classifier/image 模型、prompt templates、themes。

## Files to modify / 新建

```
LICENSE NOTICE README.md
sdk/*.go
plugin/{doc,host,protocol,discover,protocol_host,library,library_windows,library_unix,unsafe}.go
plugin/sdk/{api,context,server,protocol,types,hostcall,doc}.go
plugin/sdk/cabi/cabi.go
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

## 补记(计划定稿后的增补)

计划定稿后才补的三块,记在这里而不是改上面几节——上面的拍板记录保持原样,免得看不清哪条是当初的决定、哪条是后来改的。

- **`powershell` 工具**:「工具」节原写「第一期不做」,后来做了。理由是 nulo 要在 Windows 上真能用,而 Windows 不自带 POSIX shell。落点:`tools/shell.go`(与 Pi 同构的共享 shell 内核)+ `tools/powershell.go`(平台解析与启动参数)。
- **用量与成本**:JSONL 节原写 `usage`「第一期不生成」,后来发现**内存里也始终是零**——三条 API 都没解析流里的 usage,任何 host 拿到的用量与成本都是假的。现在 `ai/usage.go` 补齐。
- **`$NULO_CODING_AGENT_SESSION_DIR` 修复**:「配置」表里列了这个覆盖项,但 `runtime` 从未使用它,是死代码。现由 `DefaultSessionDir` + `session.SessionDirIn` 组合生效。
- **`models.json` 改为 Pi 的 provider 层**:原格式是扁平 `models` 数组、每个模型自带 `provider` / `baseUrl`(没有 provider 层)。现改为 `providers` 映射,provider 级的 `baseUrl` / `api` / `apiKey` / `headers` / `compat` 由模型继承,模型只写差异;`apiKey` 与 header 值支持 `$ENV` / `${ENV}` / `!command` / `$$` `$!` 转义;`compat` 落地了 nulo 会消费的 6 个字段(`maxTokensField` / `supportsStore` / `supportsDeveloperRole` / `supportsUsageInStreaming` / `requiresToolResultName` / `supportsMaxOutputTokens`)。**旧扁平格式不再兼容**,加载时直接报错并附迁移样例。`authHeader` 已支持(把 key 额外发成 `Authorization: Bearer`);`thinkingLevelMap` / `thinkingFormat` / `supportsReasoningEffort` / `samplingParams` / `samplingParamsByThinkingLevel` / `inputLimits` / `promptCache` / `modelOverrides` / `oauth` 见下一条补记。
- **MCP 工具暴露**:原来 MCP 工具无差别地直接进激活工具表(等于全 `direct`)。现按 Pi 的 exposure 分流,并把 codemode 沙箱的 `describeNamespace` / `searchTools({namespace})` / 工具描述都补齐;顺带修了 `wrapScript` 把工具描述丢掉——之前 `ALL_TOOLS` 里每个工具的 description 就是它自己的名字,按描述搜索永远无效。
- **MCP 对齐 Pi**:之前把 `deferred` 降级成 `codemode`、缺 `toolExposure`、任一配置错误会让整个 MCP 失败、项目 overlay 允许改 `description`、工具名不限长也不处理撞名、结果只取 text 且不截断。现按 `dist/core/mcp-servers.js` / `dist/extensions/mcp/{config,tools,index}.js` 补齐，新增 `tool_search`（BM25）与 agent loop 的 `AfterToolBatch` 钩子，让 deferred 工具在下一轮生效并记进 transcript。
- **skills 改为对齐 Pi**:旧实现有 6 处实质偏差——发现顺序项目/用户反了、没有 SKILL.md 短路递归、不支持根目录独立 .md、名字被强制转小写且非法名直接丢弃、`disable-model-invocation: yes` 被误当 true、`/skill:` 展开没有 `<skill>` 块也没有 baseDir。现按 `dist/core/skills.ts` 重写,补了 YAML 子集解析(`frontmatter.go`)和 gitignore 子集(`ignore.go`),并暴露 `Session.SkillDiagnostics()`。
- **修掉 skills frontmatter 的引号 bug**:`v = TrimSpace(Trim(v, `"'`))` 先去引号再去空格,而空格让引号不在边缘,于是惯用的 `name: "x"`(冒号后有空格)得到 `"x`,过不了名字校验,技能静默丢失。改为先 TrimSpace 再 Trim。另补了 `sdk/internal/skills` 的直接测试(之前一个都没有)。

- **`models.json` 对齐 Pi 的解析与认证语义**:之前有 6 处实质偏差——(1) apiKey 优先级是 models.json > auth.json,Pi 是 auth.json > models.json;(2) 未知 provider 回落到 `OPENAI_API_KEY`,等于把 OpenAI 的 key 发给任意网关,Pi 只认 provider 自己的环境变量;(3) `$ENV` 缺失时逐变量替换为空串,Pi 视为整个值 undefined,请求报 "Failed to resolve ... from environment variable";(4) `!command` 在建模时跑一次,Pi 每次请求都跑;(5) header 合并大小写不同就两条都发,且 `null` 无法删除继承的 header;(6) `models.json` 不支持 `//` 注释与尾逗号。现按 Pi 全部修齐,并补了 `authHeader`、`cost.tiers` 分档定价、`input` 默认 `["text"]`、同 provider 重复 model id 后者覆盖、裸 id 默认 `openai`、错误文案 `model not found: provider/id`。
- **`models.json` 全字段落地**:`thinkingLevelMap` + `thinkingFormat` + `supportsReasoningEffort` / `thinkingTokenBudgetField` 三条 API 真下发(OpenAI 兼容侧覆盖 Pi 的 11 种格式:openai / openrouter / deepseek / zai / qwen / qwen-chat-template / chat-template / baseten / together / ant-ling / string-thinking;Anthropic 走 budget 或 `forceAdaptiveThinking` 的 adaptive+effort);`samplingParams` / `samplingParamsByThinkingLevel` 按「模型 → 档位 → 请求」合入 OpenAI 兼容请求体;`inputLimits.images.resize` 在请求前把 PNG/JPEG 缩到 maxWidth/maxHeight/maxBytes(`maxPer*` / `maxRequestBytes` 只解析,Pi 也还没据此拒绝历史);`modelOverrides` 按字段合并(nulo 没有内建目录,作用于同 provider 的模型);`promptCache` 驱动缓存保温(streaming / idle 两模式,按 Pi 的 $0.05 期望收益门槛,重放请求时 max_tokens=1);`oauth` 支持 `auth.json` 的 OAuth 凭证与 Radius `refresh_token` 换发并回写(交互式浏览器/设备码登录仍未做)。Session 新增 `ThinkingLevel` 启动项、`SetThinkingLevel` / `ThinkingLevel` / `CacheWarming` 模式,档位随 `thinking_level_change` 持久化并在恢复时还原。
- **sessions 对齐 Pi/PiG**:之前只覆盖自己会写的 7 种 entry，且 session ID 是 `nulo_` 前缀、坏行直接报错、没有 v1/v2 迁移、没有 label/usage/context_edit/custom_message、resume 不恢复 model、import 不复制、缺失路径不能当新会话打开。现按 Pi 的 `session-manager.ts` / PiG `codingagent.Session` 对齐：UUIDv7 session ID、8 位十六进制 entry ID（冲突 100 次后 UUIDv4）、绝对 cwd、全部 entry 类型与字段、Pi 的打开/迁移/跳过坏行语义、名字/标签/usage/context edit、模型恢复、session 列表/重命名/删除、import 复制与 fork 未落盘报错，并让 JSONL 不再 HTML 转义（`<`/`>`/`&` 保持字面，和 `JSON.stringify` 一致）。
验证:`tools/powershell_test.go` 在 Windows 上真跑(含 UTF-8 输出、退出码、空输出、无效超时、超时杀进程树、tail 截断);`ai/usage_test.go` 用 httptest 覆盖三条 API 与三种缓存命名拼写;`sdk/runtime_test.go` 的 `TestSessionDirEnvOverride` 断言会话文件落在 env 指定的根下、且 agent 目录里不再出现 `sessions/`;`ai/models_json_test.go` 覆盖字段继承与覆盖、compat 合并与默认值、插值四形态与转义、旧格式报错、apiKey 不进 JSON、以及三条 API 的 compat 真的改变了请求体;`ai/models_parity_test.go` 覆盖 header null 删除与大小写合并、apiKey 优先级、逐请求解析、`!command` 失败推迟到请求、`authHeader`、`cost.tiers`;`ai/models_fields_test.go` 覆盖新字段解析与 override 合并、thinking 格式与 `reasoning_effort` 门控、sampling 顺序、图片缩放、promptCache 时序、OAuth 刷新与回写;`sdk/session_models_test.go` 覆盖 thinking 档位持久化/恢复、保温收益判定与 idle 模式真实重放;`session/parity_test.go` / `sdk/session_parity_test.go` 覆盖 entry 投影、迁移、坏行、label/usage/context_edit、session 列表、import/fork、resume 恢复 model/thinking。

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
