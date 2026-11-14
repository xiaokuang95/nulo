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
`slash`、`mention`、`commit`、`rewind`、`collapsed`、`collapsedgroup`、`projecthover`、
`projectnew`、`mid`、`wide`、
`threshold`、`narrow`、`narrowdock`、`narrowside`、`compact`。

## SDK 接入

- **会话**：启动时用 `sdk.ListAllSessions` 恢复所有工作区的会话；切换到历史
  会话时用 `Runtime.ResumeSession` 懒加载，首次打开新项目时创建
  `AgentSessionServices`。
- **发送**：`Session.Prompt` 在后台执行，`Session.Subscribe` 的事件投影成
  thinking、正文、工具卡、审批卡、变更摘要和状态栏。图片附件编码成
  `sdk.ImageAttachment`，作为 image content block 随 prompt 发送。停止调用
  `Session.Abort`。
- **审批**：`SessionStartOptions.ApproveTool` 把工具调用交给桌面权限模式。
  Manual/Plan 会等待 Allow once / Always allow / Deny；Auto edits 直接放行
  read/write/edit/plan，Auto 放行全部。
- **Plan**：SDK 的 `plan` 工具调用 `Session.SetPlan`，以 custom entry 持久化
  并发出 `plan_update`；桌面端实时刷新右侧 Plan 面板，重启和 fork 后仍在。
  如果 settings.json 显式配置了 `defaultTools`，需要把 `"plan"` 加进列表。
- **供应商密钥**：设置页可以写入/清除 `auth.json` 里的 provider API key，
  文件权限为 0600；模型和密钥来源无需重启。每个供应商的 `Test` 会用首个
  模型发一个很小的探针请求，验证 key、endpoint 和模型是否连通，错误只
  显示脱敏后的响应文本。
- **供应商/模型配置**：Providers & models 提供 Add provider、Add model、
  Edit、Delete 表单；保存时重建 SDK services 校验，失败会恢复原文件。高级
  字段（headers、compat、cost、oauth、modelOverrides 等）可以在表单的
  Advanced JSON 中维护，也可以继续用 `models.json` 编辑器。
- **MCP 与插件状态**：Capabilities 显示每个 MCP server 是 Connected /
  Configured / Not connected / Off，并列出当前 session 加载的 native plugins
  和加载诊断。插件从 settings 的 extensions、用户 plugins 目录和可信项目的
  `.nulo/plugins` 加载；可以直接打开目录并 Reload。
- **MCP 配置**：Capabilities 可以 Add / Edit / Delete MCP 服务器，表单覆盖
  name、description、command、args、URL、env、headers、enabled，以及
  exposure、toolExposure、timeout、cwd 等额外字段的 Advanced JSON。
- **skills 管理**：Capabilities 可以新建 skill（名称、描述、正文）、用结构化
  表单编辑描述和正文（其他 frontmatter 原样保留）、打开 Raw `SKILL.md`
  编辑器、删除用户/项目 skill，以及启停。MCP 开关在可信项目
  `.nulo/mcp.json` 写入 `enabled` 覆盖；skills 开关写入
  `desktop-settings.json` 的 `disabledSkills`。两者都会重建 services，断开并
  重连 MCP。Global/Project `mcp.json` 和用户/项目 skills 文件夹可直接打开，
  Reload 会重新发现。插件或外部路径的 skill 只读。
- **上下文**：`Session.Messages`、模型的 `ContextWindow` 和最近一轮 usage
  生成用量条；`/compact` 调用 `Session.Compact`。
- **分支与消息操作**：fork 使用 `Runtime.Fork`，回退使用
  `Session.NavigateTree`。消息菜单支持复制、编辑并重发、重试回答、从该消息
  分支、删除该消息及后续内容；编辑/重试/删除都会在 JSONL 里保留旧分支。
