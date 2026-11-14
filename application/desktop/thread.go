package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (a *app) main(c *ui.Context, s *session) {
	p := a.pal()
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.bg).Children(func() {
		a.sessionbar(c, s)
		a.conversation(c, s)
		a.composer(c, s)
	})
}

func (a *app) sessionbar(c *ui.Context, s *session) {
	p := a.pal()
	bar := ui.Row(c).Height(48).FillWidth().Padding(0, 14, 0, 18).Gap(10).
		AlignItems(ui.Center).Background(p.bg).BorderWidth(0, 0, 1, 0).BorderColor(p.line)
	bar.Children(func() {
		badge := ui.Box(c).Size(29, 29).Radius(8).Border(1, p.lineStrong).
			Background(p.surface).Center()
		badge.Children(func() {
			ui.Text(c, "n").Font("monospace").FontSize(13).FontWeight(600).TextColor(p.accent)
		})

		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			crumb := ui.Row(c).Gap(6).AlignItems(ui.Center)
			crumb.Children(func() {
				ui.Text(c, a.projectDisplayName(s.Project)).Font("monospace").FontSize(11).TextColor(p.text3)
				ui.Text(c, "/").Font("monospace").FontSize(11).TextColor(p.text4.Alpha(0.55))
				ui.Text(c, s.Env).Font("monospace").FontSize(11).TextColor(p.text4)
				if s.Worktree != "" {
					ui.Text(c, "·").Font("monospace").FontSize(11).TextColor(p.text4.Alpha(0.55))
					a.icon(c, "git-branch", 10, p.text4)
					ui.Text(c, s.Branch).Font("monospace").FontSize(11).TextColor(p.text4).SingleLine()
				} else {
					ui.Text(c, "·").Font("monospace").FontSize(11).TextColor(p.text4.Alpha(0.55))
					ui.Text(c, s.Branch).Font("monospace").FontSize(11).TextColor(p.text4).SingleLine()
				}
			})
			titleRow := ui.Row(c).Gap(9).AlignItems(ui.Center)
			if s.ForkOf != "" {
				titleRow.Gap(6)
			}
			titleRow.Children(func() {
				ui.Text(c, s.Title).FontSize(14).FontWeight(600).TextColor(p.text).SingleLine()
				if a.mainViewWidth >= 700 {
					a.statusPill(c, s.Status)
				}
				if s.ForkOf != "" {
					ui.Text(c, "fork of "+a.sessionTitle(s.ForkOf)).Font("monospace").FontSize(10).TextColor(p.text4).SingleLine()
				}
			})
		})

		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			if s.Changes != nil && len(s.Changes.Files) > 0 {
				add, del := s.changeStats()
				b := ui.ButtonBase(c).Height(26).Padding(0, 8).Gap(6).Radius(7).
					Border(1, p.line).Background(p.surface.Alpha(0.55)).Label("Changes")
				if b.Hovered() {
					b.BorderColor(p.lineStrong).Background(p.surface)
				}
				if b.Clicked() {
					a.openDock(0)
				}
				b.Children(func() {
					a.icon(c, "diff", 13, p.text4)
					ui.Textf(c, "%d files", len(s.Changes.Files)).Font("monospace").FontSize(11).TextColor(p.text3)
					ui.Textf(c, "+%d", add).Font("monospace").FontSize(11).TextColor(p.accent2)
					ui.Textf(c, "−%d", del).Font("monospace").FontSize(11).TextColor(p.bad)
				})
			}
			a.sessionMenuButton(c, s)
			if a.mainViewWidth >= 720 {
				a.iconButton(c, "corner", "Rewind to checkpoint", false, func() { a.rewindSession(s) })
				a.iconButton(c, "fork", "Fork session", false, func() { a.forkSession(s) })
			}
			dockActive := a.dockOpen
			if a.lastWidth > 0 && a.lastWidth < 1180 {
				dockActive = a.dockDrawer
			}
			a.iconButton(c, "panel-right", "Toggle review dock · ⌘J", dockActive, a.toggleDock)
		})
	})
}

func (a *app) sessionMenuButton(c *ui.Context, s *session) {
	p := a.pal()
	b := ui.ButtonBase(c).Size(28, 28).Radius(6).Label("Session actions").Tooltip("Session actions")
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Menu(func(m *ui.Menu) { a.sessionMenu(c, m, s) })
	b.Children(func() { a.icon(c, "more", 16, p.text3) })
}

