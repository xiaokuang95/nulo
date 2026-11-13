package main

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type slashCommand struct {
	Name string
	Desc string
	Key  string
}

var slashCommands = []slashCommand{
	{"/plan", "Open the plan for this session", "⇧⌘P"},
	{"/diff", "Review the working-tree changes", "⇧⌘D"},
	{"/context", "Inspect context usage and loaded files", ""},
	{"/compact", "Drop stale tool results from context", ""},
	{"/approve", "Approve the pending command", ""},
	{"/clear", "Start a clean thread", ""},
}

var mentionFiles = []string{
	"application/desktop/main.go",
	"application/desktop/app.go",
	"application/desktop/thread.go",
	"application/desktop/dock.go",
	"application/desktop/composer.go",
	"ui/design/desktop/index.html",
	"ui/design/desktop/app.css",
	"ui/design/desktop/app.js",
	"sdk/session.go",
	"sdk/session_test.go",
	"README.md",
	"AGENTS.md",
}

func (a *app) composer(c *ui.Context, s *session) {
	p := a.pal()
	approval := s.pendingApproval()
	dock := ui.Column(c).Shrink(0).FillWidth().Padding(0, 24, 13, 24).Background(p.bg)
	dock.Children(func() {
		wrap := ui.Column(c).WidthPercent(100).MaxWidth(820).AlignSelf(ui.Center)
		wrap.Children(func() {
			a.composerBox(c, s)
			a.composerMeta(c, s)
		})
	})
	_ = approval
}

func (a *app) composerBox(c *ui.Context, s *session) {
	p := a.pal()
	approval := s.pendingApproval()
	border := p.lineStrong
	if approval != nil {
		border = p.warn.Alpha(0.38)
	}
	box := ui.Column(c).FillWidth().Clip().Radius(13).Border(1, border).Background(p.surface).
		Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.22))
	if paths := box.DroppedFiles(); len(paths) > 0 {
		a.addAttachments(paths)
	}
	box.Children(func() {
		if len(a.attachments) > 0 {
			a.attachmentTray(c)
		}
		in := ui.TextAreaBase(c, &a.draft).Placeholder("Ask nulo to build, debug, or explain…").
			Label("Message nulo").FontSize(14).LineHeight(1.6).TextColor(p.text).
			Padding(13, 15, 6, 15).MinHeight(54).MaxHeight(184).Lines(1, 8)
		if a.focusComposer {
			in.AutoFocus()
			a.focusComposer = false
		}

		in.OnTextKey(func(k ui.TextKey) bool {
			if k.Key == ui.KeyEscape {
				if a.slashOpen || a.mentionOpen {
					a.slashOpen, a.mentionOpen = false, false
					return true
				}
				return false
			}
			if a.slashOpen {
				items := a.filteredSlash()
				switch k.Key {
				case ui.KeyUp:
					if len(items) > 0 {
						a.slashIndex = (a.slashIndex - 1 + len(items)) % len(items)
					}
					return true
				case ui.KeyDown:
					if len(items) > 0 {
						a.slashIndex = (a.slashIndex + 1) % len(items)
					}
					return true
				case ui.KeyTab:
					if len(items) > 0 {
						a.slashIndex = max(0, min(a.slashIndex, len(items)-1))
						text := items[a.slashIndex].Name + " "
						a.draft = text
						k.SetText(text)
						return true
					}
				case ui.KeyEnter:
					if len(items) > 0 {
						a.runSlash(items[max(0, min(a.slashIndex, len(items)-1))].Name)
						k.SetText("")
						return true
					}
				}
			}
			if a.mentionOpen {
				items := a.filteredMentions()
				switch k.Key {
				case ui.KeyUp:
					if len(items) > 0 {
						a.mentionIndex = (a.mentionIndex - 1 + len(items)) % len(items)
					}
					return true
				case ui.KeyDown:
					if len(items) > 0 {
						a.mentionIndex = (a.mentionIndex + 1) % len(items)
					}
					return true
				case ui.KeyTab:
					if len(items) > 0 {
						a.insertMention(items[max(0, min(a.mentionIndex, len(items)-1))])
						k.SetText(a.draft)
						return true
					}
				case ui.KeyEnter:
					if len(items) > 0 {
						a.insertMention(items[max(0, min(a.mentionIndex, len(items)-1))])
						k.SetText(a.draft)
						return true
					}
				}
			}
			if k.Key == ui.KeyEnter && k.Mods == 0 {
				a.send(k.Text())
				k.SetText("")
				return true
			}
			return false
		})
		if in.Changed() {
			a.syncMenus()
		}

		toolbar := ui.Row(c).FillWidth().Height(38).Padding(5, 8, 8, 9).Gap(4).AlignItems(ui.Center)
		toolbar.Children(func() {
			a.iconButton(c, "paperclip", "Attach files", false, a.pickAttachments)

			perm := a.chip(c, "shield", approvalLabel(s.Approval), a.showPerm)
			if perm.Clicked() {
				a.showPerm = !a.showPerm
				a.showEnv, a.showModel = false, false
			}
			if a.showPerm {
				ui.Popover(c, perm, &a.showPerm, func() { a.permissionPopover(c, s) })
			}

			env := a.chip(c, "git-branch", "Local · "+s.Branch, a.showEnv)
			if env.Clicked() {
				a.showEnv = !a.showEnv
				a.showPerm, a.showModel = false, false
			}
			if a.showEnv {
				ui.Popover(c, env, &a.showEnv, func() { a.environmentPopover(c, s) })
			}

			ui.Spacer(c)

			if a.mainViewWidth >= 600 {
				a.contextPill(c, s, func() {
					a.openDock(3)
				})
			}
			model := a.modelChip(c, s)
			if model.Clicked() {
				a.showModel = !a.showModel
				a.showPerm, a.showEnv = false, false
			}
			if a.showModel {
				ui.Popover(c, model, &a.showModel, func() { a.modelPopover(c, s) })
			}
			a.sendButton(c, s)
		})
	})

	current := strings.TrimSpace(a.draft)
	if strings.HasPrefix(current, "/") && a.slashOpen {
		items := a.filteredSlash()
		if len(items) > 0 {
			ui.Popover(c, box, &a.slashOpen, func() { a.slashMenu(c, items) })
		}
	}
	if a.mentionOpen {
		items := a.filteredMentions()
		if len(items) > 0 {
			ui.Popover(c, box, &a.mentionOpen, func() { a.mentionMenu(c, items) })
		}
	}
}

