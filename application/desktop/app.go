package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

const (
	sidebarWidth = float32(288)
	dockWidthMin = float32(320)
	dockWidthMax = float32(620)
)

type app struct {
	win       *mygo.Window
	sessions  []*session
	activeID  string
	projects  []projectGroup
	providers []provider
	host      *desktopHost
	hostErr   string
	fixture   bool

	paper bool

	showTimestamps   bool
	collapseThinking bool
	compactDensity   bool
	notifyDone       bool

	sidebarOpen   bool
	sidebarDrawer bool
	dockOpen      bool
	dockDrawer    bool
	dockTab       int
	dockWidth     float32
	dockSized     bool
	mainViewWidth float32
	filter        int // All, Active, Review
	search        string
	focusSearch   bool

	draft         string
	attachments   []string
	showPerm      bool
	showEnv       bool
	showModel     bool
	modelQuery    string
	slashOpen     bool
	mentionOpen   bool
	lastDraft     string
	slashIndex    int
	mentionIndex  int
	pendingToast  string
	focusComposer bool
	md            markdown.Options

	threadScroll    ui.ScrollState
	scrollToEnd     bool
	selectedFile    map[string]int
	commentFile     string
	commentRow      int
	commentLabel    string
	commentText     string
	commentKey      string
	collapsedPlan   map[string]bool
	collapsedGroups map[string]bool
	terminals       map[string]*desktopTerminal
	terminalErrs    map[string]string
	fileIndices     map[string]workspaceFileIndex
	imagePreviews   map[string]cachedImagePreview
	providerTests   map[string]string
	terminalTabs    map[string]string
	terminalSeq     int64
	focusTerminalID string
	shuttingDown    bool

	paletteOpen             bool
	paletteQuery            string
	paletteIndex            int
	settingsOpen            bool
	settingsTab             int
	commitOpen              bool
	commitMsg               string
	renameOpen              bool
	renameID                string
	renameValue             string
	rewindOpen              bool
	rewindSessionID         string
	rewindPoints            []rewindPoint
	rewindIndex             int
	rewindRestoreFiles      bool
	messageEditID           string
	messageEditIndex        int
	messageEditSessionID    string
	deleteOpen              bool
	deleteID                string
	confirmOpen             bool
	confirmTitle            string
	confirmDetail           string
	confirmLabel            string
	confirmAction           func()
	imageOpen               bool
	imagePath               string
	cropMode                bool
	cropDragging            bool
	cropStartX              float32
	cropStartY              float32
	cropEndX                float32
	cropEndY                float32
	imageRotation           int
	imageFlipX              bool
	imageFlipY              bool
	imageEditBitmap         *ui.Bitmap
	imageAnnotate           bool
	imageAnnotDrawing       bool
	imageStrokes            []imageStroke
	imageAnnotVersion       int
	imageAnnotCachedVer     int
	imageAnnotBitmap        *ui.Bitmap
	imageAnnotW             int
	imageAnnotH             int
	configOpen              bool
	configTitle             string
	configPath              string
	configText              string
	configHelp              string
	configAfter             func() error
	keyOpen                 bool
	keyProvider             string
	keyName                 string
	keyValue                string
	mcpFormOpen             bool
	mcpFormAdd              bool
	mcpFormTarget           string
	mcpFormName             string
	mcpFormOriginal         string
	mcpFormDescription      string
	mcpFormCommand          string
	mcpFormArgs             string
	mcpFormURL              string
	mcpFormEnv              string
	mcpFormHeaders          string
	mcpFormEnabled          bool
	mcpFormError            string
	mcpFormAdvanced         string
	mcpFormSessionID        string
	skillFormOpen           bool
	skillFormTarget         string
	skillFormName           string
	skillFormDescription    string
	skillFormBody           string
	skillFormEdit           bool
	skillFormPath           string
	skillFormError          string
	skillFormSessionID      string
	providerFormOpen        bool
	providerFormAdd         bool
	providerFormID          string
	providerFormName        string
	providerFormBaseURL     string
	providerFormAPI         string
	providerFormAPIKey      string
	providerFormError       string
	providerFormAdvanced    string
	providerFormSessionID   string
	modelFormOpen           bool
	modelFormAdd            bool
	modelFormProvider       string
	modelFormID             string
	modelFormName           string
	modelFormContext        string
	modelFormMaxOut         string
	modelFormReasoning      bool
	modelFormImage          bool
	modelFormError          string
	modelFormAdvanced       string
	modelFormSessionID      string
	mergeConflictOpen       bool
	mergeConflictSessionID  string
	mergeConflictRoot       string
	mergeConflictBranch     string
	mergeConflictFiles      []string
	mergeEditorOpen         bool
	mergeEditorPath         string
	mergeEditorTab          int
	mergeEditorBase         string
	mergeEditorOurs         string
	mergeEditorTheirs       string
	mergeEditorResult       string
	mergeEditorWorking      string
	mergeEditorOursExists   bool
	mergeEditorTheirsExists bool
	mergeEditorBaseExists   bool
	mergeEditorBinary       bool
	mergeEditorChosen       bool
	mergeEditorDelete       bool
	mergeEditorError        string

	lastWidth     float32
	dockDragStart float32
}

