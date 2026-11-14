package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	p := a.pal()
	c.SetTheme(p.theme())
	if a.pendingToast != "" {
		c.Toast(a.pendingToast)
		a.pendingToast = ""
	}

	w, _ := c.Size()
	a.lastWidth = w
	a.fitDockWidth(w)
	a.shortcuts(c)
	s := a.active()
	if s == nil {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "No sessions").Bold()
		})
		return
	}

	bar := c.TitleBar()
	sidebarVisible := a.sidebarOpen && w >= 900
	dockVisible := a.dockOpen && w >= 1180
	a.mainViewWidth = w
	if sidebarVisible {
		a.mainViewWidth -= sidebarWidth
	}
	if dockVisible {
		a.mainViewWidth -= a.dockWidth
	}

	sidebarDrawerOpen, dockDrawerOpen := a.sidebarDrawer, a.dockDrawer
	ui.Column(c).Fill().Background(p.bg).Children(func() {
		a.titlebar(c, s, bar, sidebarDrawerOpen, dockDrawerOpen)
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if sidebarVisible {
				a.sidebar(c, s)
			}
			a.main(c, s)
			if dockVisible {
				a.dock(c, s, a.dockWidth)
			}
		})
		a.drawers(c, s)
	})

	a.overlays(c, s)
}

func (a *app) drawers(c *ui.Context, s *session) {
	w, _ := c.Size()
	if w < 900 && a.sidebarDrawer {
		back := ui.ButtonBase(c).Absolute().Top(46).Left(0).Right(0).Bottom(0).
			Justify(ui.Start).Background(ui.RGBA(0, 0, 0, 0.42)).Label("Close sidebar")
		back.Children(func() {
			panel := ui.ButtonBase(c).Width(sidebarWidth).FillHeight().
				Background(ui.Transparent).Label("Sidebar")
			panel.Children(func() { a.sidebar(c, s) })
		})
		if back.Clicked() {
			a.sidebarDrawer = false
		}
	}
	if w < 1180 && a.dockDrawer {
		back := ui.ButtonBase(c).Absolute().Top(46).Left(0).Right(0).Bottom(0).
			Justify(ui.End).Background(ui.RGBA(0, 0, 0, 0.42)).Label("Close review dock")
		back.Children(func() {
			dw := a.dockWidth
			if dw < 360 {
				dw = 360
			}
			if dw > w-48 {
				dw = w - 48
			}
			panel := ui.ButtonBase(c).Width(dw).FillHeight().
				Background(ui.Transparent).Label("Review dock")
			panel.Children(func() { a.dock(c, s, dw) })
		})
		if back.Clicked() {
			a.dockDrawer = false
		}
	}
}

func (a *app) shortcuts(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		a.openPalette()
	}
	if c.Shortcut(ui.Cmd, ui.KeyN) {
		a.newSession()
	}
	if c.Shortcut(ui.Cmd, ui.KeyB) {
		a.toggleSidebar()
	}
	if c.Shortcut(ui.Cmd, ui.KeyJ) {
		a.toggleDock()
	}
	if c.Shortcut(ui.Cmd, ui.KeyComma) {
		a.settingsOpen = true
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		a.focusSearch = true
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		a.pickProject()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyD) {
		a.openDock(0)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyP) {
		a.openDock(1)
	}
	if c.Shortcut(ui.Shift, ui.KeyTab) {
		a.cycleApproval()
	}
	if a.sidebarDrawer && c.Shortcut(0, ui.KeyEscape) {
		a.sidebarDrawer = false
		return
	}
	if a.dockDrawer && a.dockTab != 2 && c.Shortcut(0, ui.KeyEscape) {
		a.dockDrawer = false
		return
	}
	if a.commentRow >= 0 && c.Shortcut(0, ui.KeyEscape) {
		a.cancelLineComment()
		return
	}
	if a.messageEditID != "" && c.Shortcut(0, ui.KeyEscape) {
		a.cancelMessageEdit(true)
		return
	}
	if !a.anyOverlayOpen() && c.Shortcut(0, ui.KeyEscape) {
		if s := a.active(); s != nil && s.Status == "running" {
			a.stopSession(s)
		}
	}
}

