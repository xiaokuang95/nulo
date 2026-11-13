package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) dock(c *ui.Context, s *session, width float32) {
	p := a.pal()
	if width < dockWidthMin {
		width = 420
	}
	col := ui.Column(c).Width(width).Shrink(0).FillHeight().Background(p.sidebar).
		BorderWidth(0, 0, 0, 1).BorderColor(p.line)
	col.Children(func() {
		handle := ui.Box(c).Absolute().Left(-3).Top(0).Width(6).FillHeight().
			Cursor(ui.CursorResizeEW)
		if handle.Pressed() {
			if a.dockDragStart == 0 {
				a.dockDragStart = width
			}
			dx, _, _ := handle.Dragged()
			a.dockWidth = clampFloat(a.dockDragStart-dx, dockWidthMin, dockWidthMax)
			a.dockSized = true
		} else if a.dockDragStart != 0 {
			a.dockDragStart = 0
		}
		a.dockHeader(c, s, width)
		body := ui.Scroll(c).Grow(1).MinHeight(0).Padding(12)
		body.Children(func() {
			switch a.dockTab {
			case 0:
				a.dockChanges(c, s)
			case 1:
				a.dockPlan(c, s)
			case 2:
				a.dockTerminal(c, s)
			case 3:
				a.dockContext(c, s)
			}
		})
	})
}

func (a *app) dockHeader(c *ui.Context, s *session, width float32) {
	p := a.pal()
	compact := width < 400
	tiny := width < 340
	pad := float32(7)
	left := float32(10)
	if compact {
		pad, left = 4, 7
	}
	header := ui.Row(c).Height(42).FillWidth().Padding(0, 7, 0, left).
		BorderWidth(0, 0, 1, 0).BorderColor(p.line).AlignItems(ui.Stretch)
	header.Children(func() {
		added, _ := s.changeStats()
		reviewed, total := s.reviewCount()
		_ = added
		pct := 0.0
		if s.Context.Limit > 0 {
			pct = float64(s.Context.Used) * 100 / float64(s.Context.Limit)
		}
		tabs := []struct {
			label string
			icon  string
			count string
		}{
			{"Changes", "diff", fmt.Sprintf("%d", total)},
			{"Plan", "list-checks", fmt.Sprintf("%d/%d", countDone(s.Plan.Steps), len(s.Plan.Steps))},
			{"Terminal", "terminal", ""},
			{"Context", "layers", fmt.Sprintf("%.1f%%", pct)},
		}
		for i, tab := range tabs {
			idx := i
			label := tab.label
			count := tab.count
			if compact {
				count = ""
			}
			b := ui.ButtonBase(c).Height(42).Padding(0, pad).Gap(4).Justify(ui.Start).
				Background(ui.Transparent).Label(tab.label)
			if a.dockTab == i {
				b.BorderWidth(0, 0, 1.5, 0).BorderColor(p.accent)
			}
			if b.Hovered() && a.dockTab != i {
				b.Background(p.hover)
			}
			if b.Clicked() {
				a.dockTab = i
			}
			b.Children(func() {
				color := p.text4
				if a.dockTab == idx {
					color = p.text
				}
				if !tiny {
					a.icon(c, tab.icon, 12, color)
				}
				ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(color).SingleLine()
				if count != "" {
					badge := ui.Box(c).Height(13).Padding(0, 4).Radius(3).Center()
					if a.dockTab == idx {
						badge.Background(p.accentSoft)
					} else {
						badge.Background(p.surface2)
					}
					badge.Children(func() {
						c2 := p.text4
						if a.dockTab == idx {
							c2 = p.accent
						}
						ui.Text(c, count).Font("monospace").FontSize(9).TextColor(c2)
					})
				}
			})
		}
		ui.Spacer(c)
		a.iconButton(c, "x", "Close review dock", false, a.closeDock)
		_ = reviewed
	})
}

func countDone(steps []planStep) int {
	n := 0
	for _, st := range steps {
		if st.Status == "done" {
			n++
		}
	}
	return n
}

func (a *app) dockSectionHead(c *ui.Context, title, note string) {
	p := a.pal()
	ui.Row(c).FillWidth().Height(22).AlignItems(ui.Center).Gap(7).Children(func() {
		a.sectionTitle(c, title)
		ui.Spacer(c)
		if note != "" {
			ui.Text(c, note).Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
		}
	})
}

