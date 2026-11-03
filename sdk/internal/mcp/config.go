package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// Exposure decides how a server's tools reach the model, matching Pi's mcp.json
// "exposure" field. Pi's default is codemode: the tools stay out of the model's
// tool list so a large server does not cost every request, and codemode scripts
// reach them through tools.<name>, searchTools() or ALL_TOOLS.
type Exposure string

const (
	// ExposureCodemode is Pi's default.
	ExposureCodemode Exposure = "codemode"
	// ExposureDeferred declares the tools only after tool_search loads them.
	ExposureDeferred Exposure = "deferred"
	// ExposureDirect declares the tools to the model like built-in tools, and
	// keeps them callable from codemode as well.
	ExposureDirect Exposure = "direct"
	// ExposureHidden exposes the server's tools nowhere.
	ExposureHidden Exposure = "hidden"
)

// exposureAliases are Pi's older names, replaced by their current name on load.
var exposureAliases = map[string]Exposure{"codemode-deferred": ExposureCodemode}

var exposureNames = []string{"codemode", "deferred", "direct", "hidden"}

func parseExposure(raw string) (Exposure, bool) {
	if alias, ok := exposureAliases[raw]; ok {
		raw = string(alias)
	}
	for _, name := range exposureNames {
		if raw == name {
			return Exposure(raw), true
		}
	}
	return "", false
}

// Searchable reports whether tool_search may load this tool: both codemode and
// deferred tools are hidden from the model until something reaches them.
func (e Exposure) Searchable() bool { return e == ExposureCodemode || e == ExposureDeferred }

// Indirect reports whether the tools reach the model only through codemode or
// tool_search, which is what the mcp_servers prompt section describes.
func (e Exposure) Indirect() bool { return e.Searchable() }

// serverNameRe is Pi's server name rule.
var serverNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidServerName reports whether name obeys Pi's server-name rule.
func ValidServerName(name string) bool { return serverNameRe.MatchString(name) }

// ServerConfig is one validated entry of the `mcpServers` object.
type ServerConfig struct {
	Command      string            `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	CWD          string            `json:"cwd,omitempty"`
	URL          string            `json:"url,omitempty"`
	Type         string            `json:"type,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Enabled      *bool             `json:"enabled,omitempty"`
	Timeout      float64           `json:"timeout,omitempty"`
	Exposure     string            `json:"exposure,omitempty"`
	ToolExposure map[string]string `json:"toolExposure,omitempty"`
	Description  string            `json:"description,omitempty"`
}

func (c ServerConfig) on() bool { return c.Enabled == nil || *c.Enabled }

// stillExposed reports whether the server has any tool the model can reach, which
// is Pi's test for including a server in the mcp_servers prompt section.
func (c ServerConfig) stillExposed() bool { return true }

// defaultExposure resolves the server-level field, defaulting to codemode.
func (c ServerConfig) defaultExposure() Exposure {
	if c.Exposure == "" {
		return ExposureCodemode
	}
	e, _ := parseExposure(c.Exposure)
	return e
}

// ExposureFor resolves one tool's exposure: an exact toolExposure entry, then a
// wildcard entry, then the server default.
//
// Pi walks toolExposure in JSON declaration order, so overlapping patterns are
// resolved by whichever was written first. Go maps lose that order; nulo sorts
// the patterns instead, which is deterministic and only differs when a tool
// matches two wildcards at once.
func (c ServerConfig) ExposureFor(tool string) Exposure {
	if c.ToolExposure == nil {
		return c.defaultExposure()
	}
	if raw, ok := c.ToolExposure[tool]; ok {
		if e, ok := parseExposure(raw); ok {
			return e
		}
	}
	patterns := make([]string, 0, len(c.ToolExposure))
	for pattern := range c.ToolExposure {
		if strings.Contains(pattern, "*") {
			patterns = append(patterns, pattern)
		}
	}
	sort.Strings(patterns)
	for _, pattern := range patterns {
		if matchToolPattern(pattern, tool) {
			if e, ok := parseExposure(c.ToolExposure[pattern]); ok {
				return e
			}
		}
	}
	return c.defaultExposure()
}