func (a *app) titlebar(c *ui.Context, s *session, bar ui.TitleBar, sidebarDrawerOpen, dockDrawerOpen bool) {
	p := a.pal()
	height := float32(46)
	if bar.Height > height {
		height = bar.Height
	}
	left := max(bar.Left+8, 14)
	right := max(bar.Right+12, 12)
	row := ui.Row(c).Height(height).FillWidth().Padding(0, right, 0, left).
		Gap(10).AlignItems(ui.Center).Background(p.sidebar).
		BorderWidth(0, 0, 1, 0).BorderColor(p.line).DragWindow()
	row.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			logo := ui.Box(c).Size(23, 23).Radius(7).Background(p.accent).Center()
			logo.Children(func() {
				ui.Text(c, "n").Font("monospace").FontSize(15).FontWeight(700).TextColor(p.accentInk)
			})
			ui.Text(c, "nulo").FontSize(15).FontWeight(600).TextColor(p.text)
		})

		ui.Row(c).Gap(6).AlignItems(ui.Center).Margin(0, 0, 0, 2).Children(func() {
			ui.Text(c, a.projectDisplayName(s.Project)).Font("monospace").FontSize(11).TextColor(p.text4).SingleLine()
			ui.Text(c, "/").Font("monospace").FontSize(11).TextColor(p.text4.Alpha(0.55)).SingleLine()
			ui.Text(c, s.Branch).Font("monospace").FontSize(11).TextColor(p.text3).SingleLine()
		})

		ui.Spacer(c)

		sidebarActive := a.sidebarOpen
		if a.lastWidth < 900 {
			sidebarActive = sidebarDrawerOpen
		}
		a.iconButton(c, "panel-left", "Toggle sidebar · ⌘B", sidebarActive, func() {
			if a.lastWidth < 900 {
				if sidebarDrawerOpen {
					a.sidebarDrawer = false
				} else {
					a.sidebarDrawer = true
					a.dockDrawer = false
				}
				return
			}
			a.sidebarOpen = !a.sidebarOpen
		})
		dockActive := a.dockOpen
		if a.lastWidth < 1180 {
			dockActive = dockDrawerOpen
		}
		a.iconButton(c, "panel-right", "Toggle right panel · ⌘J", dockActive, func() {
			if a.lastWidth < 1180 {
				if dockDrawerOpen {
					a.dockDrawer = false
				} else {
					a.dockDrawer = true
					a.sidebarDrawer = false
				}
				return
			}
			a.dockOpen = !a.dockOpen
		})
	})
}

type projectGroup struct {
	Key      string
	Name     string
	Letter   string
	Tint     string
	Branch   string
	CWD      string
	Sessions []*session
}

var projectDefs = []projectGroup{
	{Key: "nulo", Name: "NULO", Letter: "N", Tint: "green", Branch: "main"},
	{Key: "dotfiles", Name: "DOTFILES", Letter: "D", Tint: "blue", Branch: "main"},
	{Key: "pi-lab", Name: "PI-LAB", Letter: "P", Tint: "violet", Branch: "experiment/codemode"},
}

func (a *app) sidebar(c *ui.Context, active *session) {
	p := a.pal()
	side := ui.Column(c).Width(sidebarWidth).Shrink(0).FillHeight().Background(p.sidebar).
		BorderWidth(0, 1, 0, 0).BorderColor(p.line)
	side.Children(func() {
		ui.Column(c).Padding(12, 12, 9, 12).Gap(9).Children(func() {
			newBtn := ui.ButtonBase(c).FillWidth().Height(38).Padding(0, 10).Gap(8).Radius(7).
				Border(1, p.lineStrong).Background(p.surface).Justify(ui.Start).Label("New session")
			if newBtn.Hovered() {
				newBtn.BorderColor(p.accent.Alpha(0.36)).Background(p.surface2)
			}
			if newBtn.Clicked() {
				a.newSession()
			}
			newBtn.Children(func() {
				a.icon(c, "plus", 15, p.accent)
				ui.Text(c, "New session").FontSize(13).FontWeight(550).TextColor(p.text2).Grow(1)
				ui.Text(c, "⌘N").Font("monospace").FontSize(10).TextColor(p.text4)
			})

			a.searchField(c)
			a.filterRow(c)
		})

		a.sessionTree(c, active)

		ui.Column(c).BorderWidth(1, 0, 0, 0).BorderColor(p.line).PaddingY(5).Children(func() {
			capabilities := "3 MCP · 2 skills"
			if a.host != nil && active != nil {
				capabilities = fmt.Sprintf("%d MCP · %d skills", active.Context.MCP, active.Context.Skills)
			}
			a.sidebarFooterButton(c, "plug", "Capabilities", capabilities, func() {
				a.settingsOpen = true
				a.settingsTab = 2
			})
			a.sidebarFooterButton(c, "settings", "Settings", "", func() {
				a.settingsOpen = true
				a.settingsTab = 0
			})
		})
	})
}