func (a *app) sessionMenu(c *ui.Context, m *ui.Menu, s *session) {
	if m.Item("Rename…").Chosen() {
		a.beginRename(s)
	}
	if m.Item("Fork from here").Chosen() {
		a.forkSession(s)
	}
	if m.Item("Rewind to checkpoint").Chosen() {
		a.rewindSession(s)
	}
	if m.Item("Copy session ID").Chosen() {
		if c != nil {
			c.Services().WriteClipboard(s.ID)
		}
		a.pendingToast = "Copied session ID"
	}
	m.Separator()
	if m.Item("Delete session").Disabled(len(a.sessions) <= 1).Chosen() {
		a.beginDelete(s)
	}
}

func (a *app) sessionTitle(id string) string {
	for _, s := range a.sessions {
		if s.ID == id {
			return s.Title
		}
	}
	return id
}

func (a *app) forkSession(s *session) {
	if a.host != nil && s.Real {
		if _, err := a.host.fork(a, s); err != nil {
			a.pendingToast = "Fork: " + err.Error()
			return
		}
		a.scrollToEnd = true
		a.pendingToast = "Forked session"
		return
	}
	n := len(a.sessions) + 1
	fork := *s
	fork.ID = fmt.Sprintf("fork-%d", n)
	fork.Title = s.Title + " · fork"
	fork.ForkOf = s.ID
	fork.Started, fork.Updated = "now", "now"
	fork.Status = "done"
	fork.Welcome = false
	fork.Note = "now · forked"
	fork.Changes = nil
	fork.Terminal = nil
	fork.Messages = []message{
		{Role: "notice", Time: "now", Text: "Forked from " + s.Title + ". The parent thread stays intact."},
	}
	a.sessions = append([]*session{&fork}, a.sessions...)
	a.activeID = fork.ID
	a.scrollToEnd = true
	a.pendingToast = "Forked session"
}

func (a *app) conversation(c *ui.Context, s *session) {
	scroll := ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&a.threadScroll)
	scroll.Children(func() {
		pad := float32(20)
		gap := float32(32)
		if a.compactDensity {
			pad, gap = 12, 20
		}
		content := ui.Column(c).WidthPercent(100).MaxWidth(820).AlignSelf(ui.Center).
			Padding(pad, 30, gap, 30)
		content.Children(func() {
			a.threadMeta(c, s)
			if s.Welcome {
				a.welcome(c, s)
			} else {
				for mi := range s.Messages {
					a.messageView(c, s, mi)
				}
			}
			end := ui.Box(c).Height(1)
			if a.scrollToEnd {
				end.ScrollIntoView()
				a.scrollToEnd = false
			}
		})
	})
}

func (a *app) threadMeta(c *ui.Context, s *session) {
	p := a.pal()
	ui.Row(c).FillWidth().Padding(0, 0, 13, 0).Gap(9).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.line.Alpha(0.7)).Children(func() {
		a.icon(c, "sparkles", 12, p.text4)
		ui.Text(c, s.modelName()).Font("monospace").FontSize(11).TextColor(p.text4).SingleLine()
		ui.Text(c, "·").Font("monospace").FontSize(11).TextColor(p.text4)
		ui.Text(c, s.thinkingName()+" thinking").Font("monospace").FontSize(11).TextColor(p.text4).SingleLine()
		ui.Text(c, "·").Font("monospace").FontSize(11).TextColor(p.text4)
		a.icon(c, "git-branch", 11, p.text4)
		ui.Text(c, s.Branch).Font("monospace").FontSize(11).TextColor(p.text4).SingleLine()
		ui.Spacer(c)
		a.icon(c, "clock", 11, p.text4)
		ui.Text(c, s.Started).Font("monospace").FontSize(11).TextColor(p.text4).SingleLine()
	})
}

