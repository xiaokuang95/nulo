package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

var mcpNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var mcpFormHandled = jsonSet("command", "args", "url", "description", "env", "headers", "enabled", "type")

func (a *app) mcpPath(s *session, target string) string {
	if target == "project" {
		return filepath.Join(s.CWD, ".nulo", "mcp.json")
	}
	return filepath.Join(a.host.agentDir, "mcp.json")
}

func mcpDefinition(entry map[string]any) bool {
	if entry == nil {
		return false
	}
	for _, key := range []string{"command", "url", "type"} {
		if _, ok := entry[key]; ok {
			return true
		}
	}
	return false
}

func mcpServersFromFile(path string) (map[string]any, map[string]any, []byte, bool, error) {
	root, data, existed, err := readJSONObject(path)
	if err != nil {
		return nil, nil, nil, existed, err
	}
	servers := jsonObjectField(root, "mcpServers")
	if servers == nil {
		servers = map[string]any{}
	}
	return root, servers, data, existed, nil
}

func (a *app) mcpEntryFor(s *session, name string) (string, map[string]any, bool) {
	if a == nil || a.host == nil || s == nil {
		return "global", nil, false
	}
	_, globalServers, _, _, globalErr := mcpServersFromFile(a.mcpPath(s, "global"))
	_, projectServers, _, _, projectErr := mcpServersFromFile(a.mcpPath(s, "project"))
	if globalErr != nil || projectErr != nil {
		return "global", nil, false
	}
	if entry, ok := projectServers[name].(map[string]any); ok && mcpDefinition(entry) {
		return "project", entry, true
	}
	if entry, ok := globalServers[name].(map[string]any); ok {
		return "global", entry, true
	}
	if entry, ok := projectServers[name].(map[string]any); ok {
		return "project", entry, true
	}
	return "global", nil, false
}

func (a *app) mcpNameUsed(s *session, name string) bool {
	if a == nil || a.host == nil || s == nil {
		return false
	}
	for _, target := range []string{"global", "project"} {
		_, servers, _, _, err := mcpServersFromFile(a.mcpPath(s, target))
		if err != nil {
			continue
		}
		if _, ok := servers[name]; ok {
			return true
		}
	}
	return false
}

func (a *app) openMCPForm(s *session, name string) {
	if a == nil || a.host == nil || s == nil {
		return
	}
	a.mcpFormAdd = name == ""
	a.mcpFormOriginal = name
	a.mcpFormName = name
	a.mcpFormDescription = ""
	a.mcpFormCommand = ""
	a.mcpFormArgs = ""
	a.mcpFormURL = ""
	a.mcpFormEnv = ""
	a.mcpFormHeaders = ""
	a.mcpFormAdvanced = "{}"
	a.mcpFormEnabled = true
	a.mcpFormError = ""
	a.mcpFormSessionID = s.ID
	a.mcpFormTarget = "global"
	if a.host.sessionTrusted(s) {
		a.mcpFormTarget = "project"
	}
	if name != "" {
		target, entry, ok := a.mcpEntryFor(s, name)
		if !ok {
			a.pendingToast = "MCP server not found in mcp.json"
			return
		}
		a.mcpFormTarget = target
		a.mcpFormDescription = jsonStringField(entry, "description")
		a.mcpFormCommand = jsonStringField(entry, "command")
		a.mcpFormArgs = strings.Join(jsonStringList(entry["args"]), " ")
		a.mcpFormURL = jsonStringField(entry, "url")
		a.mcpFormEnabled = jsonBoolField(entry, "enabled", true)
		a.mcpFormEnv = formatKeyValueMap(entry["env"], "=")
		a.mcpFormHeaders = formatKeyValueMap(entry["headers"], ": ")
		a.mcpFormAdvanced = advancedJSON(entry, mcpFormHandled)
	}
	a.mcpFormOpen = true
}

func formatKeyValueMap(value any, separator string) string {
	values, _ := value.(map[string]any)
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		text := ""
		switch typed := values[key].(type) {
		case string:
			text = typed
		case nil:
			text = "null"
		default:
			text = fmt.Sprint(typed)
		}
		lines = append(lines, key+strings.TrimSpace(separator)+text)
	}
	return strings.Join(lines, "\n")
}

func (a *app) mcpFormSession() *session {
	for _, s := range a.sessions {
		if s.ID == a.mcpFormSessionID {
			return s
		}
	}
	return nil
}

