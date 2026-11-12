/*
 * nulo desktop — prototype content.
 *
 * This file is design fixture data, not application state. app.js clones it on
 * load so approvals, plans and context meters can be driven by the prototype.
 */
window.NULO_DATA = (function () {
  "use strict";

  const providers = [
    {
      id: "tokenharbor",
      name: "Token Harbor",
      baseUrl: "https://www.apikeys.kdns.fr/v1",
      api: "openai-completions",
      apiKey: "env:TOKENHARBOR_API_KEY",
      models: [
        {
          id: "deepseek-v4.1-flash",
          name: "DeepSeek V4.1 Flash",
          context: 512000,
          maxOut: 128000,
          reasoning: true,
          image: true,
        },
        {
          id: "kimi-k2-thinking",
          name: "Kimi K2 Thinking",
          context: 256000,
          maxOut: 32768,
          reasoning: true,
          image: false,
        },
        {
          id: "glm-5-air",
          name: "GLM-5 Air",
          context: 200000,
          maxOut: 65536,
          reasoning: true,
          image: false,
        },
      ],
    },
    {
      id: "openai",
      name: "OpenAI",
      baseUrl: "https://api.openai.com/v1",
      api: "openai-responses",
      apiKey: "env:OPENAI_API_KEY",
      models: [
        {
          id: "gpt-5.1",
          name: "GPT-5.1",
          context: 400000,
          maxOut: 128000,
          reasoning: true,
          image: true,
        },
        {
          id: "gpt-5.1-mini",
          name: "GPT-5.1 mini",
          context: 400000,
          maxOut: 128000,
          reasoning: true,
          image: true,
        },
        {
          id: "o3",
          name: "o3",
          context: 200000,
          maxOut: 100000,
          reasoning: true,
          image: true,
        },
      ],
    },
    {
      id: "local",
      name: "Local",
      baseUrl: "http://127.0.0.1:11434/v1",
      api: "openai-completions",
      apiKey: "none",
      models: [
        {
          id: "qwen3-coder",
          name: "Qwen3 Coder",
          context: 262144,
          maxOut: 32768,
          reasoning: false,
          image: false,
        },
        {
          id: "llama-4-scout",
          name: "Llama 4 Scout",
          context: 128000,
          maxOut: 16384,
          reasoning: false,
          image: true,
        },
      ],
    },
  ];

  const desktopDiff = [
    {
      path: "ui/design/desktop/index.html",
      status: "M",
      add: 318,
      del: 12,
      language: "html",
      diff: [
        "@@ -1,9 +1,10 @@",
        " <!doctype html>",
        " <html lang=\"en\">",
        " <head>",
        "+<meta name=\"color-scheme\" content=\"dark light\">",
        " <meta charset=\"utf-8\">",
        "-<title>nulo — desktop</title>",
        "+<title>nulo — desktop shell</title>",
        " <link rel=\"stylesheet\" href=\"app.css\">",
        " </head>",
        "@@ -18,13 +19,21 @@",
        " <body>",
        "-  <div class=\"app\">",
        "-    <header class=\"topbar\">",
        "+  <div class=\"app\" data-theme=\"moss\">",
        "+    <header class=\"titlebar\">",
        "+      <div class=\"titlebar-brand\">",
        "+        <span class=\"logo-mark\">n</span>",
        "+        <span>nulo</span>",
        "+      </div>",
        "+      <button class=\"command-trigger\">",
        "+        <svg class=\"icon\"><use href=\"#i-search\"/></svg>",
        "+        <span>Search sessions and commands</span>",
        "+        <kbd>Ctrl K</kbd>",
        "+      </button>",
        "     </header>",
        "-    <main class=\"thread\">",
        "+    <main class=\"workbench\">",
        "   </div>",
        " </body>",
        "</html>",
      ],
    },
    {
      path: "ui/design/desktop/app.css",
      status: "A",
      add: 1486,
      del: 0,
      language: "css",
      diff: [
        "@@ -0,0 +1,24 @@",
        "+:root {",
        "+  --bg: #080b09;",
        "+  --sidebar: #0b0e0c;",
        "+  --surface: #111612;",
        "+  --line: #1d261e;",
        "+  --text: #e4e9df;",
        "+  --muted: #75816f;",
        "+  --accent: #b7e08b;",
        "+}",
        "+",
        "+.workbench {",
        "+  display: grid;",
        "+  grid-template-columns: var(--sidebar-w) minmax(0, 1fr) var(--dock-w);",
        "+  min-height: 0;",
        "+}",
        "+",
        "+.conversation {",
        "+  scrollbar-width: thin;",
        "+  scrollbar-color: var(--line-strong) transparent;",
        "+}",
        "+",
        "+.approval {",
        "+  border-color: color-mix(in srgb, var(--warn) 34%, var(--line));",
        "+  background: color-mix(in srgb, var(--warn) 6%, var(--surface));",
        "+}",
      ],
    },
    {
      path: "ui/design/desktop/app.js",
      status: "A",
      add: 982,
      del: 0,
      language: "js",
      diff: [
        "@@ -0,0 +1,26 @@",
        "+const state = {",
        "+  activeSessionId: \"s1\",",
        "+  dockTab: \"changes\",",
        "+  paletteOpen: false,",
        "+  approval: \"auto-edits\",",
        "+};",
        "+",
        "+function renderAll() {",
        "+  renderSidebar();",
        "+  renderSessionHeader();",
        "+  renderConversation();",
        "+  renderDock();",
        "+}",
        "+",
        "+function resolveApproval(decision) {",
        "+  const session = activeSession();",
        "+  const block = findApproval(session);",
        "+  block.status = decision;",
        "+  session.status = decision === \"approved\" ? \"running\" : \"paused\";",
        "+  renderAll();",
        "+  if (decision === \"approved\") runVerification(session);",
        "+}",
        "+",
        "+document.addEventListener(\"keydown\", (event) => {",
        "+  if (event.ctrlKey && event.key.toLowerCase() === \"k\") openPalette();",
        "+});",
      ],
    },
  ];

  const providerDiff = [
    {
      path: "ui/design/desktop/settings.js",
      status: "M",
      add: 186,
      del: 44,
      language: "js",
      diff: [
        "@@ -14,18 +14,25 @@",
        " function renderProviders() {",
        "-  const providers = state.providers;",
        "-  const models = state.models;",
        "+  const groups = state.providers.map((provider) => ({",
        "+    ...provider,",
        "+    models: provider.models || [],",
        "+    isDefault: provider.id === state.defaultProvider,",
        "+  }));",
        "   return `",
        "     <section class=\"providers\">",
        "-      <ul class=\"providers\">",
        "-        ${providers.map(providerRow)}",
        "-      </ul>",
        "-      <ul class=\"models\">",
        "-        ${models.map(modelRow)}",
        "-      </ul>",
        "+      ${groups.map((group) => `",
        "+        <article class=\"provider\" data-provider=\"${group.id}\">",
        "+          ${providerRow(group)}",
        "+          <div class=\"model-list\">",
        "+            ${group.models.map(modelRow).join(\"\")}",
        "+          </div>",
        "+        </article>",
        "+      `).join(\"\")}",
        "     </section>",
        "   `;",
        " }",
      ],
    },
    {
      path: "ui/design/desktop/settings.css",
      status: "M",
      add: 64,
      del: 18,
      language: "css",
      diff: [
        "@@ -41,11 +41,17 @@",
        " .provider-row {",
        "   display: flex;",
        "   align-items: center;",
        "+  gap: 10px;",
        " }",
        " ",
        " .model-list {",
        "+  margin-left: 30px;",
        "+  border-left: 1px solid var(--line);",
        "+  padding-left: 12px;",
        " }",
        " ",
        " .provider-row.is-default {",
        "-  color: var(--text-3);",
        "+  color: var(--accent);",
        " }",
      ],
    },
  ];

  const pluginDiff = [
    {
      path: "sdk/internal/plugin/manifest.go",
      status: "M",
      add: 38,
      del: 9,
      language: "go",
      diff: [
        "@@ -63,12 +63,19 @@",
        " type Manifest struct {",
        "   Name    string `json:\"name\"`",
        "   Version string `json:\"version\"`",
        "   Protocol string `json:\"protocol\"`",
        "   Command string `json:\"command\"`",
        "+  ABI     string `json:\"abi,omitempty\"`",
        "+  Trusted bool   `json:\"trusted,omitempty\"`",
        " }",
        " ",
        " func (m Manifest) Validate() error {",
        "-  if m.Name == \"\" {",
        "+  if m.Name == \"\" || len(m.Name) > 64 {",
        "     return errors.New(\"plugin: name is required\")",
        "   }",
        "+  if m.Protocol != \"\" && m.Protocol != \"stdio\" {",
        "+    return fmt.Errorf(\"plugin: unsupported protocol %q\", m.Protocol)",
        "+  }",
        "   return nil",
        " }",
      ],
    },
  ];

  const sessions = {
    s1: {
      id: "s1",
      project: "nulo",
      title: "Desktop shell for nulo",
      status: "awaiting",
      branch: "main",
      env: "Local",
      worktree: null,
      model: "tokenharbor/deepseek-v4.1-flash",
      thinking: "high",
      approval: "auto-edits",
      started: "12m ago",
      updated: "12m",
      plan: {
        title: "Desktop shell",
        steps: [
          {
            title: "Map the existing design language",
            detail: "Keep nulo's ink + moss palette and terminal rhythm.",
            status: "done",
            duration: "3m",
          },
          {
            title: "Build the desktop shell",
            detail: "Titlebar, sidebar, conversation, dock, composer.",
            status: "done",
            duration: "9m",
          },
          {
            title: "Verify the approval + review flow",
            detail: "Pending bash command, reviewable diff, plan sync.",
            status: "active",
          },
        ],
        next: "Approve the test run to finish this session.",
      },
      changes: {
        reviewed: false,
        files: desktopDiff,
      },
      context: {
        used: 42800,
        limit: 512000,
        breakdown: [
          { label: "System prompt", value: 6100, color: "var(--accent)" },
          { label: "Messages", value: 26400, color: "var(--info)" },
          { label: "Tool results", value: 4900, color: "var(--warn)" },
          { label: "Files", value: 5400, color: "var(--violet)" },
        ],
        entries: [
          { kind: "instructions", path: "AGENTS.md", tokens: 2100 },
          { kind: "file", path: "ui/design/index.html", tokens: 1900 },
          { kind: "file", path: "README.md", tokens: 1200 },
          { kind: "file", path: "sdk/session.go", tokens: 1400 },
          { kind: "skill", path: "ponytail", tokens: 640 },
          { kind: "skill", path: "vision", tokens: 420 },
        ],
        skills: 2,
        mcp: 3,
        plugins: 2,
      },
      terminal: [
        { type: "cmd", text: "git status --short" },
        { type: "out", text: " M ui/design/index.html" },
        { type: "out", text: "?? ui/design/desktop/" },
        { type: "cmd", text: "go test ./sdk/..." },
        { type: "wait", text: "awaiting approval" },
      ],
      messages: [
        {
          role: "user",
          time: "12:04",
          text:
            "The TUI prototype is good. Now set up a desktop UI in ui/design/desktop. It should feel native next to Codex, Claude Code, and OpenCode: sessions and projects on the left, the conversation in the middle, changes and plan on the right. Keep the SDK untouched.",
        },
        {
          role: "assistant",
          time: "12:04",
          blocks: [
            {
              type: "thinking",
              duration: "8s",
              text:
                "Reading the design folder and the session surface before touching the shell. Three columns, one active thread, one reviewable change set.",
            },
            {
              type: "text",
              md:
                "I've got the shape. Three columns, one active session, and a right dock that holds **changes**, **plan**, **terminal**, and **context** without stealing focus from the thread. The SDK stays untouched — this is a browser prototype under `ui/design/desktop/`.",
            },
            { type: "plan" },
            {
              type: "subagent",
              name: "scout",
              agent: "Explore",
              status: "done",
              duration: "24s",
              summary:
                "Mapped the design surface: one TUI prototype, no desktop host, no build step.",
              details: [
                "ui/design/index.html — 2,395-line vanilla TUI prototype",
                "ui/design/desktop/ — empty before this session",
                "No package.json or frontend build in the repo",
                "Session SDK lives under sdk/ and must stay untouched",
              ],
            },
            {
              type: "tool",
              tool: "read",
              title: "Read 4 files",
              meta: "408 lines",
              status: "done",
              output:
                "README.md               wrote 138 lines\ndocs/PLAN.md             read 1,204 lines\nui/design/index.html      read 2,395 lines\nsdk/session.go            read 86 lines",
            },
            {
              type: "tool",
              tool: "bash",
              title: "git status --short",
              meta: "0.2s",
              status: "done",
              output: " M ui/design/index.html\n?? ui/design/desktop/",
            },
            {
              type: "text",
              md:
                "The shell is in place. Layout, density, and the review dock are wired; the only action that needs you is the test run below.",
            },
            { type: "changes" },
            {
              type: "approval",
              id: "ap-test",
              command: "go test ./sdk/...",
              cwd: "~/Downloads/nulo",
              reason:
                "Verify the SDK suite before handing the prototype back. The desktop shell itself is static.",
              risk: "runs local tests · no network",
              status: "pending",
            },
          ],
        },
      ],
    },

    s2: {
      id: "s2",
      project: "nulo",
      title: "Provider & model settings hierarchy",
      status: "done",
      branch: "main",
      env: "Local",
      worktree: null,
      model: "tokenharbor/deepseek-v4.1-flash",
      thinking: "medium",
      approval: "manual",
      started: "18m ago",
      updated: "18m",
      plan: {
        title: "Settings hierarchy",
        steps: [
          {
            title: "Flatten the old parallel lists",
            detail: "Providers and models were rendered as two unrelated arrays.",
            status: "done",
            duration: "2m",
          },
          {
            title: "Nest models under providers",
            detail: "One provider row, one model list, one default badge.",
            status: "done",
            duration: "6m",
          },
          {
            title: "Derive the default model",
            detail: "Read from settings.defaultModel instead of duplicating it.",
            status: "done",
            duration: "2m",
          },
        ],
        next: "Ready to review.",
      },
      changes: {
        reviewed: true,
        files: providerDiff,
      },
      context: {
        used: 31800,
        limit: 512000,
        breakdown: [
          { label: "System prompt", value: 6100, color: "var(--accent)" },
          { label: "Messages", value: 17200, color: "var(--info)" },
          { label: "Tool results", value: 3600, color: "var(--warn)" },
          { label: "Files", value: 4900, color: "var(--violet)" },
        ],
        entries: [
          { kind: "instructions", path: "AGENTS.md", tokens: 2100 },
          { kind: "file", path: "ui/design/index.html", tokens: 1900 },
          { kind: "file", path: "ui/design/desktop/settings.js", tokens: 2400 },
          { kind: "file", path: "ui/design/desktop/settings.css", tokens: 1100 },
        ],
        skills: 2,
        mcp: 3,
        plugins: 2,
      },
      terminal: [
        { type: "cmd", text: "git diff --stat" },
        { type: "out", text: " settings.css |  82 ++++++++++-----" },
        { type: "out", text: " settings.js  | 230 +++++++++++++++++++--------" },
        { type: "out", text: " 2 files changed, 250 insertions(+), 62 deletions(-)" },
      ],
      messages: [
        {
          role: "user",
          time: "11:46",
          text:
            "Provider and model settings still feel flat. Make providers own their models, and derive the default model instead of duplicating it.",
        },
        {
          role: "assistant",
          time: "11:46",
          blocks: [
            {
              type: "thinking",
              duration: "5s",
              text:
                "Looking at how the TUI nests providers before changing the desktop settings surface.",
            },
            {
              type: "text",
              md:
                "The flat pair of lists is the problem. I grouped models under their provider, kept `settings.defaultModel` as the single source of truth, and let the provider row own its default badge.",
            },
            { type: "plan" },
            {
              type: "tool",
              tool: "edit",
              title: "Edited settings.js + settings.css",
              meta: "+250 −62",
              status: "done",
              output:
                "settings.js   provider tree, derived default, model rows\nsettings.css  nested list, default states, focus rings",
            },
            { type: "changes" },
            {
              type: "text",
              md:
                "Default model now resolves in one place. Selecting a provider updates the nested list immediately; the old duplicate model array is gone. No SDK changes.\n\n```js\nconst defaultModel = settings.defaultModel;\nconst provider = providers.find((item) => item.models.some((model) => model.id === defaultModel));\n```\n\nThe provider row owns its default badge, so there is no second source of truth.",
            },
          ],
        },
      ],
    },

    s3: {
      id: "s3",
      project: "nulo",
      title: "Compaction drops tool results",
      status: "running",
      branch: "fix/compaction-projection",
      env: "Worktree",
      worktree: "nulo-wt-compaction",
      model: "openai/gpt-5.1",
      thinking: "high",
      approval: "manual",
      started: "31m ago",
      updated: "3m",
      plan: {
        title: "Compaction projection",
        steps: [
          {
            title: "Reproduce the missing tool result",
            detail: "Fixture session compacts after a bash call.",
            status: "done",
            duration: "11m",
          },
          {
            title: "Trace the projection window",
            detail: "Compare firstKeptEntryId against the tool source order.",
            status: "active",
          },
          {
            title: "Fix the projection range",
            detail: "Keep tool results attached to their originating call.",
            status: "pending",
          },
          {
            title: "Verify with the session fixture",
            detail: "go test ./sdk/... -run Compaction",
            status: "pending",
          },
        ],
        next: "Tracing the projection boundary now.",
      },
      changes: null,
      context: {
        used: 186400,
        limit: 400000,
        breakdown: [
          { label: "System prompt", value: 6100, color: "var(--accent)" },
          { label: "Messages", value: 121000, color: "var(--info)" },
          { label: "Tool results", value: 48200, color: "var(--warn)" },
          { label: "Files", value: 11100, color: "var(--violet)" },
        ],
        entries: [
          { kind: "instructions", path: "AGENTS.md", tokens: 2100 },
          { kind: "file", path: "sdk/session.go", tokens: 8400 },
          { kind: "file", path: "sdk/session_test.go", tokens: 14200 },
          { kind: "file", path: "sessions/fixture-compaction.jsonl", tokens: 22100 },
        ],
        skills: 2,
        mcp: 3,
        plugins: 2,
      },
      terminal: [
        { type: "cmd", text: "go test ./sdk/... -run Compaction -v" },
        { type: "out", text: "=== RUN   TestCompactionProjection" },
        { type: "out", text: "    session_test.go:482: expected 2 tool results, got 1" },
        { type: "run", text: "running · 3m" },
      ],
      messages: [
        {
          role: "user",
          time: "11:33",
          text:
            "Compaction is dropping tool results from the next request. Reproduce it against the fixture and trace where the projection loses them.",
        },
        {
          role: "assistant",
          time: "11:33",
          blocks: [
            {
              type: "thinking",
              duration: "12s",
              text:
                "Starting from the fixture, then walking the projection from the latest compaction entry forward.",
            },
            {
              type: "text",
              md:
                "I can reproduce it: the compaction keeps the assistant call but drops the following tool result, so the next request has an orphaned tool call. Tracing the range now.",
            },
            { type: "plan" },
            {
              type: "tool",
              tool: "read",
              title: "Read sdk/session.go + fixture",
              meta: "1,482 lines",
              status: "done",
              output:
                "sdk/session.go        projection + compaction helpers\nsdk/session_test.go   existing fixture assertions\nsessions/fixture-compaction.jsonl",
            },
            {
              type: "tool",
              tool: "bash",
              title: "go test ./sdk/... -run Compaction",
              meta: "running",
              status: "running",
              output:
                "=== RUN   TestCompactionProjection\n    session_test.go:482: expected 2 tool results, got 1",
            },
            {
              type: "text",
              md:
                "The cut point lands between the tool call and its result. I'm checking whether the fix belongs in the projection or at compaction time — I'll keep the scope to one file if the fixture agrees.",
            },
          ],
        },
      ],
    },

    s4: {
      id: "s4",
      project: "nulo",
      title: "Review plugin ABI changes",
      status: "done",
      branch: "feat/plugin-abi",
      env: "Worktree",
      worktree: "nulo-wt-plugin-abi",
      model: "openai/gpt-5.1",
      thinking: "medium",
      approval: "manual",
      started: "2h ago",
      updated: "2h",
      plan: {
        title: "ABI review",
        steps: [
          {
            title: "Read manifest + host call paths",
            detail: "Check the shared-library and stdio paths.",
            status: "done",
            duration: "6m",
          },
          {
            title: "Flag compatibility risks",
            detail: "Name length, protocol validation, trusted flag.",
            status: "done",
            duration: "4m",
          },
        ],
        next: "Review notes are in the thread.",
      },
      changes: {
        reviewed: true,
        files: pluginDiff,
      },
      context: {
        used: 61200,
        limit: 400000,
        breakdown: [
          { label: "System prompt", value: 6100, color: "var(--accent)" },
          { label: "Messages", value: 31400, color: "var(--info)" },
          { label: "Tool results", value: 13700, color: "var(--warn)" },
          { label: "Files", value: 10000, color: "var(--violet)" },
        ],
        entries: [
          { kind: "instructions", path: "AGENTS.md", tokens: 2100 },
          { kind: "file", path: "sdk/internal/plugin/manifest.go", tokens: 3100 },
          { kind: "file", path: "sdk/plugin/plugin.go", tokens: 4200 },
          { kind: "file", path: "examples/plugin-subagent", tokens: 5800 },
        ],
        skills: 2,
        mcp: 3,
        plugins: 2,
      },
      terminal: [
        { type: "cmd", text: "go vet ./sdk/..." },
        { type: "out", text: "ok  github.com/xiaokuang95/nulo/sdk" },
      ],
      messages: [
        {
          role: "user",
          time: "10:12",
          text:
            "Review the plugin ABI change before I merge it. I care about backward compatibility and the stdio path.",
        },
        {
          role: "assistant",
          time: "10:12",
          blocks: [
            {
              type: "thinking",
              duration: "9s",
              text:
                "Comparing manifest validation across the shared-library and protocol host modes.",
            },
            {
              type: "text",
              md:
                "Two things before merge: the new `abi` field is additive, but `protocol` validation now rejects anything that is not empty or `stdio`. That is a behavior change for manifests that used a custom protocol string. The `trusted` flag is read but not enforced on the shared-library path.",
            },
            {
              type: "subagent",
              name: "reviewer",
              agent: "general-purpose",
              status: "done",
              duration: "38s",
              summary: "Checked both host modes and the plugin-subagent example.",
              details: [
                "Shared library path ignores Trusted — flag is inert",
                "stdio path rejects unknown protocol strings (breaking)",
                "Name length cap is safe; no existing plugin exceeds 64 chars",
                "Example manifests still parse",
              ],
            },
            { type: "changes" },
            {
              type: "text",
              md:
                "I left the validation change uncommitted so you can decide. If you want it strict, add a migration note; otherwise make it a warning. Nothing else blocks the merge.",
            },
          ],
        },
      ],
    },

    s5: {
      id: "s5",
      project: "nulo",
      title: "TUI density pass",
      status: "done",
      branch: "main",
      env: "Local",
      worktree: null,
      model: "tokenharbor/deepseek-v4.1-flash",
      thinking: "low",
      approval: "auto-edits",
      started: "Yesterday",
      updated: "Yesterday",
      plan: {
        title: "Density pass",
        steps: [
          { title: "Tighten the transcript rhythm", detail: "Less air, same hierarchy.", status: "done" },
          { title: "Check 80 / 100 / 120 columns", detail: "No clipped metadata.", status: "done" },
        ],
        next: "Done.",
      },
      changes: null,
      context: {
        used: 24100,
        limit: 512000,
        breakdown: [
          { label: "System prompt", value: 6100, color: "var(--accent)" },
          { label: "Messages", value: 12200, color: "var(--info)" },
          { label: "Tool results", value: 2800, color: "var(--warn)" },
          { label: "Files", value: 3000, color: "var(--violet)" },
        ],
        entries: [
          { kind: "instructions", path: "AGENTS.md", tokens: 2100 },
          { kind: "file", path: "ui/design/index.html", tokens: 1900 },
        ],
        skills: 2,
        mcp: 3,
        plugins: 2,
      },
      terminal: [{ type: "cmd", text: "git diff --stat ui/design/index.html" }],
      messages: [
        {
          role: "user",
          time: "Yesterday",
          text: "Tighten the TUI transcript density. Keep the plan and review views intact.",
        },
        {
          role: "assistant",
          time: "Yesterday",
          blocks: [
            {
              type: "text",
              md:
                "Transcript rows are 18% tighter at 80 columns, and the status line still fits at 120. Plan and review are unchanged.",
            },
          ],
        },
      ],
    },

    s6: {
      id: "s6",
      project: "dotfiles",
      title: "Windows terminal profile",
      status: "done",
      branch: "main",
      env: "Local",
      worktree: null,
      model: "local/qwen3-coder",
      thinking: "off",
      approval: "auto-edits",
      started: "Yesterday",
      updated: "Yesterday",
      plan: {
        title: "Terminal profile",
        steps: [{ title: "Match nulo colors", detail: "Cascadia Mono, moss accents.", status: "done" }],
        next: "Done.",
      },
      changes: null,
      context: {
        used: 9800,
        limit: 262144,
        breakdown: [
          { label: "System prompt", value: 4200, color: "var(--accent)" },
          { label: "Messages", value: 3100, color: "var(--info)" },
          { label: "Tool results", value: 1500, color: "var(--warn)" },
          { label: "Files", value: 1000, color: "var(--violet)" },
        ],
        entries: [{ kind: "instructions", path: "AGENTS.md", tokens: 900 }],
        skills: 0,
        mcp: 1,
        plugins: 0,
      },
      terminal: [{ type: "cmd", text: "ls ~/.config/terminal" }],
      messages: [
        {
          role: "user",
          time: "Yesterday",
          text: "Align the Windows Terminal profile with the nulo palette.",
        },
        {
          role: "assistant",
          time: "Yesterday",
          blocks: [
            {
              type: "text",
              md:
                "Profile updated: Cascadia Mono, `#080b09` background, `#b7e08b` cursor. The existing font fallback stays for CJK.",
            },
          ],
        },
      ],
    },

    s7: {
      id: "s7",
      project: "pi-lab",
      title: "Codemode sandbox limits",
      status: "paused",
      branch: "experiment/codemode",
      env: "Worktree",
      worktree: "pi-lab-wt-codemode",
      model: "local/qwen3-coder",
      thinking: "medium",
      approval: "manual",
      started: "Tue",
      updated: "Tue",
      plan: {
        title: "Sandbox limits",
        steps: [
          { title: "Measure heap ceiling", detail: "Find the moejs safe operating range.", status: "done" },
          { title: "Add a timeout guard", detail: "Abort runaway scripts at 2s.", status: "blocked" },
          { title: "Verify tool cancellation", detail: "Return a structured abort.", status: "pending" },
        ],
        next: "Paused — waiting on a decision about the timeout budget.",
      },
      changes: null,
      context: {
        used: 301200,
        limit: 262144,
        breakdown: [
          { label: "System prompt", value: 6100, color: "var(--accent)" },
          { label: "Messages", value: 189000, color: "var(--info)" },
          { label: "Tool results", value: 78000, color: "var(--warn)" },
          { label: "Files", value: 28100, color: "var(--violet)" },
        ],
        entries: [
          { kind: "instructions", path: "AGENTS.md", tokens: 2100 },
          { kind: "file", path: "sdk/internal/codemode/host.go", tokens: 12800 },
        ],
        skills: 1,
        mcp: 2,
        plugins: 1,
      },
      terminal: [{ type: "cmd", text: "go test ./sdk/internal/codemode -run Timeout" }],
      messages: [
        {
          role: "user",
          time: "Tue",
          text: "Find a safe timeout for codemode, then add cancellation.",
        },
        {
          role: "assistant",
          time: "Tue",
          blocks: [
            {
              type: "text",
              md:
                "The heap ceiling is fine under 2s. The cancellation path is the open question: `moejs` can interrupt between ticks, but a tight loop never yields. I paused before changing the host contract.",
            },
            {
              type: "tool",
              tool: "bash",
              title: "go test ./sdk/internal/codemode -run Timeout",
              meta: "failed · 2.1s",
              status: "fail",
              error: "moejs did not yield before the 2s budget.",
              output:
                "=== RUN   TestCodemodeTimeout\n    host_test.go:88: panic: moejs: execution did not yield\n--- FAIL: TestCodemodeTimeout (2.10s)\nFAIL	github.com/xiaokuang95/nulo/sdk/internal/codemode	2.114s",
            },
            { type: "plan" },
          ],
        },
      ],
    },

    s8: {
      id: "s8",
      project: "pi-lab",
      title: "MCP reconnect backoff",
      status: "done",
      branch: "main",
      env: "Local",
      worktree: null,
      model: "tokenharbor/kimi-k2-thinking",
      thinking: "medium",
      approval: "auto-edits",
      started: "Mon",
      updated: "Mon",
      plan: {
        title: "Reconnect backoff",
        steps: [
          { title: "Add capped exponential backoff", detail: "100ms → 30s, jittered.", status: "done" },
          { title: "Keep tool calls pending", detail: "Queue until the server is back.", status: "done" },
        ],
        next: "Merged.",
      },
      changes: null,
      context: {
        used: 44200,
        limit: 256000,
        breakdown: [
          { label: "System prompt", value: 6100, color: "var(--accent)" },
          { label: "Messages", value: 21800, color: "var(--info)" },
          { label: "Tool results", value: 9800, color: "var(--warn)" },
          { label: "Files", value: 6500, color: "var(--violet)" },
        ],
        entries: [
          { kind: "instructions", path: "AGENTS.md", tokens: 2100 },
          { kind: "file", path: "sdk/internal/mcp/client.go", tokens: 6400 },
        ],
        skills: 0,
        mcp: 4,
        plugins: 1,
      },
      terminal: [{ type: "cmd", text: "go test ./sdk/internal/mcp -run Reconnect" }],
      messages: [
        {
          role: "user",
          time: "Mon",
          text: "MCP reconnects are hot-looping when the server drops. Add backoff and keep pending tool calls alive.",
        },
        {
          role: "assistant",
          time: "Mon",
          blocks: [
            {
              type: "text",
              md:
                "Reconnect now uses capped exponential backoff with jitter. Pending tool calls wait on the same generation, so a reconnect does not orphan them. Covered by the existing MCP integration test.",
            },
          ],
        },
      ],
    },

    s9: {
      id: "s9",
      project: "nulo",
      parent: "s2",
      title: "Provider defaults · local-first",
      status: "done",
      branch: "main",
      env: "Local",
      worktree: null,
      model: "local/qwen3-coder",
      thinking: "low",
      approval: "auto-edits",
      started: "6m ago",
      updated: "6m",
      plan: {
        title: "Local default",
        steps: [
          { title: "Keep the provider tree", detail: "Inherit the settings hierarchy from the parent session.", status: "done", duration: "1m" },
          { title: "Prefer the local model", detail: "Make qwen3-coder the default for this branch.", status: "done", duration: "3m" },
        ],
        next: "Branch ready for review.",
      },
      changes: {
        reviewed: false,
        files: [providerDiff[0]],
      },
      context: {
        used: 18400,
        limit: 262144,
        breakdown: [
          { label: "System prompt", value: 6100, color: "var(--accent)" },
          { label: "Messages", value: 7600, color: "var(--info)" },
          { label: "Tool results", value: 2400, color: "var(--warn)" },
          { label: "Files", value: 2300, color: "var(--violet)" },
        ],
        entries: [
          { kind: "instructions", path: "AGENTS.md", tokens: 2100 },
          { kind: "file", path: "ui/design/desktop/settings.js", tokens: 2400 },
        ],
        skills: 2,
        mcp: 3,
        plugins: 2,
      },
      terminal: [
        { type: "cmd", text: "git diff --stat ui/design/desktop/settings.js" },
        { type: "out", text: " settings.js | 230 +++++++++++++++++++--------" },
      ],
      messages: [
        {
          role: "user",
          time: "11:52",
          text: "Branch from the provider work and make the local model the default. Keep the settings hierarchy intact.",
        },
        {
          role: "assistant",
          time: "11:52",
          blocks: [
            {
              type: "thinking",
              duration: "4s",
              text: "Branching from the provider hierarchy. The nested provider list stays; only the default leaf changes.",
            },
            {
              type: "text",
              md:
                "This branch keeps the provider tree from the parent and points the default at `local/qwen3-coder`. The diff is one settings file, so it should review quickly.",
            },
            { type: "plan" },
            { type: "changes" },
            {
              type: "text",
              md:
                "Ready for review. If the local default is wrong, switch back to the parent session — the provider hierarchy is untouched.",
            },
          ],
        },
      ],
    },
  };

  return {
    meta: {
      version: "0.8.0",
      runtime: "local · cgo",
      agentDir: "~/.nulo/agent",
      cwd: "~/Downloads/nulo",
      sessionDir: "~/.nulo/agent/sessions",
    },
    projects: [
      {
        id: "nulo",
        name: "nulo",
        path: "~/Downloads/nulo",
        branch: "main",
        tint: "green",
        sessions: ["s1", "s2", "s9", "s3", "s4", "s5"],
      },
      {
        id: "dotfiles",
        name: "dotfiles",
        path: "~/dotfiles",
        branch: "main",
        tint: "blue",
        sessions: ["s6"],
      },
      {
        id: "pi-lab",
        name: "pi-lab",
        path: "~/labs/pi-lab",
        branch: "experiment/codemode",
        tint: "violet",
        sessions: ["s7", "s8"],
      },
    ],
    providers,
    defaultModel: "tokenharbor/deepseek-v4.1-flash",
    defaultThinking: "high",
    defaultApproval: "auto-edits",
    files: [
      { path: "README.md", language: "md" },
      { path: "docs/PLAN.md", language: "md" },
      { path: "sdk/session.go", language: "go" },
      { path: "sdk/session_test.go", language: "go" },
      { path: "sdk/runtime.go", language: "go" },
      { path: "sdk/internal/mcp/client.go", language: "go" },
      { path: "sdk/internal/plugin/manifest.go", language: "go" },
      { path: "sdk/internal/codemode/host.go", language: "go" },
      { path: "ui/design/index.html", language: "html" },
      { path: "ui/design/desktop/index.html", language: "html" },
      { path: "ui/design/desktop/app.css", language: "css" },
      { path: "ui/design/desktop/app.js", language: "js" },
      { path: "ui/design/desktop/data.js", language: "js" },
      { path: "ui/design/desktop/README.md", language: "md" },
      { path: "sessions/fixture-compaction.jsonl", language: "jsonl" },
      { path: "examples/plugin-subagent", language: "go" },
    ],
    sessions,
    suggestions: [
      {
        icon: "diff",
        title: "Review uncommitted changes",
        desc: "3 files in the working tree",
        prompt: "Review the uncommitted changes and flag anything risky.",
      },
      {
        icon: "plug",
        title: "Add a provider",
        desc: "Token Harbor, OpenAI, Local",
        prompt: "Walk me through adding a provider and its models.",
      },
      {
        icon: "book",
        title: "Explain the session SDK",
        desc: "Map the public surface",
        prompt: "Explain the nulo session SDK and how the desktop host should consume it.",
      },
      {
        icon: "test",
        title: "Fix failing tests",
        desc: "go test ./sdk/...",
        prompt: "Run the SDK tests, find the failure, and propose the smallest fix.",
      },
    ],
    shortcuts: [
      {
        group: "Global",
        items: [
          { keys: ["Ctrl", "K"], label: "Command palette" },
          { keys: ["Ctrl", "N"], label: "New session" },
          { keys: ["Ctrl", "B"], label: "Toggle sidebar" },
          { keys: ["Ctrl", "J"], label: "Toggle right panel" },
          { keys: ["Ctrl", ","], label: "Settings" },
          { keys: ["Ctrl", "/"], label: "Keyboard shortcuts" },
        ],
      },
      {
        group: "Composer",
        items: [
          { keys: ["Enter"], label: "Send message" },
          { keys: ["Shift", "Enter"], label: "New line" },
          { keys: ["/"], label: "Slash commands" },
          { keys: ["Shift", "Tab"], label: "Cycle permission mode" },
          { keys: ["@"], label: "Mention a file" },
          { keys: ["Esc"], label: "Stop generation" },
        ],
      },
      {
        group: "Review",
        items: [
          { keys: ["Ctrl", "J"], label: "Open changes dock" },
          { keys: ["Alt", "←/→"], label: "Previous / next file" },
          { keys: ["Ctrl", "Enter"], label: "Save a review note" },
          { keys: ["Esc"], label: "Close overlay" },
        ],
      },
    ],
  };
})();