func (a *app) messageView(c *ui.Context, s *session, mi int) {
	m := &s.Messages[mi]
	p := a.pal()
	switch m.Role {
	case "user":
		bottom := float32(28)
		if a.compactDensity {
			bottom = 16
		}
		ui.Row(c).FillWidth().Justify(ui.End).Margin(0, 0, bottom, 0).Children(func() {
			bubble := ui.Column(c).MaxWidth(620).Padding(11, 14, 12, 14).
				Radius(12, 12, 3, 12).Border(1, p.line).Background(p.userBubble)
			bubble.Children(func() {
				label := ui.Row(c).Gap(6).AlignItems(ui.Center).Margin(0, 0, 5, 0)
				label.Children(func() {
					a.icon(c, "message", 11, p.text4)
					ui.Text(c, "YOU").Font("monospace").FontSize(10).TextColor(p.text4).LetterSpacing(0.5)
					if a.showTimestamps && m.Time != "" {
						ui.Text(c, "· "+m.Time).Font("monospace").FontSize(10).TextColor(p.text4)
					}
					ui.Spacer(c)
					a.messageMenuButton(c, s, m, mi)
				})
				ui.Text(c, m.Text).FontSize(14).LineHeight(1.62).TextColor(p.text2).Selectable()
				if len(m.Attachments) > 0 {
					tray := ui.Row(c).Gap(5).Margin(5, 0, 0, 0).Wrap()
					tray.Children(func() {
						for _, path := range m.Attachments {
							height := float32(24)
							if isImagePath(path) {
								height = 38
							}
							chip := ui.Row(c).Height(height).Padding(0, 7).Gap(5).Radius(5).
								Border(1, p.line).Background(p.surface2).AlignItems(ui.Center)
							chip.Children(func() {
								if !a.imageThumbButton(c, path, 32) {
									a.icon(c, "paperclip", 10, p.text4)
								}
								ui.Text(c, filepath.Base(path)).Font("monospace").FontSize(10).
									TextColor(p.text3).SingleLine()
							})
						}
					})
				}
			})
		})
	case "notice":
		ui.Row(c).FillWidth().Justify(ui.Center).Margin(0, 2, 20, 2).Children(func() {
			row := ui.Row(c).Gap(6).Padding(5, 9).Radius(5).Border(1, p.line).Background(p.surface.Alpha(0.5))
			row.Children(func() {
				a.icon(c, "compress", 11, p.text4)
				ui.Text(c, m.Text).Font("monospace").FontSize(10).TextColor(p.text4).SingleLine()
			})
		})
	case "assistant":
		gap := float32(11)
		if a.compactDensity {
			gap = 7
		}
		ui.Column(c).FillWidth().Gap(gap).Margin(0, 0, 4, 0).Children(func() {
			a.assistantHead(c, s, m, mi)
			for bi := range m.Blocks {
				a.blockView(c, s, mi, bi)
			}
		})
	}
}

func (a *app) assistantHead(c *ui.Context, s *session, m *message, mi int) {
	p := a.pal()
	ui.Row(c).FillWidth().Height(22).Gap(8).AlignItems(ui.Center).Children(func() {
		avatar := ui.Box(c).Size(22, 22).Radius(6).Background(p.accent).Center()
		avatar.Children(func() {
			ui.Text(c, "n").Font("monospace").FontSize(13).FontWeight(700).TextColor(p.accentInk)
		})
		ui.Text(c, "nulo").FontSize(13).FontWeight(600).TextColor(p.text2)
		ui.Text(c, s.modelName()).Font("monospace").FontSize(11).TextColor(p.text4).SingleLine()
		ui.Spacer(c)
		if a.showTimestamps {
			ui.Text(c, m.Time).Font("monospace").FontSize(11).TextColor(p.text4)
		}
		a.messageMenuButton(c, s, m, mi)
	})
}

func (a *app) blockView(c *ui.Context, s *session, mi, bi int) {
	b := &s.Messages[mi].Blocks[bi]
	switch b.Type {
	case "thinking":
		a.thinkingBlock(c, b)
	case "text":
		a.markdownBlock(c, b.Text)
	case "plan":
		a.planCard(c, s)
	case "subagent":
		a.subagentBlock(c, b)
	case "tool":
		a.toolCard(c, s, b)
	case "changes":
		a.changesCard(c, s)
	case "approval":
		a.approvalCard(c, s, b)
	}
}