// matchToolPattern implements Pi's toolPatternRegExp: `*` matches any run of
// characters, everything else is literal.
func matchToolPattern(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	var b strings.Builder
	b.WriteString("^")
	for i, part := range parts {
		if i > 0 {
			b.WriteString(".*")
		}
		b.WriteString(regexp.QuoteMeta(part))
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	return err == nil && re.MatchString(name)
}

// Config is a merged mcp.json. Errors collects every problem Pi would report at
// startup: a bad entry is skipped, the rest of the file still loads.
type Config struct {
	Servers            map[string]ServerConfig
	AutoEnableCodemode bool
	Errors             []string
}

// Load merges the user file with the project file, which needs project trust.
// A project entry with no command, url, or type is an override of the same-named
// user server and may only set enabled, exposure, and toolExposure.
func Load(agentDir, cwd string, projectTrusted bool) Config {
	cfg := Config{
		Servers:            map[string]ServerConfig{},
		AutoEnableCodemode: true,
	}
	if agentDir != "" {
		cfg.readFile(filepath.Join(agentDir, "mcp.json"), "global")
	}
	if projectTrusted && cwd != "" {
		cfg.readFile(filepath.Join(cwd, ".nulo", "mcp.json"), "project")
	}
	return cfg
}

// overrideKeys are the only fields a project entry may set when it overrides a
// globally defined server.
var overrideKeys = map[string]bool{"enabled": true, "exposure": true, "toolExposure": true}

func (c *Config) readFile(path, scope string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			c.Errors = append(c.Errors, fmt.Sprintf("%s: %v", path, err))
		}
		return
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")) // strip a UTF-8 BOM

	var parsed struct {
		MCPServers         json.RawMessage `json:"mcpServers"`
		AutoEnableCodemode *bool           `json:"autoEnableCodemode"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		c.Errors = append(c.Errors, fmt.Sprintf("%s: %v", path, err))
		return
	}
	if parsed.AutoEnableCodemode != nil {
		c.AutoEnableCodemode = *parsed.AutoEnableCodemode
	}

	var fields map[string]json.RawMessage
	if len(parsed.MCPServers) == 0 {
		return
	}
	if err := json.Unmarshal(parsed.MCPServers, &fields); err != nil {
		c.Errors = append(c.Errors, fmt.Sprintf("%s: expected an object with an \"mcpServers\" object", path))
		return
	}

	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		value := fields[name]
		var keys map[string]json.RawMessage
		if json.Unmarshal(value, &keys) == nil && scope == "project" && isOverrideEntry(keys) {
			c.applyOverride(path, name, keys)
			continue
		}
		server, msg := validateServerConfig(name, value)
		if msg != "" {
			c.Errors = append(c.Errors, fmt.Sprintf("%s: %s", path, msg))
			continue
		}
		if clash := c.namespaceClash(name); clash != "" {
			c.Errors = append(c.Errors, fmt.Sprintf("%s: server %q conflicts with %q", path, name, clash))
			continue
		}
		c.Servers[name] = server
	}
}

// isOverrideEntry reports whether a project entry only tunes an existing server,
// which is Pi's test: no command, no url, no type.
func isOverrideEntry(keys map[string]json.RawMessage) bool {
	_, hasCommand := keys["command"]
	_, hasURL := keys["url"]
	_, hasType := keys["type"]
	return !hasCommand && !hasURL && !hasType
}

func (c *Config) applyOverride(path, name string, keys map[string]json.RawMessage) {
	base, ok := c.Servers[name]
	if !ok {
		c.Errors = append(c.Errors, fmt.Sprintf("%s: server %q needs \"command\" or \"url\", or a global server to override", path, name))
		return
	}
	for key := range keys {
		if !overrideKeys[key] {
			c.Errors = append(c.Errors, fmt.Sprintf("%s: server %q: an override can only set enabled, exposure, toolExposure", path, name))
			return
		}
	}
	// Merge the override over the global entry, the way Pi spreads the two.
	merged := base
	if raw, ok := keys["enabled"]; ok {
		var enabled bool
		if err := json.Unmarshal(raw, &enabled); err != nil {
			c.Errors = append(c.Errors, fmt.Sprintf("%s: server %q: enabled must be a boolean", path, name))
			return
		}
		merged.Enabled = &enabled
	}
	if raw, ok := keys["exposure"]; ok {
		var exposure string
		if err := json.Unmarshal(raw, &exposure); err != nil {
			c.Errors = append(c.Errors, fmt.Sprintf("%s: server %q: exposure must be one of %q", path, name, exposureNames))
			return
		}
		merged.Exposure = exposure
	}
	if raw, ok := keys["toolExposure"]; ok {
		var toolExposure map[string]string
		if err := json.Unmarshal(raw, &toolExposure); err != nil {
			c.Errors = append(c.Errors, fmt.Sprintf("%s: server %q: toolExposure must map tool names to exposures", path, name))
			return
		}
		merged.ToolExposure = toolExposure
	}
	if _, msg := validateServerConfig(name, mustJSON(merged)); msg != "" {
		c.Errors = append(c.Errors, fmt.Sprintf("%s: %s", path, msg))
		return
	}
	c.Servers[name] = merged
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (c *Config) namespaceClash(name string) string {
	ns := mcpNamespace(name)
	others := make([]string, 0, len(c.Servers))
	for other := range c.Servers {
		others = append(others, other)
	}
	sort.Strings(others)
	for _, other := range others {
		if other != name && mcpNamespace(other) == ns {
			return other
		}
	}
	return ""
}

var urlRe = regexp.MustCompile(`^https?://`)

// validateServerConfig mirrors Pi's validateMcpServerConfig, returning a copy
// with exposure aliases resolved or a human-readable problem.
func validateServerConfig(name string, raw json.RawMessage) (ServerConfig, string) {
	if !serverNameRe.MatchString(name) {
		return ServerConfig{}, fmt.Sprintf("invalid server name %q (use letters, digits, \"_\" and \"-\")", name)
	}
	if len(raw) == 0 || raw[0] != '{' {
		return ServerConfig{}, fmt.Sprintf("server %q must be an object", name)
	}

	var cfg ServerConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields() // no: Pi ignores unknown fields
	_ = dec
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ServerConfig{}, fmt.Sprintf("server %q: %v", name, err)
	}

	// Resolve aliases first, exactly like Pi, so validation sees the real name.
	if cfg.Exposure != "" {
		if e, ok := parseExposure(cfg.Exposure); ok {
			cfg.Exposure = string(e)
		} else {
			return ServerConfig{}, fmt.Sprintf("server %q: exposure must be one of %s", name, quotedExposures())
		}
	}
	for tool, exposure := range cfg.ToolExposure {
		if e, ok := parseExposure(exposure); ok {
			cfg.ToolExposure[tool] = string(e)
		} else {
			return ServerConfig{}, fmt.Sprintf("server %q: toolExposure %q must be one of %s", name, tool, quotedExposures())
		}
	}
	if cfg.Timeout < 0 {
		return ServerConfig{}, fmt.Sprintf("server %q: timeout must be a positive number of seconds", name)
	}
	switch cfg.Type {
	case "sse":
		return ServerConfig{}, fmt.Sprintf("server %q: legacy SSE transport is not supported; use the streamable HTTP URL", name)
	}

	switch {
	case cfg.URL != "":
		if cfg.Type != "" && cfg.Type != "http" && cfg.Type != "streamable-http" {
			return ServerConfig{}, fmt.Sprintf("server %q: url requires type \"http\" or \"streamable-http\"", name)
		}
		if !urlRe.MatchString(cfg.URL) {
			return ServerConfig{}, fmt.Sprintf("server %q: url must be an http or https URL", name)
		}
		return cfg, ""
	case cfg.Command != "":
		if cfg.Type != "" && cfg.Type != "stdio" {
			return ServerConfig{}, fmt.Sprintf("server %q: command requires type \"stdio\"", name)
		}
		return cfg, ""
	default:
		return ServerConfig{}, fmt.Sprintf("server %q needs either \"command\" (stdio) or \"url\" (streamable HTTP)", name)
	}
}

func quotedExposures() string {
	parts := make([]string, len(exposureNames))
	for i, name := range exposureNames {
		parts[i] = `"` + name + `"`
	}
	return strings.Join(parts, ", ")
}

// ExposedTool is one server tool together with the exposure resolved for it.
type ExposedTool struct {
	Tool     agent.AgentTool
	Exposure Exposure
}

// Connected is one server nulo reached, with everything the session needs to
// decide where its tools go.
type Connected struct {
	Name         string
	Description  string // configured description, if any
	Instructions string // the server's own initialize instructions, if any
	Exposure     Exposure
	// Exposures is every exposure the configuration can produce: the server
	// default plus every toolExposure value, which is what Pi calls
	// configuredExposures and uses to label the prompt section.
	Exposures map[Exposure]bool
	Client    *Client
	Tools     []ExposedTool
}

func (c Connected) hasExposure(e Exposure) bool {
	for _, t := range c.Tools {
		if t.Exposure == e {
			return true
		}
	}
	return false
}

// Namespace is the identifier scripts and the prompt use for one server's tools.
func Namespace(server string) string { return mcpNamespace(server) }

func mcpNamespace(server string) string {
	return "mcp__" + strings.ReplaceAll(server, "-", "_")
}

// ConnectAll reaches every enabled server and returns them sorted by name. A
// server that fails to connect is skipped.
func ConnectAll(ctx context.Context, cfgs map[string]ServerConfig) []Connected {
	names := make([]string, 0, len(cfgs))
	for name := range cfgs {
		names = append(names, name)
	}
	sort.Strings(names)

	// Tool names are sanitized, so two servers can collapse onto one name; the
	// set makes the later one pick up a hash suffix instead of colliding.
	taken := map[string]bool{}

	out := make([]Connected, 0, len(names))
	for _, name := range names {
		cfg := cfgs[name]
		if !cfg.on() {
			continue
		}
		cl, mcpTools, err := connect(ctx, name, cfg, taken)
		if err != nil {
			continue
		}
		tools := make([]ExposedTool, 0, len(mcpTools))
		for _, t := range mcpTools {
			t := t
			params := t.InputSchema
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object"}`)
			}
			tool := agent.AgentTool{
				Tool:  ai.Tool{Name: t.Name, Description: t.Description, Parameters: params},
				Label: t.Name,
				Execute: func(execCtx context.Context, _ string, args map[string]any) (agent.ToolResult, error) {
					blocks, isErr, err := t.call(execCtx, args)
					if err != nil {
						return agent.ToolResult{Content: []ai.ContentBlock{ai.TextBlock(err.Error())}, IsError: true}, nil
					}
					return agent.ToolResult{Content: blocks, IsError: isErr}, nil
				},
			}
			tools = append(tools, ExposedTool{
				Tool:     tool,
				Exposure: cfg.ExposureFor(t.OriginalName),
			})
		}
		exposures := map[Exposure]bool{cfg.defaultExposure(): true}
		for _, raw := range cfg.ToolExposure {
			if e, ok := parseExposure(raw); ok {
				exposures[e] = true
			}
		}
		out = append(out, Connected{
			Name:         name,
			Description:  cfg.Description,
			Instructions: cl.Instructions(),
			Exposure:     cfg.defaultExposure(),
			Exposures:    exposures,
			Client:       cl,
			Tools:        tools,
		})
	}
	return out
}

// timeoutDuration is the per-request timeout configured for a server, if any.
func (c ServerConfig) timeoutDuration() time.Duration {
	if c.Timeout <= 0 {
		return 0
	}
	return time.Duration(c.Timeout * float64(time.Second))
}