- **Rewind**：会话菜单列出每个用户消息之前的 checkpoint，可跳到任意一个；
  线程回退用 `NavigateTree`。发送前会在 agent 目录保存该 checkpoint 的
  工作区变更快照（git 变更和未忽略的 untracked 文件，最多 16 MB / 256
  文件），回退时可选择恢复这些文件；HEAD 已经移动或工作区已经切换时拒绝
  恢复，避免把提交后的工作区写回旧状态。
- **重命名/删除**：写回 SDK 的 session JSONL，不只是改内存；删除前要求确认。
- **变更审核**：每次写类工具结束后读取 `git status` / `git diff`；支持单文件
  Stage / Unstage / Revert、按 hunk 反向应用、打开本地编辑器、逐文件
  review note 和逐行 review note；Stage all、Commit 执行真实 git 命令。
  行内 note、文件 note 和 reviewed 状态在刷新 diff 时按路径保留。
- **项目信任**：项目默认不可信，项目级 MCP、skills、plugins 不加载。设置页
  或 Capabilities 可以信任/撤销；信任根按 git common dir 计算，worktree 与
  主仓库共享，保存在 `desktop-settings.json`，刷新服务时会重建会话引擎。
- **桌面偏好**：默认权限、默认 thinking、时间戳、thinking 折叠状态、紧凑
  密度、完成通知和项目信任存在 `~/.nulo/agent/desktop-settings.json`
  （0600），下次启动继续生效。窗口失焦时，turn 完成、需要审批或失败会发
  系统通知；点击通知回到对应会话。
- **终端**：右侧 Terminal 有 `Shell` 和 `Agent` 两个页签。Shell 是 MyGo
  terminal 插件的真实 pty（libghostty-vt，自绘终端），可直接输入、运行
  全屏程序、复制粘贴；Agent 页签保留 `bash` / `powershell` 工具的输出镜像。
  `mygo build` 会下载并打包对应平台的 libghostty-vt。
- **Worktree**：环境选择里的 Worktree 会执行 `git worktree add`，把当前会话
  切换到隔离目录并从 HEAD 创建 `nulo/wt-*` 分支；切换 Local 会回到仓库根目录。
  已有会话文件的会话会 resume 同一份 JSONL；尚未落盘的新会话会在目标 CWD
  新建 SDK session。worktree 从当前 HEAD 创建，未提交改动不会带过去；工具、
  终端和 Changes 都使用 worktree 的 CWD。环境菜单还提供 Merge into Local
  （要求本地工作树干净，使用 `--no-ff`）和 Remove worktree（确认后切回 Local，
  删除目录和分支）。合并冲突会打开冲突对话框：可以逐个打开文件，在内置
  三方视图里查看 base / local / worktree，编辑 Result 或整文件采用其中一侧，
  保存后自动 stage；也可以 Mark resolved、Refresh，最后 Complete merge 或
  Abort merge。映射保存在
  `~/.nulo/agent/desktop-worktrees.json`，重启后恢复；删除会话时会同时删除
  该会话的 worktree 和 `nulo/wt-*` 分支，确认框会明确提示。
- **附件**：文件选择器、拖拽或 composer 里的剪贴板图片（⌘V）都会随 prompt
  发送；图片按 image block 上传（单张上限 10 MB），以缩略图预览。点开大图后
  可裁剪、旋转 90°、水平/垂直翻转、自由画红线标注，Save image 生成新的图片
  文件并替换 composer 附件；其他文件以路径写入 prompt。

## 界面

- **Titlebar**：品牌、`workspace / branch`、侧栏与 dock 开关。
- **Sidebar**：New session、搜索（标题、项目、review note、消息正文和工具
  输出）、All / Active / Review、可折叠的项目分组、
  分组悬停后的 `+` 在该项目下新建会话、fork 子节点、Capabilities 与 Settings。
  顶部 New session 沿用当前会话所在项目；⌘O 打开新的项目目录。
