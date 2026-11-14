package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	mygoterm "github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

const (
	terminalModeShell = "shell"
	terminalModeAgent = "agent"
)

type desktopTerminal struct {
	term   *mygoterm.Terminal
	cwd    string
	title  string
	exited bool
	code   int
	token  int64
}

func (a *app) terminalMode(s *session) string {
	if s == nil {
		return terminalModeShell
	}
	if a.terminalTabs[s.ID] == terminalModeAgent {
		return terminalModeAgent
	}
	return terminalModeShell
}

func (a *app) setTerminalMode(s *session, mode string) {
	if s == nil {
		return
	}
	if a.terminalTabs == nil {
		a.terminalTabs = make(map[string]string)
	}
	if mode == terminalModeAgent {
		a.terminalTabs[s.ID] = terminalModeAgent
	} else {
		delete(a.terminalTabs, s.ID)
	}
}

func (a *app) terminalTheme() *mygoterm.Theme {
	p := a.pal()
	theme := mygoterm.DarkTheme()
	if !p.dark {
		theme = mygoterm.LightTheme()
	}
	theme.Foreground = p.text
	theme.Background = p.codeBg
	theme.Cursor = p.accent
	theme.CursorText = p.bg
	theme.Selection = p.accent.Alpha(0.28)
	theme.SelectionText = p.text
	return theme
}

func (a *app) ensureTerminal(s *session) (*desktopTerminal, error) {
	if s == nil || !s.Real || a.host == nil {
		return nil, errors.New("terminal needs a real session")
	}
	if strings.TrimSpace(s.CWD) == "" {
		return nil, errors.New("session has no working directory")
	}
	if a.terminals == nil {
		a.terminals = make(map[string]*desktopTerminal)
	}
	if existing := a.terminals[s.ID]; existing != nil {
		if existing.cwd == s.CWD {
			return existing, nil
		}
		a.closeTerminal(s.ID)
	}
	key := s.ID
	a.terminalSeq++
	token := a.terminalSeq
	state := &desktopTerminal{cwd: s.CWD, title: "shell", token: token}
	term, err := mygoterm.New(mygoterm.Options{
		Dir:              s.CWD,
		ShellIntegration: true,
		Font: mygoterm.Font{
			Family:     "monospace",
			Size:       11.5,
			LineHeight: 1.05,
		},
		Theme:   a.terminalTheme(),
		Padding: mygoterm.Padding{X: 4, Y: 2},
		OnTitle: func(title string) {
			a.updateTerminal(key, token, func(t *desktopTerminal) {
				if strings.TrimSpace(title) != "" {
					t.title = strings.TrimSpace(title)
				}
			})
		},
		OnExit: func(code int) {
			a.updateTerminal(key, token, func(t *desktopTerminal) {
				t.exited = true
				t.code = code
			})
		},
	})
	if err != nil {
		return nil, err
	}
	state.term = term
	a.terminals[key] = state
	return state, nil
}

func (a *app) updateTerminal(key string, token int64, fn func(*desktopTerminal)) {
	if a.shuttingDown {
		return
	}
	update := func() {
		if terminal := a.terminals[key]; terminal != nil && terminal.token == token {
			fn(terminal)
		}
		if a.win != nil {
			a.win.Invalidate()
		}
	}
	if a.win == nil {
		update()
		return
	}
	mygo.RunOnMain(update)
}

func (a *app) restartTerminal(s *session) {
	if s == nil {
		return
	}
	a.closeTerminal(s.ID)
	a.setTerminalMode(s, terminalModeShell)
	if _, err := a.ensureTerminal(s); err != nil {
		a.pendingToast = "Terminal: " + err.Error()
		return
	}
	a.focusTerminalID = s.ID
	a.pendingToast = "Shell restarted"
}

func (a *app) closeTerminal(sessionID string) {
	terminal := a.terminals[sessionID]
	if terminal == nil {
		return
	}
	delete(a.terminals, sessionID)
	delete(a.terminalErrs, sessionID)
	if terminal.term != nil {
		_ = terminal.term.Close()
	}
}

func (a *app) closeTerminals() {
	for id := range a.terminals {
		a.closeTerminal(id)
	}
}

func (a *app) terminalToolbar(c *ui.Context, s *session, mode string, state *desktopTerminal) {
	p := a.pal()
	ui.Row(c).FillWidth().Height(28).Gap(7).AlignItems(ui.Center).Children(func() {
		a.icon(c, "terminal", 12, p.accent2)
		location := s.CWD
		if state != nil && state.term != nil {
			if dir := state.term.Dir(); dir != "" {
				location = dir
			}
		}
		if location == "" {
			location = "workspace"
		}
		ui.Text(c, location).Font("monospace").FontSize(10).TextColor(p.text4).Grow(1).SingleLine()
		if state != nil && state.exited {
			label := fmt.Sprintf("exited %d", state.code)
			if state.code == 0 {
				label = "exited"
			}
			ui.Text(c, label).Font("monospace").FontSize(9).TextColor(chooser(state.code == 0, p.text4, p.bad))
		} else if state != nil && state.title != "" {
			ui.Text(c, state.title).Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
		}
		if a.smallButton(c, "Clear").Clicked() {
			if mode == terminalModeAgent {
				s.Terminal = nil
			} else if state != nil && state.term != nil {
				state.term.ClearScreen()
			}
		}
		if mode == terminalModeShell && a.smallButton(c, "Restart").Clicked() {
			a.restartTerminal(s)
		}
	})
}

