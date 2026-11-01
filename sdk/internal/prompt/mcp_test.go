package prompt

import (
	"strings"
	"testing"
)

func TestRenderMCPServers(t *testing.T) {
	body := renderMCPServers([]MCPServer{
		{Namespace: "mcp__beta", Summary: "Beta server", Reach: "codemode"},
		{Namespace: "mcp__alpha", Summary: "Alpha server", Reach: "codemode"},
	})
	lines := strings.Split(body, "\n")
	if !strings.HasPrefix(lines[0], "MCP servers whose tools are not declared") {
		t.Fatalf("intro %q", lines[0])
	}
	if !strings.Contains(lines[0], "`codemode` servers from codemode scripts") {
		t.Fatalf("intro should explain codemode reach: %q", lines[0])
	}
	// Sorted by namespace, each line carrying its summary.
	if lines[1] != "- mcp__alpha (codemode): Alpha server" {
		t.Fatalf("line %q", lines[1])
	}
	if lines[2] != "- mcp__beta (codemode): Beta server" {
		t.Fatalf("line %q", lines[2])
	}
}

func TestRenderMCPServersTruncatesSummary(t *testing.T) {
	long := strings.Repeat("x", maxMCPServerSummaryRunes+50)
	body := renderMCPServers([]MCPServer{{Namespace: "mcp__a", Summary: long, Reach: "codemode"}})
	line := strings.Split(body, "\n")[1]
	summary := strings.TrimPrefix(line, "- mcp__a (codemode): ")
	if !strings.HasSuffix(summary, "…") {
		t.Fatalf("summary should be truncated: %q", summary)
	}
	if got := len([]rune(summary)); got > maxMCPServerSummaryRunes {
		t.Fatalf("summary length %d exceeds the cap", got)
	}
}

func TestRenderMCPServersDropsOverflowIntoOneLine(t *testing.T) {
	// Enough servers that the section cannot hold them all.
	var servers []MCPServer
	for i := 0; i < 400; i++ {
		servers = append(servers, MCPServer{
			Namespace: "mcp__server" + strings.Repeat("x", 20) + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Summary:   "a summary",
			Reach:     "codemode",
		})
	}
	body := renderMCPServers(servers)
	if got := len([]rune(body)); got > maxMCPServersSectionRunes {
		t.Fatalf("section is %d runes, over the %d cap", got, maxMCPServersSectionRunes)
	}
	if !strings.Contains(body, "more servers; find their tools with searchTools()") {
		t.Fatalf("overflow should be reported, not silent: %q", body)
	}
}

func TestRenderMCPServersSingularOverflow(t *testing.T) {
	// One server past the cap reads "1 more server", not "1 more servers". The
	// short namespace sorts first, so it is the one that survives the cap.
	body := renderMCPServers([]MCPServer{
		{Namespace: "mcp__a", Reach: "codemode"},
		{Namespace: "mcp__" + strings.Repeat("z", maxMCPServersSectionRunes), Reach: "codemode"},
	})
	if !strings.Contains(body, "… 1 more server; find their tools with searchTools()") {
		t.Fatalf("body %q", body)
	}
}

func TestRenderMCPServersEmpty(t *testing.T) {
	if got := renderMCPServers(nil); got != "" {
		t.Fatalf("no servers should mean no section, got %q", got)
	}
}