func (a *app) searchField(c *ui.Context) {
	p := a.pal()
	box := ui.Row(c).FillWidth().Height(30).Padding(0, 9).Gap(7).Radius(6).
		Border(1, p.line).Background(p.surface.Alpha(0.5)).AlignItems(ui.Center)
	box.Children(func() {
		a.icon(c, "search", 13, p.text4)
		in := ui.TextInputBase(c, &a.search).Placeholder("Search sessions").Label("Search sessions").
			FontSize(12).TextColor(p.text2).Grow(1)
		if a.focusSearch {
			in.AutoFocus().KeepFocus()
			a.focusSearch = false
		}
	})
}

func (a *app) filterRow(c *ui.Context) {
	p := a.pal()
	all, active, review := a.filterCounts()
	labels := []string{"All", "Active", "Review"}
	counts := []int{all, active, review}
	ui.Row(c).FillWidth().Gap(4).Children(func() {
		for i := range labels {
			idx := i
			b := ui.ButtonBase(c).Height(24).Padding(0, 6).Gap(5).Radius(5).
				Background(ui.Transparent).Label(labels[i])
			if a.filter == i {
				b.Background(p.accentSoft)
			} else if b.Hovered() {
				b.Background(p.hover)
			}
			if b.Clicked() {
				a.filter = i
			}
			b.Children(func() {
				color := p.text4
				if a.filter == idx {
					color = p.accent
				}
				ui.Text(c, strings.ToUpper(labels[idx])).Font("monospace").FontSize(10).
					TextColor(color).LetterSpacing(0.4).SingleLine()
				ui.Textf(c, "%d", counts[idx]).Font("monospace").FontSize(10).TextColor(color).SingleLine()
			})
		}
	})
}

func (a *app) sessionTree(c *ui.Context, active *session) {
	p := a.pal()
	groups := append([]projectGroup(nil), a.projects...)
	for i := range groups {
		for _, s := range a.visibleSessions() {
			if s.Project == groups[i].Key {
				groups[i].Sessions = append(groups[i].Sessions, s)
			}
		}
		sort.SliceStable(groups[i].Sessions, func(x, y int) bool {
			left, right := groups[i].Sessions[x], groups[i].Sessions[y]
			if a.host != nil {
				return left.Modified > right.Modified
			}
			return groupOrder(left) < groupOrder(right)
		})
	}
	scroll := ui.Scroll(c).Grow(1).Padding(0, 8, 8, 8)
	scroll.Children(func() {
		empty := true
		for _, g := range groups {
			if len(g.Sessions) == 0 {
				continue
			}
			empty = false
			ui.Column(c).Key("group-" + g.Key).MarginY(6).Children(func() {
				collapsed := a.collapsedGroups[g.Key]
				a.projectHead(c, g, collapsed)
				if collapsed {
					return
				}
				for _, s := range g.Sessions {
					indent := s.ForkOf != "" && s.ForkOf != s.ID
					a.sessionRow(c, s, s.ID == active.ID, indent)
				}
			})
		}
		if empty {
			ui.Column(c).Padding(18, 16).Gap(5).Children(func() {
				ui.Text(c, "No sessions match.").FontSize(12).TextColor(p.text3)
				ui.Text(c, "Clear the search or choose another filter.").FontSize(11).TextColor(p.text4)
			})
		}
	})
}

func groupOrder(s *session) int {
	if strings.HasPrefix(s.ID, "new-") || strings.HasPrefix(s.ID, "fork-") {
		return -1
	}
	switch s.ID {
	case "s1":
		return 0
	case "s2":
		return 1
	case "s9":
		return 2
	case "s3":
		return 3
	case "s4":
		return 4
	case "s5":
		return 5
	}
	return 20
}