func newFixtureApp() *app {
	a := baseApp()
	a.fixture = true
	a.projects = append([]projectGroup(nil), projectDefs...)
	a.sessions = fixtureSessions()
	a.activeID = "s1"
	return a
}

func newApp() *app { return newAppWithHost(hostOptions{}) }

func newAppWithHost(opts hostOptions) *app {
	a := baseApp()
	a.startSDKHost(opts)
	return a
}

func baseApp() *app {
	return &app{
		providers:        append([]provider(nil), providers...),
		showTimestamps:   true,
		collapseThinking: true,
		notifyDone:       true,
		commentRow:       -1,
		sidebarOpen:      true,
		dockOpen:         true,
		dockWidth:        420,
		selectedFile:     make(map[string]int),
		collapsedPlan:    make(map[string]bool),
		collapsedGroups:  make(map[string]bool),
		terminals:        make(map[string]*desktopTerminal),
		terminalTabs:     make(map[string]string),
		fileIndices:      make(map[string]workspaceFileIndex),
		providerTests:    make(map[string]string),
		scrollToEnd:      true,
	}
}

func (a *app) projectByKey(key string) projectGroup {
	for _, g := range a.projects {
		if g.Key == key {
			return g
		}
	}
	name := strings.ToUpper(key)
	if name == "" {
		name = "WORKSPACE"
	}
	return projectGroup{Key: key, Name: name, Letter: name[:1], Tint: "green", Branch: "main"}
}

func (a *app) projectDisplayName(key string) string {
	return strings.ToLower(a.projectByKey(key).Name)
}

func (a *app) active() *session {
	for _, s := range a.sessions {
		if s.ID == a.activeID {
			return s
		}
	}
	if len(a.sessions) > 0 {
		a.activeID = a.sessions[0].ID
		return a.sessions[0]
	}
	return nil
}

func (a *app) selectSession(id string) {
	if a.activeID == id {
		return
	}
	if a.host != nil {
		if s := a.sessionByID(id); s != nil {
			if _, err := a.host.ensureEngine(a, s); err != nil {
				a.pendingToast = "Could not load session: " + err.Error()
			}
		}
	}
	a.activeID = id
	a.scrollToEnd = true
	a.sidebarDrawer = false
	a.dockTab = 0
	a.showPerm, a.showEnv, a.showModel = false, false, false
	if a.messageEditID != "" {
		a.cancelMessageEdit(true)
	}
	if a.commentRow >= 0 {
		a.cancelLineComment()
	}
}

func (a *app) fitDockWidth(w float32) {
	if a.dockSized {
		return
	}
	switch {
	case w < 1240:
		a.dockWidth = 320
	case w < 1360:
		a.dockWidth = 360
	default:
		a.dockWidth = 420
	}
}

func (a *app) toggleSidebar() {
	if a.lastWidth > 0 && a.lastWidth < 900 {
		a.sidebarDrawer = !a.sidebarDrawer
		if a.sidebarDrawer {
			a.dockDrawer = false
		}
		return
	}
	a.sidebarOpen = !a.sidebarOpen
}

func (a *app) toggleDock() {
	if a.lastWidth > 0 && a.lastWidth < 1180 {
		a.dockDrawer = !a.dockDrawer
		if a.dockDrawer {
			a.sidebarDrawer = false
		}
		return
	}
	a.dockOpen = !a.dockOpen
}

