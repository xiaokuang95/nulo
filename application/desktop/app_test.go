package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func newTester() (*app, *ui.Tester) {
	a := newFixtureApp()
	return a, ui.NewTester(a.view, 1440, 900)
}

func TestApprovalRunsTestsAndCompletesPlan(t *testing.T) {
	a, tt := newTester()
	if !tt.HasText("Needs approval") || !tt.HasText("Run this command?") {
		t.Fatalf("initial thread missing approval: %q", tt.Texts())
	}
	if err := tt.Click("Allow once"); err != nil {
		t.Fatal(err)
	}
	if got := a.active().Status; got != "done" {
		t.Errorf("status = %q, want done", got)
	}
	if !tt.HasText("Approved · tests passed in 4.8s") {
		t.Errorf("approved result missing: %q", tt.Texts())
	}
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), "All steps are complete") {
		t.Errorf("plan did not complete: %q", tt.Texts())
	}
	term := ""
	for _, line := range a.active().Terminal {
		term += line.Text + "\n"
	}
	if !strings.Contains(term, "tests passed · 4.8s") {
		t.Errorf("terminal not updated: %q", term)
	}
}

func TestDenyPausesSession(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Deny"); err != nil {
		t.Fatal(err)
	}
	if got := a.active().Status; got != "paused" {
		t.Errorf("status = %q, want paused", got)
	}
	if !tt.HasText("Denied · the session is paused") {
		t.Errorf("denied result missing: %q", tt.Texts())
	}
}

func TestSessionSwitchUpdatesThreadAndDock(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Compaction drops tool results"); err != nil {
		t.Fatal(err)
	}
	if a.activeID != "s3" {
		t.Fatalf("active = %q, want s3", a.activeID)
	}
	if !tt.HasText("Running") || !tt.HasText("Compaction is dropping tool results from the next request. Reproduce it against the fixture and trace where the projection loses them.") {
		t.Errorf("thread not switched: %q", tt.Texts())
	}
	if !tt.HasText("Tracing the projection boundary now.") {
		t.Errorf("plan not switched: %q", tt.Texts())
	}
}