func (a *app) mcpFormDialog(c *ui.Context) {
	p := a.pal()
	s := a.mcpFormSession()
	if s == nil {
		a.mcpFormOpen = false
		return
	}
	ui.Modal(c, &a.mcpFormOpen, func() {
		title := "Add MCP server"
		if !a.mcpFormAdd {
			title = "Edit MCP server"
		}
		ui.Text(c, title).FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, "Command (stdio) or URL (streamable HTTP). Settings are stored in mcp.json.").
			FontSize(11).TextColor(p.text4)
		ui.Scroll(c).FillWidth().MaxHeight(520).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				a.formFieldLabel(c, "Name", "letters, digits, _ and -")
				if a.mcpFormAdd {
					a.formInput(c, &a.mcpFormName, "Name", "filesystem")
				} else {
					ui.Text(c, a.mcpFormName).Font("monospace").FontSize(12).TextColor(p.text2)
					ui.Text(c, "Stored in "+a.mcpFormTarget+" mcp.json").Font("monospace").FontSize(9).TextColor(p.text4)
				}
				a.formFieldLabel(c, "Description", "shown in Capabilities")
				a.formInput(c, &a.mcpFormDescription, "Description", "Project files and local tools")
				a.formFieldLabel(c, "Command", "stdio server command")
				a.formInput(c, &a.mcpFormCommand, "Command", "npx")
				a.formFieldLabel(c, "Arguments", "space separated")
				a.formInput(c, &a.mcpFormArgs, "Arguments", "-y @modelcontextprotocol/server-filesystem /tmp")
				a.formFieldLabel(c, "URL", "streamable HTTP endpoint; leave empty for stdio")
				a.formInput(c, &a.mcpFormURL, "URL", "https://example.com/mcp")
				a.formFieldLabel(c, "Environment", "one KEY=VALUE per line")
				a.formTextArea(c, &a.mcpFormEnv, "Environment", 2)
				a.formFieldLabel(c, "Headers", "one Key: Value per line")
				a.formTextArea(c, &a.mcpFormHeaders, "Headers", 2)
				a.formFieldLabel(c, "Advanced JSON", "extra mcp.json keys: exposure, timeout, cwd, toolExposure")
				a.formTextArea(c, &a.mcpFormAdvanced, "Advanced JSON", 5)
				row := ui.Row(c).Gap(8).AlignItems(ui.Center)
				row.Children(func() {
					if a.checkbox(c, a.mcpFormEnabled, "MCP server enabled").Clicked() {
						a.mcpFormEnabled = !a.mcpFormEnabled
					}
					ui.Text(c, "Enabled").FontSize(12).TextColor(p.text3)
				})
				if a.mcpFormAdd {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "Save to").FontSize(11).TextColor(p.text4)
						if a.smallButton(c, chooser(a.mcpFormTarget == "global", "● Global", "Global")).Clicked() {
							a.mcpFormTarget = "global"
						}
						if a.smallButton(c, chooser(a.mcpFormTarget == "project", "● Project", "Project")).
							Disabled(!a.host.sessionTrusted(s)).Clicked() && a.host.sessionTrusted(s) {
							a.mcpFormTarget = "project"
						}
					})
				}
				if a.mcpFormError != "" {
					ui.Text(c, a.mcpFormError).FontSize(11).TextColor(p.bad)
				}
			})
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if !a.mcpFormAdd {
				if a.dangerButton(c, "Delete").Clicked() {
					name := a.mcpFormOriginal
					a.mcpFormOpen = false
					a.confirmDestructive("Delete MCP server?", "Remove "+name+" from global and project mcp.json. This cannot be undone.", "Delete server", func() {
						if err := a.deleteMCPServer(s, name); err != nil {
							a.pendingToast = "MCP: " + err.Error()
						}
					})
				}
			}
			ui.Spacer(c)
			if a.button(c, "Cancel").Clicked() {
				a.mcpFormOpen = false
			}
			if a.primaryButton(c, "Save server", "check").Clicked() {
				a.finishMCPForm()
			}
		})
	})
}

func (a *app) formFieldLabel(c *ui.Context, title, hint string) {
	p := a.pal()
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(11).FontWeight(500).TextColor(p.text3)
		ui.Text(c, hint).Font("monospace").FontSize(9).TextColor(p.text4).SingleLine()
	})
}

func (a *app) formInput(c *ui.Context, value *string, label, placeholder string) {
	p := a.pal()
	ui.TextInputBase(c, value).FillWidth().Height(32).Padding(0, 9).Radius(6).
		Border(1, p.lineStrong).Background(p.surface2).FontSize(12).TextColor(p.text).
		Placeholder(placeholder).Label(label)
}

func (a *app) formTextArea(c *ui.Context, value *string, label string, lines int) {
	p := a.pal()
	ui.TextAreaBase(c, value).FillWidth().Height(float32(lines)*20+12).Padding(6, 9).Radius(6).
		Border(1, p.lineStrong).Background(p.surface2).FontSize(11).TextColor(p.text).
		Label(label)
}

