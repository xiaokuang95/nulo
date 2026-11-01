package prompt

import (
	"fmt"
	"sort"
	"strings"
)

// MCPServer is one entry of the mcp_servers section: a server whose tools are
// deliberately not declared to the model. The caller decides which servers
// qualify and what their summary is; this file only renders.
type MCPServer struct {
	// Namespace is the identifier scripts and describeNamespace() use, e.g.
	// "mcp__dev-radius" for tools named "mcp__dev-radius__search".
	Namespace string
	// Summary is the configured description, or the server's own instructions.
	Summary string
	// Reach is how the tools are reached: "codemode", or "tool_search" when the
	// server's tools are only loaded on demand.
	Reach string
}

// Limits Pi applies to the section: one server's summary, and the whole section.
const (
	maxMCPServerSummaryRunes  = 250
	maxMCPServersSectionRunes = 4096
)

// renderMCPServers mirrors Pi's renderServersSection. Servers are listed shortest
// form first; if the section would overflow, trailing servers are dropped into a
// final line pointing at searchTools().
func renderMCPServers(servers []MCPServer) string {
	if len(servers) == 0 {
		return ""
	}
	sorted := append([]MCPServer(nil), servers...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Namespace < sorted[j].Namespace })

	reaches := map[string]bool{}
	for _, s := range sorted {
		reaches[s.Reach] = true
	}
	intro := "MCP servers whose tools are not declared to you."
	if reaches["codemode"] {
		intro += " Call the tools of `codemode` servers from codemode scripts."
	}
	if reaches["tool_search"] {
		intro += " Load the tools of `tool_search` servers with `tool_search`."
	}

	head := func(i int) string {
		return "- " + sorted[i].Namespace + " (" + sorted[i].Reach + ")"
	}
	omitted := func(n int) []string {
		if n <= 0 {
			return nil
		}
		plural := "s"
		if n == 1 {
			plural = ""
		}
		return []string{fmt.Sprintf("- … %d more server%s; find their tools with searchTools()", n, plural)}
	}
	// size measures the section without summaries, which is what the fit loop uses.
	size := func(kept int) int {
		parts := make([]string, 0, kept+2)
		parts = append(parts, intro)
		for i := 0; i < kept; i++ {
			parts = append(parts, head(i))
		}
		parts = append(parts, omitted(len(sorted)-kept)...)
		return len([]rune(strings.Join(parts, "\n")))
	}

	kept := len(sorted)
	for kept > 0 && size(kept) > maxMCPServersSectionRunes {
		kept--
	}
	perServer := 0
	if kept > 0 {
		if perServer = (maxMCPServersSectionRunes-size(kept))/kept - 2; perServer > maxMCPServerSummaryRunes {
			perServer = maxMCPServerSummaryRunes
		}
		if perServer < 0 {
			perServer = 0
		}
	}

	parts := make([]string, 0, kept+2)
	parts = append(parts, intro)
	for i := 0; i < kept; i++ {
		line := head(i)
		if perServer > 0 {
			if s := truncateRunes(sorted[i].Summary, perServer); s != "" {
				line += ": " + s
			}
		}
		parts = append(parts, line)
	}
	parts = append(parts, omitted(len(sorted)-kept)...)
	return strings.Join(parts, "\n")
}

// truncateRunes caps text at max runes, cutting on a rune boundary so a
// multi-byte character is never split.
func truncateRunes(text string, max int) string {
	r := []rune(text)
	if len(r) <= max {
		return text
	}
	if max <= 1 {
		return ""
	}
	return strings.TrimRight(string(r[:max-1]), " \t") + "…"
}
