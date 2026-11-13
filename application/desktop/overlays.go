package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) overlays(c *ui.Context, s *session) {
	if a.paletteOpen {
		a.palette(c)
	}
	if a.settingsOpen {
		a.settings(c, s)
	}
	if a.commitOpen {
		a.commitDialog(c, s)
	}
	if a.renameOpen {
		a.renameDialog(c)
	}
}

func (a *app) palette(c *ui.Context) {
	p := a.pal()
	ui.DialogBase(c, &a.paletteOpen, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.52))
		panel.Width(640).MaxHeight(520).Padding(0).Radius(12).
			Border(1, p.lineStrong).Background(p.panel).Clip()
		ui.Column(c).FillWidth().Children(func() {
			search := ui.Row(c).FillWidth().Height(54).Padding(0, 15).Gap(10).
				BorderWidth(0, 0, 1, 0).BorderColor(p.line).AlignItems(ui.Center)
			search.Children(func() {
				a.icon(c, "search", 15, p.text4)
				in := ui.TextInputBase(c, &a.paletteQuery).
					Placeholder("Search sessions, actions, and settings…").
					Label("Search sessions, actions, and settings").
					FontSize(14).TextColor(p.text).Grow(1).AutoFocus().KeepFocus()
				items := a.paletteItems()
				in.OnTextKey(func(k ui.TextKey) bool {
					switch k.Key {
					case ui.KeyUp:
						if len(items) > 0 {
							a.paletteIndex = (a.paletteIndex - 1 + len(items)) % len(items)
						}
						return true
					case ui.KeyDown:
						if len(items) > 0 {
							a.paletteIndex = (a.paletteIndex + 1) % len(items)
						}
						return true
					case ui.KeyEnter:
						a.choosePalette(a.paletteIndex)
						return true
					case ui.KeyEscape:
						a.paletteOpen = false
						return true
					}
					return false
				})
				if in.Changed() {
					a.paletteIndex = 0
				}
				ui.Text(c, "esc").Font("monospace").FontSize(10).TextColor(p.text4).
					Padding(2, 6).Radius(4).Border(1, p.lineStrong).Background(p.surface2)
			})

			list := ui.Scroll(c).FillWidth().MaxHeight(380).Padding(6)
			list.Children(func() {
				items := a.paletteItems()
				if len(items) == 0 {
					ui.Column(c).Padding(24).AlignItems(ui.Center).Gap(4).Children(func() {
						a.icon(c, "search", 18, p.text4)
						ui.Text(c, "No matches").FontSize(13).TextColor(p.text3)
						ui.Text(c, "Try a session name, “theme”, or “settings”.").FontSize(11).TextColor(p.text4)
					})
					return
				}
				if a.paletteIndex >= len(items) {
					a.paletteIndex = len(items) - 1
				}
				for i, it := range items {
					idx := i
					item := it
					row := ui.ButtonBase(c).FillWidth().Height(43).Padding(7, 9).Gap(9).
						Radius(6).Justify(ui.Start).Label(it.Title)
					if i == a.paletteIndex {
						row.Background(p.accentSoft)
					} else if row.Hovered() {
						row.Background(p.surface2)
					}
					if row.Clicked() {
						a.choosePalette(idx)
					}
					row.Children(func() {
						iconBox := ui.Box(c).Size(24, 24).Radius(6).
							Background(chooser(idx == a.paletteIndex, p.accent.Alpha(0.12), p.surface2)).Center()
						iconBox.Children(func() {
							a.icon(c, item.Icon, 12, chooser(idx == a.paletteIndex, p.accent, p.text4))
						})
						ui.Column(c).Grow(1).Gap(1).MinWidth(0).Children(func() {
							ui.Text(c, item.Title).FontSize(12).FontWeight(550).
								TextColor(chooser(idx == a.paletteIndex, p.text, p.text2)).SingleLine()
							ui.Text(c, item.Detail).Font("monospace").FontSize(10).TextColor(p.text4).SingleLine()
						})
						if item.Key != "" {
							a.shortcutKey(c, item.Key)
						}
					})
				}
			})

			foot := ui.Row(c).FillWidth().Height(34).Padding(0, 14).Gap(14).
				BorderWidth(1, 0, 0, 0).BorderColor(p.line).AlignItems(ui.Center)
			foot.Children(func() {
				ui.Text(c, "↑↓ navigate").Font("monospace").FontSize(9).TextColor(p.text4)
				ui.Text(c, "↵ select").Font("monospace").FontSize(9).TextColor(p.text4)
				ui.Spacer(c)
				ui.Text(c, "nulo 0.9 · native").Font("monospace").FontSize(9).TextColor(p.text4)
			})
		})
	})
}

