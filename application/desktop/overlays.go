package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
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
	if a.deleteOpen {
		a.deleteDialog(c)
	}
	if a.mcpFormOpen {
		a.mcpFormDialog(c)
	}
	if a.skillFormOpen {
		a.skillFormDialog(c)
	}
	if a.providerFormOpen {
		a.providerFormDialog(c)
	}
	if a.modelFormOpen {
		a.modelFormDialog(c)
	}
	if a.mergeConflictOpen {
		a.mergeConflictDialog(c)
	}
	if a.mergeEditorOpen {
		a.mergeEditorDialog(c)
	}
	if a.rewindOpen {
		a.rewindDialog(c)
	}
	if a.confirmOpen {
		a.confirmDialog(c)
	}
	if a.imageOpen {
		a.imageDialog(c)
	}
	if a.configOpen {
		a.configDialog(c)
	}
	if a.keyOpen {
		a.apiKeyDialog(c)
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
		if a.host != nil && s != nil {
			trusted := a.host.sessionTrusted(s)
			root := s.TrustRoot
			if root == "" {
				root = "current project"
			}
			a.settingRow(c, "Project trust", root, func() {
				if a.host.trusted {
					ui.Text(c, "Always trusted").Font("monospace").FontSize(9).TextColor(p.accent2)
					return
				}
				if a.smallButton(c, chooser(trusted, "Revoke trust", "Trust project")).Clicked() {
					a.setProjectTrusted(s, !trusted)
				}
			})
		}
		a.settingRow(c, "Default model", s.modelName(), func() {
			ui.Text(c, a.providerLabel(s.modelProvider())).Font("monospace").FontSize(10).TextColor(p.text4)
		})
		a.settingRow(c, "Default permission", approvalLabel(s.Approval), func() {
			a.permissionChoices(c, s)
		})
		a.settingRow(c, "Default thinking", s.thinkingName(), func() {
			a.thinkingChoices(c, s)
		})
	})
	a.settingsGroup(c, "Session defaults", func() {
		envDesc := s.CWD
		if envDesc == "" {
			envDesc = "current working tree"
		}
		if s.Env == "Worktree" && s.Worktree != "" {
			envDesc = s.Worktree + " · " + s.Branch
		}
		a.settingRow(c, "Environment", envDesc, func() {
			ui.Row(c).Gap(4).Children(func() {
				for _, env := range []string{"Local", "Worktree"} {
					e := env
					b := ui.ButtonBase(c).Height(25).Padding(0, 9).Radius(5).Label(e).
						Border(1, p.line).Background(p.surface2)
					if s.Env == e {
						b.Background(p.accentSoft).BorderColor(p.accent.Alpha(0.28))
					}
					if b.Clicked() {
						a.switchEnvironment(s, e)
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
			if a.smallButton(c, chooser(a.showTimestamps, "On", "Off")).Clicked() {
				a.toggleShowTimestamps()
			}
		})
		a.settingRow(c, "Reasoning summaries", "Collapse long thinking blocks", func() {
			if a.smallButton(c, chooser(a.collapseThinking, "Collapsed", "Expanded")).Clicked() {
				a.toggleCollapseThinking()
			}
		})
		a.settingRow(c, "Desktop notifications", "Notify when a turn finishes in the background", func() {
			if a.smallButton(c, chooser(a.notifyDone, "On", "Off")).Clicked() {
				a.toggleNotifications()
			}
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
			a.setDefaultApproval(s, mode)
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
	ui.Row(c).Gap(4).Wrap().Justify(ui.End).Children(func() {
		for _, level := range a.thinkingLevelOptions(s) {
			l := level
			b := ui.ButtonBase(c).Height(25).Padding(0, 8).Radius(5).Label(l).
				Border(1, p.line).Background(p.surface2)
			if s.Thinking == l {
				b.Background(p.accentSoft).BorderColor(p.accent.Alpha(0.28))
			}
			if b.Clicked() {
				a.setDefaultThinking(s, l)
			}
			b.Children(func() {
				color := p.text3
				if s.Thinking == l {
					color = p.accent
				}
				ui.Text(c, strings.ToUpper(l)).Font("monospace").FontSize(9).TextColor(color)
			})
		}
	})
}

func (a *app) settingsProviders(c *ui.Context, s *session) {
	p := a.pal()
	a.settingsHeading(c, "Providers & models", "Models and API keys stay grouped under the provider that serves them.")
	if a.host != nil {
		path := filepath.Join(a.host.agentDir, "models.json")
		a.settingRow(c, "models.json", "Add a provider with the form, or edit the file directly for advanced fields.", func() {
			if a.smallButton(c, "Add provider").Clicked() {
				a.openProviderForm(s, "")
			}
			if a.smallButton(c, "Edit JSON").Clicked() {
				a.openConfigEditor("Models", path, "Provider objects use baseUrl/api/apiKey and a models array. Saving validates by rebuilding the SDK services.", func() error {
					return a.reloadConfig("Models reloaded")
				})
			}
		})
	}
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
				if a.host != nil && s != nil {
					if a.smallButton(c, "Add model").Clicked() {
						a.openModelForm(s, provider.ID, model{})
					}
					if a.smallButton(c, "Edit").Clicked() {
						a.openProviderForm(s, provider.ID)
					}
					if a.smallButton(c, "Delete").Clicked() {
						a.confirmDestructive("Delete provider?", "Delete "+provider.Name+" and all of its models from models.json. This cannot be undone.", "Delete provider", func() {
							if err := a.deleteProvider(provider.ID); err != nil {
								a.pendingToast = "Providers: " + err.Error()
							}
						})
					}
				}
			})
			for _, mdl := range provider.Models {
				m := mdl
				id := provider.ID + "/" + m.ID
				ui.Row(c).FillWidth().Height(42).Padding(0, 8).Gap(4).AlignItems(ui.Center).Children(func() {
					row := ui.ButtonBase(c).Grow(1).Height(38).Padding(0, 8, 0, 38).Gap(8).
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
					if a.host != nil && s != nil {
						if a.smallButton(c, "Edit").Clicked() {
							a.openModelForm(s, provider.ID, m)
						}
					}
				})
			}
			a.providerKeyRow(c, provider, defaultSpec)
		})
	}
}

func (a *app) providerKeyRow(c *ui.Context, provider provider, defaultSpec string) {
	if a.host == nil || provider.ID == "faux" {
		return
	}
	p := a.pal()
	source := provider.APIKeySource
	status := a.providerTests[provider.ID]
	spec := ""
	if len(provider.Models) > 0 {
		spec = provider.ID + "/" + provider.Models[0].ID
		if strings.HasPrefix(defaultSpec, provider.ID+"/") {
			spec = defaultSpec
		}
	}
	ui.Row(c).FillWidth().MinHeight(48).PaddingY(8).Gap(10).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(p.line.Alpha(0.7)).Children(func() {
		a.icon(c, "lock", 13, chooser(source != "", p.accent2, p.text4))
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, "API key").FontSize(12).FontWeight(500).TextColor(p.text2)
			ui.Text(c, providerKeySourceLabel(source)).Font("monospace").FontSize(10).TextColor(p.text4)
			switch {
			case status == "testing":
				ui.Text(c, "Testing connection…").Font("monospace").FontSize(9).TextColor(p.text4)
			case status == "ok":
				ui.Text(c, "Connected").Font("monospace").FontSize(9).TextColor(p.accent2)
			case strings.HasPrefix(status, "error: "):
				ui.Text(c, strings.TrimPrefix(status, "error: ")).Font("monospace").FontSize(9).
					TextColor(p.bad).SingleLine()
			}
		})
		if a.smallButton(c, chooser(status == "testing", "Testing…", "Test")).Disabled(status == "testing").Clicked() {
			a.testProvider(provider, spec)
		}
		if source == "auth" {
			if a.smallButton(c, "Clear key").Clicked() {
				a.keyProvider = provider.ID
				a.keyName = provider.Name
				a.clearProviderKey()
			}
		}
		label := "Set key"
		if source == "auth" {
			label = "Replace"
		}
		if a.button(c, label).Clicked() {
			a.openProviderKey(provider)
		}
	})
}

