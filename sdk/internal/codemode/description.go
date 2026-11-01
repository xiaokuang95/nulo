package codemode

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
)

// DefaultInlineBudget is the estimated token budget the description may spend on
// tool declarations, matching Pi's codemode.inlineBudget default.
const DefaultInlineBudget = 3000

const descriptionIntro = "Run JavaScript that calls other tools. The input is raw JavaScript (not JSON, no code fence), " +
	"run as an async function body in a sandboxed JavaScript engine: top-level `await` and `return` work. " +
	"No Node, file system, network, or timers.\n" +
	"- `await tools.<name>({ ...args })` resolves to a string, or an object if the tool's declaration says so, " +
	"and rejects with an Error on failure. A tool whose declaration returns an object resolves on failure too when " +
	"it returned that object; check its fields. Calls still running when the script ends are cancelled.\n" +
	"- Optional first line: `// @options: {\"max_output_tokens\": 10000, \"timeout_ms\": 60000}`"

const globalsHelp = "Globals:\n" +
	"- `text(value)`, `image(dataUrlOrImageBlock)`, `console.log(...)`, and top-level `return` add output; " +
	"`exit()` ends the script. With several text items, each starts with a `==> text N/M <==` line, and `console` " +
	"lines follow the other output in one `<console_output>` block.\n" +
	"- `store(key, value)` and `load(key)` keep JSON values across codemode calls.\n" +
	"- `ALL_TOOLS`, `await searchTools(query, { limit?, namespace? })`, `await describeTool(name)`, " +
	"`await describeNamespace(name)`: find unlisted tools, such as MCP tools."