func (a *app) settings(c *ui.Context, s *session) {
	p := a.pal()
	ui.DialogBase(c, &a.settingsOpen, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.48))
		panel.Width(880).Height(610).Padding(0).Radius(12).
			Border(1, p.lineStrong).Background(p.sidebar).Clip()
		ui.Column(c).FillWidth().FillHeight().Children(func() {
			head := ui.Row(c).FillWidth().Height(66).Padding(0, 20).Gap(12).
				BorderWidth(0, 0, 1, 0).BorderColor(p.line).AlignItems(ui.Center)
			head.Children(func() {
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, "NULO DESKTOP").Font("monospace").FontSize(9).TextColor(p.accent2).LetterSpacing(0.8)
					ui.Text(c, "Settings").FontSize(20).FontWeight(600).TextColor(p.text)
				})
				a.iconButton(c, "x", "Close settings", false, func() { a.settingsOpen = false })
			})
			body := ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
			body.Children(func() {
				a.settingsNav(c)
				content := ui.Scroll(c).Grow(1).MinHeight(0).Padding(22, 24)
				content.Children(func() {
					switch a.settingsTab {
					case 0:
						a.settingsGeneral(c, s)
					case 1:
						a.settingsProviders(c, s)
					case 2:
						a.settingsCapabilities(c, s)
					case 3:
						a.settingsAppearance(c)
					case 4:
						a.settingsShortcuts(c)
					case 5:
						a.settingsAbout(c)
					}
				})
			})
			foot := ui.Row(c).FillWidth().Height(48).Padding(0, 18).Gap(10).
				BorderWidth(1, 0, 0, 0).BorderColor(p.line).AlignItems(ui.Center)
			foot.Children(func() {
				ui.Text(c, "Preferences apply immediately. Sessions and workspace files stay local.").
					Font("monospace").FontSize(9).TextColor(p.text4).Grow(1)
				if a.primaryButton(c, "Done", "").Clicked() {
					a.settingsOpen = false
				}
			})
		})
	})
}

func (a *app) settingsNav(c *ui.Context) {
	p := a.pal()
	items := []struct {
		label string
		icon  string
	}{
		{"General", "settings"},
		{"Providers & models", "sparkles"},
		{"Capabilities", "plug"},
		{"Appearance", "sun"},
		{"Shortcuts", "keyboard"},
		{"About", "info"},
	}
	nav := ui.Column(c).Width(192).Shrink(0).FillHeight().Padding(10, 9).
		BorderWidth(0, 1, 0, 0).BorderColor(p.line).Gap(2)
	nav.Children(func() {
		for i, item := range items {
			idx := i
			b := ui.ButtonBase(c).FillWidth().Height(34).Padding(0, 9).Gap(8).Radius(6).
				Justify(ui.Start).Label(item.label)
			if a.settingsTab == i {
				b.Background(p.accentSoft)
			} else if b.Hovered() {
				b.Background(p.hover)
			}
			if b.Clicked() {
				a.settingsTab = i
			}
			b.Children(func() {
				color := p.text4
				if a.settingsTab == idx {
					color = p.accent
				}
				a.icon(c, item.icon, 13, color)
				ui.Text(c, item.label).FontSize(12).TextColor(chooser(a.settingsTab == idx, p.text, p.text3)).SingleLine()
			})
		}
	})
}