func (a *app) markdownBlock(c *ui.Context, text string) {
	p := a.pal()
	th := markdown.DefaultTheme(p.theme())
	th.FontSize = 14
	th.LineHeight = 1.66
	th.Text = p.text2
	th.Muted = p.text4
	th.Link = p.accent
	th.Strong = p.text
	th.Code = p.text
	th.CodeBackground = p.surface2
	th.CodeSize = 0.9
	th.BlockSpacing = 0.6
	if p.dark {
		th.CodeBlockBackground = p.codeBg
	} else {
		th.CodeBlockBackground = p.surface2
	}
	th.CodeBlockBorder = p.line
	th.CodeBlockRadius = 8
	th.CodeBlockPadding = 0.85
	a.md.Theme = th
	ui.Box(c).FillWidth().Children(func() {
		markdown.View(c, text, &a.md)
	})
}

func (a *app) thinkingBlock(c *ui.Context, b *block) {
	p := a.pal()
	box := ui.Column(c).FillWidth()
	open := ui.Local(box, "open", func() bool { return !a.collapseThinking })
	head := ui.ButtonBase(c).Height(20).Padding(0, 0).Gap(6).Justify(ui.Start).
		Background(ui.Transparent).Label("Thinking")
	if *open {
		head.Label("Hide thinking")
	}
	if head.Hovered() {
		head.Background(p.surface.Alpha(0.45)).Radius(4)
	}
	if head.Clicked() {
		*open = !*open
	}
	head.Children(func() {
		if *open {
			a.icon(c, "chevron-down", 10, p.text4)
		} else {
			a.icon(c, "chevron-right", 10, p.text4)
		}
		ui.Text(c, "Thinking · "+b.Duration).Font("monospace").FontSize(11).TextColor(p.text4)
	})
	box.Children(func() {
		head.Radius(4)
		if *open {
			ui.Column(c).Margin(0, 0, 0, 4).Padding(0, 0, 0, 17).
				BorderWidth(0, 0, 0, 1).BorderColor(p.line).Children(func() {
				ui.Text(c, b.Text).FontSize(12).LineHeight(1.62).TextColor(p.text3).Selectable()
			})
		}
	})
}

func (a *app) planCard(c *ui.Context, s *session) {
	p := a.pal()
	if s.Plan.Title == "" {
		return
	}
	done := 0
	for _, st := range s.Plan.Steps {
		if st.Status == "done" {
			done++
		}
	}
	card := ui.Column(c).FillWidth().Clip().Radius(6).Border(1, p.line).
		Background(p.surface.Alpha(0.72))
	card.Children(func() {
		head := ui.Row(c).Height(34).FillWidth().Padding(0, 12).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.line)
		head.Children(func() {
			a.icon(c, "list-checks", 14, p.accent)
			ui.Text(c, s.Plan.Title).FontSize(13).FontWeight(600).TextColor(p.text2).Grow(1)
			ui.Textf(c, "%d/%d", done, len(s.Plan.Steps)).Font("monospace").FontSize(11).TextColor(p.text4)
		})
		a.progress(c, s.planProgress(), 2, p.accent)
		steps := ui.Column(c).Padding(5, 12, 8, 12).Gap(1)
		steps.Children(func() {
			for i, st := range s.Plan.Steps {
				a.planStep(c, i, st)
			}
		})
		if s.Plan.Next != "" {
			next := ui.Row(c).FillWidth().Height(29).Padding(0, 12).Gap(6).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(p.line)
			next.Children(func() {
				a.icon(c, "arrow-right", 11, p.accent2)
				ui.Text(c, s.Plan.Next).FontSize(12).TextColor(p.text4).SingleLine()
			})
		}
	})
}

