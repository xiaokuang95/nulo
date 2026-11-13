package main

import (
	"fmt"
	"strings"
)

// provider is one model provider shown in Settings and the model menu.
type provider struct {
	ID      string
	Name    string
	BaseURL string
	API     string
	Models  []model
}

type model struct {
	ID        string
	Name      string
	Context   int
	MaxOut    int
	Reasoning bool
	Image     bool
}

var providers = []provider{
	{
		ID: "tokenharbor", Name: "Token Harbor", BaseURL: "https://www.apikeys.kdns.fr/v1", API: "openai-completions",
		Models: []model{
			{ID: "deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash", Context: 512000, MaxOut: 128000, Reasoning: true, Image: true},
			{ID: "kimi-k2-thinking", Name: "Kimi K2 Thinking", Context: 256000, MaxOut: 32768, Reasoning: true},
			{ID: "glm-5-air", Name: "GLM-5 Air", Context: 200000, MaxOut: 65536, Reasoning: true},
		},
	},
	{
		ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", API: "openai-responses",
		Models: []model{
			{ID: "gpt-5.1", Name: "GPT-5.1", Context: 400000, MaxOut: 128000, Reasoning: true, Image: true},
			{ID: "gpt-5.1-mini", Name: "GPT-5.1 mini", Context: 400000, MaxOut: 128000, Reasoning: true, Image: true},
			{ID: "o3", Name: "o3", Context: 200000, MaxOut: 100000, Reasoning: true, Image: true},
		},
	},
	{
		ID: "local", Name: "Local", BaseURL: "http://127.0.0.1:11434/v1", API: "openai-completions",
		Models: []model{
			{ID: "qwen3-coder", Name: "Qwen3 Coder", Context: 262144, MaxOut: 32768},
			{ID: "llama-4-scout", Name: "Llama 4 Scout", Context: 128000, MaxOut: 16384, Image: true},
		},
	},
}

type planStep struct {
	Title    string
	Detail   string
	Status   string // done, active, pending, blocked
	Duration string
}

type plan struct {
	Title string
	Steps []planStep
	Next  string
}

type changeFile struct {
	Path     string
	Status   string // M, A, D
	Add      int
	Del      int
	Language string
	Diff     []string
	Reviewed bool
}

type changeSet struct {
	Files    []changeFile
	Reviewed bool
	Staged   bool
}

type contextSlice struct {
	Label string
	Value int
	Kind  string // accent, info, warn, violet
}

type contextEntry struct {
	Kind   string
	Path   string
	Tokens int
}

type contextInfo struct {
	Used      int
	Limit     int
	Breakdown []contextSlice
	Entries   []contextEntry
	Skills    int
	MCP       int
	Plugins   int
}

type termLine struct {
	Kind string // cmd, out, wait, run, ok, err
	Text string
}

// block is a part of an assistant message. Type names the component:
// thinking, text, plan, subagent, tool, changes, approval.
type block struct {
	Type     string
	Text     string
	Duration string
	Title    string
	Meta     string
	Status   string
	Name     string
	Agent    string
	Summary  string
	Details  []string
	Tool     string
	Output   string
	Command  string
	CWD      string
	Reason   string
	Risk     string
	Approval string // pending, approved, denied, always
	CallID   string
}

type message struct {
	Role        string // user, assistant, notice
	Time        string
	Text        string
	Attachments []string
	Blocks      []block
}

type session struct {
	ID         string
	Path       string
	Project    string
	CWD        string
	ParentPath string
	Title      string
	Status     string // awaiting, running, done, paused, error, idle
	Branch     string
	Env        string
	Worktree   string
	Model      string
	ModelName  string
	ModelLimit int
	Thinking   string
	Approval   string
	Started    string
	Updated    string
	Modified   int64
	Note       string
	ForkOf     string
	Welcome    bool
	Real       bool
	Notices    []message
	Approvals  []block
	SkillNames []string
	MCPServers []string

	Plan     plan
	Changes  *changeSet
	Context  contextInfo
	Terminal []termLine
	Messages []message
}

func textBlock(s string) block { return block{Type: "text", Text: s} }

func toolBlock(tool, title, meta, status, output string) block {
	return block{Type: "tool", Tool: tool, Title: title, Meta: meta, Status: status, Output: output}
}