func (a *app) settingsGeneral(c *ui.Context, s *session) {
	p := a.pal()
	a.settingsHeading(c, "General", "Defaults for new sessions in this window.")
	a.settingsGroup(c, "Workspace", func() {
		workspace := "~/Downloads/nulo · main"
		if s.CWD != "" {
			workspace = s.CWD + " · " + s.Branch
		}
		a.settingRow(c, "Workspace", workspace, func() {
			ui.Text(c, "Local").Font("monospace").FontSize(10).TextColor(p.accent2)
		})
		a.settingRow(c, "Default model", s.modelName(), func() {
			ui.Text(c, a.providerLabel(s.modelProvider())).Font("monospace").FontSize(10).TextColor(p.text4)
		})
		a.settingRow(c, "Permission mode", approvalLabel(s.Approval), func() {
			a.permissionChoices(c, s)
		})
		a.settingRow(c, "Thinking level", s.thinkingName(), func() {
			a.thinkingChoices(c, s)
		})
	})
	a.settingsGroup(c, "Session defaults", func() {
		a.settingRow(c, "Environment", s.Env, func() {
			if a.host != nil {
				ui.Text(c, "Local").FontSize(11).TextColor(p.text4)
				return
			}
			ui.Row(c).Gap(4).Children(func() {
				for _, env := range []string{"Local", "Worktree"} {
					e := env
					b := ui.ButtonBase(c).Height(25).Padding(0, 9).Radius(5).Label(e).
						Border(1, p.line).Background(p.surface2)
					if s.Env == e {
						b.Background(p.accentSoft).BorderColor(p.accent.Alpha(0.28))
					}
					if b.Clicked() {
						s.Env = e
					}
					b.Children(func() {
						color := p.text3
						if s.Env == e {
							color = p.accent
						}
						ui.Text(c, e).FontSize(11).TextColor(color)
					})
				}
			})
		})
		a.settingRow(c, "Show timestamps", "Inline metadata next to messages", func() {
			ui.Text(c, "On").Font("monospace").FontSize(10).TextColor(p.accent2)
		})
		a.settingRow(c, "Reasoning summaries", "Collapse long thinking blocks", func() {
			ui.Text(c, "Collapsed").Font("monospace").FontSize(10).TextColor(p.text4)
		})
	})
}

func (a *app) settingsHeading(c *ui.Context, title, sub string) {
	p := a.pal()
	ui.Column(c).FillWidth().Gap(3).Margin(0, 0, 18, 0).Children(func() {
		ui.Text(c, title).FontSize(20).FontWeight(600).TextColor(p.text)
		ui.Text(c, sub).FontSize(12).TextColor(p.text4)
	})
}

func (a *app) settingsGroup(c *ui.Context, title string, build func()) {
	p := a.pal()
	ui.Column(c).FillWidth().Gap(7).Margin(0, 0, 16, 0).Children(func() {
		a.sectionTitle(c, title)
		card := ui.Column(c).FillWidth().Clip().Radius(8).Border(1, p.line).
			Background(p.surface.Alpha(0.5))
		card.Children(func() {
			ui.Column(c).FillWidth().Padding(0, 12).Children(build)
		})
	})
}

func (a *app) settingRow(c *ui.Context, title, desc string, control func()) {
	p := a.pal()
	ui.Row(c).FillWidth().MinHeight(48).PaddingY(8).Gap(12).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.line.Alpha(0.6)).Children(func() {
		ui.Column(c).Grow(1).Gap(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(12).FontWeight(500).TextColor(p.text2)
			ui.Text(c, desc).FontSize(11).TextColor(p.text4).SingleLine()
		})
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(control)
	})
}