func (a *app) planStep(c *ui.Context, i int, st planStep) {
	p := a.pal()
	row := ui.Row(c).FillWidth().Padding(4, 0).Gap(9).AlignItems(ui.Start)
	row.Children(func() {
		marker := ui.Box(c).Size(15, 15).Radius(8).Border(1, p.lineStrong).
			Background(ui.Transparent).Margin(0, 0, 0, 1)
		switch st.Status {
		case "done":
			marker.Background(p.accentSoft).BorderColor(ui.Transparent)
		case "active":
			marker.Background(p.accentSoft).BorderColor(p.accent.Alpha(0.55))
		case "blocked":
			marker.Background(p.badSoft).BorderColor(p.bad.Alpha(0.28))
		}
		marker.Center().Children(func() {
			switch st.Status {
			case "done":
				a.icon(c, "check", 9, p.accent)
			case "active":
				ui.Box(c).Size(5, 5).Radius(5).Background(p.accent)
			case "blocked":
				a.icon(c, "alert", 8, p.bad)
			default:
				ui.Textf(c, "%d", i+1).Font("monospace").FontSize(9).TextColor(p.text4)
			}
		})
		ui.Column(c).Grow(1).Gap(1).MinWidth(0).Children(func() {
			color := p.text3
			if st.Status == "active" {
				color = p.text
			} else if st.Status == "blocked" {
				color = p.bad
			}
			ui.Text(c, st.Title).FontSize(13).LineHeight(1.4).TextColor(color)
			if st.Detail != "" {
				ui.Text(c, st.Detail).FontSize(12).LineHeight(1.4).TextColor(p.text4)
			}
		})
		if st.Duration != "" {
			ui.Text(c, st.Duration).Font("monospace").FontSize(10).TextColor(p.text4).SingleLine()
		}
	})
}

func (a *app) subagentBlock(c *ui.Context, b *block) {
	p := a.pal()
	box := ui.Column(c).FillWidth().Clip().Radius(6).Border(1, p.line).
		Background(p.surface.Alpha(0.62))
	open := ui.Local(box, "open", func() bool { return false })
	box.Children(func() {
		head := ui.ButtonBase(c).FillWidth().Height(38).Padding(8, 11).Gap(8).
			Justify(ui.Start).Background(ui.Transparent).Label(b.Name)
		if head.Hovered() {
			head.Background(p.surface)
		}
		if head.Clicked() {
			*open = !*open
		}
		head.Children(func() {
			icon := ui.Box(c).Size(21, 21).Radius(6).Border(1, p.line).Background(p.surface2).Center()
			icon.Children(func() { a.icon(c, "fork", 12, p.violet) })
			ui.Text(c, b.Name).Font("monospace").FontSize(12).FontWeight(600).TextColor(p.text2)
			ui.Text(c, b.Agent).FontSize(12).TextColor(p.text4)
			ui.Text(c, b.Summary).FontSize(12).TextColor(p.text3).Grow(1).SingleLine()
			a.icon(c, "check", 10, p.accent2)
			ui.Text(c, b.Duration).Font("monospace").FontSize(10).TextColor(p.text4)
			if *open {
				a.icon(c, "chevron-down", 12, p.text4)
			} else {
				a.icon(c, "chevron-right", 12, p.text4)
			}
		})
		if *open {
			details := ui.Column(c).FillWidth().Padding(2, 12, 10, 40).
				BorderWidth(1, 0, 0, 0).BorderColor(p.line)
			details.Children(func() {
				for _, d := range b.Details {
					ui.Row(c).Gap(7).PaddingY(2).Children(func() {
						ui.Text(c, "·").Font("monospace").FontSize(11).TextColor(p.text4)
						ui.Text(c, d).Font("monospace").FontSize(11).LineHeight(1.5).TextColor(p.text3)
					})
				}
			})
		}
	})
}

