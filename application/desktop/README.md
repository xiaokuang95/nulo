# nulo desktop · native shell

`ui/design/desktop/` 的 MyGo 原生实现，并接入仓库里的
`github.com/xiaokuang95/nulo/sdk`。界面沿用原型的 ink + moss 语言，渲染、
布局、弹层、菜单和文件选择都由 MyGo 原生 UI 完成，不嵌浏览器。

## 运行

```sh
cd application/desktop
go tool mygo build
open dist/darwin-amd64/nulo.app
```

`go run .` 每次都会先编译 Go/MyGo 依赖，首次或改代码后会明显变慢；慢的是
编译，不是窗口本身。开发时也可以：

```sh
go build -o /tmp/nulo-dev .
/tmp/nulo-dev
```

工作区默认取 `git rev-parse --show-toplevel`，否则取当前目录；启动后会记住
最后一次打开的项目。需要覆盖时：

```sh
NULO_WORKSPACE=/path/to/repo /tmp/nulo-dev
```

`NULO_HOME`、`NULO_CODING_AGENT_DIR`、`NULO_CODING_AGENT_SESSION_DIR`
沿用 SDK 的约定。模型和供应商读取
`~/.nulo/agent/models.json`；默认模型、思考等级读取
`~/.nulo/agent/settings.json`。

检查：

```sh
go test ./...
go tool mygo vet
```

测试里的模型服务器是本地 `httptest`，不会访问外网。截图回归仍使用
`model.go` 里的固定数据，不依赖 SDK：

```sh
NULO_SCREENSHOT=/tmp/nulo.png NULO_STATE=paper \
  go test -run TestNativeScreenshot
```

可用状态：`appearance`、`settings`、`paper`、`new`、`plan`、`terminal`、
`context`、`approved`、`model`、`palette`、`rename`、`attachments`、
`slash`、`mention`、`commit`、`collapsed`、`collapsedgroup`、`projecthover`、
`projectnew`、`mid`、`wide`、
`threshold`、`narrow`、`narrowdock`、`narrowside`、`compact`。

## SDK 接入

- **会话**：启动时用 `sdk.ListAllSessions` 恢复所有工作区的会话；切换到历史
  会话时用 `Runtime.ResumeSession` 懒加载，首次打开新项目时创建
  `AgentSessionServices`。
- **发送**：`Session.Prompt` 在后台执行，`Session.Subscribe` 的事件投影成
  thinking、正文、工具卡、审批卡、变更摘要和状态栏。停止调用
  `Session.Abort`。
- **审批**：`SessionStartOptions.ApproveTool` 把工具调用交给桌面权限模式。
  Manual/Plan 会等待 Allow once / Always allow / Deny；Auto edits 直接放行
  read/write/edit，Auto 放行全部。
- **上下文**：`Session.Messages`、模型的 `ContextWindow` 和最近一轮 usage
  生成用量条；`/compact` 调用 `Session.Compact`。
- **分支**：fork 使用 `Runtime.Fork`，回退使用 `Session.NavigateTree`。
- **重命名/删除**：写回 SDK 的 session JSONL，不只是改内存。
- **变更审核**：每次写类工具结束后读取 `git status` / `git diff`；Stage all、
  Commit 执行真实 git 命令。
- **终端**：右侧 terminal 镜像 `bash` / `powershell` 工具的命令与输出，不是
  可输入的交互式 pty。
- **附件**：文件选择器拿到的路径会随 prompt 发给模型；当前还不是图片 block
  上传。Worktree 环境选择显示为未接入。

## 界面

- **Titlebar**：品牌、`workspace / branch`、侧栏与 dock 开关。
- **Sidebar**：New session、搜索、All / Active / Review、可折叠的项目分组、
  分组悬停后的 `+` 在该项目下新建会话、fork 子节点、Capabilities 与 Settings。
  顶部 New session 沿用当前会话所在项目；⌘O 打开新的项目目录。
- **Thread**：会话状态、fork 血统、用户消息、thinking、工具卡、审批卡、
  变更摘要与 composer。
