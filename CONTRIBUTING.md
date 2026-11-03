# 贡献指南

## 提交信息

提交信息使用 [Conventional Commits](https://www.conventionalcommits.org/)：

```text
<type>(<scope>)!: <subject>
```

- `type`：`feat`、`fix`、`refactor`、`test`、`docs`、`chore`、`perf`、`build`、`ci`
- `scope`：可选，使用模块名，例如 `sdk`、`session`、`plugin`、`codemode`、`ai`、`tools`
- `subject`：简短说明做了什么，使用中文，动词开头，不加句号
- 提交信息描述 nulo 自身新增或修复的能力，不以“对齐某上游项目”作为主题；兼容性细节放在 body
- `!`：有破坏性变更时添加
- body：可选，说明改动内容和原因
- breaking change：在 footer 中写 `BREAKING CHANGE: <说明>`

示例：

```text
feat(plugin): 支持 stdio 协议插件

新增 NDJSON stdio 运行时和 ServeProtocol SDK。
```

```text
feat(plugin)!: 支持插件自定义子会话并统一插件目录

BREAKING CHANGE: 插件作者 SDK 从 plugin/sdk 移到 sdk/plugin。
```

可以使用本地提交模板：

```sh
git config commit.template .gitmessage
```