func (a *app) pickAttachments() {
	if a.win == nil {
		a.pendingToast = "Drop files onto the composer to attach them"
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.win, Title: "Attach files", ButtonLabel: "Attach", Multiple: true,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	a.addAttachments(paths)
}

func (a *app) addAttachments(paths []string) {
	seen := make(map[string]bool, len(a.attachments))
	for _, p := range a.attachments {
		seen[p] = true
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		a.attachments = append(a.attachments, p)
	}
	if len(a.attachments) > 0 {
		a.pendingToast = "Attached " + filepath.Base(a.attachments[len(a.attachments)-1])
	}
}

func (a *app) attachmentTray(c *ui.Context) {
	p := a.pal()
	items := append([]string(nil), a.attachments...)
	remove := -1
	tray := ui.Row(c).FillWidth().Padding(9, 12, 2, 12).Gap(6).Wrap()
	tray.Children(func() {
		for i, path := range items {
			idx := i
			chip := ui.Row(c).Height(25).Padding(0, 7).Gap(5).Radius(5).
				Border(1, p.line).Background(p.surface2).AlignItems(ui.Center)
			chip.Children(func() {
				a.icon(c, "file", 11, p.text4)
				ui.Text(c, filepath.Base(path)).Font("monospace").FontSize(10).
					TextColor(p.text3).SingleLine()
				button := ui.ButtonBase(c).Size(17, 17).Radius(4).Label("Remove " + filepath.Base(path))
				if button.Hovered() {
					button.Background(p.hover)
				}
				if button.Clicked() {
					remove = idx
				}
				button.Children(func() { a.icon(c, "x", 9, p.text4) })
			})
		}
	})
	if remove >= 0 && remove < len(a.attachments) {
		a.attachments = append(a.attachments[:remove], a.attachments[remove+1:]...)
	}
}

func (a *app) syncMenus() {
	d := a.draft
	if d == a.lastDraft {
		return
	}
	a.lastDraft = d
	trimmed := strings.TrimSpace(d)
	if strings.HasPrefix(trimmed, "/") && !strings.Contains(trimmed, " ") {
		a.slashOpen = true
		a.slashIndex = 0
	} else {
		a.slashOpen = false
	}
	if _, ok := a.mentionQuery(); ok {
		a.mentionOpen = true
		a.mentionIndex = 0
	} else {
		a.mentionOpen = false
	}
}

func (a *app) mentionQuery() (string, bool) {
	d := a.draft
	i := strings.LastIndexByte(d, '@')
	if i < 0 {
		return "", false
	}
	if i > 0 {
		prev := d[i-1]
		if prev != ' ' && prev != '\n' && prev != '\t' {
			return "", false
		}
	}
	q := d[i+1:]
	if strings.ContainsAny(q, " \n\t") {
		return "", false
	}
	return q, true
}

func (a *app) filteredSlash() []slashCommand {
	trimmed := strings.TrimSpace(a.draft)
	q := strings.TrimPrefix(trimmed, "/")
	if i := strings.IndexAny(q, " \n\t"); i >= 0 {
		q = q[:i]
	}
	q = strings.ToLower(q)
	var out []slashCommand
	for _, cmd := range slashCommands {
		if q == "" || strings.Contains(strings.ToLower(strings.TrimPrefix(cmd.Name, "/")), q) {
			out = append(out, cmd)
		}
	}
	return out
}

func (a *app) filteredMentions() []string {
	q, _ := a.mentionQuery()
	q = strings.ToLower(q)
	var out []string
	for _, f := range mentionFiles {
		if q == "" || strings.Contains(strings.ToLower(f), q) {
			out = append(out, f)
		}
	}
	return out
}

func (a *app) runSlash(name string) {
	a.draft = ""
	a.lastDraft = ""
	a.slashOpen = false
	a.send(name)
}

func (a *app) insertMention(path string) {
	i := strings.LastIndexByte(a.draft, '@')
	if i < 0 {
		a.draft += "@" + path + " "
		return
	}
	a.draft = a.draft[:i] + "@" + path + " "
	a.mentionOpen = false
	a.lastDraft = a.draft
}

func (a *app) slashMenu(c *ui.Context, items []slashCommand) {
	p := a.pal()
	panel := ui.Column(c).Width(368).Padding(5)
	panel.Children(func() {
		head := ui.Row(c).Height(24).Padding(0, 7).AlignItems(ui.Center)
		head.Children(func() {
			ui.Text(c, "COMMANDS").Font("monospace").FontSize(9).TextColor(p.text4).LetterSpacing(0.7).Grow(1)
			ui.Text(c, "↑↓ · tab · enter").Font("monospace").FontSize(9).TextColor(p.text4)
		})
		for i, cmd := range items {
			idx := i
			row := ui.ButtonBase(c).FillWidth().Height(36).Padding(6, 8).Gap(9).Radius(5).
				Justify(ui.Start).Label(cmd.Name)
			if i == a.slashIndex {
				row.Background(p.accentSoft)
			} else if row.Hovered() {
				row.Background(p.surface2)
			}
			if row.Clicked() {
				a.runSlash(cmd.Name)
			}
			row.Children(func() {
				a.icon(c, "command", 13, chooser(i == a.slashIndex, p.accent, p.text4))
				ui.Column(c).Grow(1).Gap(1).Children(func() {
					ui.Text(c, cmd.Name).Font("monospace").FontSize(12).TextColor(p.text2)
					ui.Text(c, cmd.Desc).FontSize(11).TextColor(p.text4).SingleLine()
				})
				if cmd.Key != "" {
					a.shortcutKey(c, cmd.Key)
				}
				_ = idx
			})
		}
	})
}

func (a *app) mentionMenu(c *ui.Context, items []string) {
	p := a.pal()
	panel := ui.Column(c).Width(420).Padding(5)
	panel.Children(func() {
		head := ui.Row(c).Height(24).Padding(0, 7).AlignItems(ui.Center)
		head.Children(func() {
			a.icon(c, "at", 11, p.text4)
			ui.Text(c, "FILES").Font("monospace").FontSize(9).TextColor(p.text4).LetterSpacing(0.7).Grow(1)
			ui.Text(c, "↑↓ · enter").Font("monospace").FontSize(9).TextColor(p.text4)
		})
		for i, f := range items {
			idx := i
			row := ui.ButtonBase(c).FillWidth().Height(30).Padding(6, 8).Gap(8).Radius(5).
				Justify(ui.Start).Label(f)
			if i == a.mentionIndex {
				row.Background(p.accentSoft)
			} else if row.Hovered() {
				row.Background(p.surface2)
			}
			if row.Clicked() {
				a.insertMention(f)
			}
			row.Children(func() {
				a.icon(c, "file", 12, chooser(idx == a.mentionIndex, p.accent, p.text4))
				ui.Text(c, f).Font("monospace").FontSize(11).TextColor(p.text2).SingleLine()
			})
		}
	})
}

func (a *app) contextPill(c *ui.Context, s *session, onClick func()) {
	p := a.pal()
	pct := 0
	if s.Context.Limit > 0 {
		pct = s.Context.Used * 100 / s.Context.Limit
	}
	b := ui.ButtonBase(c).Height(25).Padding(0, 6).Gap(6).Radius(6).
		Background(ui.Transparent).Label("Context usage")
	if b.Hovered() {
		b.Background(p.surface2)
	}
	if b.Clicked() {
		onClick()
	}
	b.Children(func() {
		ring := ui.Box(c).Size(11, 11)
		ring.Draw(func(painter *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			bg := (&ui.Path{}).Circle(cx, cy, 4.2)
			painter.StrokePath(bg, 1.6, p.lineStrong)
			if pct > 0 {
				fg := (&ui.Path{}).Circle(cx, cy, 4.2).Trim(0, float32(min(pct, 100))/100)
				painter.StrokePath(fg, 1.6, p.accent)
			}
		})
		ui.Textf(c, "%s / %s", formatTokens(s.Context.Used), formatTokens(s.Context.Limit)).
			Font("monospace").FontSize(10).TextColor(p.text4)
	})
}

func (a *app) modelChip(c *ui.Context, s *session) ui.Element {
	p := a.pal()
	label := a.trimmed(s.modelName(), 26)
	b := ui.ButtonBase(c).Height(26).Padding(0, 7).Gap(5).Radius(6).
		Background(ui.Transparent).Label("Switch model")
	if a.showModel {
		b.Background(p.surface2).BorderColor(p.lineStrong)
	} else if b.Hovered() {
		b.Background(p.surface2).BorderColor(p.line)
	}
	b.Children(func() {
		a.icon(c, "sparkles", 13, p.accent2)
		ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(p.text3).SingleLine()
		a.icon(c, "chevron-down", 11, p.text4)
	})
	return b
}

func (a *app) sendButton(c *ui.Context, s *session) {
	p := a.pal()
	empty := strings.TrimSpace(a.draft) == ""
	running := s.Status == "running" && empty
	bg := p.accent
	color := p.accentInk
	iconName := "arrow-up"
	label := "Send message"
	if empty {
		bg, color = p.surface3, p.text4
	}
	if running {
		iconName, label = "stop", "Stop generation"
		bg, color = p.warnSoft, p.warn
	}
	b := ui.ButtonBase(c).Size(29, 29).Radius(8).Background(bg).Label(label)
	if b.Hovered() && (!empty || running) {
		b.Background(p.accent2)
	}
	if b.Clicked() {
		if running {
			a.stopSession(s)
		} else if !empty {
			a.send(a.draft)
		}
	}
	b.Children(func() {
		a.icon(c, iconName, 14, color)
	})
}

func (a *app) composerMeta(c *ui.Context, s *session) {
	p := a.pal()
	row := ui.Row(c).FillWidth().Padding(6, 3, 0, 3).Gap(8).AlignItems(ui.Center)
	row.Children(func() {
		left := ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1)
		left.Children(func() {
			if s.pendingApproval() != nil {
				a.icon(c, "alert", 10, p.warn)
				ui.Text(c, "1 command awaiting approval").Font("monospace").FontSize(10).TextColor(p.warn).SingleLine()
			} else if s.Status == "running" {
				ui.Box(c).Size(6, 6).Radius(6).Background(p.accent)
				ui.Text(c, "working · Esc to stop").Font("monospace").FontSize(10).TextColor(p.accent2).SingleLine()
			} else {
				ui.Text(c, "Enter to send · Shift+Enter for a new line").Font("monospace").FontSize(10).TextColor(p.text4).SingleLine()
			}
		})
		right := ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.providerLabel(s.modelProvider())).Font("monospace").FontSize(10).TextColor(p.text4)
			ui.Text(c, "·").Font("monospace").FontSize(10).TextColor(p.text4.Alpha(0.45))
			ui.Text(c, s.thinkingName()+" reasoning").Font("monospace").FontSize(10).TextColor(p.text4)
		})
		_ = right
	})
}