func (a *app) permissionChoices(c *ui.Context, s *session) {
	p := a.pal()
	for _, id := range []string{"manual", "auto-edits", "plan", "auto"} {
		mode := id
		b := ui.ButtonBase(c).Height(25).Padding(0, 8).Radius(5).Label(approvalLabel(mode)).
			Border(1, p.line).Background(p.surface2)
		if s.Approval == mode {
			b.Background(p.accentSoft).BorderColor(p.accent.Alpha(0.28))
		}
		if b.Clicked() {
			s.Approval = mode
		}
		b.Children(func() {
			color := p.text3
			if s.Approval == mode {
				color = p.accent
			}
			ui.Text(c, approvalLabel(mode)).FontSize(10).TextColor(color)
		})
	}
}

func (a *app) thinkingChoices(c *ui.Context, s *session) {
	p := a.pal()
	for _, level := range []string{"low", "medium", "high"} {
		l := level
		b := ui.ButtonBase(c).Height(25).Padding(0, 9).Radius(5).Label(l).
			Border(1, p.line).Background(p.surface2)
		if s.Thinking == l {
			b.Background(p.accentSoft).BorderColor(p.accent.Alpha(0.28))
		}
		if b.Clicked() {
			a.setSessionThinking(s, l)
		}
		b.Children(func() {
			color := p.text3
			if s.Thinking == l {
				color = p.accent
			}
			ui.Text(c, strings.ToUpper(l)).Font("monospace").FontSize(9).TextColor(color)
		})
	}
}

func (a *app) settingsProviders(c *ui.Context, s *session) {
	p := a.pal()
	a.settingsHeading(c, "Providers & models", "Models stay grouped under the provider that serves them.")
	defaultSpec := s.Model
	if a.host != nil && a.host.defaultMdl != "" {
		defaultSpec = a.host.defaultMdl
	}
	for _, prov := range a.providers {
		provider := prov
		card := ui.Column(c).FillWidth().Clip().Radius(9).Border(1, p.line).
			Background(p.surface.Alpha(0.5)).Margin(0, 0, 10, 0)
		card.Children(func() {
			head := ui.Row(c).FillWidth().Height(48).Padding(0, 12).Gap(9).
				BorderWidth(0, 0, 1, 0).BorderColor(p.line).AlignItems(ui.Center)
			head.Children(func() {
				box := ui.Box(c).Size(28, 28).Radius(7).Background(p.surface2).Center()
				box.Children(func() { a.icon(c, "globe", 14, p.accent2) })
				ui.Column(c).Grow(1).Gap(1).Children(func() {
					ui.Text(c, provider.Name).FontSize(13).FontWeight(600).TextColor(p.text2)
					ui.Text(c, provider.BaseURL).Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
				})
				ui.Text(c, provider.API).Font("monospace").FontSize(9).TextColor(p.text4).
					Padding(2, 6).Radius(4).Background(p.surface2)
			})
			for _, mdl := range provider.Models {
				m := mdl
				id := provider.ID + "/" + m.ID
				row := ui.ButtonBase(c).FillWidth().Height(42).Padding(0, 12, 0, 46).Gap(8).
					Justify(ui.Start).Background(ui.Transparent).Label(m.Name)
				if defaultSpec == id {
					row.Background(p.accentSofter)
				} else if row.Hovered() {
					row.Background(p.surface2)
				}
				if row.Clicked() {
					a.setDefaultModel(s, id)
					a.pendingToast = "Default model: " + m.Name
				}
				row.Children(func() {
					a.icon(c, "sparkles", 13, chooser(defaultSpec == id, p.accent2, p.text4))
					ui.Text(c, m.Name).FontSize(12).TextColor(p.text2).Grow(1)
					ui.Textf(c, "%s context", formatTokens(m.Context)).Font("monospace").FontSize(9).TextColor(p.text4)
					if defaultSpec == id {
						ui.Text(c, "DEFAULT").Font("monospace").FontSize(8).TextColor(p.accent).
							Padding(2, 5).Radius(3).Background(p.accentSoft)
					}
				})
			}
		})
	}
}