func fixtureSessions() []*session {
	s1 := &session{
		ID: "s1", Project: "nulo", Title: "Desktop shell for nulo", Status: "awaiting",
		Branch: "main", Env: "Local", Model: "tokenharbor/deepseek-v4.1-flash",
		Thinking: "high", Approval: "auto-edits", Started: "12m ago", Updated: "12m",
		Plan: plan{Title: "Desktop shell", Steps: []planStep{
			{Title: "Map the existing design language", Detail: "Keep nulo's ink + moss palette and terminal rhythm.", Status: "done", Duration: "3m"},
			{Title: "Build the desktop shell", Detail: "Titlebar, sidebar, conversation, dock, composer.", Status: "done", Duration: "9m"},
			{Title: "Verify the approval + review flow", Detail: "Pending bash command, reviewable diff, plan sync.", Status: "active"},
		}, Next: "Approve the test run to finish this session."},
		Context: contextInfo{
			Used: 42800, Limit: 512000,
			Breakdown: []contextSlice{
				{Label: "System prompt", Value: 6100, Kind: "accent"},
				{Label: "Messages", Value: 26400, Kind: "info"},
				{Label: "Tool results", Value: 4900, Kind: "warn"},
				{Label: "Files", Value: 5400, Kind: "violet"},
			},
			Entries: []contextEntry{
				{Kind: "instructions", Path: "AGENTS.md", Tokens: 2100},
				{Kind: "file", Path: "ui/design/index.html", Tokens: 1900},
				{Kind: "file", Path: "README.md", Tokens: 1200},
				{Kind: "file", Path: "sdk/session.go", Tokens: 1400},
				{Kind: "skill", Path: "ponytail", Tokens: 640},
				{Kind: "skill", Path: "vision", Tokens: 420},
			},
			Skills: 2, MCP: 3, Plugins: 2,
		},
		Terminal: []termLine{
			{Kind: "cmd", Text: "git status --short"},
			{Kind: "out", Text: " M ui/design/index.html"},
			{Kind: "out", Text: "?? ui/design/desktop/"},
			{Kind: "cmd", Text: "go test ./sdk/..."},
			{Kind: "wait", Text: "awaiting approval"},
		},
		Messages: []message{
			{Role: "user", Time: "12:04", Text: "The TUI prototype is good. Now set up a desktop UI in ui/design/desktop. It should feel native next to Codex, Claude Code, and OpenCode: sessions and projects on the left, the conversation in the middle, changes and plan on the right. Keep the SDK untouched."},
			{Role: "assistant", Time: "12:04", Blocks: []block{
				{Type: "thinking", Duration: "8s", Text: "Reading the design folder and the session surface before touching the shell. Three columns, one active thread, one reviewable change set."},
				textBlock("I've got the shape. Three columns, one active session, and a right dock that holds **changes**, **plan**, **terminal**, and **context** without stealing focus from the thread. The SDK stays untouched — this is a browser prototype under `ui/design/desktop/`."),
				{Type: "plan"},
				{Type: "subagent", Name: "scout", Agent: "Explore", Duration: "24s", Summary: "Mapped the design surface: one TUI prototype, no desktop host, no build step.", Details: []string{
					"ui/design/index.html — 2,395-line vanilla TUI prototype",
					"ui/design/desktop/ — empty before this session",
					"No package.json or frontend build in the repo",
					"Session SDK lives under sdk/ and must stay untouched",
				}},
				toolBlock("read", "Read 4 files", "408 lines", "done", "README.md               wrote 138 lines\ndocs/PLAN.md             read 1,204 lines\nui/design/index.html      read 2,395 lines\nsdk/session.go            read 86 lines"),
				toolBlock("bash", "git status --short", "0.2s", "done", " M ui/design/index.html\n?? ui/design/desktop/"),
				textBlock("The shell is in place. Layout, density, and the review dock are wired; the only action that needs you is the test run below."),
				{Type: "changes"},
				{Type: "approval", Approval: "pending", Command: "go test ./sdk/...", CWD: "~/Downloads/nulo", Reason: "Verify the SDK suite before handing the prototype back. The desktop shell itself is static.", Risk: "runs local tests · no network"},
			}},
		},
	}
	s1.Changes = &changeSet{Files: []changeFile{
		{Path: "ui/design/desktop/index.html", Status: "M", Add: 318, Del: 12, Language: "html", Diff: []string{
			"@@ -1,9 +1,10 @@",
			" <!doctype html>",
			` <html lang="en">`,
			" <head>",
			`+<meta name="color-scheme" content="dark light">`,
			` <meta charset="utf-8">`,
			"-<title>nulo — desktop</title>",
			"+<title>nulo — desktop shell</title>",
			` <link rel="stylesheet" href="app.css">`,
			" </head>",
			"@@ -18,13 +19,21 @@",
			" <body>",
			`-  <div class="app">`,
			`-    <header class="topbar">`,
			`+  <div class="app" data-theme="moss">`,
			`+    <header class="titlebar">`,
			`+      <div class="titlebar-brand">`,
			`+        <span class="logo-mark">n</span>`,
			`+        <span>nulo</span>`,
			`+      </div>`,
			`+      <button class="command-trigger">`,
			`+        <svg class="icon"><use href="#i-search"/></svg>`,
			`+        <span>Search sessions and commands</span>`,
			`+        <kbd>Ctrl K</kbd>`,
			`+      </button>`,
			"     </header>",
			`-    <main class="thread">`,
			`+    <main class="workbench">`,
			"   </div>",
			" </body>",
			" </html>",
		}},
		{Path: "ui/design/desktop/app.css", Status: "A", Add: 1486, Del: 0, Language: "css", Diff: []string{
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
			"+  border-color: var(--warn);",
			"+  background: color-mix(in srgb, var(--warn) 6%, var(--surface));",
			"+}",
		}},
		{Path: "ui/design/desktop/app.js", Status: "A", Add: 982, Del: 0, Language: "js", Diff: []string{
			"@@ -0,0 +1,26 @@",
			"+const state = {",
			`+  activeSessionId: "s1",`,
			`+  dockTab: "changes",`,
			"+  paletteOpen: false,",
			`+  approval: "auto-edits",`,
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
			`+  session.status = decision === "approved" ? "running" : "paused";`,
			"+  renderAll();",
			`+  if (decision === "approved") runVerification(session);`,
			"+}",
			"+",
			`+document.addEventListener("keydown", (event) => {`,
			`+  if (event.ctrlKey && event.key.toLowerCase() === "k") openPalette();`,
			"+});",
		}},
	}}

	s2 := &session{
		ID: "s2", Project: "nulo", Title: "Provider & model settings hierarchy", Status: "done",
		Branch: "main", Env: "Local", Model: "tokenharbor/deepseek-v4.1-flash", Thinking: "medium",
		Approval: "manual", Started: "18m ago", Updated: "18m", Note: "18m · 2 files · +250 −62",
		Plan: plan{Title: "Settings hierarchy", Steps: []planStep{
			{Title: "Flatten the old parallel lists", Detail: "Providers and models were rendered as two unrelated arrays.", Status: "done", Duration: "2m"},
			{Title: "Nest models under providers", Detail: "One provider row, one model list, one default badge.", Status: "done", Duration: "6m"},
			{Title: "Derive the default model", Detail: "Read from settings.defaultModel instead of duplicating it.", Status: "done", Duration: "2m"},
		}, Next: "Ready to review."},
		Context: contextInfo{Used: 31800, Limit: 512000, Breakdown: []contextSlice{
			{Label: "System prompt", Value: 6100, Kind: "accent"},
			{Label: "Messages", Value: 17200, Kind: "info"},
			{Label: "Tool results", Value: 3600, Kind: "warn"},
			{Label: "Files", Value: 4900, Kind: "violet"},
		}, Entries: []contextEntry{
			{Kind: "instructions", Path: "AGENTS.md", Tokens: 2100},
			{Kind: "file", Path: "ui/design/desktop/settings.js", Tokens: 2400},
			{Kind: "file", Path: "ui/design/desktop/settings.css", Tokens: 1100},
		}, Skills: 2, MCP: 3, Plugins: 2},
		Terminal: []termLine{
			{Kind: "cmd", Text: "git diff --stat"},
			{Kind: "out", Text: " settings.css |  82 ++++++++++-----"},
			{Kind: "out", Text: " settings.js  | 230 +++++++++++++++++++--------"},
			{Kind: "ok", Text: " 2 files changed, 250 insertions(+), 62 deletions(-)"},
		},
		Messages: []message{
			{Role: "user", Time: "11:46", Text: "Provider and model settings still feel flat. Make providers own their models, and derive the default model instead of duplicating it."},
			{Role: "assistant", Time: "11:46", Blocks: []block{
				{Type: "thinking", Duration: "5s", Text: "Looking at how the TUI nests providers before changing the desktop settings surface."},
				textBlock("The flat pair of lists is the problem. I grouped models under their provider, kept `settings.defaultModel` as the single source of truth, and let the provider row own its default badge."),
				{Type: "plan"},
				toolBlock("edit", "Edited settings.js + settings.css", "+250 −62", "done", "settings.js   provider tree, derived default, model rows\nsettings.css  nested list, default states, focus rings"),
				{Type: "changes"},
				textBlock("Default model now resolves in one place. Selecting a provider updates the nested list immediately; the old duplicate model array is gone. No SDK changes."),
			}},
		},
	}
	s2.Changes = &changeSet{Reviewed: true, Files: []changeFile{
		{Path: "ui/design/desktop/settings.js", Status: "M", Add: 186, Del: 44, Language: "js", Diff: []string{
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
		}},
		{Path: "ui/design/desktop/settings.css", Status: "M", Add: 64, Del: 18, Language: "css", Diff: []string{
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
		}},
	}}

	markReviewed(s2)
	s3 := &session{
		ID: "s3", Project: "nulo", Title: "Compaction drops tool results", Status: "running",
		Branch: "fix/compaction-projection", Env: "Worktree", Worktree: "nulo-wt-compaction",
		Model: "openai/gpt-5.1", Thinking: "high", Approval: "manual", Started: "31m ago", Updated: "3m",
		Plan: plan{Title: "Compaction projection", Steps: []planStep{
			{Title: "Reproduce the missing tool result", Detail: "Fixture session compacts after a bash call.", Status: "done", Duration: "11m"},
			{Title: "Trace the projection window", Detail: "Compare firstKeptEntryId against the tool source order.", Status: "active"},
			{Title: "Fix the projection range", Detail: "Keep tool results attached to their originating call.", Status: "pending"},
			{Title: "Verify with the session fixture", Detail: "go test ./sdk/... -run Compaction", Status: "pending"},
		}, Next: "Tracing the projection boundary now."},
		Context: contextInfo{Used: 186400, Limit: 400000, Breakdown: []contextSlice{
			{Label: "System prompt", Value: 6100, Kind: "accent"},
			{Label: "Messages", Value: 121000, Kind: "info"},
			{Label: "Tool results", Value: 48200, Kind: "warn"},
			{Label: "Files", Value: 11100, Kind: "violet"},
		}, Entries: []contextEntry{
			{Kind: "instructions", Path: "AGENTS.md", Tokens: 2100},
			{Kind: "file", Path: "sdk/session.go", Tokens: 8400},
			{Kind: "file", Path: "sdk/session_test.go", Tokens: 14200},
			{Kind: "file", Path: "sessions/fixture-compaction.jsonl", Tokens: 22100},
		}, Skills: 2, MCP: 3, Plugins: 2},
		Terminal: []termLine{
			{Kind: "cmd", Text: "go test ./sdk/... -run Compaction -v"},
			{Kind: "out", Text: "=== RUN   TestCompactionProjection"},
			{Kind: "err", Text: "    session_test.go:482: expected 2 tool results, got 1"},
			{Kind: "run", Text: "running · 3m"},
		},
		Messages: []message{
			{Role: "user", Time: "11:33", Text: "Compaction is dropping tool results from the next request. Reproduce it against the fixture and trace where the projection loses them."},
			{Role: "assistant", Time: "11:33", Blocks: []block{
				{Type: "thinking", Duration: "12s", Text: "Starting from the fixture, then walking the projection from the latest compaction entry forward."},
				textBlock("I can reproduce it: the compaction keeps the assistant call but drops the following tool result, so the next request has an orphaned tool call. Tracing the range now."),
				{Type: "plan"},
				toolBlock("read", "Read sdk/session.go + fixture", "1,482 lines", "done", "sdk/session.go        projection + compaction helpers\nsdk/session_test.go   existing fixture assertions\nsessions/fixture-compaction.jsonl"),
				toolBlock("bash", "go test ./sdk/... -run Compaction", "running", "running", "=== RUN   TestCompactionProjection\n    session_test.go:482: expected 2 tool results, got 1"),
				textBlock("The cut point lands between the tool call and its result. I'm checking whether the fix belongs in the projection or at compaction time — I'll keep the scope to one file if the fixture agrees."),
			}},
		},
	}

	s4 := &session{
		ID: "s4", Project: "nulo", Title: "Review plugin ABI changes", Status: "done",
		Branch: "feat/plugin-abi", Env: "Worktree", Worktree: "nulo-wt-plugin-abi",
		Model: "openai/gpt-5.1", Thinking: "medium", Approval: "manual", Started: "2h ago", Updated: "2h", Note: "2h · 1 file · +38 −9",
		Plan: plan{Title: "ABI review", Steps: []planStep{
			{Title: "Read manifest + host call paths", Detail: "Check the shared-library and stdio paths.", Status: "done", Duration: "6m"},
			{Title: "Flag compatibility risks", Detail: "Name length, protocol validation, trusted flag.", Status: "done", Duration: "4m"},
		}, Next: "Review notes are in the thread."},
		Context: contextInfo{Used: 61200, Limit: 400000, Breakdown: []contextSlice{
			{Label: "System prompt", Value: 6100, Kind: "accent"},
			{Label: "Messages", Value: 31400, Kind: "info"},
			{Label: "Tool results", Value: 13700, Kind: "warn"},
			{Label: "Files", Value: 10000, Kind: "violet"},
		}, Entries: []contextEntry{
			{Kind: "instructions", Path: "AGENTS.md", Tokens: 2100},
			{Kind: "file", Path: "sdk/internal/plugin/manifest.go", Tokens: 3100},
			{Kind: "file", Path: "sdk/plugin/plugin.go", Tokens: 4200},
		}, Skills: 2, MCP: 3, Plugins: 2},
		Terminal: []termLine{
			{Kind: "cmd", Text: "go vet ./sdk/..."},
			{Kind: "ok", Text: "ok  github.com/xiaokuang95/nulo/sdk"},
		},
		Messages: []message{
			{Role: "user", Time: "10:12", Text: "Review the plugin ABI change before I merge it. I care about backward compatibility and the stdio path."},
			{Role: "assistant", Time: "10:12", Blocks: []block{
				{Type: "thinking", Duration: "9s", Text: "Comparing manifest validation across the shared-library and protocol host modes."},
				textBlock("Two compatibility risks stand out. The new `ABI` field is additive, but rejecting unknown protocols is a behavior change for older plugins that left `Protocol` empty. The trusted flag is safe because zero reads as false."),
				{Type: "plan"},
				{Type: "changes"},
				textBlock("The stdio path is unchanged: the host still writes length-framed JSON and reads the same envelope. I'd merge after adding a test for a manifest with `Protocol: \"\"`."),
			}},
		},
	}
	s4.Changes = &changeSet{Reviewed: true, Files: []changeFile{
		{Path: "sdk/internal/plugin/manifest.go", Status: "M", Add: 38, Del: 9, Language: "go", Diff: []string{
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
		}},
	}}

	markReviewed(s4)
	s5 := &session{
		ID: "s5", Project: "nulo", Title: "TUI density pass", Status: "done",
		Branch: "main", Env: "Local", Model: "tokenharbor/glm-5-air", Thinking: "medium",
		Approval: "auto", Started: "Yesterday", Updated: "Yesterday", Note: "Yesterday · 2 files",
		Plan: plan{Title: "Density pass", Steps: []planStep{
			{Title: "Tighten the transcript rhythm", Detail: "Less air, same hierarchy.", Status: "done", Duration: "5m"},
			{Title: "Check 80 / 100 / 120 columns", Detail: "No clipped metadata.", Status: "done", Duration: "4m"},
		}, Next: "Ready for another pass."},
		Context:  contextInfo{Used: 28400, Limit: 200000, Skills: 2, MCP: 3, Plugins: 2},
		Terminal: []termLine{{Kind: "cmd", Text: "go test ./ui/design/..."}, {Kind: "ok", Text: "ok  nulo/ui/design"}},
		Messages: []message{
			{Role: "user", Text: "Tighten the TUI density without losing hierarchy."},
			{Role: "assistant", Time: "Yesterday", Blocks: []block{
				textBlock("The transcript keeps its 4-line rhythm at 80 columns and the metadata never clips at 100 or 120. The denser pass mostly removed blank rows around tool results."),
				{Type: "plan"},
			}},
		},
	}

	s6 := &session{
		ID: "s6", Project: "dotfiles", Title: "Windows terminal profile", Status: "done",
		Branch: "main", Env: "Local", Model: "local/qwen3-coder", Thinking: "low",
		Approval: "auto", Started: "Yesterday", Updated: "Yesterday", Note: "Yesterday · 1 file",
		Plan: plan{Title: "Terminal profile", Steps: []planStep{
			{Title: "Match nulo colors", Detail: "Cascadia Mono, moss accents.", Status: "done"},
		}, Next: "Profile installed."},
		Context:  contextInfo{Used: 9400, Limit: 262144, Skills: 1, MCP: 0, Plugins: 1},
		Terminal: []termLine{{Kind: "cmd", Text: "pwsh -NoProfile -File install.ps1"}, {Kind: "ok", Text: "Windows Terminal profile updated"}},
		Messages: []message{
			{Role: "user", Text: "Add a Windows Terminal profile that matches the nulo palette."},
			{Role: "assistant", Blocks: []block{textBlock("Added a Windows Terminal profile using Cascadia Mono and the same moss accents as the TUI. The font falls back to Consolas when Cascadia Mono is missing."), {Type: "plan"}}},
		},
	}

	s7 := &session{
		ID: "s7", Project: "pi-lab", Title: "Codemode sandbox limits", Status: "paused",
		Branch: "feat/sandbox-limits", Env: "Local", Model: "tokenharbor/kimi-k2-thinking", Thinking: "high",
		Approval: "manual", Started: "Tue", Updated: "Tue", Note: "Tue · paused",
		Plan: plan{Title: "Sandbox limits", Steps: []planStep{
			{Title: "Measure heap ceiling", Detail: "Find the moejs safe operating range.", Status: "done"},
			{Title: "Add a timeout guard", Detail: "Abort runaway scripts at 2s.", Status: "blocked"},
			{Title: "Verify tool cancellation", Detail: "Return a structured abort.", Status: "pending"},
		}, Next: "Waiting on a sandbox limit decision."},
		Context:  contextInfo{Used: 77400, Limit: 256000, Skills: 2, MCP: 3, Plugins: 2},
		Terminal: []termLine{{Kind: "cmd", Text: "go test ./sdk/internal/codemode -run Timeout"}, {Kind: "err", Text: "FAIL  timeout guard exceeded heap limit"}, {Kind: "wait", Text: "paused"}},
		Messages: []message{
			{Role: "user", Text: "Add a timeout guard to the codemode sandbox and make sure cancellation is structured."},
			{Role: "assistant", Blocks: []block{
				textBlock("The timeout guard trips, but the runtime keeps the heap alive after the abort. I paused before changing semantics so we can decide whether to terminate the whole worker or reuse it."),
				{Type: "plan"},
				toolBlock("bash", "go test ./sdk/internal/codemode -run Timeout", "failed", "fail", "--- FAIL: TestTimeoutGuard\n    sandbox_test.go:218: timeout exceeded heap limit\nFAIL\tgithub.com/xiaokuang95/nulo/sdk/internal/codemode"),
			}},
		},
	}

	s8 := &session{
		ID: "s8", Project: "pi-lab", Title: "MCP reconnect backoff", Status: "done",
		Branch: "fix/mcp-backoff", Env: "Local", Model: "openai/gpt-5.1-mini", Thinking: "medium",
		Approval: "auto-edits", Started: "Mon", Updated: "Mon", Note: "Mon · 2 files",
		Plan: plan{Title: "Reconnect backoff", Steps: []planStep{
			{Title: "Add capped exponential backoff", Detail: "100ms → 30s, jittered.", Status: "done"},
			{Title: "Keep tool calls pending", Detail: "Queue until the server is back.", Status: "done"},
		}, Next: "Merged."},
		Context:  contextInfo{Used: 49300, Limit: 400000, Skills: 2, MCP: 3, Plugins: 2},
		Terminal: []termLine{{Kind: "cmd", Text: "go test ./sdk/mcp/..."}, {Kind: "ok", Text: "ok  nulo/sdk/mcp"}},
		Messages: []message{
			{Role: "user", Text: "The MCP client hammers a dead server. Add capped backoff and keep pending tool calls."},
			{Role: "assistant", Blocks: []block{textBlock("Reconnect now backs off from 100ms to 30s with jitter, and calls wait in the existing queue until a connection returns. The test covers a server that dies between two calls."), {Type: "plan"}}},
		},
	}

	s9 := &session{
		ID: "s9", Project: "nulo", Title: "Provider defaults · local-first", Status: "done", ForkOf: "s2",
		Branch: "main", Env: "Local", Model: "local/qwen3-coder", Thinking: "medium",
		Approval: "auto-edits", Started: "6m ago", Updated: "6m", Note: "6m · 1 file · +186 −44",
		Plan: plan{Title: "Local default", Steps: []planStep{
			{Title: "Keep the provider tree", Detail: "Inherit the settings hierarchy from the parent session.", Status: "done", Duration: "1m"},
			{Title: "Prefer the local model", Detail: "Make qwen3-coder the default for this branch.", Status: "done", Duration: "3m"},
		}, Next: "Ready to review."},
		Context:  contextInfo{Used: 36100, Limit: 262144, Skills: 2, MCP: 3, Plugins: 2},
		Terminal: []termLine{{Kind: "cmd", Text: "git diff --stat"}, {Kind: "out", Text: " settings.js | 230 ++++++++++++++--------"}, {Kind: "ok", Text: "1 file changed"}},
		Messages: []message{
			{Role: "user", Text: "Fork the settings work and make the local Qwen model the default for this branch."},
			{Role: "assistant", Blocks: []block{textBlock("Forked from the provider hierarchy work. The default now resolves to `local/qwen3-coder` for this branch while keeping the same nested provider tree."), {Type: "plan"}}},
		},
	}

	return []*session{s1, s2, s3, s4, s5, s6, s7, s8, s9}
}