func (a *app) toolCard(c *ui.Context, s *session, b *block) {
	p := a.pal()
	card := ui.Column(c).FillWidth().Clip().Radius(6).Border(1, p.line).
		Background(p.surface.Alpha(0.58))
	open := ui.Local(card, "open", func() bool { return false })
	card.Children(func() {
		head := ui.ButtonBase(c).FillWidth().Height(35).Padding(7, 10).Gap(8).
			Justify(ui.Start).Background(ui.Transparent).Label(b.Title)
		if head.Hovered() {
			head.Background(p.surface)
		}
		if head.Clicked() {
			*open = !*open
		}
		head.Children(func() {
			icName := "terminal"
			if b.Tool == "read" {
				icName = "file"
			} else if b.Tool == "edit" {
				icName = "code"
			}
			iconBox := ui.Box(c).Size(20, 20).Radius(5).Background(p.surface2).Center()
			iconBox.Children(func() { a.icon(c, icName, 12, p.text3) })
			ui.Text(c, b.Title).Font("monospace").FontSize(12).TextColor(p.text2).SingleLine()
			if b.Meta != "" {
				ui.Text(c, b.Meta).Font("monospace").FontSize(10).TextColor(p.text4).SingleLine()
			}
			ui.Spacer(c)
			if b.Status == "running" {
				ui.Box(c).Size(6, 6).Radius(6).Background(p.warn)
				ui.Text(c, "running").Font("monospace").FontSize(10).TextColor(p.warn)
			} else if b.Status == "fail" {
				a.icon(c, "alert", 10, p.bad)
				ui.Text(c, "failed").Font("monospace").FontSize(10).TextColor(p.bad)
			} else {
				a.icon(c, "check", 10, p.accent2)
				ui.Text(c, "done").Font("monospace").FontSize(10).TextColor(p.accent2)
			}
			if *open {
				a.icon(c, "chevron-down", 12, p.text4)
			} else {
				a.icon(c, "chevron-right", 12, p.text4)
			}
		})
		if *open {
			body := ui.Column(c).FillWidth().Padding(9, 12, 11, 12).
				BorderWidth(1, 0, 0, 0).BorderColor(p.line).Background(p.codeBg).Gap(8)
			body.Children(func() {
				ui.Text(c, b.Output).Font("monospace").FontSize(12).LineHeight(1.62).
					TextColor(p.text3).Selectable()
				if b.Status == "fail" {
					if b.CallID == "" {
						if a.smallButton(c, "Retry").Clicked() {
							b.Status = "done"
							b.Output += "\n\nRetried: command completed with exit code 0."
							for i := range s.Plan.Steps {
								if s.Plan.Steps[i].Status == "active" {
									s.Plan.Steps[i].Status = "done"
								}
							}
							a.pendingToast = "Retried " + b.Title
						}
					} else {
						ui.Text(c, "Resend the message to retry this tool call.").
							Font("monospace").FontSize(10).TextColor(p.text4)
					}
				}
			})
		}
	})
}

func (a *app) changesCard(c *ui.Context, s *session) {
	if s.Changes == nil || len(s.Changes.Files) == 0 {
		return
	}
	p := a.pal()
	add, del := s.changeStats()
	reviewed, total := s.reviewCount()
	card := ui.Column(c).FillWidth().Clip().Radius(6).Border(1, p.line).
		Background(p.surface.Alpha(0.72))
	card.Children(func() {
		head := ui.Row(c).Height(36).FillWidth().Padding(0, 12).Gap(8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.line)
		head.Children(func() {
			a.icon(c, "file", 14, p.text3)
			ui.Textf(c, "%d files changed", len(s.Changes.Files)).FontSize(13).FontWeight(550).TextColor(p.text2).Grow(1)
			ui.Textf(c, "+%d", add).Font("monospace").FontSize(11).TextColor(p.accent2)
			ui.Textf(c, "−%d", del).Font("monospace").FontSize(11).TextColor(p.bad)
		})
		files := ui.Column(c).Padding(5, 6).Gap(1)
		files.Children(func() {
			for i := range s.Changes.Files {
				idx := i
				f := &s.Changes.Files[i]
				row := ui.ButtonBase(c).FillWidth().Height(28).Padding(5, 6).Gap(8).
					Radius(5).Justify(ui.Start).Background(ui.Transparent).Label(f.Path)
				if row.Hovered() {
					row.Background(p.surface2)
				}
				if row.Clicked() {
					a.selectedFile[s.ID] = idx
					a.openDock(0)
				}
				row.Children(func() {
					st := ui.Box(c).Size(15, 15).Radius(4).Background(p.surface2).Center()
					st.Children(func() {
						color := p.text4
						if f.Status == "M" {
							color = p.warn
						} else if f.Status == "A" {
							color = p.accent2
						}
						ui.Text(c, f.Status).Font("monospace").FontSize(10).FontWeight(600).TextColor(color)
					})
					ui.Text(c, f.Path).Font("monospace").FontSize(12).TextColor(p.text3).Grow(1).SingleLine()
					ui.Textf(c, "+%d", f.Add).Font("monospace").FontSize(10).TextColor(p.accent2)
					ui.Textf(c, "−%d", f.Del).Font("monospace").FontSize(10).TextColor(p.bad)
				})
			}
		})
		foot := ui.Row(c).FillWidth().Height(33).Padding(0, 10).Gap(8).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(p.line)
		foot.Children(func() {
			link := ui.ButtonBase(c).Height(24).Padding(0, 2).Gap(4).Radius(4).
				Background(ui.Transparent).Label("Review changes")
			if link.Clicked() {
				a.openDock(0)
			}
			link.Children(func() {
				ui.Text(c, "Review changes").FontSize(12).FontWeight(550).TextColor(p.accent)
				a.icon(c, "arrow-right", 11, p.accent)
			})
			ui.Spacer(c)
			ui.Textf(c, "%d/%d reviewed", reviewed, total).Font("monospace").FontSize(10).TextColor(p.text4)
		})
	})
}