func (a *app) settingsCapabilities(c *ui.Context, s *session) {
	p := a.pal()
	a.settingsHeading(c, "Capabilities", "Loaded by the SDK for this session and workspace.")
	if a.host == nil {
		a.settingsGroup(c, "MCP servers", func() {
			for _, server := range []struct{ name, status, detail string }{
				{"filesystem", "connected", "read, write, search · local"},
				{"browser", "connected", "devtools · 127.0.0.1:9222"},
				{"pencil", "idle", "pen.dev design files"},
			} {
				srv := server
				a.settingRow(c, srv.name, srv.detail, func() {
					ui.Text(c, srv.status).Font("monospace").FontSize(9).TextColor(chooser(srv.status == "connected", p.accent2, p.text4))
				})
			}
		})
		a.settingsGroup(c, "Skills", func() {
			for _, skill := range []struct{ name, desc string }{
				{"ponytail", "Smallest change that fully solves the task"},
				{"vision", "Inspect screenshots and images"},
			} {
				sk := skill
				a.settingRow(c, sk.name, sk.desc, func() {
					ui.Text(c, "enabled").Font("monospace").FontSize(9).TextColor(p.accent2)
				})
			}
		})
		return
	}
	a.settingsGroup(c, "MCP servers", func() {
		if s == nil || len(s.MCPServers) == 0 {
			a.settingRow(c, "No MCP servers", "Add mcp.json under the nulo agent directory", func() {
				ui.Text(c, "—").Font("monospace").FontSize(10).TextColor(p.text4)
			})
			return
		}
		for _, name := range s.MCPServers {
			server := name
			a.settingRow(c, server, "configured in mcp.json", func() {
				ui.Text(c, "connected").Font("monospace").FontSize(9).TextColor(p.accent2)
			})
		}
	})
	a.settingsGroup(c, "Skills", func() {
		if s == nil || len(s.SkillNames) == 0 {
			a.settingRow(c, "No skills discovered", "Add SKILL.md under .nulo/skills or ~/.nulo/agent/skills", func() {
				ui.Text(c, "—").Font("monospace").FontSize(10).TextColor(p.text4)
			})
			return
		}
		for _, name := range s.SkillNames {
			skill := name
			a.settingRow(c, skill, "discovered by the SDK", func() {
				ui.Text(c, "enabled").Font("monospace").FontSize(9).TextColor(p.accent2)
			})
		}
	})
}