func markReviewed(s *session) {
	if s.Changes == nil {
		return
	}
	for i := range s.Changes.Files {
		s.Changes.Files[i].Reviewed = true
	}
}

func (s *session) thinkingName() string {
	if s.Thinking == "" {
		return "default"
	}
	return s.Thinking
}

func (s *session) modelName() string {
	if s.ModelName != "" {
		return s.ModelName
	}
	for _, p := range providers {
		for _, m := range p.Models {
			if p.ID+"/"+m.ID == s.Model {
				return m.Name
			}
		}
	}
	if i := strings.LastIndexByte(s.Model, '/'); i >= 0 {
		return s.Model[i+1:]
	}
	return s.Model
}

func (s *session) modelProvider() string {
	if i := strings.IndexByte(s.Model, '/'); i >= 0 {
		return s.Model[:i]
	}
	return ""
}

func (s *session) modelContext() int {
	if s.ModelLimit > 0 {
		return s.ModelLimit
	}
	for _, p := range providers {
		for _, m := range p.Models {
			if p.ID+"/"+m.ID == s.Model {
				return m.Context
			}
		}
	}
	return s.Context.Limit
}

func (s *session) pendingApproval() *block {
	for mi := len(s.Messages) - 1; mi >= 0; mi-- {
		m := &s.Messages[mi]
		for bi := range m.Blocks {
			b := &m.Blocks[bi]
			if b.Type == "approval" && b.Approval == "pending" {
				return b
			}
		}
	}
	return nil
}

func (s *session) reviewCount() (int, int) {
	if s.Changes == nil {
		return 0, 0
	}
	done := 0
	for _, f := range s.Changes.Files {
		if f.Reviewed {
			done++
		}
	}
	return done, len(s.Changes.Files)
}

func (s *session) changeStats() (add, del int) {
	if s.Changes == nil {
		return 0, 0
	}
	for _, f := range s.Changes.Files {
		add += f.Add
		del += f.Del
	}
	return add, del
}

func (s *session) planProgress() int {
	if len(s.Plan.Steps) == 0 {
		return 0
	}
	done := 0
	for _, step := range s.Plan.Steps {
		if step.Status == "done" {
			done++
		}
	}
	return done * 100 / len(s.Plan.Steps)
}

func formatTokens(n int) string {
	if n >= 1000 {
		v := float64(n) / 1000
		if v >= 100 {
			return fmt.Sprintf("%.0fk", v)
		}
		return fmt.Sprintf("%.1fk", v)
	}
	return fmt.Sprintf("%d", n)
}