func (a *app) dockChanges(c *ui.Context, s *session) {
	p := a.pal()
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		if s.Changes == nil {
			empty := ui.Column(c).FillWidth().Padding(18).Gap(7).Radius(8).Border(1, p.line).
				Background(p.surface.Alpha(0.55)).AlignItems(ui.Center)
			empty.Children(func() {
				a.icon(c, "diff", 24, p.text4)
				ui.Text(c, "No changes yet").FontSize(13).FontWeight(550).TextColor(p.text2)
				ui.Text(c, "File edits from this session will collect here for review.").
					FontSize(11).TextColor(p.text4).TextAlign(ui.Center)
			})
			return
		}
		add, del := s.changeStats()
		reviewed, total := s.reviewCount()
		idx := a.selectedFileIndex(s)
		file := &s.Changes.Files[idx]

		summary := ui.Column(c).FillWidth().Clip().Radius(9).Border(1, p.line).
			Background(p.surface.Alpha(0.64))
		summary.Children(func() {
			top := ui.Column(c).FillWidth().Padding(11, 12, 10, 12).Gap(8)
			top.Children(func() {
				row := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8)
				row.Children(func() {
					ui.Textf(c, "%d files changed", total).FontSize(13).FontWeight(600).TextColor(p.text2).Grow(1)
					ui.Text(c, s.Branch).Font("monospace").FontSize(10).TextColor(p.text4).SingleLine()
				})
				row2 := ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Center)
				row2.Children(func() {
					ui.Textf(c, "+%d", add).Font("monospace").FontSize(11).TextColor(p.accent2)
					ui.Textf(c, "−%d", del).Font("monospace").FontSize(11).TextColor(p.bad)
					ui.Text(c, "·").Font("monospace").FontSize(10).TextColor(p.text4)
					ui.Textf(c, "%d/%d reviewed", reviewed, total).Font("monospace").FontSize(10).TextColor(p.text4)
					ui.Spacer(c)
					if s.Changes.Staged {
						if a.primaryButton(c, "Commit…", "").Clicked() {
							a.commitMsg = ""
							a.commitOpen = true
						}
					} else {
						if a.smallButton(c, "Stage all").Clicked() {
							if a.host != nil && s.Real {
								a.stageAll(s)
							} else {
								s.Changes.Staged = true
								a.pendingToast = "Staged all changes"
							}
						}
					}
				})
				a.progress(c, percentInt(reviewed, total), 3, p.accent)
			})
			if s.pendingApproval() != nil {
				banner := ui.Row(c).FillWidth().Padding(8, 10).Gap(8).AlignItems(ui.Center).
					BorderWidth(1, 0, 0, 0).BorderColor(p.warn.Alpha(0.20)).Background(p.warnSoft)
				banner.Children(func() {
					a.icon(c, "alert", 12, p.warn)
					ui.Text(c, "1 command awaiting approval. Review it in the thread before handing off.").
						FontSize(11).LineHeight(1.45).TextColor(p.warn).Grow(1)
				})
			}
		})

		a.dockSectionHead(c, "Files", fmt.Sprintf("%d/%d reviewed", reviewed, total))
		files := ui.Column(c).FillWidth().Gap(1)
		files.Children(func() {
			for i := range s.Changes.Files {
				fi := i
				f := &s.Changes.Files[i]
				active := fi == idx
				row := ui.ButtonBase(c).FillWidth().Height(34).Padding(5, 7).Gap(7).
					Radius(5).Justify(ui.Start).Background(ui.Transparent).Label(f.Path)
				if active {
					row.Background(p.surface2).BorderWidth(0, 0, 0, 2).BorderColor(p.accent)
				} else if row.Hovered() {
					row.Background(p.hover)
				}
				if row.Clicked() {
					a.selectedFile[s.ID] = fi
				}
				row.Children(func() {
					if a.checkbox(c, f.Reviewed, "Mark "+f.Path+" reviewed").Clicked() {
						f.Reviewed = !f.Reviewed
						if f.Reviewed {
							a.pendingToast = "Marked " + f.Path + " reviewed"
						}
					}
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
					ui.Text(c, f.Path).Font("monospace").FontSize(11).TextColor(chooser(active, p.text2, p.text3)).
						Grow(1).SingleLine()
					ui.Textf(c, "+%d", f.Add).Font("monospace").FontSize(9).TextColor(p.accent2)
					ui.Textf(c, "−%d", f.Del).Font("monospace").FontSize(9).TextColor(p.bad)
				})
			}
		})
		ui.Row(c).FillWidth().Gap(4).Children(func() {
			if reviewed < total {
				if a.smallButton(c, "Mark all reviewed").Clicked() {
					for i := range s.Changes.Files {
						s.Changes.Files[i].Reviewed = true
					}
					a.pendingToast = "All files reviewed"
				}
			}
			ui.Spacer(c)
			if a.ghostButton(c, "Prev").Disabled(idx == 0).Clicked() {
				a.selectedFile[s.ID] = idx - 1
			}
			if a.ghostButton(c, "Next").Disabled(idx >= total-1).Clicked() {
				a.selectedFile[s.ID] = idx + 1
			}
		})

		a.dockSectionHead(c, "Diff", fmt.Sprintf("%s · %d/%d", file.Language, idx+1, total))
		a.diffCard(c, s, file, idx, total)
	})
}

