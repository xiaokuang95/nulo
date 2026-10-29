package ai

import (
	"sort"
	"strings"
)

func MessageText(m Message) string {
	switch m.Role {
	case "system":
		return SystemMessageText(m)
	case "assistant":
		var parts []string
		for _, b := range m.ContentBlocks() {
			switch b.Type {
			case "text":
				parts = append(parts, b.Text)
			case "thinking":
				parts = append(parts, b.Thinking)
			case "toolCall":
				parts = append(parts, b.Name)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return m.ContentString()
	}
}

func SystemMessageText(m Message) string {
	parts := []string{m.ContentString()}
	if m.Sections != nil {
		for _, name := range sectionOrder(m.Sections) {
			if v := m.Sections[name]; v != nil && *v != "" {
				parts = append(parts, *v)
			}
		}
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

func sectionOrder(sections map[string]*string) []string {
	preferred := []string{"preamble", "tools", "rules", "docs", "addendum", "project_context", "skills", "cwd"}
	seen := map[string]bool{}
	var names []string
	for _, n := range preferred {
		if _, ok := sections[n]; ok {
			names = append(names, n)
			seen[n] = true
		}
	}
	var extra []string
	for n := range sections {
		if !seen[n] {
			extra = append(extra, n)
		}
	}
	sort.Strings(extra)
	return append(names, extra...)
}