func (a *app) settingsAppearance(c *ui.Context) {
	p := a.pal()
	a.settingsHeading(c, "Appearance", "Moss is the default. Paper keeps the same hierarchy in daylight.")
	ui.Row(c).FillWidth().Gap(10).Margin(0, 0, 18, 0).Children(func() {
		for i, theme := range []struct {
			name, subtitle string
			paper          bool
		}{
			{"Moss", "Ink and green · dark", false},
			{"Paper", "Warm white · light", true},
		} {
			th := theme
			_ = i
			card := ui.ButtonBase(c).Width(220).Height(128).Padding(12).Gap(9).
				Radius(9).Column().AlignItems(ui.Stretch).Label(th.name)
			selected := a.paper == th.paper
			card.Border(1, chooser(selected, p.accent.Alpha(0.45), p.line)).Background(p.surface.Alpha(0.58))
			if card.Hovered() {
				card.BorderColor(p.lineStrong).Background(p.surface)
			}
			if card.Clicked() {
				a.setTheme(th.paper)
			}
			card.Children(func() {
				preview := ui.Column(c).FillWidth().Height(56).Clip().Radius(6).
					Border(1, chooser(th.paper, ui.Hex("#e3dfd4"), ui.Hex("#35353b"))).
					Background(chooser(th.paper, ui.Hex("#faf9f5"), ui.Hex("#0a0a0b")))
				preview.Children(func() {
					ui.Row(c).FillWidth().Height(14).Padding(0, 4).Gap(3).AlignItems(ui.Center).
						Background(chooser(th.paper, ui.Hex("#f3f1ea"), ui.Hex("#0e0e10"))).Children(func() {
						ui.Box(c).Size(4, 4).Radius(4).Background(chooser(th.paper, ui.Hex("#4f7a2f"), ui.Hex("#b7e08b")))
						ui.Box(c).Height(3).Width(22).Radius(2).Background(chooser(th.paper, ui.Hex("#c9c3b5"), ui.Hex("#35353b")))
					})
					ui.Row(c).Grow(1).Padding(5).Gap(4).Children(func() {
						ui.Column(c).Width(30).FillHeight().Radius(3).Background(chooser(th.paper, ui.Hex("#f3f1ea"), ui.Hex("#0e0e10")))
						ui.Column(c).Grow(1).Gap(3).Children(func() {
							ui.Box(c).Height(3).WidthPercent(60).Radius(2).Background(chooser(th.paper, ui.Hex("#c9c3b5"), ui.Hex("#35353b")))
							ui.Box(c).Height(3).WidthPercent(42).Radius(2).Background(chooser(th.paper, ui.Hex("#e3dfd4"), ui.Hex("#232327")))
							ui.Box(c).Height(12).WidthPercent(74).Radius(3).Background(chooser(th.paper, ui.Hex("#ffffff"), ui.Hex("#161618")))
						})
					})
				})
				ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, th.name).FontSize(12).FontWeight(600).TextColor(p.text2)
						ui.Text(c, th.subtitle).Font("monospace").FontSize(9).TextColor(p.text4)
					})
					if selected {
						a.icon(c, "check", 13, p.accent)
					}
				})
			})
		}
	})
	a.settingsGroup(c, "Interface", func() {
		a.settingRow(c, "Density", "Comfortable spacing for a 1440-wide window", func() {
			ui.Text(c, "Comfortable").Font("monospace").FontSize(10).TextColor(p.text4)
		})
		a.settingRow(c, "Motion", "Follow the system reduce-motion preference", func() {
			ui.Text(c, "System").Font("monospace").FontSize(10).TextColor(p.text4)
		})
	})
}

func (a *app) settingsShortcuts(c *ui.Context) {
	p := a.pal()
	a.settingsHeading(c, "Shortcuts", "The keyboard path exists for every primary action.")
	rows := [][2]string{
		{"⌘ K", "Command palette"},
		{"⌘ N", "New session"},
		{"⌘ B", "Toggle sidebar"},
		{"⌘ J", "Toggle review dock"},
		{"⌘ ,", "Settings"},
		{"⌘ O", "Open project folder"},
		{"⌘ F", "Search sessions"},
		{"⇧⌘ D", "Open changes"},
		{"⇧⌘ P", "Open plan"},
		{"Enter", "Send message"},
		{"⇧ Enter", "New line"},
		{"/", "Slash commands"},
		{"@", "Mention a file"},
		{"Esc", "Close overlay or menu"},
	}
	a.settingsGroup(c, "Window & composer", func() {
		for _, row := range rows {
			r := row
			ui.Row(c).FillWidth().Height(34).AlignItems(ui.Center).Gap(10).
				BorderWidth(0, 0, 1, 0).BorderColor(p.line.Alpha(0.55)).Children(func() {
				ui.Text(c, r[1]).FontSize(12).TextColor(p.text3).Grow(1)
				a.shortcutKey(c, r[0])
			})
		}
	})
}