func (a *app) openDock(tab int) {
	a.dockTab = tab
	a.dockOpen = true
	if tab == 2 && a.active() != nil {
		a.focusTerminalID = a.active().ID
	}
	if a.lastWidth > 0 && a.lastWidth < 1180 {
		a.dockDrawer = true
		a.sidebarDrawer = false
	}
}

func (a *app) closeDock() {
	a.dockOpen = false
	a.dockDrawer = false
}

func (a *app) newSession() {
	g := a.defaultProject()
	if active := a.active(); active != nil {
		g = a.projectByKey(active.Project)
	}
	a.newSessionFor(g)
}

func (a *app) defaultProject() projectGroup {
	if len(a.projects) > 0 {
		return a.projects[0]
	}
	return projectGroup{Key: "workspace", Name: "WORKSPACE", Letter: "W", Tint: "green", Branch: "main"}
}

func (a *app) newSessionFor(g projectGroup) {
	if a.host != nil && g.CWD != "" {
		saveWorkspace(g.CWD)
	}
	if a.host != nil {
		s, err := a.host.newSession(a, g)
		if err != nil {
			a.pendingToast = "New session failed: " + err.Error()
			return
		}
		a.activatePrepared(s, g)
		return
	}
	s := &session{
		ID:       a.nextID("new"),
		Project:  g.Key,
		Title:    "Untitled session",
		Status:   "idle",
		Branch:   g.Branch,
		Env:      "Local",
		Model:    "tokenharbor/deepseek-v4.1-flash",
		Thinking: "high",
		Approval: "auto-edits",
		Started:  "now",
		Updated:  "now",
		Welcome:  true,
		Context:  contextInfo{Limit: 512000, Skills: 2, MCP: 3, Plugins: 2},
	}
	a.sessions = append([]*session{s}, a.sessions...)
	a.activeID = s.ID
	a.filter = 0
	a.search = ""
	a.draft = ""
	a.attachments = nil
	a.slashOpen, a.mentionOpen = false, false
	a.cancelMessageEdit(false)
	a.sidebarDrawer = false
	delete(a.collapsedGroups, g.Key)
	a.openDock(3)
	a.scrollToEnd = true
	a.focusComposer = true
}

func (a *app) activatePrepared(s *session, g projectGroup) {
	a.activeID = s.ID
	a.filter = 0
	a.search = ""
	a.draft = ""
	a.attachments = nil
	a.slashOpen, a.mentionOpen = false, false
	a.cancelMessageEdit(false)
	a.sidebarDrawer = false
	delete(a.collapsedGroups, g.Key)
	a.openDock(3)
	a.scrollToEnd = true
	a.focusComposer = true
}

func (a *app) openWorkspace(path string) {
	if a.host == nil || path == "" {
		return
	}
	saveWorkspace(path)
	g := a.host.ensureProject(a, path, false)
	var latest *session
	for _, s := range a.sessions {
		if s.Project != g.Key {
			continue
		}
		if latest == nil || s.Modified > latest.Modified {
			latest = s
		}
	}
	if latest != nil {
		a.selectSession(latest.ID)
		return
	}
	a.newSessionFor(g)
}

func (a *app) pickProject() {
	if a.win == nil || a.host == nil {
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.win, Title: "Open project", ButtonLabel: "Open", Directory: true,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	a.openWorkspace(paths[0])
}

func (a *app) nextID(prefix string) string {
	for n := 1; ; n++ {
		id := fmt.Sprintf("%s-%d", prefix, n)
		found := false
		for _, s := range a.sessions {
			if s.ID == id {
				found = true
				break
			}
		}
		if !found {
			return id
		}
	}
}

func (a *app) beginDelete(s *session) {
	if s == nil || len(a.sessions) <= 1 {
		return
	}
	a.deleteID = s.ID
	a.deleteOpen = true
}

func (a *app) deleteSession(id string) {
	if len(a.sessions) <= 1 {
		return
	}
	remove := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, s := range a.sessions {
			if s.ForkOf != "" && remove[s.ForkOf] && !remove[s.ID] {
				remove[s.ID] = true
				changed = true
			}
		}
	}
	activeRemoved := false
	out := a.sessions[:0]
	for _, s := range a.sessions {
		if !remove[s.ID] {
			out = append(out, s)
			continue
		}
		activeRemoved = activeRemoved || a.activeID == s.ID
		a.closeTerminal(s.ID)
		delete(a.terminalTabs, s.ID)
		delete(a.selectedFile, s.ID)
		if a.host != nil {
			if s.Status == "running" || s.Status == "awaiting" {
				a.host.abort(s)
			}
			if err := a.host.deleteSession(a, s); err != nil {
				a.pendingToast = "Delete: " + err.Error()
			}
		}
	}
	a.sessions = out
	if activeRemoved || a.active() == nil {
		if len(a.sessions) > 0 {
			a.activeID = a.sessions[0].ID
			if a.host != nil {
				if _, err := a.host.ensureEngine(a, a.sessions[0]); err != nil {
					a.pendingToast = "Load: " + err.Error()
				}
			}
		} else if a.host != nil {
			a.newSessionFor(a.defaultProject())
		}
		a.scrollToEnd = true
	}
}