func (a *app) terminalTabsRow(c *ui.Context, s *session, mode string) {
	p := a.pal()
	ui.Row(c).FillWidth().Height(26).Gap(4).AlignItems(ui.Center).Children(func() {
		for _, tab := range []struct{ id, label string }{
			{terminalModeShell, "Shell"},
			{terminalModeAgent, "Agent"},
		} {
			item := tab
			selected := mode == item.id
			b := ui.ButtonBase(c).Height(24).Padding(0, 8).Gap(4).Radius(5).Label(item.label)
			if selected {
				b.Background(p.accentSoft)
			} else if b.Hovered() {
				b.Background(p.surface2)
			}
			if b.Clicked() {
				a.setTerminalMode(s, item.id)
				if item.id == terminalModeShell {
					a.focusTerminalID = s.ID
				}
			}
			b.Children(func() {
				color := p.text3
				if selected {
					color = p.accent
				}
				ui.Text(c, item.label).Font("monospace").FontSize(10).TextColor(color)
			})
		}
		ui.Spacer(c)
		if mode == terminalModeShell {
			ui.Text(c, "click the terminal to type").Font("monospace").FontSize(9).TextColor(p.text4)
		}
	})
}

func (a *app) dockRealTerminal(c *ui.Context, s *session) {
	mode := a.terminalMode(s)
	var state *desktopTerminal
	var termErr error
	if mode == terminalModeShell {
		if cached := a.terminalErrs[s.ID]; cached != "" {
			termErr = errors.New(cached)
		} else {
			state, termErr = a.ensureTerminal(s)
			if termErr != nil {
				if a.terminalErrs == nil {
					a.terminalErrs = make(map[string]string)
				}
				a.terminalErrs[s.ID] = termErr.Error()
			}
		}
	}
	ui.Column(c).FillWidth().FillHeight().Gap(7).Children(func() {
		a.terminalToolbar(c, s, mode, state)
		a.terminalTabsRow(c, s, mode)
		if mode == terminalModeAgent {
			a.dockAgentOutput(c, s)
			return
		}
		if state == nil {
			detail := "The terminal library could not start."
			if termErr != nil {
				detail = termErr.Error()
			}
			a.dockTerminalError(c, "Shell unavailable", detail)
			return
		}
		view := mygoterm.View(c, state.term).FillWidth().Grow(1).MinHeight(120)
		if a.focusTerminalID == s.ID {
			view.Focus()
			a.focusTerminalID = ""
		}
	})
}

func (a *app) dockTerminalError(c *ui.Context, title, detail string) {
	p := a.pal()
	box := ui.Column(c).FillWidth().Grow(1).MinHeight(100).Padding(16).Gap(6).
		Radius(8).Border(1, p.line).Background(p.surface.Alpha(0.5)).AlignItems(ui.Center).Center()
	box.Children(func() {
		a.icon(c, "alert", 20, p.warn)
		ui.Text(c, title).FontSize(13).FontWeight(550).TextColor(p.text2)
		ui.Text(c, detail).FontSize(11).TextColor(p.text4).TextAlign(ui.Center)
	})
}

func (a *app) dockAgentOutput(c *ui.Context, s *session) {
	p := a.pal()
	scroll := ui.Scroll(c).FillWidth().Grow(1).MinHeight(0)
	scroll.Children(func() {
		box := ui.Column(c).FillWidth().Padding(9).Gap(3).Radius(8).Border(1, p.line).
			Background(p.codeBg)
		box.Children(func() {
			if len(s.Terminal) == 0 {
				ui.Text(c, "No agent tool output in this session.").Font("monospace").FontSize(11).TextColor(p.text4)
				return
			}
			for _, line := range s.Terminal {
				a.termLine(c, line)
			}
		})
	})
}

func (a *app) dockTerminal(c *ui.Context, s *session) {
	if a.host != nil && s.Real {
		a.dockRealTerminal(c, s)
		return
	}
	a.dockMockTerminal(c, s)
}

func (a *app) dockMockTerminal(c *ui.Context, s *session) {
	p := a.pal()
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		toolbar := ui.Row(c).FillWidth().Height(30).AlignItems(ui.Center).Gap(7)
		toolbar.Children(func() {
			a.icon(c, "terminal", 13, p.accent2)
			location := "pty · ~/Downloads/nulo"
			if s.CWD != "" {
				location = "shell · " + s.CWD
			}
			ui.Text(c, location).Font("monospace").FontSize(10).TextColor(p.text4).Grow(1).SingleLine()
			if a.smallButton(c, "Clear").Clicked() {
				s.Terminal = nil
			}
		})
		term := ui.Column(c).FillWidth().Clip().Radius(8).Border(1, p.line).
			Background(p.codeBg).Padding(12, 12, 13, 12).Gap(3)
		term.Children(func() {
			if len(s.Terminal) == 0 {
				ui.Text(c, "No output in this session.").Font("monospace").FontSize(11).TextColor(p.text4)
			}
			for _, line := range s.Terminal {
				a.termLine(c, line)
			}
		})
		ui.Row(c).FillWidth().Height(32).Padding(0, 9).Gap(7).Radius(7).Border(1, p.line).
			Background(p.surface.Alpha(0.5)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "❯").Font("monospace").FontSize(12).TextColor(p.accent)
			ui.Text(c, "Run a command in this session…").Font("monospace").FontSize(11).TextColor(p.text4).Grow(1)
			ui.Text(c, "host pty").Font("monospace").FontSize(9).TextColor(p.text4)
		})
	})
}