func percentInt(a, b int) int {
	if b == 0 {
		return 0
	}
	return a * 100 / b
}

func (a *app) diffCard(c *ui.Context, s *session, f *changeFile, idx, total int) {
	p := a.pal()
	card := ui.Column(c).FillWidth().Clip().Radius(7).Border(1, p.line).
		Background(p.surface.Alpha(0.50))
	card.Children(func() {
		head := ui.Row(c).FillWidth().Height(36).Padding(0, 9).Gap(7).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.line)
		head.Children(func() {
			a.icon(c, "file", 12, p.text4)
			ui.Text(c, f.Path).Font("monospace").FontSize(10).TextColor(p.text3).Grow(1).SingleLine()
			ui.Text(c, strings.ToUpper(f.Language)).Font("monospace").FontSize(9).TextColor(p.text4)
			ui.Text(c, "‹ ›").Font("monospace").FontSize(10).TextColor(p.text4)
			a.iconButton(c, "copy", "Copy patch", false, func() {
				c.Services().WriteClipboard(strings.Join(f.Diff, "\n"))
				a.pendingToast = "Copied patch"
			})
		})
		diff := ui.Column(c).FillWidth().Background(p.codeBg)
		diff.Children(func() {
			oldLine, newLine := 0, 0
			for _, line := range f.Diff {
				if strings.HasPrefix(line, "@@") {
					fmt.Sscanf(line, "@@ -%d", &oldLine)
					if i := strings.Index(line, " +"); i >= 0 {
						fmt.Sscanf(line[i+2:], "%d", &newLine)
					}
					row := ui.Row(c).FillWidth().Height(22).Padding(0, 9).AlignItems(ui.Center).
						Background(p.surface2)
					row.Children(func() {
						ui.Text(c, line).Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
					})
					continue
				}
				if oldLine == 0 && newLine == 0 {
					oldLine, newLine = 1, 1
				}
				display := newLine
				kind := "ctx"
				if strings.HasPrefix(line, "+") {
					kind = "add"
					newLine++
				} else if strings.HasPrefix(line, "-") {
					kind = "del"
					display = 0
					oldLine++
				} else {
					oldLine++
					newLine++
				}
				a.diffRow(c, line, display, kind)
			}
		})
	})
}

func (a *app) diffRow(c *ui.Context, line string, number int, kind string) {
	p := a.pal()
	bg := ui.Transparent
	textColor := p.text3
	mark := " "
	if kind == "add" {
		bg = p.accent.Alpha(0.07)
		textColor = p.text2
		mark = "+"
	} else if kind == "del" {
		bg = p.bad.Alpha(0.07)
		textColor = p.text3
		mark = "−"
	}
	row := ui.Row(c).FillWidth().Height(19).Padding(0, 0).AlignItems(ui.Center).Background(bg)
	label := ""
	if number > 0 {
		label = fmt.Sprintf("%d", number)
	}
	body := line
	if strings.HasPrefix(body, "+") || strings.HasPrefix(body, "-") {
		body = body[1:]
	}
	row.Children(func() {
		ui.Text(c, label).Font("monospace").FontSize(9).TextColor(p.text4).
			Width(34).TextAlign(ui.End).Padding(0, 6, 0, 0)
		barColor := ui.Transparent
		if kind == "add" {
			barColor = p.accent2
		} else if kind == "del" {
			barColor = p.bad
		}
		ui.Box(c).Width(2).Height(19).Background(barColor)
		ui.Text(c, mark+" "+body).Font("monospace").
			FontSize(10.5).TextColor(textColor).Padding(0, 5).SingleLine()
	})
}