func (a *app) send(raw string) {
	s := a.active()
	if s == nil {
		return
	}
	text := strings.TrimSpace(raw)
	if text == "" {
		return
	}
	if a.editingMessage(s) {
		a.submitMessageEdit(s, text)
		return
	}
	cmd := ""
	if strings.HasPrefix(text, "/") {
		cmd = strings.Fields(text)[0]
	}
	attachments := append([]string(nil), a.attachments...)
	a.showPerm, a.showEnv, a.showModel = false, false, false
	if cmd != "" {
		a.draft = ""
		a.attachments = nil
	}
	switch cmd {
	case "/plan":
		a.openDock(1)
		return
	case "/diff", "/review":
		a.openDock(0)
		return
	case "/context":
		a.openDock(3)
		return
	case "/terminal":
		a.openDock(2)
		return
	case "/model":
		a.showModel = true
		a.modelQuery = ""
		return
	case "/settings":
		a.settingsOpen = true
		return
	case "/compact":
		a.compact()
		return
	case "/approve":
		if b := s.pendingApproval(); b != nil {
			a.resolveApproval(s, b, "approved")
		}
		return
	case "/clear":
		if a.host != nil {
			a.newSessionFor(a.projectByKey(s.Project))
			return
		}
		s.Messages = nil
		a.scrollToEnd = true
		return
	}

	if a.host != nil && s.Real {
		if err := a.host.send(a, s, text, attachments); err != nil {
			a.pendingToast = "Send failed: " + err.Error()
			return
		}
		a.draft = ""
		a.attachments = nil
		return
	}
	a.draft = ""
	a.attachments = nil

	if s.Welcome {
		s.Welcome = false
		s.Title = titleFor(text)
		s.Status = "running"
		s.Plan = plan{Title: "Working plan", Steps: []planStep{
			{Title: "Understand the request", Detail: "Read the prompt and the current workspace.", Status: "done", Duration: "1m"},
			{Title: "Inspect the relevant code", Detail: "Find the files that own the behavior.", Status: "active"},
			{Title: "Implement and verify", Detail: "Make the change, then run the checks.", Status: "pending"},
		}, Next: "Looking through the workspace."}
		s.Changes = nil
	}
	s.Messages = append(s.Messages, message{Role: "user", Time: "now", Text: text, Attachments: attachments})
	s.Messages = append(s.Messages, message{Role: "assistant", Time: "now", Blocks: []block{
		{Type: "thinking", Duration: "3s", Text: "Reading the request and checking the session state before proposing the next step."},
		{Type: "plan"},
		textBlock("I've added that to the session. This native shell keeps the same approval and review loop as the design: changes land in the right dock, risky commands wait in the thread, and the plan stays in sync."),
	}})
	for i := range s.Plan.Steps {
		if s.Plan.Steps[i].Status != "blocked" {
			s.Plan.Steps[i].Status = "done"
		}
	}
	if s.pendingApproval() != nil {
		s.Status = "awaiting"
	} else {
		s.Status = "done"
		s.Plan.Next = "Response ready. Review it or ask a follow-up."
	}
	a.scrollToEnd = true
}

func (a *app) stopSession(s *session) {
	if s == nil {
		return
	}
	if a.host != nil && s.Real {
		a.host.abort(s)
		s.Status = "paused"
		a.pendingToast = "Stopped the running turn"
		return
	}
	if s.Status != "running" {
		return
	}
	s.Status = "paused"
	s.Plan.Next = "Stopped by you. Resume with a new message."
	s.Terminal = append(s.Terminal, termLine{Kind: "wait", Text: "stopped by user"})
	a.toast("Stopped the running turn")
}