- **Dock**：Changes / Plan / Terminal / Context，可拖拽调宽，窄窗口下变为抽屉。
  SDK 没有结构化 plan 时 Plan 面板会明确显示空状态。
- **Overlays**：命令面板、设置、模型选择、权限选择、环境选择、slash 菜单、
  @ 文件菜单、重命名、commit 对话框与 toast。

响应式断点和原型一致：窗口窄于 1180 时 dock 收为右侧抽屉，窄于 900 时
侧栏收为左侧抽屉。

## 可交互

- **审批**：Allow once / Always allow / Deny 决定真实工具是否执行，并同步会话
  状态、terminal、后续消息和 toast。
- **发送**：Enter 发送，Shift+Enter 换行；附件路径随用户消息展示。运行中的
  会话可以停止。
- **Slash**：输入 `/` 打开命令面板，支持过滤、上下选择、Tab 补全和 Esc 关闭。
  已实现 `/plan`、`/diff`、`/context`、`/compact`、`/approve`、`/clear`。
- **@ 文件**：输入 `@` 过滤并插入文件路径，支持键盘选择。
- **会话**：新建、切换、搜索、筛选、fork、重命名、复制 ID、删除、回退到上一个
  checkpoint；右侧 `…` 菜单和会话行右键都可以打开会话操作。
- **Changes**：逐文件 review、Mark all reviewed、Prev / Next、复制 patch、
  Stage all、Commit。
- **Context**：token 明细、已加载文件/skill、Compact。
- **模型 / 权限 / 环境**：composer 上的 chip 打开原生 popover；设置里的模型
  选择会同时更新当前会话和默认模型。
- **附件**：拖拽文件到 composer，或点 paperclip 打开系统文件选择器。
- **设置**：General、Providers & models、Capabilities、Appearance、
  Shortcuts、About；Moss / Paper 主题即时切换。
- **命令面板**：⌘K 搜索会话和动作，支持键盘执行。

## 快捷键

| 键 | 行为 |
|---|---|
| `⌘K` | 命令面板 |
| `⌘N` | 新会话 |
| `⌘O` | 打开项目目录 |
| `⌘B` | 侧栏 |
| `⌘J` | 右侧 dock |
| `⌘,` | 设置 |
| `⌘F` | 聚焦会话搜索 |
| `⇧⌘D` | 打开 Changes |
| `⇧⌘P` | 打开 Plan |
| `Shift+Tab` | 循环权限模式 |
| `Esc` | 关闭弹层，或停止运行中的会话 |

## 文件

```text
application/desktop/
├── main.go          # MyGo window 入口
├── host.go          # SDK host、项目/服务索引、会话创建/恢复
├── host_events.go   # SDK 事件投影、usage、terminal
├── host_actions.go  # 模型/thinking、compact、fork、rewind、rename、delete
├── app.go           # 应用状态、发送、审批、会话操作、命令面板
├── view.go          # 三栏布局、titlebar、sidebar、窄屏抽屉
├── thread.go        # 消息、工具卡、审批卡、变更摘要
├── composer.go      # composer、slash、@、附件、popover
├── dock.go          # Changes / Plan / Terminal / Context
├── git.go           # git status/diff、stage、commit
├── overlays.go      # 命令面板、设置、重命名、commit
├── model.go         # 数据结构和截图 fixture
├── theme.go         # Moss / Paper tokens
├── widgets.go       # 通用按钮、chip、进度条、checkbox
├── icons.go         # 代码绘制的 SVG 图标
├── host_test.go     # 真实 SDK host 的流式/审批/恢复/fork/回退测试
├── git_test.go      # git 变更读取测试
└── visual_test.go   # 无头截图回归（fixture）
```

## 边界

主应用已经是真实 SDK host：会启动运行时、执行工具、读写会话 JSONL、读取
git 变更并持久化项目选择。截图回归和 `newFixtureApp` 仍刻意使用内存 fixture，
只用于稳定的视觉对比。

尚未接入的部分：交互式 pty、worktree 创建、图片作为 image block 上传、在
桌面 UI 里编辑供应商 API key，以及 SDK 未提供的结构化 plan 事件。