func (a *app) settingsAbout(c *ui.Context) {
	p := a.pal()
	a.settingsHeading(c, "About", "nulo desktop, implemented with MyGo's native UI.")
	card := ui.Column(c).FillWidth().Clip().Radius(10).Border(1, p.line).
		Background(p.surface.Alpha(0.55)).Padding(18).Gap(10)
	card.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			box := ui.Box(c).Size(40, 40).Radius(11).Background(p.accent).Center()
			box.Children(func() {
				ui.Text(c, "n").Font("monospace").FontSize(21).FontWeight(700).TextColor(p.accentInk)
			})
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "nulo desktop").FontSize(16).FontWeight(600).TextColor(p.text)
				ui.Text(c, "0.9.0 · native shell").Font("monospace").FontSize(10).TextColor(p.text4)
			})
		})
		ui.Text(c, "A three-column coding-agent workspace: sessions on the left, the thread in the middle, and review, plan, terminal, and context on the right. This host runs real nulo SDK sessions, streams their events, and persists the thread in the agent session directory.").
			FontSize(12).LineHeight(1.6).TextColor(p.text3)
		ui.Row(c).Gap(6).Children(func() {
			for _, item := range []struct{ icon, text string }{
				{"file", "ui/design/desktop"},
				{"code", "application/desktop"},
				{"package", "MyGo v0.9.0"},
			} {
				it := item
				box := ui.Row(c).Height(25).Padding(0, 8).Gap(5).Radius(5).Border(1, p.line).
					Background(p.surface2).AlignItems(ui.Center)
				box.Children(func() {
					a.icon(c, it.icon, 11, p.text4)
					ui.Text(c, it.text).Font("monospace").FontSize(9).TextColor(p.text3)
				})
			}
		})
	})
}

func (a *app) renameDialog(c *ui.Context) {
	p := a.pal()
	ui.Modal(c, &a.renameOpen, func() {
		ui.Text(c, "Rename session").FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, "Give this thread a name that is easy to find in the sidebar.").
			FontSize(12).TextColor(p.text4)
		in := ui.TextInputBase(c, &a.renameValue).FillWidth().Height(34).Padding(0, 10).Radius(6).
			Border(1, p.lineStrong).Background(p.surface2).FontSize(13).TextColor(p.text).
			Label("Session name").AutoFocus()
		in.OnTextKey(func(k ui.TextKey) bool {
			if k.Key == ui.KeyEnter {
				a.finishRename()
				return true
			}
			return false
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if a.button(c, "Cancel").Clicked() {
				a.renameOpen = false
			}
			if a.primaryButton(c, "Rename", "check").Clicked() {
				a.finishRename()
			}
		})
	})
}

func (a *app) commitDialog(c *ui.Context, s *session) {
	p := a.pal()
	ui.Modal(c, &a.commitOpen, func() {
		ui.Text(c, "Commit changes").FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, "Write the reviewed changes to the branch with a commit message.").
			FontSize(12).TextColor(p.text4)
		ui.TextInputBase(c, &a.commitMsg).FillWidth().Height(34).Padding(0, 10).Radius(6).
			Border(1, p.lineStrong).Background(p.surface2).FontSize(13).TextColor(p.text).
			Placeholder("Describe this change…").AutoFocus().Label("Commit message")
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if a.button(c, "Cancel").Clicked() {
				a.commitOpen = false
			}
			if a.primaryButton(c, "Commit", "check").Clicked() && strings.TrimSpace(a.commitMsg) != "" {
				msg := strings.TrimSpace(a.commitMsg)
				if a.host != nil && s.Real {
					if err := a.commitChanges(s, msg); err != nil {
						a.pendingToast = "Commit: " + err.Error()
					} else {
						a.commitOpen = false
						a.pendingToast = "Committed changes"
						a.scrollToEnd = true
					}
					return
				}
				s.Changes = nil
				s.Status = "done"
				s.Messages = append(s.Messages, message{Role: "notice", Time: "now", Text: "Committed · " + msg})
				a.commitOpen = false
				a.pendingToast = "Committed changes"
				a.scrollToEnd = true
			}
		})
	})
}