func (a *app) rewindSession(s *session) {
	if s == nil {
		return
	}
	var points []rewindPoint
	if a.host != nil && s.Real {
		e, err := a.host.ensureEngine(a, s)
		if err != nil || e == nil {
			if err != nil {
				a.pendingToast = "Rewind: " + err.Error()
			}
			return
		}
		branch := e.sess.Branch()
		for i := len(branch) - 1; i >= 0; i-- {
			entry := branch[i]
			if entry.Type != "message" || entry.Message == nil || entry.Message.Role != "user" {
				continue
			}
			text := strings.TrimSpace(entry.Message.ContentString())
			if text == "" {
				text = "user message"
			}
			target := ""
			if entry.ParentID != nil {
				target = *entry.ParentID
			}
			snapshotKey, files, note, hasSnapshot := findCheckpointSnapshot(a.host.agentDir, s.ID, branch, target)
			points = append(points, rewindPoint{
				Label:         "Before: " + titleSnippet(text),
				Detail:        fmt.Sprintf("%s · %d entries after", messageTime(entry.Message.Timestamp), len(branch)-1-i),
				EntryID:       target,
				SnapshotKey:   snapshotKey,
				Snapshot:      hasSnapshot,
				SnapshotFiles: files,
				SnapshotNote:  note,
			})
		}
	} else {
		for i, m := range s.Messages {
			if m.Role != "user" {
				continue
			}
			points = append([]rewindPoint{{
				Label:        "Before: " + titleSnippet(m.Text),
				Detail:       fmt.Sprintf("%s · %d messages after", m.Time, len(s.Messages)-i),
				MessageIndex: i,
			}}, points...)
		}
	}
	if len(points) == 0 {
		a.pendingToast = "No checkpoints yet · send a message first"
		return
	}
	a.rewindSessionID = s.ID
	a.rewindPoints = points
	a.rewindIndex = 0
	a.rewindRestoreFiles = points[0].Snapshot
	a.rewindOpen = true
}

func titleSnippet(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if len(text) > 72 {
		text = text[:69] + "…"
	}
	return text
}

func (a *app) confirmRewind() {
	if !a.rewindOpen || len(a.rewindPoints) == 0 {
		return
	}
	var s *session
	for _, candidate := range a.sessions {
		if candidate.ID == a.rewindSessionID {
			s = candidate
			break
		}
	}
	if s == nil {
		a.rewindOpen = false
		return
	}
	if a.rewindIndex < 0 || a.rewindIndex >= len(a.rewindPoints) {
		return
	}
	point := a.rewindPoints[a.rewindIndex]
	a.rewindOpen = false
	if a.host != nil && s.Real {
		restoreFiles := a.rewindRestoreFiles && point.Snapshot
		if err := a.host.rewindTo(a, s, point.EntryID, point.Label, restoreFiles, point.SnapshotKey); err != nil {
			a.pendingToast = "Rewind: " + err.Error()
			return
		}
		a.scrollToEnd = true
		a.pendingToast = "Rewound to checkpoint"
		return
	}
	if point.MessageIndex < 0 || point.MessageIndex > len(s.Messages) {
		return
	}
	removed := len(s.Messages) - point.MessageIndex
	s.Messages = s.Messages[:point.MessageIndex]
	s.Status = "idle"
	s.Plan.Next = "Rewound to the checkpoint before your last message."
	a.pendingToast = fmt.Sprintf("Rewound %d message(s)", removed)
	a.scrollToEnd = true
}

func (a *app) beginRename(s *session) {
	if s == nil {
		return
	}
	a.renameID = s.ID
	a.renameValue = s.Title
	a.renameOpen = true
}

func (a *app) finishRename() {
	title := strings.TrimSpace(a.renameValue)
	if title == "" {
		a.renameOpen = false
		return
	}
	if a.host != nil {
		if s := a.sessionByID(a.renameID); s != nil {
			if err := a.host.rename(a, s, title); err != nil {
				a.pendingToast = "Rename: " + err.Error()
			} else {
				s.Updated = "now"
				a.pendingToast = "Renamed session"
			}
		}
		a.renameOpen = false
		return
	}
	for _, s := range a.sessions {
		if s.ID == a.renameID {
			s.Title = title
			s.Updated = "now"
			a.pendingToast = "Renamed session"
			break
		}
	}
	a.renameOpen = false
}

