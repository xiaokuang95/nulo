package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/codemode"
	"github.com/xiaokuang95/nulo/sdk/internal/toolsearch"
)

// tool_search mirrors Pi's tool-search extension: a BM25 ranker over the tools
// that are not declared to the model yet. Loading a match adds it to the active
// set, which the agent loop records in the transcript like any other tool change.
const (
	toolSearchName         = "tool_search"
	defaultToolSearchLimit = 8
)

const toolSearchDescription = "# Tool discovery\n\n" +
	"Searches over deferred tool metadata with BM25 and exposes matching tools for the next model call.\n\n" +
	"Some of the tools, such as tools of MCP servers, may not have been provided to you upfront, " +
	"and you should use this tool (`tool_search`) to search for the required tools. " +
	"For MCP tool discovery, always use `tool_search`."

var toolSearchSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "Search query for deferred tools."},
    "limit": {"type": "number", "description": "Maximum number of tools to return. Defaults to 8."}
  },
  "required": ["query"]
}`)

// toolSearchDocument is the searchable text of one tool, including the namespace
// it belongs to, which is what lets a query like "radius" find an MCP tool whose
// own name says nothing about the server.
func toolSearchDocument(tool agent.AgentTool, ns *codemode.Namespace) string {
	var extra []string
	if ns != nil {
		extra = append(extra, ns.Name, ns.Description, ns.Instructions)
	}
	return toolsearch.Document(tool.Name, tool.Description, tool.Parameters, extra...)
}

// toolSearchTool builds the tool_search tool. It searches the catalogue for
// tools that are reachable but not declared yet (codemode and deferred
// exposure) and queues the matches for the next model call.
func (s *Session) toolSearchTool() agent.AgentTool {
	return agent.AgentTool{
		Tool: ai.Tool{
			Name:        toolSearchName,
			Description: toolSearchDescription,
			Parameters:  toolSearchSchema,
		},
		Label: toolSearchName,
		Execute: func(_ context.Context, _ string, args map[string]any) (agent.ToolResult, error) {
			query, _ := args["query"].(string)
			if strings.TrimSpace(query) == "" {
				return agent.ToolResult{}, fmt.Errorf("query must not be empty")
			}
			limit := defaultToolSearchLimit
			if raw, ok := args["limit"]; ok {
				n, ok := raw.(float64)
				if !ok || n <= 0 || n != math.Trunc(n) {
					return agent.ToolResult{}, fmt.Errorf("limit must be a positive integer")
				}
				limit = int(n)
			}

			loaded := s.loadTools(query, limit)
			text := "No matching tools found."
			if len(loaded) > 0 {
				var b strings.Builder
				noun := "tools"
				if len(loaded) == 1 {
					noun = "tool"
				}
				fmt.Fprintf(&b, "Loaded %d %s. They are available from your next call:", len(loaded), noun)
				for _, t := range loaded {
					first := t.Description
					if i := strings.IndexAny(first, "\r\n"); i >= 0 {
						first = first[:i]
					}
					fmt.Fprintf(&b, "\n- %s: %s", t.Name, strings.TrimSpace(first))
				}
				text = b.String()
			}
			return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(text)}}, nil
		},
	}
}

// loadTools ranks the searchable tools that are not active yet and queues the
// matches, returning them so the caller can describe what was loaded.
func (s *Session) loadTools(query string, limit int) []agent.AgentTool {
	s.mu.Lock()
	active := make(map[string]bool, len(s.tools))
	for _, t := range s.tools {
		active[t.Name] = true
	}
	namespaces := map[string]*codemode.Namespace{}
	for _, ns := range s.mcpNamespaces() {
		ns := ns
		namespaces[ns.Name] = &ns
	}
	candidates := make([]agent.AgentTool, 0, len(s.catalog))
	for _, t := range s.catalog {
		exposure, ok := s.toolExposure[t.Name]
		if !ok || !exposure.Searchable() || active[t.Name] {
			continue
		}
		candidates = append(candidates, t)
	}
	s.mu.Unlock()

	documents := make([]string, len(candidates))
	for i, t := range candidates {
		documents[i] = toolSearchDocument(t, namespaces[s.toolNamespace[t.Name]])
	}
	matches := toolsearch.Rank(query, documents, limit)
	if len(matches) == 0 {
		return nil
	}

	loaded := make([]agent.AgentTool, 0, len(matches))
	for _, match := range matches {
		loaded = append(loaded, candidates[match.Index])
	}

	s.mu.Lock()
	s.pendingTools = append(s.pendingTools, loaded...)
	s.tools = append(s.tools, loaded...)
	s.mu.Unlock()
	return loaded
}