// renderSchemaType is a compact TypeScript rendering of a JSON schema: enough to
// show the model the argument shape without porting Pi's full declaration
// renderer.
func renderSchemaType(schema any) string {
	obj, ok := schema.(map[string]any)
	if !ok {
		return "unknown"
	}
	if enum, ok := obj["enum"].([]any); ok && len(enum) > 0 {
		parts := make([]string, 0, len(enum))
		for _, value := range enum {
			if s, ok := value.(string); ok {
				parts = append(parts, fmt.Sprintf("%q", s))
			} else {
				parts = append(parts, fmt.Sprint(value))
			}
		}
		return strings.Join(parts, " | ")
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		if variants, ok := obj[key].([]any); ok && len(variants) > 0 {
			parts := make([]string, 0, len(variants))
			for _, variant := range variants {
				parts = append(parts, renderSchemaType(variant))
			}
			return strings.Join(parts, " | ")
		}
	}
	switch obj["type"] {
	case "string":
		return "string"
	case "number", "integer":
		return "number"
	case "boolean":
		return "boolean"
	case "null":
		return "null"
	case "array":
		return renderSchemaType(obj["items"]) + "[]"
	case "object":
		props, _ := obj["properties"].(map[string]any)
		required := map[string]bool{}
		if names, ok := obj["required"].([]any); ok {
			for _, name := range names {
				if s, ok := name.(string); ok {
					required[s] = true
				}
			}
		}
		keys := make([]string, 0, len(props))
		for key := range props {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var fields []string
		for _, key := range keys {
			optional := ""
			if !required[key] {
				optional = "?"
			}
			fields = append(fields, key+optional+": "+renderSchemaType(props[key]))
		}
		if len(fields) == 0 {
			return "Record<string, unknown>"
		}
		return "{ " + strings.Join(fields, "; ") + " }"
	}
	return "unknown"
}

// renderToolSection is `### \`id\`` followed by the tool's description and the
// call the script makes.
func renderToolSection(tool agent.AgentTool) string {
	id := toIdent(tool.Name)
	heading := "### `" + id + "`"
	if id != tool.Name {
		heading = "### `" + id + "` (`" + tool.Name + "`)"
	}

	var schema any
	if len(tool.Parameters) > 0 {
		_ = json.Unmarshal(tool.Parameters, &schema)
	}
	signature := fmt.Sprintf("declare const tools: { %s(args: %s): Promise<string>; };", id, renderSchemaType(schema))

	return heading + "\n" + strings.TrimSpace(tool.Description) +
		"\n\ncodemode tool declaration:\n```ts\n" + signature + "\n```"
}

type catalogEntry struct {
	name    string
	section string
	cost    int
}

// selectCatalog picks the sections that fit the budget: in each round every group
// (tools without a namespace first, then namespaces by name) places its cheapest
// remaining tool, and a group whose next tool does not fit drops out while the
// others continue. Every namespace is represented before any is complete.
func selectCatalog(groups [][]catalogEntry, budget int) map[string]bool {
	if budget < 0 {
		shown := map[string]bool{}
		for _, group := range groups {
			for _, entry := range group {
				shown[entry.name] = true
			}
		}
		return shown
	}
	queues := make([][]catalogEntry, len(groups))
	for i, group := range groups {
		queue := append([]catalogEntry(nil), group...)
		sort.SliceStable(queue, func(a, b int) bool { return queue[a].cost < queue[b].cost })
		queues[i] = queue
	}
	shown := map[string]bool{}
	remaining := budget
	var active []int
	for i, queue := range queues {
		if len(queue) > 0 {
			active = append(active, i)
		}
	}
	for len(active) > 0 {
		var next []int
		for _, index := range active {
			if len(queues[index]) == 0 {
				continue
			}
			entry := queues[index][0]
			if entry.cost > remaining {
				continue
			}
			remaining -= entry.cost
			shown[entry.name] = true
			queues[index] = queues[index][1:]
			if len(queues[index]) > 0 {
				next = append(next, index)
			}
		}
		if len(next) == len(active) {
			// Nothing dropped out and nothing fits: stop rather than spin.
			break
		}
		active = next
	}
	return shown
}

// CreateDescription builds the codemode tool description: the helper list,
// guidance for finding unlisted tools, and one section per listed tool grouped by
// namespace. Tools that are already declared to the model are not repeated.
func CreateDescription(tools []agent.AgentTool, namespaces []Namespace, inlineBudget int) string {
	if inlineBudget == 0 {
		inlineBudget = DefaultInlineBudget
	}
	nsOf := map[string]Namespace{}
	for _, ns := range namespaces {
		for _, name := range ns.Tools {
			nsOf[name] = ns
		}
	}

	type group struct {
		ns      *Namespace
		entries []catalogEntry
	}
	plain := group{}
	byNamespace := map[string]*group{}
	var order []string
	for _, tool := range tools {
		if tool.Name == "codemode" {
			continue
		}
		section := renderToolSection(tool)
		entry := catalogEntry{name: tool.Name, section: section, cost: ceilDiv(len(section), charsPerToken)}
		ns, ok := nsOf[tool.Name]
		if !ok {
			plain.entries = append(plain.entries, entry)
			continue
		}
		g, ok := byNamespace[ns.Name]
		if !ok {
			nsCopy := ns
			g = &group{ns: &nsCopy}
			byNamespace[ns.Name] = g
			order = append(order, ns.Name)
		}
		g.entries = append(g.entries, entry)
	}
	sort.Strings(order)

	groups := [][]catalogEntry{}
	if len(plain.entries) > 0 {
		groups = append(groups, plain.entries)
	}
	for _, name := range order {
		groups = append(groups, byNamespace[name].entries)
	}
	shown := selectCatalog(groups, inlineBudget)

	sections := []string{descriptionIntro, globalsHelp}
	if len(groups) == 0 {
		return strings.Join(sections, "\n\n")
	}

	toolSections := []string{"Nested tools:"}
	if len(plain.entries) > 0 {
		for _, entry := range plain.entries {
			if shown[entry.name] {
				toolSections = append(toolSections, entry.section)
			}
		}
	}
	for _, name := range order {
		g := byNamespace[name]
		var visible []catalogEntry
		for _, entry := range g.entries {
			if shown[entry.name] {
				visible = append(visible, entry)
			}
		}
		listing := ""
		switch {
		case len(visible) == len(g.entries):
		case len(visible) == 0:
			listing = " (tools not listed)"
		default:
			listing = " (some tools not listed)"
		}
		description := strings.TrimSpace(g.ns.Description)
		heading := "## " + g.ns.Name + listing
		if description != "" {
			heading += "\n" + description
		}
		toolSections = append(toolSections, heading)
		for _, entry := range visible {
			toolSections = append(toolSections, entry.section)
		}
	}
	sections = append(sections, strings.Join(toolSections, "\n\n"))
	return strings.Join(sections, "\n\n")
}