func titleFor(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if text == "" {
		return "Untitled session"
	}
	r := []rune(text)
	if len(r) > 46 {
		text = string(r[:43]) + "…"
	}
	return text
}

func (a *app) resolveApproval(s *session, b *block, decision string) {
	if b.Approval != "pending" {
		return
	}
	if a.host != nil && b.CallID != "" {
		a.host.resolveApproval(a, s, b, decision)
		if decision == "approved" || decision == "always" {
			a.pendingToast = "Approved"
		} else {
			a.pendingToast = "Denied. The tool call is blocked."
		}
		return
	}
	b.Approval = decision
	switch decision {
	case "denied":
		s.Status = "paused"
		s.Plan.Next = "The command was denied; the session is paused."
		s.Terminal = append(s.Terminal, termLine{Kind: "err", Text: "denied · " + b.Command})
		a.toast("Command denied. The session is paused.")
	case "always":
		s.Approval = "auto"
		b.Approval = "approved"
		s.Status = "done"
		completeApprovalPlan(s)
		s.Terminal = append(s.Terminal, termLine{Kind: "out", Text: "ok  github.com/xiaokuang95/nulo/sdk"}, termLine{Kind: "ok", Text: "tests passed · 4.8s"})
		a.toast("Allowed every time. Tests passed.")
	case "approved":
		s.Status = "done"
		completeApprovalPlan(s)
		s.Terminal = append(s.Terminal, termLine{Kind: "out", Text: "ok  github.com/xiaokuang95/nulo/sdk"}, termLine{Kind: "ok", Text: "tests passed · 4.8s"})
		a.toast("Approved. Tests passed.")
	}
	a.openDock(0)
	a.scrollToEnd = true
}

func completeApprovalPlan(s *session) {
	for i := range s.Plan.Steps {
		if s.Plan.Steps[i].Status == "active" {
			s.Plan.Steps[i].Status = "done"
		}
	}
	s.Plan.Next = "All steps are complete. Review the changes."
	s.Context.Used = min(s.Context.Used+1800, s.Context.Limit)
}

func (a *app) compact() {
	s := a.active()
	if s == nil {
		return
	}
	if a.host != nil && s.Real {
		a.host.compact(a, s)
		a.openDock(3)
		return
	}
	before := s.Context.Used
	s.Context.Used = min(s.Context.Used, 21000)
	s.Messages = append(s.Messages, message{
		Role: "notice", Time: "now",
		Text: fmt.Sprintf("Context compacted · %s → %s tokens", formatTokens(before), formatTokens(s.Context.Used)),
	})
	a.toast("Context compacted")
	a.scrollToEnd = true
	a.openDock(3)
}

func (a *app) toast(s string) {
	a.pendingToast = s
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) selectedFileIndex(s *session) int {
	if s == nil || s.Changes == nil || len(s.Changes.Files) == 0 {
		return 0
	}
	i := a.selectedFile[s.ID]
	if i < 0 || i >= len(s.Changes.Files) {
		i = 0
		a.selectedFile[s.ID] = 0
	}
	return i
}

func (a *app) setTheme(paper bool) {
	a.paper = paper
	if a.win != nil {
		bg := "#0a0a0b"
		if paper {
			bg = "#faf9f5"
		}
		_ = a.win.SetBackgroundColor(bg)
	}
	theme := a.terminalTheme()
	for _, terminal := range a.terminals {
		if terminal.term != nil {
			terminal.term.SetTheme(theme, theme)
		}
	}
}