func (a *app) approvalCard(c *ui.Context, s *session, b *block) {
	p := a.pal()
	resolved := b.Approval != "pending"
	border := p.warn.Alpha(0.32)
	bg := p.surface.Mix(p.warn, 0.05)
	if resolved {
		border = p.line
		bg = p.surface
	}
	card := ui.Column(c).FillWidth().Clip().Radius(6).Border(1, border).Background(bg)
	card.Children(func() {
		head := ui.Row(c).FillWidth().Padding(12, 13, 9, 13).Gap(10).AlignItems(ui.Start)
		head.Children(func() {
			iconBox := ui.Box(c).Size(27, 27).Radius(7).Border(1, p.warn.Alpha(0.26)).
				Background(p.warnSoft).Center()
			iconBox.Children(func() {
				if resolved {
					if b.Approval == "denied" {
						a.icon(c, "x", 14, p.bad)
					} else {
						a.icon(c, "check", 14, p.accent2)
					}
				} else {
					a.icon(c, "shield", 14, p.warn)
				}
			})
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				kicker := "APPROVAL NEEDED"
				kcolor := p.warn
				if resolved {
					kicker = "APPROVAL RESOLVED"
					kcolor = p.text4
				}
				ui.Text(c, kicker).Font("monospace").FontSize(10).TextColor(kcolor).LetterSpacing(0.6)
				title := "Run this command?"
				if b.CallID != "" {
					title = "Allow this tool?"
				}
				ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(p.text)
				ui.Text(c, b.Reason).FontSize(13).LineHeight(1.5).TextColor(p.text3)
			})
		})
		cmd := ui.Row(c).FillWidth().Height(36).Margin(2, 13, 8, 13).Padding(0, 11).Gap(8).
			Radius(7).Border(1, p.line).Background(p.codeBg).AlignItems(ui.Center)
		cmd.Children(func() {
			ui.Text(c, ">").Font("monospace").FontSize(13).TextColor(p.accent)
			ui.Text(c, b.Command).Font("monospace").FontSize(13).TextColor(p.text).SingleLine()
		})
		risk := ui.Row(c).FillWidth().Padding(0, 13, 11, 13).Gap(6).AlignItems(ui.Center)
		risk.Children(func() {
			a.icon(c, "lock", 10, p.text4)
			ui.Text(c, b.CWD+"   ·   "+b.Risk).Font("monospace").FontSize(10).TextColor(p.text4).SingleLine()
		})
		if !resolved {
			actions := ui.Row(c).FillWidth().Padding(0, 13, 12, 13).Gap(7).AlignItems(ui.Center)
			actions.Children(func() {
				if a.primaryButton(c, "Allow once", "check").Clicked() {
					a.resolveApproval(s, b, "approved")
				}
				if a.button(c, "Always allow").Clicked() {
					a.resolveApproval(s, b, "always")
				}
				if a.dangerButton(c, "Deny").Clicked() {
					a.resolveApproval(s, b, "denied")
				}
				ui.Spacer(c)
				ui.Text(c, "mode · "+approvalLabel(s.Approval)).Font("monospace").FontSize(10).TextColor(p.text4)
			})
		} else {
			row := ui.Row(c).FillWidth().Padding(9, 13, 10, 13).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(p.line)
			row.Children(func() {
				color := p.accent2
				text := "Approved · tests passed in 4.8s"
				if b.CallID != "" {
					text = "Approved · tool call allowed"
				}
				if b.Approval == "denied" {
					color = p.bad
					if b.CallID != "" {
						text = "Denied · the tool call is blocked"
					} else {
						text = "Denied · the session is paused"
					}
				}
				a.icon(c, "check", 12, color)
				ui.Text(c, text).Font("monospace").FontSize(11).TextColor(color)
				ui.Spacer(c)
				ui.Text(c, "recorded in session").Font("monospace").FontSize(10).TextColor(p.text4)
			})
		}
	})
}