func (a *app) apiKeyDialog(c *ui.Context) {
	p := a.pal()
	ui.Modal(c, &a.keyOpen, func() {
		ui.Text(c, "Provider API key").FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, a.keyName+" · stored locally in the agent auth.json").FontSize(12).TextColor(p.text4)
		prov, _ := a.providerByID(a.keyProvider)
		ui.Text(c, "Current source: "+providerKeySourceLabel(prov.APIKeySource)+". Saving here overrides models.json and environment keys for future requests.").
			FontSize(11).LineHeight(1.5).TextColor(p.text3)
		in := ui.TextInputBase(c, &a.keyValue).FillWidth().Height(34).Padding(0, 10).Radius(6).
			Border(1, p.lineStrong).Background(p.surface2).FontSize(13).TextColor(p.text).
			Placeholder("sk-…").Password().AutoFocus().Label("Provider API key")
		in.OnTextKey(func(k ui.TextKey) bool {
			if k.Key == ui.KeyEnter {
				a.saveProviderKey()
				return true
			}
			return false
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if prov.APIKeySource == "auth" {
				if a.dangerButton(c, "Clear stored key").Clicked() {
					a.clearProviderKey()
				}
			}
			ui.Spacer(c)
			if a.button(c, "Cancel").Clicked() {
				a.keyOpen = false
				a.keyValue = ""
			}
			if a.primaryButton(c, "Save key", "check").Clicked() {
				a.saveProviderKey()
			}
		})
	})
}