func TestDockTabsPlanContextTerminal(t *testing.T) {
	_, tt := newTester()
	if err := tt.Click("Plan"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("STEPS") || !tt.HasText("Verify the approval + review flow") {
		t.Fatalf("plan tab missing content: %q", tt.Texts())
	}
	if err := tt.Click("Context"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("context used") || !tt.HasText("AGENTS.md") {
		t.Fatalf("context tab missing content: %q", tt.Texts())
	}
	if err := tt.Click("Terminal"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("awaiting approval") {
		t.Fatalf("terminal tab missing approval line: %q", tt.Texts())
	}
}

func TestCommandPaletteOpensAndCloses(t *testing.T) {
	_, tt := newTester()
	tt.Key(ui.Cmd, ui.KeyK)
	if !tt.HasText("Search sessions, actions, and settings") {
		t.Fatalf("palette did not open: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	if tt.HasText("Search sessions, actions, and settings") {
		t.Fatalf("palette did not close: %q", tt.Texts())
	}
}

func TestNewSessionAndWelcome(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("New session"); err != nil {
		t.Fatal(err)
	}
	if !a.active().Welcome || !tt.HasText("What should we build?") {
		t.Fatalf("welcome state missing: %q", tt.Texts())
	}
	if err := tt.Click("Review uncommitted changes"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.draft, "Review the uncommitted changes") {
		t.Errorf("suggestion did not fill composer: %q", a.draft)
	}
}

func TestReviewFileAndMarkAll(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Mark all reviewed"); err != nil {
		t.Fatal(err)
	}
	reviewed, total := a.active().reviewCount()
	if reviewed != total || total != 3 {
		t.Errorf("reviewed = %d/%d, want 3/3", reviewed, total)
	}
	if !tt.HasText("3/3 reviewed") {
		t.Errorf("review count did not update: %q", tt.Texts())
	}
}

func TestSettingsThemeSwitch(t *testing.T) {
	a, tt := newTester()
	tt.Key(ui.Cmd, ui.KeyComma)
	if !tt.HasText("Appearance") {
		t.Fatalf("settings did not open: %q", tt.Texts())
	}
	if err := tt.Click("Appearance"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Paper"); err != nil {
		t.Fatal(err)
	}
	if !a.paper {
		t.Errorf("theme did not switch to paper")
	}
}

func TestNarrowDockDrawer(t *testing.T) {
	a := newFixtureApp()
	tt := ui.NewTester(a.view, 1000, 800)
	tt.SetTitleBar(ui.TitleBar{})
	if err := tt.Click("Changes"); err != nil {
		t.Fatal(err)
	}
	if !a.dockDrawer {
		t.Fatalf("dock drawer did not open")
	}
	if !tt.HasText("3 files changed") {
		t.Errorf("drawer missing changes: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	if a.dockDrawer {
		t.Errorf("escape did not close dock drawer")
	}
}

func TestNarrowSidebarDrawer(t *testing.T) {
	a := newFixtureApp()
	tt := ui.NewTester(a.view, 880, 760)
	tt.SetTitleBar(ui.TitleBar{})
	if err := tt.Click("Toggle sidebar · ⌘B"); err != nil {
		t.Fatal(err)
	}
	if !a.sidebarDrawer {
		t.Fatalf("sidebar drawer did not open")
	}
	if err := tt.Click("Compaction drops tool results"); err != nil {
		t.Fatal(err)
	}
	if a.sidebarDrawer || a.activeID != "s3" {
		t.Errorf("drawer = %v, active = %q; want closed drawer and s3", a.sidebarDrawer, a.activeID)
	}
}

func TestSlashMenuOpensAndRunsPlan(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Message nulo"); err != nil {
		t.Fatal(err)
	}
	tt.Type("/")
	if !tt.HasText("COMMANDS") || !tt.HasText("/plan") {
		t.Fatalf("slash menu missing: %q", tt.Texts())
	}
	tt.Type("pl")
	if err := tt.Click("/plan"); err != nil {
		t.Fatal(err)
	}
	if a.dockTab != 1 || !a.dockOpen {
		t.Errorf("slash /plan did not open plan: tab=%d open=%v", a.dockTab, a.dockOpen)
	}
}

func TestMentionInsertsFile(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Message nulo"); err != nil {
		t.Fatal(err)
	}
	tt.Type("@main")
	if !tt.HasText("FILES") {
		t.Fatalf("mention menu missing: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	if !strings.Contains(a.draft, "@application/desktop/main.go") {
		t.Errorf("mention not inserted: %q", a.draft)
	}
}

func TestComposerSendsMessage(t *testing.T) {
	a, tt := newTester()
	before := len(a.active().Messages)
	if err := tt.Click("Message nulo"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Summarize the current change")
	tt.Key(0, ui.KeyEnter)
	if a.draft != "" {
		t.Errorf("draft = %q, want empty", a.draft)
	}
	if len(a.active().Messages) != before+2 {
		t.Errorf("messages = %d, want %d", len(a.active().Messages), before+2)
	}
	if !tt.HasText("Summarize the current change") {
		t.Errorf("sent message missing")
	}
}

func TestPaletteKeyboardRunsAction(t *testing.T) {
	a, tt := newTester()
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("paper")
	tt.Key(0, ui.KeyEnter)
	if !a.paper {
		t.Errorf("command palette did not switch to paper")
	}
	if a.paletteOpen {
		t.Errorf("palette did not close")
	}
}

func TestPermissionPopover(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Auto edits"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("PERMISSION MODE") || !tt.HasText("Plan only") {
		t.Fatalf("permission popover missing: %q", tt.Texts())
	}
	if err := tt.Click("Plan only"); err != nil {
		t.Fatal(err)
	}
	if a.active().Approval != "plan" || a.showPerm {
		t.Errorf("approval = %q, open = %v", a.active().Approval, a.showPerm)
	}
}

func TestModelPopover(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Switch model"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("MODEL") || !tt.HasText("Kimi K2 Thinking") {
		t.Fatalf("model popover missing: %q", tt.Texts())
	}
	if err := tt.Click("Kimi K2 Thinking"); err != nil {
		t.Fatal(err)
	}
	if a.active().Model != "tokenharbor/kimi-k2-thinking" {
		t.Errorf("model = %q", a.active().Model)
	}
}

func TestReviewCheckbox(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Mark ui/design/desktop/index.html reviewed"); err != nil {
		t.Fatal(err)
	}
	reviewed, total := a.active().reviewCount()
	if reviewed != 1 || total != 3 {
		t.Errorf("reviewed = %d/%d, want 1/3", reviewed, total)
	}
}

func TestCommitFlow(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Stage all"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Commit…"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Finish the native shell")
	if err := tt.Click("Commit"); err != nil {
		t.Fatal(err)
	}
	if a.active().Changes != nil {
		t.Errorf("changes not cleared after commit")
	}
	if !tt.HasText("Committed · Finish the native shell") {
		t.Errorf("commit notice missing: %q", tt.Texts())
	}
}

func TestSendCompletesTurn(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("New session"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Message nulo"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Add a compact mode")
	tt.Key(0, ui.KeyEnter)
	if got := a.active().Status; got != "done" {
		t.Errorf("status = %q, want done", got)
	}
	if done := countDone(a.active().Plan.Steps); done != len(a.active().Plan.Steps) {
		t.Errorf("plan = %d/%d done", done, len(a.active().Plan.Steps))
	}
}

func TestStopRunningSession(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Compaction drops tool results"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Stop generation"); err != nil {
		t.Fatal(err)
	}
	if got := a.active().Status; got != "paused" {
		t.Errorf("status = %q, want paused", got)
	}
}