- **Thread**：会话状态、fork 血统、用户消息、thinking、工具卡、审批卡、
  变更摘要与 composer；每条消息的 `…` 菜单提供复制、编辑重发、重试、分支和
  删除。
- **Dock**：Changes / Plan / Terminal / Context，可拖拽调宽，窄窗口下变为抽屉。
  Plan 面板消费 SDK 的 `plan_update` 事件；尚无计划时显示空状态。
- **Overlays**：命令面板、设置、模型选择、权限选择、环境选择、slash 菜单、
  @ 文件菜单、重命名、删除确认、commit 对话框与 toast。

响应式断点和原型一致：窗口窄于 1180 时 dock 收为右侧抽屉，窄于 900 时
侧栏收为左侧抽屉。

## 可交互

- **审批**：Allow once / Always allow / Deny 决定真实工具是否执行，并同步会话
  状态、terminal、后续消息和 toast。
- **发送**：Enter 发送，Shift+Enter 换行；附件路径随用户消息展示。运行中的
  会话可以停止。
- **Slash**：输入 `/` 打开命令面板，支持过滤、上下选择、Tab 补全和 Esc 关闭。
  已实现 `/plan`、`/diff`、`/review`、`/terminal`、`/model`、`/settings`、
  `/context`、`/compact`、`/approve`、`/clear`。
- **@ 文件**：输入 `@` 扫描当前 workspace 的 git 跟踪文件和未忽略文件，按
  文件名/路径过滤并插入，支持键盘选择；索引缓存 5 秒，写文件或 stage/revert
  后立即失效。
- **会话**：新建、切换、全文搜索、筛选、fork、重命名、复制 ID、删除（带
  确认，并清理 worktree）、回退到任意 checkpoint；右侧 `…` 菜单和会话行
  右键都可以打开会话操作。
- **Changes**：逐文件 review note、逐行 review note、Mark all reviewed、
  Prev / Next、复制 patch、Open / Stage / Unstage / Revert file、逐 hunk
  revert、Stage all、Commit。
- **Context**：token 明细、已加载文件/skill、Compact。
- **模型 / 权限 / 环境**：composer 上的 chip 打开原生 popover，模型列表可
  按 provider / model 名搜索；设置里的模型选择会同时更新当前会话和默认模型。
  Providers & models 支持表单化增删改 provider/model；Capabilities 支持
  MCP/skills 的新增、编辑、删除和启停。
- **供应商 API key**：Providers & models 每个供应商有 Test、Set/Replace/Clear；
  弹窗里用密码输入框写入本地 auth.json，Test 异步显示 Testing / Connected /
  错误原因。
- **附件**：拖拽文件到 composer、点 paperclip 打开系统文件选择器，或直接粘贴
  剪贴板图片；图片会以 image block 发送，缩略图可点开查看大图、裁剪、旋转、
  翻转和自由标注。