func (a *app) providerLabel(id string) string {
	for _, p := range a.providers {
		if p.ID == id {
			return p.Name
		}
	}
	return id
}

func (a *app) permissionPopover(c *ui.Context, s *session) {
	p := a.pal()
	panel := ui.Column(c).Width(320).Padding(6)
	panel.Children(func() {
		head := ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center)
		head.Children(func() {
			ui.Text(c, "PERMISSION MODE").Font("monospace").FontSize(9).TextColor(p.text4).LetterSpacing(0.7)
		})
		options := []struct{ id, title, desc string }{
			{"manual", "Manual", "Ask before commands and file edits"},
			{"auto-edits", "Auto edits", "Run commands; ask before writes"},
			{"plan", "Plan only", "Read and plan; never change files"},
			{"auto", "Auto", "Run commands and edits without asking"},
		}
		for _, o := range options {
			opt := o
			row := ui.ButtonBase(c).FillWidth().Height(44).Padding(7, 8).Gap(8).Radius(6).
				Justify(ui.Start).Label(opt.title)
			if s.Approval == opt.id {
				row.Background(p.accentSoft)
			} else if row.Hovered() {
				row.Background(p.surface2)
			}
			if row.Clicked() {
				s.Approval = opt.id
				a.showPerm = false
				a.pendingToast = "Permission mode: " + opt.title
			}
			row.Children(func() {
				a.icon(c, "shield", 14, chooser(s.Approval == opt.id, p.accent, p.text4))
				ui.Column(c).Grow(1).Gap(1).Children(func() {
					ui.Text(c, opt.title).FontSize(12).TextColor(p.text2)
					ui.Text(c, opt.desc).FontSize(11).TextColor(p.text4).SingleLine()
				})
				if s.Approval == opt.id {
					a.icon(c, "check", 12, p.accent)
				}
			})
		}
	})
}

