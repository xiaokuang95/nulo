package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *app) icon(c *ui.Context, name string, size float32, color ui.Color) ui.Element {
	ic := icons[name]
	if ic == nil {
		return ui.Box(c).Size(size, size)
	}
	return ui.Icon(c, ic).Size(size, size).TextColor(color).Shrink(0)
}

func (a *app) iconButton(c *ui.Context, name, label string, active bool, onClick func()) ui.Element {
	t := a.pal()
	b := ui.ButtonBase(c).Size(28, 28).Radius(6).Label(label).Tooltip(label)
	if active {
		b.Background(t.accentSoft)
	} else if b.Hovered() {
		b.Background(t.hover)
	}
	if b.Clicked() && onClick != nil {
		onClick()
	}
	color := t.text3
	if active {
		color = t.accent
	}
	b.Children(func() {
		a.icon(c, name, 16, color)
	})
	return b
}

func (a *app) button(c *ui.Context, label string) ui.Element {
	t := a.pal()
	b := ui.ButtonBase(c).Height(28).Padding(0, 11).Radius(6).
		Border(1, t.lineStrong).Background(t.surface2).Label(label)
	if b.Hovered() {
		b.BorderColor(t.text4).Background(t.surface3)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(550).TextColor(t.text2).SingleLine()
	})
	return b
}

func (a *app) ghostButton(c *ui.Context, label string) ui.Element {
	t := a.pal()
	b := ui.ButtonBase(c).Height(28).Padding(0, 10).Radius(6).
		Border(1, t.line).Background(ui.Transparent).Label(label)
	if b.Hovered() {
		b.BorderColor(t.lineStrong).Background(t.surface2)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(t.text3).SingleLine()
	})
	return b
}

func (a *app) smallButton(c *ui.Context, label string) ui.Element {
	t := a.pal()
	b := ui.ButtonBase(c).Height(24).Padding(0, 8).Radius(5).Border(1, t.line).
		Background(ui.Transparent).Label(label)
	if b.Hovered() {
		b.BorderColor(t.lineStrong).Background(t.surface2)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(11).FontWeight(500).TextColor(t.text3).SingleLine()
	})
	return b
}

func (a *app) dangerButton(c *ui.Context, label string) ui.Element {
	t := a.pal()
	b := ui.ButtonBase(c).Height(28).Padding(0, 10).Radius(6).
		Border(1, t.bad.Alpha(0.28)).Background(ui.Transparent).Label(label)
	if b.Hovered() {
		b.BorderColor(t.bad.Alpha(0.45)).Background(t.badSoft)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(550).TextColor(t.bad).SingleLine()
	})
	return b
}

func (a *app) primaryButton(c *ui.Context, label, icon string) ui.Element {
	t := a.pal()
	b := ui.ButtonBase(c).Height(28).Padding(0, 11).Gap(6).Radius(6).Background(t.accent).Label(label)
	if b.Hovered() {
		b.Background(t.accent2)
	}
	b.Children(func() {
		if icon != "" {
			a.icon(c, icon, 12, t.accentInk)
		}
		ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(t.accentInk).SingleLine()
	})
	return b
}

func (a *app) statusPill(c *ui.Context, status string) ui.Element {
	t := a.pal()
	color, bg, border := t.text4, ui.Transparent, ui.Transparent
	switch status {
	case "awaiting":
		color, bg, border = t.warn, t.warnSoft, t.warn.Alpha(0.24)
	case "running":
		color, bg, border = t.accent, t.accentSoft, t.accent.Alpha(0.20)
	case "paused":
		color, bg, border = t.info, t.info.Alpha(0.06), t.info.Alpha(0.18)
	case "error":
		color, bg, border = t.bad, t.badSoft, t.bad.Alpha(0.20)
	case "done":
		color = t.text4
	default:
		color = t.text3
	}
	pill := ui.Row(c).Height(19).Padding(0, 7).Gap(5).Radius(5).
		Border(1, border).Background(bg).AlignItems(ui.Center)
	pill.Children(func() {
		dot := t.text4
		switch status {
		case "awaiting":
			dot = t.warn
		case "running":
			dot = t.accent
		case "paused":
			dot = t.info
		case "error":
			dot = t.bad
		}
		ui.Box(c).Size(5, 5).Radius(9).Background(dot)
		ui.Text(c, statusLabel(status)).Font("monospace").FontSize(11).TextColor(color).SingleLine()
	})
	return pill
}

func (a *app) statusDot(c *ui.Context, status string, size float32) ui.Element {
	t := a.pal()
	color := t.lineStrong
	switch status {
	case "awaiting":
		color = t.warn
	case "running":
		color = t.accent
	case "paused":
		color = t.info
	case "error":
		color = t.bad
	case "done":
		color = t.text4
	case "idle":
		color = t.text4
	}
	return ui.Box(c).Size(size, size).Radius(size).Background(color)
}

func (a *app) progress(c *ui.Context, percent int, height float32, color ui.Color) ui.Element {
	t := a.pal()
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	track := ui.Box(c).FillWidth().Height(height).Radius(height).Background(t.line)
	track.Children(func() {
		if percent > 0 {
			ui.Box(c).WidthPercent(float32(percent)).FillHeight().Radius(height).Background(color)
		}
	})
	return track
}

func (a *app) chip(c *ui.Context, icon, label string, open bool) ui.Element {
	t := a.pal()
	b := ui.ButtonBase(c).Height(26).Padding(0, 7).Gap(5).Radius(6).
		Border(1, ui.Transparent).Background(ui.Transparent).Label(label)
	if open {
		b.Background(t.surface2).BorderColor(t.lineStrong)
	} else if b.Hovered() {
		b.Background(t.surface2).BorderColor(t.line)
	}
	b.Children(func() {
		if icon != "" {
			a.icon(c, icon, 13, t.text4)
		}
		ui.Text(c, label).FontSize(12).TextColor(t.text3).SingleLine()
		a.icon(c, "chevron-down", 11, t.text4)
	})
	return b
}

func (a *app) checkbox(c *ui.Context, checked bool, label string) ui.Element {
	t := a.pal()
	b := ui.ButtonBase(c).Size(14, 14).Radius(3).Border(1, t.lineStrong).
		Background(ui.Transparent).Label(label).FocusRingOffset(-2)
	if checked {
		b.Background(t.accent).BorderColor(t.accent)
	} else if b.Hovered() {
		b.BorderColor(t.text4).Background(t.surface2)
	}
	b.Children(func() {
		if checked {
			a.icon(c, "check", 9, t.accentInk)
		}
	})
	return b
}

func (a *app) shortcutKey(c *ui.Context, s string) {
	if s == "" {
		return
	}
	ui.Text(c, s).Font("monospace").FontSize(11).TextColor(a.pal().text4).
		Padding(2, 5).Radius(4).Border(1, a.pal().lineStrong).Background(a.pal().surface2).SingleLine()
}

func (a *app) sectionTitle(c *ui.Context, s string) ui.Element {
	return ui.Text(c, strings.ToUpper(s)).Font("monospace").FontSize(10).
		TextColor(a.pal().text3).LetterSpacing(0.7)
}

func (a *app) kv(c *ui.Context, k, v string, accent bool) {
	t := a.pal()
	color := t.text3
	if accent {
		color = t.accent2
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, k).Font("monospace").FontSize(11).TextColor(t.text4).Grow(1).SingleLine()
		ui.Text(c, v).Font("monospace").FontSize(11).TextColor(color).SingleLine()
	})
}

func (a *app) trimmed(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

func durationShort(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	return fmt.Sprintf("%.0fs", d.Seconds())
}