- **设置**：General、Providers & models、Capabilities、Appearance、
  Shortcuts、About；Moss / Paper 主题即时切换。General 里的默认权限、
  thinking、时间戳、完成通知会写入桌面偏好；Appearance 的 thinking 折叠、
  紧凑密度和 reduced-motion 状态即时生效。
  Capabilities 可以新增/编辑/删除 MCP 与 skills、开关启停、编辑
  `mcp.json` / `SKILL.md`、打开 skills / plugins 文件夹、查看 MCP 连接状态
  和插件诊断，并触发重新加载。
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
├── host_events.go   # SDK 事件投影、plan、usage、terminal
├── host_actions.go  # 模型/thinking、compact、fork、rewind、消息操作、rename、delete
├── checkpoints.go   # rewind 的工作区文件快照
├── host_media.go    # 图片附件读取、MIME 与大小检查
├── app.go           # 应用状态、发送、审批、会话操作、命令面板
├── view.go          # 三栏布局、titlebar、sidebar、窄屏抽屉
├── thread.go        # 消息、工具卡、审批卡、变更摘要
├── message_actions.go # 消息复制/编辑/重试/删除/分支菜单
├── rewind.go        # checkpoint 列表与恢复对话框
├── composer.go      # composer、slash、@、附件、popover
├── dock.go          # Changes / Plan / Terminal / Context
├── diff_comments.go # 逐行 review note
├── terminal_dock.go # 真实 pty shell、Shell/Agent 页签
├── worktree.go      # worktree 创建/切换、状态持久化
├── git.go           # git status/diff、单文件/hunk 操作、stage、commit
├── preferences.go   # 桌面偏好、项目信任、disabled skills、thinking/权限默认值
├── notifications.go # turn 完成/审批/失败的系统通知
├── config_editor.go # models.json / mcp.json / SKILL.md 编辑器与保存回滚
├── config_json.go   # JSON 配置读写、Advanced JSON 合并和 KEY=VALUE 解析
├── mcp_config.go    # MCP 列表、项目 enabled 覆盖、启停重载
├── mcp_form.go      # MCP 新增/编辑/删除表单
├── skill_form.go    # skill 新建/编辑/删除与路径保护
├── skill_edit.go    # skill 描述/正文结构化编辑与 frontmatter 保留
├── provider_form.go # provider/model 新增/编辑/删除表单
├── merge_conflict.go # worktree 合并冲突列表与三方合并编辑器
├── skills_config.go # skills 列表、禁用集合、启停重载
├── file_index.go    # workspace 文件扫描与 @ 过滤缓存
├── image_preview.go # 图片缩略图、剪贴板粘贴、裁剪、旋转/翻转与标注
├── overlays.go      # 命令面板、设置、重命名、删除确认、commit
├── model.go         # 数据结构和截图 fixture
├── theme.go         # Moss / Paper tokens
├── widgets.go       # 通用按钮、chip、进度条、checkbox
├── icons.go         # 代码绘制的 SVG 图标
├── host_test.go     # 真实 SDK host 的流式/审批/恢复/fork/回退测试
├── preferences_test.go # 桌面偏好与项目信任测试
├── config_editor_test.go # 配置编辑与失败回滚测试
├── config_json_test.go # Advanced JSON 合并与校验测试
├── mcp_config_test.go # MCP 项目覆盖测试
├── mcp_form_test.go # MCP 表单增删改测试
├── skills_config_test.go # skills 启停与持久化测试
├── skill_form_test.go # skill 新建/编辑/删除测试
├── provider_form_test.go # provider/model 表单测试
├── file_index_test.go # workspace 文件索引测试
├── image_preview_test.go # 剪贴板图片粘贴、裁剪、旋转与标注测试
├── git_actions_test.go # 单文件/hunk 审核、stage/unstage 测试
├── diff_comments_test.go # 逐行 review note 测试
├── checkpoints_test.go # checkpoint 快照/恢复测试
├── skill_edit_test.go  # 结构化 skill 编辑测试
├── merge_conflict_test.go # 二进制与 rename/delete 合并冲突测试
├── worktree_test.go # worktree 创建/切换/合并冲突/重启恢复测试
├── git_test.go      # git 变更读取测试
└── visual_test.go   # 无头截图回归（fixture）
```

## 边界

主应用已经是真实 SDK host：会启动运行时、执行工具、读写会话 JSONL、读取
git 变更并持久化项目选择。截图回归和 `newFixtureApp` 仍刻意使用内存 fixture，
只用于稳定的视觉对比。

仍保留的边界：rewind 的文件快照只覆盖 checkpoint 时记录过的变更文件，不会
删除之后新建的未跟踪文件；HEAD 移动后需要手动处理。二进制、删除和
rename/delete 冲突可以在内置三方视图中整侧采用；rename/rename 冲突需要
手动决定保留哪个路径，仍建议用外部编辑器处理。