func (a *app) settingsCapabilities(c *ui.Context, s *session) {
	p := a.pal()
	a.settingsHeading(c, "Capabilities", "Loaded by the SDK for this session and workspace.")
	if a.host == nil {
		a.settingsGroup(c, "MCP servers", func() {
			for _, server := range []struct{ name, desc string }{
				{"pencil", "Read and validate .pen design files"},
				{"browser", "Inspect the active browser page"},
			} {
				item := server
				a.settingRow(c, item.name, item.desc, func() {
					ui.Text(c, "On").Font("monospace").FontSize(9).TextColor(p.accent2)
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
					ui.Text(c, "On").Font("monospace").FontSize(9).TextColor(p.accent2)
				})
			}
		})
		return
	}
	a.settingsGroup(c, "MCP servers", func() {
		if s != nil {
			global := filepath.Join(a.host.agentDir, "mcp.json")
			project := filepath.Join(s.CWD, ".nulo", "mcp.json")
			a.settingRow(c, "Configuration", "mcpServers objects; disabled servers stay listed here.", func() {
				if a.smallButton(c, "Global").Clicked() {
					a.openConfigEditor("Global MCP", global, "User-level mcp.json. Saving reloads SDK services and reconnects servers.", func() error {
						return a.reloadConfig("MCP configuration reloaded")
					})
				}
				if a.smallButton(c, "Project").Clicked() {
					a.openConfigEditor("Project MCP", project, "Project-level mcp.json. Project resources load only when the project is trusted.", func() error {
						return a.reloadConfig("MCP configuration reloaded")
					})
				}
				if a.smallButton(c, "Reload").Clicked() {
					if err := a.reloadConfig("MCP configuration reloaded"); err != nil {
						a.pendingToast = "MCP: " + err.Error()
					}
				}
				if a.smallButton(c, "Add").Clicked() {
					a.openMCPForm(s, "")
				}
			})
		}
		var servers []mcpServerInfo
		if s != nil {
			servers = s.MCPAll
		}
		if len(servers) == 0 && s != nil {
			for _, name := range s.MCPServers {
				servers = append(servers, mcpServerInfo{Name: name, Enabled: true})
			}
		}
		if len(servers) == 0 {
			a.settingRow(c, "No MCP servers", "Add mcp.json under the nulo agent directory", func() {
				ui.Text(c, "—").Font("monospace").FontSize(10).TextColor(p.text4)
			})
			return
		}
		for _, item := range servers {
			server := item
			desc := server.Description
			if desc == "" {
				desc = "configured in mcp.json"
			}
			a.settingRow(c, server.Name, desc, func() {
				if s == nil {
					ui.Text(c, "—").Font("monospace").FontSize(9).TextColor(p.text4)
					return
				}
				if a.smallButton(c, "Edit").Clicked() {
					a.openMCPForm(s, server.Name)
				}
				if !a.host.sessionTrusted(s) {
					ui.Text(c, "trust project").Font("monospace").FontSize(9).TextColor(p.text4)
					return
				}
				statusLabel := "Off"
				statusColor := p.text4
				if server.Enabled {
					statusLabel = "Configured"
					if server.Connected {
						statusLabel = "Connected"
						statusColor = p.accent2
					} else {
						statusLabel = "Not connected"
						statusColor = p.warn
					}
				}
				ui.Text(c, statusLabel).Font("monospace").FontSize(9).TextColor(statusColor).SingleLine()
				label := "Off"
				if server.Enabled {
					label = "On"
				}
				if a.smallButton(c, label).Clicked() {
					a.toggleMCPServer(s, server.Name, !server.Enabled)
				}
			})
		}
	})
	a.settingsGroup(c, "Skills", func() {
		if s != nil {
			a.settingRow(c, "Skill folders", "Edit SKILL.md files, then reload to pick up the changes.", func() {
				if a.smallButton(c, "User").Clicked() {
					if err := openFolder(filepath.Join(a.host.agentDir, "skills")); err != nil {
						a.pendingToast = "Skills: " + err.Error()
					}
				}
				if a.smallButton(c, "Project").Clicked() {
					if err := openFolder(filepath.Join(s.CWD, ".nulo", "skills")); err != nil {
						a.pendingToast = "Skills: " + err.Error()
					}
				}
				if a.smallButton(c, "Reload").Clicked() {
					if err := a.reloadConfig("Skills reloaded"); err != nil {
						a.pendingToast = "Skills: " + err.Error()
					}
				}
				if a.smallButton(c, "New").Clicked() {
					a.openSkillForm(s)
				}
			})
		}
		var skills []skillInfo
		if s != nil {
			skills = s.SkillAll
		}
		if len(skills) == 0 && s != nil {
			for _, name := range s.SkillNames {
				skills = append(skills, skillInfo{Name: name, Description: "discovered by the SDK", Enabled: true})
			}
		}
		if len(skills) == 0 {
			a.settingRow(c, "No skills discovered", "Add SKILL.md under .nulo/skills or the user skills folder", func() {
				ui.Text(c, "—").Font("monospace").FontSize(10).TextColor(p.text4)
			})
			return
		}
		for _, item := range skills {
			skill := item
			desc := skill.Description
			if desc == "" {
				desc = "discovered by the SDK"
			}
			if skill.Source != "" {
				desc += " · " + skill.Source
			}
			a.settingRow(c, skill.Name, desc, func() {
				if s == nil {
					ui.Text(c, "—").Font("monospace").FontSize(9).TextColor(p.text4)
					return
				}
				if _, writable := skillRootFor(s, skill, a.host.agentDir); writable {
					if a.smallButton(c, "Edit").Clicked() {
						a.openSkillEditForm(s, skill)
					}
					if a.smallButton(c, "Delete").Clicked() {
						current := skill
						a.confirmDestructive("Delete skill?", "Remove "+current.Path+". This cannot be undone.", "Delete skill", func() {
							if err := a.deleteSkill(s, current); err != nil {
								a.pendingToast = "Skills: " + err.Error()
							}
						})
					}
				}
				label := "Off"
				if skill.Enabled {
					label = "On"
				}
				if a.smallButton(c, label).Clicked() {
					a.toggleSkill(s, skill.Name, !skill.Enabled)
				}
			})
		}
	})
	a.settingsGroup(c, "Plugins", func() {
		if s != nil {
			userPlugins := filepath.Join(a.host.agentDir, "plugins")
			projectPlugins := filepath.Join(s.CWD, ".nulo", "plugins")
			a.settingRow(c, "Plugin folders", "Native .so/.dylib/.dll plugins or plugin.json manifests.", func() {
				if a.smallButton(c, "User").Clicked() {
					if err := openFolder(userPlugins); err != nil {
						a.pendingToast = "Plugins: " + err.Error()
					}
				}
				if a.smallButton(c, "Project").Clicked() {
					if err := openFolder(projectPlugins); err != nil {
						a.pendingToast = "Plugins: " + err.Error()
					}
				}
				if a.smallButton(c, "Reload").Clicked() {
					if err := a.reloadConfig("Plugins reloaded"); err != nil {
						a.pendingToast = "Plugins: " + err.Error()
					}
				}
			})
		}
		if s == nil || len(s.PluginNames) == 0 {
			a.settingRow(c, "No plugins loaded", "Add a plugin under the user or trusted project plugin folder", func() {
				ui.Text(c, "—").Font("monospace").FontSize(10).TextColor(p.text4)
			})
		}
		if s != nil {
			for _, name := range s.PluginNames {
				pluginName := name
				a.settingRow(c, pluginName, "native plugin loaded for this session", func() {
					ui.Text(c, "On").Font("monospace").FontSize(9).TextColor(p.accent2)
				})
			}
			for _, diag := range s.PluginDiags {
				message := diag
				a.settingRow(c, "Plugin diagnostic", message, func() {
					ui.Text(c, "Error").Font("monospace").FontSize(9).TextColor(p.bad)
				})
			}
		}
	})
	if s != nil && !a.host.sessionTrusted(s) {
		a.settingsGroup(c, "Project resources", func() {
			a.settingRow(c, "Project is not trusted", "Project-level MCP, skills, and plugins stay disabled", func() {
				if a.smallButton(c, "Trust project").Clicked() {
					a.setProjectTrusted(s, true)
				}
			})
		})
	}
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
		a.settingRow(c, "Density", "Compact tightens the thread and composer spacing", func() {
			if a.smallButton(c, chooser(a.compactDensity, "Compact", "Comfortable")).Clicked() {
				a.toggleCompactDensity()
			}
		})
		a.settingRow(c, "Motion", "MyGo follows the system reduce-motion preference", func() {
			label := "Full"
			if c.Preferences().ReduceMotion {
				label = "Reduced"
			}
			ui.Text(c, label).Font("monospace").FontSize(10).TextColor(p.text4)
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

func (a *app) deleteDialog(c *ui.Context) {
	p := a.pal()
	target := a.sessionByID(a.deleteID)
	title := "this session"
	children := 0
	if target != nil {
		title = target.Title
		children = a.sessionDescendantCount(target.ID)
	}
	ui.Modal(c, &a.deleteOpen, func() {
		ui.Text(c, "Delete session?").FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, title).FontSize(12).TextColor(p.text3).SingleLine()
		detail := "The session JSONL will be removed. This cannot be undone."
		if target != nil && (target.Status == "running" || target.Status == "awaiting") {
			detail = "The running turn will be stopped and the session JSONL removed. This cannot be undone."
		}
		if children > 0 {
			detail = fmt.Sprintf("The session and %d forked descendant session(s) will be removed. This cannot be undone.", children)
		}
		if target != nil && (target.Env == "Worktree" || target.Worktree != "") {
			detail += " Its isolated worktree and nulo/wt branch will be deleted, including unmerged changes."
		}
		ui.Text(c, detail).FontSize(12).LineHeight(1.5).TextColor(p.text4)
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			if a.button(c, "Cancel").Clicked() {
				a.deleteOpen = false
				a.deleteID = ""
			}
			if a.dangerButton(c, "Delete session").Clicked() {
				id := a.deleteID
				a.deleteOpen = false
				a.deleteID = ""
				a.deleteSession(id)
			}
		})
	})
}

