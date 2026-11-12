# nulo desktop — 交互原型

这是 `nulo` 的桌面端设计原型，放在 `ui/design/desktop/`，和 TUI 原型
`ui/design/index.html` 平行。目标是先把桌面 host 的信息层级、审批流和评审流做对，
再接到 Go SDK 上。

纯 HTML / CSS / JavaScript，无构建步骤、无外部字体或图片依赖。

## 打开

直接打开 `index.html`，或：

```sh
cd ui/design/desktop
python -m http.server 8765
# http://127.0.0.1:8765
```

建议 1440 × 900 以上。窗口缩到 1280 宽时三栏仍然成立；窄于 1180 时右侧 dock
变成抽屉，窄于 900 时侧栏变成抽屉。

## 参考了什么

| 产品 | 借用的模式 |
|---|---|
| Codex app | 项目/线程侧栏、三栏工作台、右侧 review dock、权限与环境选择、并行会话 |
| Claude Code desktop | 会话状态与筛选、权限模式、内建终端、可视化 diff、逐文件 review |
| OpenCode desktop | 项目树、provider/model 层级、命令面板、主题、终端感较强的排版 |

没有照搬品牌色或组件，视觉仍沿用 TUI 原型的 **ink + moss** 语言：近黑绿背景、
单一苔绿强调色、等宽字体处理路径/代码/状态、细边框低噪音面板。

## 界面结构

- **Titlebar**：品牌、workspace/branch、全局命令入口、active 会话数、侧栏与 dock 开关。
- **Sidebar**：New session、会话搜索、All / Active / Review 筛选、按项目分组的会话树、fork 子节点与父节点分支标记、能力入口与本地 runtime。
- **Thread**：会话标题与状态、fork 血统、上下文摘要、用户消息、assistant 文本、thinking、plan、subagent、工具卡、失败重试、审批卡、变更摘要、composer。
- **Dock**：Changes / Plan / Terminal / Context 四个视图，可拖拽调宽（320–620px），双击重置；Changes 支持逐文件 review、行内评论、文件前后导航。
- **Overlays**：命令面板、设置、模型选择、权限选择、环境选择、slash 菜单、@ 文件菜单、重命名/删除会话对话框、toast。

## 可以点的东西

- **审批**：`go test ./sdk/...` 的 Allow once / Always allow / Deny 都会改变会话状态、plan、
  terminal 和后续消息。
- **Changes**：切换文件、查看 diff、复制 patch、逐文件勾选 reviewed、全部标记 reviewed、文件前后导航、在 diff 行上添加/删除 review note；审批未完成时会有跨面板提示。
- **Model**：8 个模型 / 3 个 provider，切换后 composer、context 和 model 卡片同步；
  thinking level 也在同一个弹层里。
- **附件**：paperclip 选择文件、拖拽到 composer、粘贴图片；附件会进入发送后的用户消息。
- **@ 文件**：输入 `@` 打开文件菜单，支持过滤、上下选择和插入路径。
- **代码块**：assistant 消息里的 fenced code block 带语言标签和 copy 按钮。
- **会话管理**：sessionbar 的更多菜单支持重命名、fork、复制 session id、删除；删除会处理子分支和当前会话切换。
- **失败与重试**：`Codemode sandbox limits` 里有一个失败的工具卡；Retry 会重新跑一次并更新 plan。
- **Permission**：Manual / Auto edits / Plan only / Auto；`Shift+Tab` 可循环切换。
- **Environment**：Local / Worktree 预览。
- **Context**：显示 token 明细、加载的文件/skill、模型窗口；可以手动 compact，transcript
  会插入一条 compaction notice，meter 同步下降。
- **新会话**：点 New session 进入欢迎页，选 suggestion 或直接输入。新会话会在浏览器里
  模拟三段式回复和 plan 完成；这是设计演示，不是真实工具执行。
- **Fork**：已完成会话可以从当前叶子 fork 出新会话；新会话会成为原会话的子节点，
  sidebar 显示分支线，sessionbar 显示 `fork of …`。
- **设置**：General / Providers & models / Capabilities / Appearance / Shortcuts / About。
  默认模型、权限、thinking、主题、密度、时间戳、reasoning 摘要都可以改。
- **主题**：Moss（深色）与 Paper（浅色），设置里的 Appearance 或命令面板切换。
- **命令面板**：会话、动作、设置统一搜索，支持键盘上下和回车。

## 快捷键

