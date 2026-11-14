package main

import "github.com/egoist/mygo/ui"

// Icons are the 24×24 line icons of the prototype, kept as SVG so they stay
// crisp at any scale and take the color of the text around them.
var icons = func() map[string]*ui.SVG {
	paths := map[string]string{
		"plus":          `<path d="M12 5v14M5 12h14"/>`,
		"search":        `<circle cx="11" cy="11" r="7"/><path d="m21 21-4.35-4.35"/>`,
		"chevron-down":  `<path d="m6 9 6 6 6-6"/>`,
		"chevron-right": `<path d="m9 18 6-6-6-6"/>`,
		"chevron-left":  `<path d="m15 18-6-6 6-6"/>`,
		"folder":        `<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7l-2-2H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2Z"/>`,
		"image":         `<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/>`,
		"git-branch":    `<path d="M6 3v12"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/>`,
		"settings":      `<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3"/><path d="M1 14h6M9 8h6M17 16h6"/>`,
		"panel-left":    `<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M9 3v18"/>`,
		"panel-right":   `<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M15 3v18"/>`,
		"check":         `<path d="M20 6 9 17l-5-5"/>`,
		"x":             `<path d="M18 6 6 18M6 6l12 12"/>`,
		"more":          `<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`,
		"terminal":      `<path d="m4 17 6-6-6-6M12 19h8"/>`,
		"diff":          `<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6M12 12v6M9 15h6"/>`,
		"list-checks":   `<path d="m3 17 2 2 4-4M3 7l2 2 4-4M13 6h8M13 12h8M13 18h8"/>`,
		"layers":        `<path d="m12 2 10 5-10 5L2 7z"/><path d="m2 17 10 5 10-5M2 12l10 5 10-5"/>`,
		"paperclip":     `<path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48"/>`,
		"arrow-up":      `<path d="M12 19V5M5 12l7-7 7 7"/>`,
		"arrow-right":   `<path d="M5 12h14M12 5l7 7-7 7"/>`,
		"stop":          `<rect x="6" y="6" width="12" height="12" rx="2"/>`,
		"cpu":           `<rect x="5" y="5" width="14" height="14" rx="2"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3"/>`,
		"shield":        `<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>`,
		"globe":         `<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a15 15 0 0 1 0 18M12 3a15 15 0 0 0 0 18"/>`,
		"sun":           `<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.42 1.42M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"/>`,
		"moon":          `<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>`,
		"copy":          `<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`,
		"external":      `<path d="M15 3h6v6M10 14 21 3M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5"/>`,
		"fork":          `<circle cx="6" cy="18" r="3"/><circle cx="18" cy="18" r="3"/><circle cx="12" cy="6" r="3"/><path d="M12 9v3a3 3 0 0 0 3 3h0a3 3 0 0 1 3 3M12 12a3 3 0 0 1-3 3h0a3 3 0 0 0-3 3"/>`,
		"play":          `<path d="M6 3l14 9-14 9z"/>`,
		"pause":         `<path d="M8 4v16M16 4v16"/>`,
		"trash":         `<path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6M10 11v6M14 11v6"/>`,
		"command":       `<path d="M18 3a3 3 0 0 0-3 3v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3V6a3 3 0 1 0-3 3h12a3 3 0 0 0 0-6z"/>`,
		"keyboard":      `<rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 10h.01M10 10h.01M14 10h.01M18 10h.01M6 14h.01M18 14h.01M9 14h6"/>`,
		"info":          `<circle cx="12" cy="12" r="9"/><path d="M12 16v-4M12 8h.01"/>`,
		"alert":         `<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/>`,
		"sparkles":      `<path d="M12 3l1.9 5.8L20 11l-6.1 2.2L12 19l-1.9-5.8L4 11l6.1-2.2z"/><path d="M19 3v4M17 5h4"/>`,
		"clock":         `<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`,
		"book":          `<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/>`,
		"plug":          `<path d="M12 22v-5M9 8V2M15 8V2M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8z"/>`,
		"test":          `<path d="M9 3h6M10 3v6L4.5 19a1.5 1.5 0 0 0 1.3 2h12.4a1.5 1.5 0 0 0 1.3-2L14 9V3"/><path d="M7 15h10"/>`,
		"file":          `<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/>`,
		"code":          `<path d="m16 18 6-6-6-6M8 6l-6 6 6 6"/>`,
		"eye":           `<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/>`,
		"server":        `<rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><path d="M6 6h.01M6 18h.01"/>`,
		"package":       `<path d="m7.5 4.27 9 5.15M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><path d="m3.3 7 8.7 5 8.7-5M12 22V12"/>`,
		"corner":        `<path d="M15 10l5 5-5 5M4 4v7a4 4 0 0 0 4 4h12"/>`,
		"compress":      `<path d="M4 14h6v6M20 10h-6V4M14 10l7-7M3 21l7-7"/>`,
		"refresh":       `<path d="M21 12a9 9 0 1 1-2.64-6.36L21 8"/><path d="M21 3v5h-5"/>`,
		"lock":          `<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>`,
		"at":            `<circle cx="12" cy="12" r="4"/><path d="M16 8v5a3 3 0 0 0 6 0v-1a10 10 0 1 0-4 8"/>`,
	}

	out := make(map[string]*ui.SVG, len(paths))
	for name, inner := range paths {
		out[name] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + inner + `</svg>`))
	}
	return out
}()