func TestRewindRemovesLastTurn(t *testing.T) {
	a, tt := newTester()
	before := len(a.active().Messages)
	if err := tt.Click("Rewind to checkpoint"); err != nil {
		t.Fatal(err)
	}
	if !a.rewindOpen || !tt.HasText("Rewind to checkpoint") {
		t.Fatalf("rewind dialog missing: %q", tt.Texts())
	}
	if err := tt.Click("Restore checkpoint"); err != nil {
		t.Fatal(err)
	}
	after := len(a.active().Messages)
	if after >= before {
		t.Errorf("messages = %d, want fewer than %d", after, before)
	}
	if got := a.active().Status; got != "idle" {
		t.Errorf("status = %q, want idle", got)
	}
}

func TestRenameSessionFromMenu(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Session actions"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Menu(); len(got) == 0 {
		t.Fatalf("session menu did not open")
	}
	if err := tt.ChooseMenuItem("Rename…"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Session name"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Desktop review")
	if err := tt.Click("Rename"); err != nil {
		t.Fatal(err)
	}
	if got := a.active().Title; got != "Desktop review" {
		t.Errorf("title = %q", got)
	}
}

func TestAttachmentDropAndSend(t *testing.T) {
	a, tt := newTester()
	if !tt.DropFiles(650, 850, "/tmp/notes.md") {
		t.Fatalf("drop not accepted")
	}
	if len(a.attachments) != 1 || a.attachments[0] != "/tmp/notes.md" {
		t.Fatalf("attachments = %v", a.attachments)
	}
	if !tt.HasText("notes.md") {
		t.Errorf("attachment tray missing: %q", tt.Texts())
	}
	if err := tt.Click("Message nulo"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Review the attachment")
	tt.Key(0, ui.KeyEnter)
	if len(a.attachments) != 0 {
		t.Errorf("attachments not cleared: %v", a.attachments)
	}
	msgs := a.active().Messages
	if len(msgs) < 2 || len(msgs[len(msgs)-2].Attachments) != 1 {
		t.Errorf("sent message missing attachment: %+v", msgs)
	}
}

func TestDeleteSessionFromMenu(t *testing.T) {
	a, tt := newTester()
	before := len(a.sessions)
	if err := tt.Click("Session actions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Delete session"); err != nil {
		t.Fatal(err)
	}
	if !a.deleteOpen {
		t.Fatal("delete confirmation did not open")
	}
	if err := tt.Click("Delete session"); err != nil {
		t.Fatal(err)
	}
	if got := len(a.sessions); got != before-1 {
		t.Errorf("sessions = %d, want %d", got, before-1)
	}
	if a.activeID == "s1" {
		t.Errorf("deleted session is still active")
	}
}

func TestForkFromSessionMenu(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Session actions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Fork from here"); err != nil {
		t.Fatal(err)
	}
	if a.active() == nil || a.active().ForkOf != "s1" {
		t.Fatalf("active fork = %+v", a.active())
	}
	if !strings.HasPrefix(a.active().Title, "Desktop shell for nulo") {
		t.Errorf("fork title = %q", a.active().Title)
	}
}