func (a *app) dockPlan(c *ui.Context, s *session) {
	p := a.pal()
	if len(s.Plan.Steps) == 0 {
		empty := ui.Column(c).FillWidth().Padding(18).Gap(7).Radius(8).Border(1, p.line).
			Background(p.surface.Alpha(0.55)).AlignItems(ui.Center).Margin(0, 0, 0, 0)
		empty.Children(func() {
			a.icon(c, "list-checks", 24, p.text4)
			ui.Text(c, "No plan yet").FontSize(13).FontWeight(550).TextColor(p.text2)
			text := "Send a request and the agent will lay out its steps here."
			if a.host != nil && s.Real {
				text = "The SDK session has no structured plan. Tool activity appears in the thread and terminal."
			}
			ui.Text(c, text).FontSize(11).TextColor(p.text4).TextAlign(ui.Center)
		})
		return
	}
	done, total := countDone(s.Plan.Steps), len(s.Plan.Steps)
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		card := ui.Column(c).FillWidth().Clip().Radius(9).Padding(12).Gap(9).
			Border(1, p.line).Background(p.surface.Alpha(0.64))
		card.Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				a.icon(c, "list-checks", 15, p.accent)
				ui.Text(c, s.Plan.Title).FontSize(14).FontWeight(600).TextColor(p.text).Grow(1)
				ui.Textf(c, "%d/%d", done, total).Font("monospace").FontSize(10).TextColor(p.text4)
			})
			a.progress(c, s.planProgress(), 4, p.accent)
			ui.Textf(c, "%d%% complete", s.planProgress()).Font("monospace").FontSize(10).TextColor(p.text4)
		})

		a.dockSectionHead(c, "Steps", "live plan")
		steps := ui.Column(c).FillWidth().Clip().Radius(8).Border(1, p.line).
			Background(p.surface.Alpha(0.52)).Padding(6, 10)
		steps.Children(func() {
			for i, st := range s.Plan.Steps {
				stepCard := ui.Column(c).FillWidth().Padding(7, 4).Gap(1)
				if i > 0 {
					stepCard.BorderWidth(1, 0, 0, 0).BorderColor(p.line.Alpha(0.6))
				}
				stepCard.Children(func() {
					a.planStep(c, i, st)
					if st.Status == "active" {
						ui.Text(c, "in progress").Font("monospace").FontSize(9).TextColor(p.accent).
							Margin(0, 0, 0, 24)
					}
				})
			}
		})
		if s.Plan.Next != "" {
			next := ui.Row(c).FillWidth().Padding(10, 11).Gap(7).Radius(7).Border(1, p.line).
				Background(p.surface.Alpha(0.5)).AlignItems(ui.Center)
			next.Children(func() {
				a.icon(c, "arrow-right", 12, p.accent2)
				ui.Text(c, s.Plan.Next).FontSize(12).TextColor(p.text3).Grow(1)
			})
		}
	})
}