| 键 | 行为 |
|---|---|
| `Ctrl/⌘ K` | 命令面板 |
| `Ctrl/⌘ N` | 新会话 |
| `Ctrl/⌘ B` | 收起/展开侧栏 |
| `Ctrl/⌘ J` | 收起/展开右侧 dock |
| `Ctrl/⌘ ,` | 设置 |
| `Ctrl/⌘ F` | 聚焦会话搜索 |
| `Ctrl/⌘ /` 或 `F1` | 快捷键页 |
| `Enter` | 发送 |
| `Shift+Enter` | 换行 |
| `/` | slash 命令 |
| `Shift+Tab` | 循环权限模式 |
| `@` | 文件 mention 菜单 |
| `Alt + ←/→` | 上一个 / 下一个变更文件 |
| `Ctrl/⌘ + Enter` | 在 diff 评论框里保存 note |
| `Esc` | 关闭弹层 / 停止生成 |

## 文件

```text
ui/design/desktop/
├── index.html   # 外壳、SVG icon sprite、overlay 容器
├── app.css      # design tokens、三栏布局、diff/terminal/settings、Moss + Paper
├── data.js      # 会话、plan、diff、provider/model、快捷键等设计 fixture
├── app.js       # 渲染、状态、审批/compact/fork/模拟新会话、命令面板与设置
└── README.md
```

## 和真实 host 的映射

这个原型只负责「长什么样、操作怎么走」，不启动 SDK、不跑工具、不发请求。

| 原型里的东西 | 真实 desktop host |
|---|---|
| 会话列表、项目树、fork | `sdk.Runtime.ListSessions` / `ResumeSession` / `Fork` |
| 状态、thinking、工具卡、plan、diff | `sdk.Session.Subscribe` 事件投影 |
| 审批卡 | host 在工具执行前的 approval gate |
| Terminal | host 自己的 pty；SDK 不启动终端 UI，也不内置 HTTP 服务 |
| provider/model/thinking | `~/.nulo/agent/models.json` + `settings.json` |
| 会话持久化 | `~/.nulo/agent/sessions/.../*.jsonl`（树 + compaction/branch summary） |
| 上下文 meter / compact | `Session.Compact` 与事件里的 usage |

建议的真实接线点：

```go
svcs, _ := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
    CWD: cwd, AgentDir: sdk.DefaultAgentDir(),
})
rt, _ := sdk.NewRuntime(sdk.RuntimeOptions{Services: svcs})
sess, _ := rt.New(sdk.SessionStartOptions{Model: model})
sess.Subscribe(func(ev agent.AgentEvent) { /* 投影到 thread / plan / diff / terminal */ })
```

## 原型边界

- 所有会话、审批、diff、上下文数字都是 `data.js` 的 fixture；刷新页面会回到初始状态。
- 新会话的回复、失败重试、review note 都是本地定时器/内存状态，不会执行 shell、读写文件或调用模型。
- 附件只保留在浏览器内存和 object URL 中，不会上传；刷新后消失。
- 重命名/删除会话只修改本地 fixture，不会写回 JSONL。
- Provider 编辑是只读的，避免伪造一个不存在的持久化路径；真实宿主编辑 `models.json`。
- 窗口 chrome、文件选择器、真实终端和真正的 worktree 创建由宿主负责。
- 偏好设置写入 `localStorage`（key: `nulo.desktop.prototype.v1`），可以在 DevTools 里
  删除后刷新恢复默认。
- 低版本的 `color-mix()` / mask 支持不足时，建议用 Chrome/Edge 111+ 查看。

## 设计取舍

- **一个强调色**：苔绿只用于动作、当前会话、成功和 focus；警告用暖黄，危险用灰红，
  不让界面变成仪表盘。
- **信息密度分层**：会话标题、当前状态、审批卡优先；时间、token、路径退到 mono 小字。
- **审批是产品的一部分**：不是 toast，也不是隐藏按钮。命令、cwd、风险、三个动作和
  状态回写都在同一个卡片里。
- **右侧 dock 是工作台，不是侧边栏垃圾场**：Changes / Plan / Terminal / Context 各自
  有明确职责，且和 center 的状态互相引用（plan 计数、审批 banner、context meter）。
- **键盘优先**：所有主要动作都有快捷键；鼠标路径也存在。
- **两个主题共享 token**：Moss 和 Paper 不各写一套组件，只换颜色变量和少量对比度补丁。

## 验收

- 1440 / 1280 / 1728 宽度下三栏不重叠、不横向溢出。
- 批准、拒绝、停止、compact、fork、新会话、重命名、删除都能走通。
- 切换会话后 composer、dock、plan、context、model 同步。
- 会话树里 fork 子节点缩进、连接线和父节点分支计数可见。
- 逐文件 review、行内 note、文件前后导航、Mark all reviewed 同步到 sidebar / changes card。
- 附件、拖拽、粘贴、@ 文件、代码块 copy 可用。
- 失败工具卡可以 Retry，并同步 plan 与 session 状态。
- 浅色主题下正文、元数据、diff、审批卡、评论和附件可读。
- 刷新后偏好保留，会话 fixture 重置。