func TestSidebarToggle(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Toggle sidebar · ⌘B"); err != nil {
		t.Fatal(err)
	}
	if a.sidebarOpen {
		t.Fatalf("sidebarOpen = true after collapse")
	}
	tt.Frame()
	if tt.HasText("New session") {
		t.Errorf("sidebar still rendered after collapse")
	}
	if err := tt.Click("Toggle sidebar · ⌘B"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.sidebarOpen || !tt.HasText("New session") {
		t.Errorf("sidebar did not reopen")
	}
}

func TestCollapseProjectGroup(t *testing.T) {
	_, tt := newTester()
	if err := tt.Click("NULO"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Provider & model settings hierarchy") {
		t.Errorf("NULO sessions still visible after collapse")
	}
	if err := tt.Click("NULO"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Provider & model settings hierarchy") {
		t.Errorf("NULO sessions did not return")
	}
}

func TestSidebarDrawerToggleAtNarrowWidth(t *testing.T) {
	a := newFixtureApp()
	tt := ui.NewTester(a.view, 880, 760)
	tt.SetTitleBar(ui.TitleBar{})
	if err := tt.Click("Toggle sidebar · ⌘B"); err != nil {
		t.Fatal(err)
	}
	if !a.sidebarDrawer {
		t.Fatalf("sidebar drawer did not open")
	}
	if err := tt.Click("Toggle sidebar · ⌘B"); err != nil {
		t.Fatal(err)
	}
	if a.sidebarDrawer {
		t.Errorf("sidebar drawer did not close")
	}
	tt.Frame()
	if tt.HasText("New session") {
		t.Errorf("sidebar still rendered after drawer close")
	}
}

func TestDrawerBackdropClose(t *testing.T) {
	a := newFixtureApp()
	tt := ui.NewTester(a.view, 880, 760)
	tt.SetTitleBar(ui.TitleBar{})
	if err := tt.Click("Toggle sidebar · ⌘B"); err != nil {
		t.Fatal(err)
	}
	tt.ClickAt(600, 300)
	if a.sidebarDrawer {
		t.Errorf("sidebar drawer stayed open after backdrop click")
	}
}

func TestDockDrawerToggleAtNarrowWidth(t *testing.T) {
	a := newFixtureApp()
	tt := ui.NewTester(a.view, 1000, 800)
	tt.SetTitleBar(ui.TitleBar{})
	if err := tt.Click("Toggle right panel · ⌘J"); err != nil {
		t.Fatal(err)
	}
	if !a.dockDrawer {
		t.Fatalf("dock drawer did not open")
	}
	if err := tt.Click("Toggle right panel · ⌘J"); err != nil {
		t.Fatal(err)
	}
	if a.dockDrawer {
		t.Errorf("dock drawer did not close from titlebar")
	}
}

func TestCollapseProjectGroupUnderFilters(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Active"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("NULO"); err != nil {
		t.Fatal(err)
	}
	if !a.collapsedGroups["nulo"] {
		t.Fatalf("NULO did not collapse under Active")
	}
	if tt.HasText("Compaction drops tool results") {
		t.Errorf("Active sessions still visible in collapsed NULO")
	}
	if err := tt.Click("Review"); err != nil {
		t.Fatal(err)
	}
	if !a.collapsedGroups["nulo"] {
		t.Errorf("collapse did not persist into Review")
	}
	if err := tt.Click("NULO"); err != nil {
		t.Fatal(err)
	}
	if a.collapsedGroups["nulo"] {
		t.Errorf("NULO did not expand under Review")
	}
}