func (a *app) welcome(c *ui.Context, s *session) {
	p := a.pal()
	ui.Column(c).FillWidth().MaxWidth(660).PaddingY(34).AlignSelf(ui.Center).Children(func() {
		mark := ui.Box(c).Size(40, 40).Radius(11).Background(p.accent).Center().Margin(0, 0, 18, 0)
		mark.Children(func() {
			ui.Text(c, "n").Font("monospace").FontSize(21).FontWeight(700).TextColor(p.accentInk)
		})
		ui.Text(c, strings.ToUpper(a.projectDisplayName(s.Project))+" DESKTOP").
			Font("monospace").FontSize(11).TextColor(p.accent2).LetterSpacing(0.8)
		ui.Text(c, "What should we build?").FontSize(28).FontWeight(600).TextColor(p.text).
			LetterSpacing(-0.8).Margin(0, 0, 7, 0)
		ui.Text(c, "Describe a task, point at a change, or start from a workspace suggestion. This session keeps its plan, terminal, and review state together.").
			FontSize(14).LineHeight(1.65).TextColor(p.text3).MaxWidth(520)
		ui.Row(c).Gap(6).Wrap().Margin(0, 0, 14, 0).Children(func() {
			for _, chip := range []struct {
				icon, text string
				accent     bool
			}{
				{"folder", a.projectDisplayName(s.Project) + " / " + s.Branch, false},
				{"shield", approvalLabel(s.Approval), false},
				{"sparkles", formatTokens(s.Context.Limit) + " context", true},
			} {
				ch := chip
				box := ui.Row(c).Height(21).Padding(0, 7).Gap(5).Radius(5).
					Border(1, chooser(ch.accent, p.accent.Alpha(0.16), p.line)).
					Background(chooser(ch.accent, p.accentSofter, ui.Transparent)).AlignItems(ui.Center)
				box.Children(func() {
					color := p.text4
					if ch.accent {
						color = p.accent2
					}
					a.icon(c, ch.icon, 10, color)
					ui.Text(c, ch.text).Font("monospace").FontSize(10).TextColor(color)
				})
			}
		})
		suggestions := []struct {
			icon, title, desc, prompt string
		}{
			{"eye", "Review uncommitted changes", "Walk the working tree and flag risky edits", "Review the uncommitted changes in this workspace and flag anything risky."},
			{"plug", "Add a provider", "Wire a model provider into settings", "Add a model provider and make it available in the model picker."},
			{"book", "Explain the session SDK", "Map events, approvals, and persistence", "Explain how the session SDK projects events, approvals, and persistence."},
			{"test", "Fix failing tests", "Run the suite and close the gap", "Run the test suite and fix the first real failure."},
		}
		grid := ui.Grid(c).FillWidth().Columns(2).Gap(7).Margin(0, 0, 24, 0)
		grid.Children(func() {
			for _, sg := range suggestions {
				sugg := sg
				card := ui.ButtonBase(c).Height(62).Padding(10, 11).Gap(10).Radius(9).
					Border(1, p.line).Background(p.surface.Alpha(0.48)).Justify(ui.Start).
					AlignItems(ui.Start).Label(sugg.title)
				if card.Hovered() {
					card.BorderColor(p.lineStrong).Background(p.surface)
				}
				if card.Clicked() {
					a.draft = sugg.prompt
					a.focusComposer = true
				}
				card.Children(func() {
					iconBox := ui.Box(c).Size(24, 24).Radius(6).Border(1, p.line).Background(p.surface2).Center()
					iconBox.Children(func() { a.icon(c, sugg.icon, 13, p.text3) })
					ui.Column(c).Grow(1).Gap(2).MinWidth(0).Children(func() {
						ui.Text(c, sugg.title).FontSize(13).FontWeight(550).TextColor(p.text2).SingleLine()
						ui.Text(c, sugg.desc).Font("monospace").FontSize(10).TextColor(p.text4).SingleLine()
					})
					a.icon(c, "arrow-right", 12, p.text4).AlignSelf(ui.Center)
				})
			}
		})
	})
}