func (a *app) projectHead(c *ui.Context, g projectGroup, collapsed bool) {
	p := a.pal()
	tint := p.accent
	switch g.Tint {
	case "blue":
		tint = p.info
	case "violet":
		tint = p.violet
	}
	head := ui.Row(c).FillWidth().Height(28).AlignItems(ui.Center).Radius(5)
	if head.Hovered() {
		head.Background(p.hover)
	}
	head.Children(func() {
		toggle := ui.ButtonBase(c).Grow(1).Height(28).Padding(0, 8).Gap(7).Radius(5).
			Justify(ui.Start).Label(g.Name)
		if toggle.Clicked() {
			a.collapsedGroups[g.Key] = !a.collapsedGroups[g.Key]
		}
		toggle.Children(func() {
			if collapsed {
				a.icon(c, "chevron-right", 10, p.text4)
			} else {
				a.icon(c, "chevron-down", 10, p.text4)
			}
			badge := ui.Box(c).Size(18, 18).Radius(5).Border(1, tint.Alpha(0.22)).
				Background(tint.Alpha(0.10)).Center()
			badge.Children(func() {
				ui.Text(c, g.Letter).Font("monospace").FontSize(10).FontWeight(600).TextColor(tint)
			})
			ui.Text(c, g.Name).Font("monospace").FontSize(10).FontWeight(600).TextColor(p.text3).
				LetterSpacing(0.5).Grow(1).SingleLine()
			ui.Textf(c, "%d", len(g.Sessions)).Font("monospace").FontSize(10).TextColor(p.text4)
		})

		add := ui.ButtonBase(c).Size(22, 22).Radius(5).
			Label("New session in " + g.Name).Tooltip("New session in " + g.Name)
		if add.Hovered() {
			add.Background(p.surface2)
		}
		if add.Clicked() {
			a.newSessionFor(g)
		}
		add.Children(func() {
			if head.Hovered() || add.Hovered() {
				a.icon(c, "plus", 12, p.text4)
			}
		})
	})
}

func (a *app) sessionRow(c *ui.Context, s *session, active, indent bool) {
	p := a.pal()
	row := ui.ButtonBase(c).Key("session-"+s.ID).FillWidth().Height(48).Padding(6, 8).Gap(7).
		Radius(5).Justify(ui.Start).AlignItems(ui.Start).Label(s.Title)
	if active {
		row.Background(p.accentSofter).BorderWidth(0, 0, 0, 2).BorderColor(p.accent)
	} else if row.Hovered() {
		row.Background(p.hover)
	}
	if row.Clicked() {
		a.selectSession(s.ID)
	}
	row.Children(func() {
		if indent {
			thread := ui.Column(c).Width(14).FillHeight().Padding(0, 0, 0, 6).Children(func() {
				ui.Box(c).Width(1).Grow(1).Background(p.line).Margin(2, 0, 0, 0)
			})
			thread.Children(func() {
				a.icon(c, "corner", 12, p.text4).Margin(12, 0, 0, 0)
			})
		}
		dotWrap := ui.Row(c).Width(6).Height(20).AlignItems(ui.Center).Justify(ui.Center)
		dotWrap.Children(func() {
			a.statusDot(c, s.Status, 6)
		})
		ui.Column(c).Grow(1).Gap(3).MinWidth(0).Children(func() {
			ui.Text(c, s.Title).FontSize(12.5).FontWeight(500).
				TextColor(chooser(active, p.text, p.text2)).SingleLine()
			note := s.Note
			if note == "" {
				note = s.Updated + " · " + statusLabel(s.Status)
			}
			color := p.text4
			if s.Status == "awaiting" {
				note = s.Updated + " · needs approval"
				color = p.warn
			} else if s.Status == "running" {
				note = s.Updated + " · running"
				color = p.accent2
			} else if s.Status == "error" {
				note = s.Updated + " · error"
				color = p.bad
			}
			ui.Text(c, note).Font("monospace").FontSize(10).TextColor(color).SingleLine()
		})
		if s.ForkOf != "" {
			a.icon(c, "fork", 11, p.text4)
		}
		if count := a.forkCount(s.ID); count > 0 {
			badge := ui.Box(c).Height(16).Padding(0, 4).Radius(4).Background(p.surface2).Center()
			badge.Children(func() {
				ui.Textf(c, "%d", count).Font("monospace").FontSize(9).TextColor(p.text4)
			})
		}
	})
}

func (a *app) hasFork(id string) bool { return a.forkCount(id) > 0 }

func (a *app) forkCount(id string) int {
	count := 0
	for _, s := range a.sessions {
		if s.ForkOf == id {
			count++
		}
	}
	return count
}

func (a *app) sidebarFooterButton(c *ui.Context, icon, label, right string, onClick func()) {
	p := a.pal()
	b := ui.ButtonBase(c).FillWidth().Height(31).Padding(0, 16).Gap(8).Radius(4).
		Justify(ui.Start).Label(label)
	if b.Hovered() {
		b.Background(p.hover)
	}
	if b.Clicked() {
		onClick()
	}
	b.Children(func() {
		a.icon(c, icon, 14, p.text4)
		ui.Text(c, label).FontSize(12).TextColor(p.text3).Grow(1).SingleLine()
		if right != "" {
			ui.Text(c, right).Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
		}
	})
}

func chooser[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