func TestNewSessionInProject(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("New session in DOTFILES"); err != nil {
		t.Fatal(err)
	}
	s := a.active()
	if s == nil || s.Project != "dotfiles" || s.Branch != "main" {
		t.Fatalf("active = %+v, want dotfiles/main", s)
	}
	if !s.Welcome || !strings.HasPrefix(s.ID, "new-") {
		t.Errorf("welcome=%v id=%q", s.Welcome, s.ID)
	}
	if !tt.HasText("What should we build?") {
		t.Errorf("welcome screen missing")
	}
	if !tt.HasText("DOTFILES DESKTOP") || !tt.HasText("dotfiles / main") {
		t.Errorf("welcome did not follow the project: %q", tt.Texts())
	}
}

func TestNewSessionInPiLabUsesProjectBranch(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("New session in PI-LAB"); err != nil {
		t.Fatal(err)
	}
	if got := a.active().Branch; got != "experiment/codemode" {
		t.Errorf("branch = %q", got)
	}
}

func TestTopNewSessionFollowsActiveProject(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Windows terminal profile"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("New session"); err != nil {
		t.Fatal(err)
	}
	if got := a.active().Project; got != "dotfiles" {
		t.Errorf("project = %q, want dotfiles", got)
	}
}

func TestProjectNewExpandsGroupAndClearsFilters(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Review"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("New session in NULO"); err != nil {
		t.Fatal(err)
	}
	if a.filter != 0 || a.search != "" {
		t.Errorf("filter=%d search=%q, want all and empty", a.filter, a.search)
	}
	if a.collapsedGroups["nulo"] {
		t.Errorf("target group stayed collapsed")
	}
}

func TestDeleteSessionRemovesForkDescendants(t *testing.T) {
	a := baseApp()
	a.sessions = []*session{
		{ID: "root", Title: "root"},
		{ID: "child", Title: "child", ForkOf: "root"},
		{ID: "grand", Title: "grand", ForkOf: "child"},
		{ID: "other", Title: "other"},
	}
	a.activeID = "root"
	if got := a.sessionDescendantCount("root"); got != 2 {
		t.Fatalf("descendants = %d, want 2", got)
	}
	a.deleteSession("root")
	if len(a.sessions) != 1 || a.sessions[0].ID != "other" {
		t.Fatalf("sessions after delete = %+v", a.sessions)
	}
	if a.activeID != "other" {
		t.Fatalf("active = %q", a.activeID)
	}
}

func TestEscapeStopsRunningSession(t *testing.T) {
	a, tt := newTester()
	s := a.active()
	s.Status = "running"
	if err := tt.Click("Message nulo"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	if s.Status != "paused" {
		t.Fatalf("status = %q, want paused", s.Status)
	}
}

func TestSettingsCapabilitiesWithHost(t *testing.T) {
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	a := newAppWithHost(hostOptions{Workspace: t.TempDir(), AgentDir: t.TempDir(), Trusted: true, Sync: true})
	defer a.closeHost()
	a.settingsOpen = true
	a.settingsTab = 2
	tt := ui.NewTester(a.view, 1440, 900)
	tt.Frame()
	if !tt.HasText("MCP servers") || !tt.HasText("Skill folders") {
		t.Fatalf("capabilities did not render: %q", tt.Texts())
	}
	if err := tt.Click("Add"); err != nil {
		t.Fatal(err)
	}
	if !a.mcpFormOpen || !tt.HasText("Add MCP server") {
		t.Fatalf("MCP form did not open: %q", tt.Texts())
	}
	a.mcpFormOpen = false
	tt.Frame()
	if err := tt.Click("New"); err != nil {
		t.Fatal(err)
	}
	if !a.skillFormOpen || !tt.HasText("New skill") {
		t.Fatalf("skill form did not open: %q", tt.Texts())
	}
	a.skillFormOpen = false
	tt.Frame()
	if err := tt.Click("Global"); err != nil {
		t.Fatal(err)
	}
	if !a.configOpen || !strings.HasSuffix(a.configPath, "mcp.json") {
		t.Fatalf("config editor = open:%v path:%q", a.configOpen, a.configPath)
	}
}

func TestSettingsProviderFormsWithHost(t *testing.T) {
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	a := newAppWithHost(hostOptions{Workspace: t.TempDir(), AgentDir: t.TempDir(), Trusted: true, Sync: true})
	defer a.closeHost()
	a.settingsOpen = true
	a.settingsTab = 1
	tt := ui.NewTester(a.view, 1440, 900)
	tt.Frame()
	if err := tt.Click("Add provider"); err != nil {
		t.Fatal(err)
	}
	if !a.providerFormOpen || !tt.HasText("Provider ID") {
		t.Fatalf("provider form did not open: %q", tt.Texts())
	}
	a.providerFormOpen = false
	tt.Frame()
	if err := tt.Click("Add model"); err != nil {
		t.Fatal(err)
	}
	if !a.modelFormOpen || !tt.HasText("Model ID") {
		t.Fatalf("model form did not open: %q", tt.Texts())
	}
}

func TestVisibleSessionsSearchesMessageContent(t *testing.T) {
	a := baseApp()
	first := &session{ID: "a", Title: "Alpha", Status: "done"}
	second := &session{ID: "b", Title: "Beta", Status: "done", Messages: []message{
		{Role: "user", Text: "plain"},
		{Role: "assistant", Blocks: []block{{Type: "tool", Title: "Search", Command: "rg needle", Output: "found in transcript"}}},
	}}
	a.sessions = []*session{first, second}
	a.filter = 0
	a.search = "transcript"
	got := a.visibleSessions()
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("visible = %+v", got)
	}
	a.search = "not-present"
	if got := a.visibleSessions(); len(got) != 0 {
		t.Fatalf("unexpected matches = %+v", got)
	}
}