func (a *app) environmentPopover(c *ui.Context, s *session) {
	p := a.pal()
	panel := ui.Column(c).Width(330).Padding(6)
	panel.Children(func() {
		head := ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center)
		head.Children(func() {
			ui.Text(c, "ENVIRONMENT").Font("monospace").FontSize(9).TextColor(p.text4).LetterSpacing(0.7)
		})
		localDesc := s.CWD + " · current working tree"
		if s.CWD == "" {
			localDesc = "current working tree"
		}
		for _, o := range []struct{ id, title, desc string }{
			{"Local", "Local", localDesc},
			{"Worktree", "Worktree", "Not connected to the SDK yet"},
		} {
			opt := o
			row := ui.ButtonBase(c).FillWidth().Height(44).Padding(7, 8).Gap(8).Radius(6).
				Justify(ui.Start).Label(opt.title)
			if s.Env == opt.id {
				row.Background(p.accentSoft)
			} else if row.Hovered() {
				row.Background(p.surface2)
			}
			if row.Clicked() {
				if opt.id == "Worktree" {
					a.showEnv = false
					a.pendingToast = "Worktree sessions are not wired up yet"
					continue
				}
				s.Env = opt.id
				a.showEnv = false
			}
			row.Children(func() {
				a.icon(c, "folder", 14, chooser(s.Env == opt.id, p.accent, p.text4))
				ui.Column(c).Grow(1).Gap(1).Children(func() {
					ui.Text(c, opt.title).FontSize(12).TextColor(p.text2)
					ui.Text(c, opt.desc).FontSize(11).TextColor(p.text4).SingleLine()
				})
				if s.Env == opt.id {
					a.icon(c, "check", 12, p.accent)
				}
			})
		}
	})
}