func (a *app) configDialog(c *ui.Context) {
	p := a.pal()
	ui.Modal(c, &a.configOpen, func() {
		ui.Column(c).Width(760).Gap(9).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(1).Children(func() {
					ui.Text(c, a.configTitle).FontSize(17).FontWeight(600).TextColor(p.text)
					ui.Text(c, a.configPath).Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
				})
			})
			ui.Text(c, a.configHelp).FontSize(12).LineHeight(1.5).TextColor(p.text4)
			editor := ui.TextAreaBase(c, &a.configText).FillWidth().Height(360).Padding(10).Radius(7).
				Border(1, p.lineStrong).Background(p.codeBg).Font("monospace").FontSize(11).
				LineHeight(1.55).TextColor(p.text2).Lines(8, 24).Label("Config JSON")
			editor.AutoFocus()
			ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
				if a.button(c, "Cancel").Clicked() {
					a.configOpen = false
				}
				if a.primaryButton(c, "Save and reload", "check").Clicked() {
					a.saveConfigEditor()
				}
			})
		})
	})
}

func (a *app) imageDialog(c *ui.Context) {
	p := a.pal()
	bitmap := a.attachmentBitmap(a.imagePath)
	if bitmap == nil {
		a.imageOpen = false
		return
	}
	if a.imageRotation != 0 || a.imageFlipX || a.imageFlipY {
		if a.imageEditBitmap == nil {
			a.imageEditBitmap = a.transformedPreviewBitmap()
		}
		if a.imageEditBitmap != nil {
			bitmap = a.imageEditBitmap
		}
	}
	nativeW, nativeH, ok := imageDimensions(a.imagePath)
	if !ok || nativeW <= 0 || nativeH <= 0 {
		nativeW, nativeH = 1000, 700
	}
	if a.imageRotation == 90 || a.imageRotation == 270 {
		nativeW, nativeH = nativeH, nativeW
	}
	maxW := float32(760)
	if a.lastWidth > 0 && a.lastWidth < maxW+120 {
		maxW = max(240, a.lastWidth-100)
	}
	maxH := float32(520)
	scale := min(maxW/float32(nativeW), maxH/float32(nativeH))
	width := max(80, float32(nativeW)*scale)
	height := max(60, float32(nativeH)*scale)
	ui.Modal(c, &a.imageOpen, func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(1).Children(func() {
				ui.Text(c, filepath.Base(a.imagePath)).Font("monospace").FontSize(12).TextColor(p.text2).SingleLine()
				ui.Text(c, a.imagePath).Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
			})
			if a.cropMode {
				if a.smallButton(c, "Reset").Clicked() {
					a.resetImageEdit()
				}
				if a.smallButton(c, "Save image").Clicked() {
					if err := a.saveCroppedImage(); err != nil {
						a.pendingToast = "Crop: " + err.Error()
					}
				}
				if a.smallButton(c, "Cancel crop").Clicked() {
					a.cropMode = false
					a.resetImageEdit()
				}
			} else {
				if a.smallButton(c, "Crop").Clicked() {
					a.cropMode = true
					a.cropStartX, a.cropStartY, a.cropEndX, a.cropEndY = 0, 0, 1, 1
				}
				if a.smallButton(c, "Open").Clicked() {
					if err := mygo.Shell.OpenPath(a.imagePath); err != nil {
						a.pendingToast = "Open: " + err.Error()
					}
				}
			}
			if a.smallButton(c, "Close").Clicked() {
				a.imageOpen = false
			}
		})
		if a.cropMode {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				if a.smallButton(c, chooser(a.imageAnnotate, "Annotate ✓", "Annotate")).Clicked() {
					a.imageAnnotate = !a.imageAnnotate
					a.imageAnnotDrawing = false
				}
				if a.smallButton(c, chooser(len(a.imageStrokes) > 0, "Clear marks", "Clear marks")).
					Disabled(len(a.imageStrokes) == 0).Clicked() {
					a.imageStrokes = nil
					a.imageAnnotDrawing = false
					a.invalidateAnnotations()
				}
				if a.smallButton(c, "Rotate L").Clicked() {
					a.imageRotation = (a.imageRotation + 270) % 360
					a.cropStartX, a.cropStartY, a.cropEndX, a.cropEndY = 0, 0, 1, 1
					a.imageEditBitmap = nil
					a.imageStrokes = nil
					a.imageAnnotDrawing = false
					a.invalidateAnnotations()
				}
				if a.smallButton(c, "Rotate R").Clicked() {
					a.imageRotation = (a.imageRotation + 90) % 360
					a.cropStartX, a.cropStartY, a.cropEndX, a.cropEndY = 0, 0, 1, 1
					a.imageEditBitmap = nil
					a.imageStrokes = nil
					a.imageAnnotDrawing = false
					a.invalidateAnnotations()
				}
				if a.smallButton(c, chooser(a.imageFlipX, "Flip H ✓", "Flip H")).Clicked() {
					a.imageFlipX = !a.imageFlipX
					a.cropStartX, a.cropStartY, a.cropEndX, a.cropEndY = 0, 0, 1, 1
					a.imageEditBitmap = nil
					a.imageStrokes = nil
					a.imageAnnotDrawing = false
					a.invalidateAnnotations()
				}
				if a.smallButton(c, chooser(a.imageFlipY, "Flip V ✓", "Flip V")).Clicked() {
					a.imageFlipY = !a.imageFlipY
					a.cropStartX, a.cropStartY, a.cropEndX, a.cropEndY = 0, 0, 1, 1
					a.imageEditBitmap = nil
					a.imageStrokes = nil
					a.imageAnnotDrawing = false
					a.invalidateAnnotations()
				}
			})
			instruction := "Drag on the image to choose the crop area."
			if a.imageAnnotate {
				instruction = "Drag on the image to draw. Toggle Annotate off to adjust the crop."
			}
			ui.Text(c, instruction).Font("monospace").FontSize(9).TextColor(p.text4)
		}
		area := ui.Box(c).Size(width, height).Clip().Radius(8).Background(p.codeBg)
		area.Children(func() {
			ui.Image(c, bitmap).Size(width, height).Fit(ui.Cover).Radius(8)
			if overlay := a.annotationOverlay(int(width), int(height)); overlay != nil {
				ui.Image(c, overlay).Size(width, height).Fit(ui.Cover).Radius(8).IgnoreInput(true)
			}
			if a.cropMode && !a.imageAnnotate {
				x0, x1 := min(a.cropStartX, a.cropEndX), max(a.cropStartX, a.cropEndX)
				y0, y1 := min(a.cropStartY, a.cropEndY), max(a.cropStartY, a.cropEndY)
				x0, y0, x1, y1 = clamp01(x0), clamp01(y0), clamp01(x1), clamp01(y1)
				shade := ui.RGBA(0, 0, 0, 0.52)
				ui.Box(c).Absolute().Left(0).Top(0).Width(width).Height(y0 * height).Background(shade)
				ui.Box(c).Absolute().Left(0).Top(y1 * height).Width(width).Height((1 - y1) * height).Background(shade)
				ui.Box(c).Absolute().Left(0).Top(y0 * height).Width(x0 * width).Height((y1 - y0) * height).Background(shade)
				ui.Box(c).Absolute().Left(x1 * width).Top(y0 * height).Width((1 - x1) * width).Height((y1 - y0) * height).Background(shade)
				ui.Box(c).Absolute().Left(x0*width).Top(y0*height).Width((x1-x0)*width).Height((y1-y0)*height).
					Border(1, p.accent)
			}
			if a.cropMode {
				hit := ui.Box(c).Absolute().Fill().Background(ui.Transparent).Label("Crop area")
				pressed := hit.Pressed()
				x, y, over := hit.PointerPosition()
				nx := clamp01(x / width)
				ny := clamp01(y / height)
				if a.imageAnnotate {
					if a.imageAnnotDrawing {
						if pressed && over {
							stroke := a.imageStrokes[len(a.imageStrokes)-1]
							last := stroke[len(stroke)-1]
							if absf(nx-last.X) > 0.003 || absf(ny-last.Y) > 0.003 {
								a.imageStrokes[len(a.imageStrokes)-1] = append(stroke, imagePoint{X: nx, Y: ny})
								a.invalidateAnnotations()
							}
						} else {
							a.imageAnnotDrawing = false
						}
					} else if hit.PressedInside() && over {
						a.imageAnnotDrawing = true
						a.imageStrokes = append(a.imageStrokes, imageStroke{{X: nx, Y: ny}})
						a.invalidateAnnotations()
					}
				} else if a.cropDragging {
					if pressed && over {
						a.cropEndX, a.cropEndY = nx, ny
					} else {
						a.cropDragging = false
						if absf(a.cropEndX-a.cropStartX) < 0.02 || absf(a.cropEndY-a.cropStartY) < 0.02 {
							a.cropStartX, a.cropStartY, a.cropEndX, a.cropEndY = 0, 0, 1, 1
						}
					}
				} else if hit.PressedInside() && over {
					a.cropDragging = true
					a.cropStartX, a.cropStartY = nx, ny
					a.cropEndX, a.cropEndY = nx, ny
				}
			}
		})
	})
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func (a *app) sessionDescendantCount(id string) int {
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
	return len(remove) - 1
}

func (a *app) confirmDialog(c *ui.Context) {
	p := a.pal()
	ui.Modal(c, &a.confirmOpen, func() {
		ui.Text(c, a.confirmTitle).FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, a.confirmDetail).FontSize(12).LineHeight(1.5).TextColor(p.text4)
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			if a.button(c, "Cancel").Clicked() {
				a.confirmOpen = false
				a.confirmAction = nil
			}
			if a.dangerButton(c, a.confirmLabel).Clicked() {
				action := a.confirmAction
				a.confirmOpen = false
				a.confirmAction = nil
				if action != nil {
					action()
				}
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