func (a *app) visibleSessions() []*session {
	q := strings.ToLower(strings.TrimSpace(a.search))
	out := make([]*session, 0, len(a.sessions))
	for _, s := range a.sessions {
		if !a.matchesFilter(s) {
			continue
		}
		if q != "" {
			hay := strings.ToLower(a.sessionSearchText(s))
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

func (a *app) sessionSearchText(s *session) string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(s.Title)
	b.WriteByte(' ')
	b.WriteString(s.Project)
	b.WriteByte(' ')
	b.WriteString(s.Branch)
	b.WriteByte(' ')
	b.WriteString(s.Note)
	for _, m := range s.Messages {
		b.WriteByte(' ')
		b.WriteString(m.Text)
		for _, block := range m.Blocks {
			b.WriteByte(' ')
			b.WriteString(block.Text)
			b.WriteByte(' ')
			b.WriteString(block.Title)
			b.WriteByte(' ')
			b.WriteString(block.Output)
			b.WriteByte(' ')
			b.WriteString(block.Command)
		}
	}
	return b.String()
}

func (a *app) matchesFilter(s *session) bool {
	switch a.filter {
	case 1:
		return s.Status == "running" || s.Status == "awaiting"
	case 2:
		return s.pendingApproval() != nil || (s.Changes != nil && !s.Changes.Reviewed)
	}
	return true
}

func (a *app) filterCounts() (all, active, review int) {
	all = len(a.sessions)
	for _, s := range a.sessions {
		if s.Status == "running" || s.Status == "awaiting" {
			active++
		}
		if s.pendingApproval() != nil || (s.Changes != nil && !s.Changes.Reviewed) {
			review++
		}
	}
	return
}

func statusLabel(s string) string {
	switch s {
	case "awaiting":
		return "Needs approval"
	case "running":
		return "Running"
	case "done":
		return "Done"
	case "paused":
		return "Paused"
	case "error":
		return "Error"
	case "idle":
		return "Ready"
	}
	return s
}

func approvalLabel(s string) string {
	switch s {
	case "auto-edits":
		return "Auto edits"
	case "manual":
		return "Manual"
	case "plan":
		return "Plan only"
	case "auto":
		return "Auto"
	}
	return s
}

func (a *app) cycleApproval() {
	s := a.active()
	if s == nil {
		return
	}
	order := []string{"manual", "auto-edits", "plan", "auto"}
	next := order[0]
	for i, mode := range order {
		if mode == s.Approval {
			next = order[(i+1)%len(order)]
			break
		}
	}
	s.Approval = next
	a.pendingToast = "Permission · " + approvalLabel(next)
}

func (a *app) openPalette() {
	a.paletteOpen = true
	a.paletteQuery = ""
	a.paletteIndex = 0
}

type paletteItem struct {
	Title  string
	Detail string
	Key    string
	Icon   string
	Run    func()
}

func (a *app) paletteItems() []paletteItem {
	items := []paletteItem{
		{Title: "New session", Detail: "Start a fresh thread", Key: "⌘N", Icon: "plus", Run: a.newSession},
		{Title: "Toggle sidebar", Detail: "Sessions and projects", Key: "⌘B", Icon: "panel-left", Run: a.toggleSidebar},
		{Title: "Toggle review dock", Detail: "Changes, plan, terminal, context", Key: "⌘J", Icon: "panel-right", Run: a.toggleDock},
		{Title: "Open project folder", Detail: "Add a workspace to the sidebar", Key: "⌘O", Icon: "folder", Run: a.pickProject},
		{Title: "Settings", Detail: "Models, appearance, shortcuts", Key: "⌘,", Icon: "settings", Run: func() { a.settingsOpen = true }},
		{Title: "Open changes", Detail: "Review the active workspace", Key: "⇧⌘D", Icon: "diff", Run: func() { a.openDock(0) }},
		{Title: "Open plan", Detail: "Steps for this session", Key: "", Icon: "list-checks", Run: func() { a.openDock(1) }},
		{Title: "Open terminal", Detail: "Session shell output", Key: "", Icon: "terminal", Run: func() { a.openDock(2) }},
		{Title: "Compact context", Detail: "Drop stale tool results", Key: "", Icon: "compress", Run: a.compact},
		{Title: "Use Moss theme", Detail: "Dark ink and green", Key: "", Icon: "moon", Run: func() { a.setTheme(false) }},
		{Title: "Use Paper theme", Detail: "Warm light surface", Key: "", Icon: "sun", Run: func() { a.setTheme(true) }},
	}
	for _, s := range a.sessions {
		ss := s
		items = append(items, paletteItem{
			Title: s.Title, Detail: strings.ToUpper(s.Project) + " · " + statusLabel(s.Status), Key: s.Updated,
			Icon: "message", Run: func() { a.selectSession(ss.ID) },
		})
	}
	q := strings.ToLower(strings.TrimSpace(a.paletteQuery))
	if q == "" {
		return items
	}
	var out []paletteItem
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Title+" "+it.Detail), q) {
			out = append(out, it)
		}
	}
	return out
}

func (a *app) choosePalette(i int) {
	items := a.paletteItems()
	if len(items) == 0 {
		return
	}
	i = max(0, min(i, len(items)-1))
	a.paletteOpen = false
	items[i].Run()
}