func (a *app) dockTerminal(c *ui.Context, s *session) {
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

func (a *app) termLine(c *ui.Context, line termLine) {
	p := a.pal()
	color := p.text3
	prefix := ""
	switch line.Kind {
	case "cmd":
		color = p.text2
		prefix = "❯ "
	case "ok":
		color = p.accent2
	case "err":
		color = p.bad
	case "wait":
		color = p.warn
		prefix = "… "
	case "run":
		color = p.warn
		prefix = "● "
	}
	ui.Text(c, prefix+line.Text).Font("monospace").FontSize(11).LineHeight(1.6).
		TextColor(color).Selectable()
}

func (a *app) dockContext(c *ui.Context, s *session) {
	p := a.pal()
	pct := 0.0
	if s.Context.Limit > 0 {
		pct = float64(s.Context.Used) * 100 / float64(s.Context.Limit)
	}
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		card := ui.Column(c).FillWidth().Clip().Radius(9).Border(1, p.line).
			Background(p.surface.Alpha(0.64)).Padding(12)
		card.Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(9).Children(func() {
				big := ui.Row(c).Gap(2).AlignItems(ui.Baseline)
				big.Children(func() {
					ui.Textf(c, "%.1f", pct).Font("monospace").FontSize(28).FontWeight(600).TextColor(p.accent)
					ui.Text(c, "%").Font("monospace").FontSize(14).TextColor(p.accent2)
				})
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, "context used").FontSize(12).TextColor(p.text3)
					ui.Textf(c, "%s of %s tokens", formatTokens(s.Context.Used), formatTokens(s.Context.Limit)).
						Font("monospace").FontSize(10).TextColor(p.text4)
				})
				if a.primaryButton(c, "Compact", "compress").Clicked() {
					a.compact()
				}
			})
			a.contextBar(c, s)
			legend := ui.Grid(c).FillWidth().Columns(2).Gap(6).Margin(2, 0, 0, 0)
			legend.Children(func() {
				for _, br := range s.Context.Breakdown {
					item := br
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(6, 6).Radius(6).Background(a.contextColor(item.Kind))
						ui.Text(c, item.Label).FontSize(10).TextColor(p.text4).Grow(1).SingleLine()
						ui.Text(c, formatTokens(item.Value)).Font("monospace").FontSize(9).TextColor(p.text3)
					})
				}
			})
		})

		files := 0
		for _, e := range s.Context.Entries {
			if e.Kind != "skill" {
				files++
			}
		}
		a.dockSectionHead(c, "Loaded", fmt.Sprintf("%d files · %d skills", files, s.Context.Skills))
		entries := ui.Column(c).FillWidth().Clip().Radius(8).Border(1, p.line).
			Background(p.surface.Alpha(0.5)).Padding(4, 8)
		entries.Children(func() {
			for i, e := range s.Context.Entries {
				entry := e
				row := ui.Row(c).FillWidth().Height(29).AlignItems(ui.Center).Gap(8)
				if i > 0 {
					row.BorderWidth(1, 0, 0, 0).BorderColor(p.line.Alpha(0.55))
				}
				row.Children(func() {
					icon := "file"
					color := p.text4
					if entry.Kind == "skill" {
						icon, color = "sparkles", p.violet
					} else if entry.Kind == "instructions" {
						icon, color = "book", p.accent2
					}
					a.icon(c, icon, 11, color)
					ui.Text(c, entry.Path).Font("monospace").FontSize(10).TextColor(p.text3).Grow(1).SingleLine()
					ui.Text(c, formatTokens(entry.Tokens)).Font("monospace").FontSize(9).TextColor(p.text4)
				})
			}
		})

		a.dockSectionHead(c, "Capabilities", "local runtime")
		caps := ui.Row(c).FillWidth().Gap(6).Children(func() {
			for _, cp := range []struct {
				icon, label string
			}{
				{"plug", fmt.Sprintf("%d MCP", s.Context.MCP)},
				{"sparkles", fmt.Sprintf("%d skills", s.Context.Skills)},
				{"package", fmt.Sprintf("%d plugins", s.Context.Plugins)},
			} {
				item := cp
				box := ui.Row(c).Height(26).Padding(0, 8).Gap(5).Radius(6).Border(1, p.line).
					Background(p.surface2).AlignItems(ui.Center)
				box.Children(func() {
					a.icon(c, item.icon, 11, p.text4)
					ui.Text(c, item.label).Font("monospace").FontSize(9).TextColor(p.text3)
				})
			}
		})
		_ = caps
	})
}

func (a *app) contextBar(c *ui.Context, s *session) {
	p := a.pal()
	bar := ui.Row(c).FillWidth().Height(7).Radius(7).Background(p.line).Clip()
	bar.Children(func() {
		for _, br := range s.Context.Breakdown {
			if br.Value <= 0 || s.Context.Limit <= 0 {
				continue
			}
			ui.Box(c).WidthPercent(float32(br.Value) * 100 / float32(s.Context.Limit)).
				FillHeight().Background(a.contextColor(br.Kind))
		}
	})
}

func (a *app) contextColor(kind string) ui.Color {
	p := a.pal()
	switch kind {
	case "accent":
		return p.accent
	case "info":
		return p.info
	case "warn":
		return p.warn
	case "violet":
		return p.violet
	}
	return p.text4
}

func clampFloat(v, lo, hi float32) float32 {
	return max(lo, min(v, hi))
}