func TestModelSearchFiltersPopover(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Switch model"); err != nil {
		t.Fatal(err)
	}
	tt.Type("llama")
	if !tt.HasText("Llama 4 Scout") || tt.HasText("Kimi K2 Thinking") {
		t.Fatalf("filtered models = %q", tt.Texts())
	}
	if err := tt.Click("Llama 4 Scout"); err != nil {
		t.Fatal(err)
	}
	if a.active().Model != "local/llama-4-scout" {
		t.Fatalf("model = %q", a.active().Model)
	}
}

func TestDiffLineReviewNote(t *testing.T) {
	a, tt := newTester()
	if err := tt.Click("Review note on line 4"); err != nil {
		t.Fatal(err)
	}
	tt.Type("keep this branch")
	if err := tt.Click("Add note"); err != nil {
		t.Fatal(err)
	}
	files := a.active().Changes.Files
	found := false
	for _, file := range files {
		for _, comment := range file.Comments {
			if comment.Text == "keep this branch" && comment.Row >= 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("review note missing: %+v", files)
	}
}

func TestSlashCommandsOpenDockModelAndSettings(t *testing.T) {
	a, _ := newTester()
	a.send("/terminal")
	if a.dockTab != 2 || !a.dockOpen {
		t.Fatalf("terminal command: tab=%d open=%v", a.dockTab, a.dockOpen)
	}
	a.send("/settings")
	if !a.settingsOpen {
		t.Fatal("settings command did not open settings")
	}
	a.settingsOpen = false
	a.send("/model")
	if !a.showModel || a.modelQuery != "" {
		t.Fatalf("model command: open=%v query=%q", a.showModel, a.modelQuery)
	}
	a.showModel = false
	a.send("/review")
	if a.dockTab != 0 || !a.dockOpen {
		t.Fatalf("review command: tab=%d open=%v", a.dockTab, a.dockOpen)
	}
}

func TestRewindDialogShowsFileSnapshot(t *testing.T) {
	a, tt := newTester()
	a.rewindOpen = true
	a.rewindSessionID = a.activeID
	a.rewindPoints = []rewindPoint{{
		Label: "Before: test", Detail: "now · 2 entries after", EntryID: "e1",
		SnapshotKey: "e1", Snapshot: true, SnapshotFiles: 3,
	}}
	a.rewindIndex = 0
	a.rewindRestoreFiles = true
	tt.Frame()
	if !tt.HasText("Restore 3 changed file(s) from this checkpoint") {
		t.Fatalf("snapshot option missing: %q", tt.Texts())
	}
	if err := tt.Click("Restore 3 changed file(s) from this checkpoint"); err != nil {
		t.Fatal(err)
	}
	if a.rewindRestoreFiles {
		t.Fatal("snapshot checkbox did not toggle")
	}
}