func (a *app) modelPopover(c *ui.Context, s *session) {
	p := a.pal()
	panel := ui.Column(c).Width(360).Padding(6)
	panel.Children(func() {
		head := ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center)
		head.Children(func() {
			ui.Text(c, "MODEL").Font("monospace").FontSize(9).TextColor(p.text4).LetterSpacing(0.7).Grow(1)
			ui.Text(c, formatTokens(s.Context.Limit)+" context").Font("monospace").FontSize(9).TextColor(p.text4)
		})
		for _, prov := range a.providers {
			ui.Row(c).Height(20).Padding(0, 8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, strings.ToUpper(prov.Name)).Font("monospace").FontSize(9).TextColor(p.text4).LetterSpacing(0.5)
			})
			provider := prov
			for _, m := range provider.Models {
				mdl := m
				id := provider.ID + "/" + mdl.ID
				row := ui.ButtonBase(c).FillWidth().Height(38).Padding(6, 8).Gap(8).Radius(5).
					Justify(ui.Start).Label(mdl.Name)
				if s.Model == id {
					row.Background(p.accentSoft)
				} else if row.Hovered() {
					row.Background(p.surface2)
				}
				if row.Clicked() {
					a.setSessionModel(s, id)
					a.showModel = false
					a.pendingToast = "Model: " + mdl.Name
				}
				row.Children(func() {
					a.icon(c, "sparkles", 13, chooser(s.Model == id, p.accent2, p.text4))
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, mdl.Name).FontSize(12).TextColor(p.text2)
						ui.Textf(c, "%s · %s out", formatTokens(mdl.Context), formatTokens(mdl.MaxOut)).
							Font("monospace").FontSize(10).TextColor(p.text4)
					})
					if s.Model == id {
						a.icon(c, "check", 12, p.accent)
					}
				})
			}
		}
		ui.Box(c).Height(1).FillWidth().Background(p.line).Margin(4, 0)
		ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "THINKING").Font("monospace").FontSize(9).TextColor(p.text4).LetterSpacing(0.7).Grow(1)
		})
		ui.Row(c).Padding(0, 6, 4, 6).Gap(4).Children(func() {
			for _, level := range []string{"low", "medium", "high"} {
				l := level
				b := ui.ButtonBase(c).Height(26).Padding(0, 10).Radius(5).Grow(1).Label(l).
					Background(p.surface2).Border(1, p.line)
				if s.Thinking == l {
					b.Background(p.accentSoft).BorderColor(p.accent.Alpha(0.3))
				} else if b.Hovered() {
					b.Background(p.surface3)
				}
				if b.Clicked() {
					a.setSessionThinking(s, l)
				}
				b.Children(func() {
					color := p.text3
					if s.Thinking == l {
						color = p.accent
					}
					ui.Text(c, strings.ToUpper(l)).Font("monospace").FontSize(10).TextColor(color)
				})
			}
		})
	})
}