func (a *app) finishMCPForm() {
	s := a.mcpFormSession()
	if s == nil {
		a.mcpFormError = "session is no longer available"
		return
	}
	name := strings.TrimSpace(a.mcpFormName)
	if !mcpNamePattern.MatchString(name) {
		a.mcpFormError = "name must use letters, digits, _ and -"
		return
	}
	if a.mcpFormTarget == "project" && !a.host.sessionTrusted(s) {
		a.mcpFormError = "trust this project before saving project MCP config"
		return
	}
	command := strings.TrimSpace(a.mcpFormCommand)
	url := strings.TrimSpace(a.mcpFormURL)
	if (command == "") == (url == "") {
		a.mcpFormError = "set either command or URL"
		return
	}
	env, err := parseKeyValueLines(a.mcpFormEnv, "=")
	if err != nil {
		a.mcpFormError = "Environment: " + err.Error()
		return
	}
	headers, err := parseKeyValueLines(a.mcpFormHeaders, ":")
	if err != nil {
		a.mcpFormError = "Headers: " + err.Error()
		return
	}
	if a.mcpFormAdd && a.mcpNameUsed(s, name) {
		a.mcpFormError = "an MCP server with this name already exists; edit it instead"
		return
	}
	if err := a.host.validateConfigReload(a); err != nil {
		a.mcpFormError = err.Error()
		return
	}
	path := a.mcpPath(s, a.mcpFormTarget)
	root, servers, previous, existed, err := mcpServersFromFile(path)
	if err != nil {
		a.mcpFormError = err.Error()
		return
	}
	entry, _ := servers[name].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	if err := applyAdvancedJSON(entry, mcpFormHandled, a.mcpFormAdvanced); err != nil {
		a.mcpFormError = "Advanced JSON: " + err.Error()
		return
	}
	if url != "" {
		entry["url"] = url
		delete(entry, "command")
		delete(entry, "args")
		if typ := jsonStringField(entry, "type"); typ == "" || typ == "stdio" {
			delete(entry, "type")
		}
	} else {
		entry["command"] = command
		if args := strings.Fields(a.mcpFormArgs); len(args) > 0 {
			entry["args"] = args
		} else {
			delete(entry, "args")
		}
		delete(entry, "url")
		if typ := jsonStringField(entry, "type"); typ == "" || typ == "stdio" || typ == "http" || typ == "streamable-http" {
			delete(entry, "type")
		}
	}
	if description := strings.TrimSpace(a.mcpFormDescription); description != "" {
		entry["description"] = description
	} else {
		delete(entry, "description")
	}
	if len(env) > 0 {
		entry["env"] = env
	} else {
		delete(entry, "env")
	}
	if len(headers) > 0 {
		entry["headers"] = headers
	} else {
		delete(entry, "headers")
	}
	entry["enabled"] = a.mcpFormEnabled
	servers[name] = entry
	root["mcpServers"] = servers
	if err := writeJSONObject(path, root); err != nil {
		a.mcpFormError = err.Error()
		return
	}
	if err := a.reloadConfig("MCP configuration reloaded"); err != nil {
		restoreConfigFile(path, previous, existed)
		a.mcpFormError = "reload failed: " + err.Error()
		return
	}
	a.mcpFormOpen = false
	a.refreshCapabilities(s)
	if a.mcpFormAdd {
		a.pendingToast = "Added MCP server · " + name
	} else {
		a.pendingToast = "Updated MCP server · " + name
	}
}

func restoreConfigFile(path string, previous []byte, existed bool) {
	if existed {
		_ = writeConfigFile(path, previous)
		return
	}
	_ = os.Remove(path)
}

func (a *app) deleteMCPServer(s *session, name string) error {
	if a == nil || a.host == nil || s == nil || !mcpNamePattern.MatchString(name) {
		return errors.New("invalid MCP server")
	}
	if err := a.host.validateConfigReload(a); err != nil {
		return err
	}
	type snapshot struct {
		path     string
		previous []byte
		existed  bool
	}
	var snapshots []snapshot
	for _, target := range []string{"global", "project"} {
		path := a.mcpPath(s, target)
		root, servers, previous, existed, err := mcpServersFromFile(path)
		if err != nil {
			return err
		}
		if _, ok := servers[name]; !ok {
			continue
		}
		delete(servers, name)
		root["mcpServers"] = servers
		if err := writeJSONObject(path, root); err != nil {
			return err
		}
		snapshots = append(snapshots, snapshot{path: path, previous: previous, existed: existed})
	}
	if len(snapshots) == 0 {
		return errors.New("MCP server not found")
	}
	if err := a.reloadConfig("MCP configuration reloaded"); err != nil {
		for _, snap := range snapshots {
			restoreConfigFile(snap.path, snap.previous, snap.existed)
		}
		return err
	}
	a.refreshCapabilities(s)
	a.pendingToast = "Deleted MCP server · " + name
	return nil
}
