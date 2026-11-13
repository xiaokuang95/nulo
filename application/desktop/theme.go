package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// palette is the design's color set. The prototype ships two themes that
// share the same components; this keeps that property in the native app too.
type palette struct {
	dark bool

	bg         ui.Color
	bgElevated ui.Color
	sidebar    ui.Color
	panel      ui.Color
	surface    ui.Color
	surface2   ui.Color
	surface3   ui.Color
	hover      ui.Color
	line       ui.Color
	lineStrong ui.Color

	text  ui.Color
	text2 ui.Color
	text3 ui.Color
	text4 ui.Color

	accent       ui.Color
	accent2      ui.Color
	accentSoft   ui.Color
	accentSofter ui.Color
	accentInk    ui.Color

	warn     ui.Color
	warnSoft ui.Color
	bad      ui.Color
	badSoft  ui.Color
	info     ui.Color
	violet   ui.Color

	codeBg     ui.Color
	userBubble ui.Color
}

var moss = palette{
	dark:       true,
	bg:         ui.Hex("#0a0a0b"),
	bgElevated: ui.Hex("#0d0d0e"),
	sidebar:    ui.Hex("#0e0e10"),
	panel:      ui.Hex("#121214"),
	surface:    ui.Hex("#161618"),
	surface2:   ui.Hex("#1b1b1e"),
	surface3:   ui.Hex("#222226"),
	hover:      ui.Hex("#1a1a1d"),
	line:       ui.Hex("#232327"),
	lineStrong: ui.Hex("#35353b"),

	text:  ui.Hex("#f0f0f1"),
	text2: ui.Hex("#c2c2c6"),
	text3: ui.Hex("#9a9aa1"),
	text4: ui.Hex("#8a8a92"),

	accent:       ui.Hex("#b7e08b"),
	accent2:      ui.Hex("#9dcc72"),
	accentSoft:   ui.RGBA(183, 224, 139, 0.10),
	accentSofter: ui.RGBA(183, 224, 139, 0.055),
	accentInk:    ui.Hex("#0a1007"),

	warn:     ui.Hex("#e5c07b"),
	warnSoft: ui.RGBA(229, 192, 123, 0.09),
	bad:      ui.Hex("#e39a9a"),
	badSoft:  ui.RGBA(227, 154, 154, 0.09),
	info:     ui.Hex("#8fb8dc"),
	violet:   ui.Hex("#c2a4dc"),

	codeBg:     ui.Hex("#08080a"),
	userBubble: ui.Hex("#151517"),
}

var paper = palette{
	dark:       false,
	bg:         ui.Hex("#faf9f5"),
	bgElevated: ui.Hex("#f6f4ee"),
	sidebar:    ui.Hex("#f3f1ea"),
	panel:      ui.Hex("#fbfaf6"),
	surface:    ui.Hex("#ffffff"),
	surface2:   ui.Hex("#f4f2ec"),
	surface3:   ui.Hex("#eae7de"),
	hover:      ui.Hex("#eeece4"),
	line:       ui.Hex("#e3dfd4"),
	lineStrong: ui.Hex("#c9c3b5"),

	text:  ui.Hex("#1b1b19"),
	text2: ui.Hex("#3c3c38"),
	text3: ui.Hex("#5d5d56"),
	text4: ui.Hex("#6b6b64"),

	accent:       ui.Hex("#4f7a2f"),
	accent2:      ui.Hex("#416826"),
	accentSoft:   ui.RGBA(79, 122, 47, 0.10),
	accentSofter: ui.RGBA(79, 122, 47, 0.05),
	accentInk:    ui.Hex("#ffffff"),

	warn:     ui.Hex("#8d6119"),
	warnSoft: ui.RGBA(141, 97, 25, 0.12),
	bad:      ui.Hex("#a14444"),
	badSoft:  ui.RGBA(161, 68, 68, 0.10),
	info:     ui.Hex("#33678f"),
	violet:   ui.Hex("#6f4e88"),

	codeBg:     ui.Hex("#f5f3ec"),
	userBubble: ui.Hex("#f7f5ef"),
}

func (p palette) theme() *ui.Theme {
	return &ui.Theme{
		Dark:            p.dark,
		Background:      p.bg,
		Surface:         p.surface,
		SurfaceHover:    p.surface2,
		SurfacePressed:  p.surface3,
		Border:          p.line,
		Text:            p.text,
		TextMuted:       p.text4,
		Accent:          p.accent,
		AccentHover:     p.accent2,
		AccentPressed:   p.accent2,
		AccentText:      p.accentInk,
		InactiveAccent:  p.accent,
		Danger:          p.bad,
		Warning:         p.warn,
		Success:         p.accent2,
		Selection:       p.accent.Alpha(0.32),
		Focus:           p.accent,
		FocusRingWidth:  1.5,
		FocusRingOffset: 1,
		Inverse:         p.text,
		InverseText:     p.bg,
		Scrollbar:       p.lineStrong,
		ScrollbarWidth:  8,
		Radius:          6,
		Spacing:         4,
		FontSize:        13,
		TooltipDelay:    450 * time.Millisecond,
	}
}

func (a *app) pal() palette {
	if a.paper {
		return paper
	}
	return moss
}
