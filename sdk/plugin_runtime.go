package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/mcp"
	"github.com/xiaokuang95/nulo/sdk/internal/plugin"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
)

// ActiveToolNames returns the names declared to the model.
func (s *Session) ActiveToolNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.tools))
	for _, tool := range s.tools {
		out = append(out, tool.Name)
	}
	return out
}

func (s *Session) toolInfos() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.catalog))
	for _, tool := range s.catalog {
		exposure := "direct"
		source := "builtin"
		if binding, ok := s.pluginToolByName[tool.Name]; ok {
			exposure = binding.spec.ToolExposure()
			source = "plugin"
		} else if configured, ok := s.toolExposure[tool.Name]; ok {
			exposure = string(configured)
			source = "mcp"
		}
		info := map[string]any{
			"name":        tool.Name,
			"label":       tool.Label,
			"description": tool.Description,
			"parameters":  tool.Parameters,
			"exposure":    exposure,
			"source":      source,
		}
		if binding, ok := s.pluginToolByName[tool.Name]; ok {
			info["promptSnippet"] = binding.spec.PromptSnippet
			info["promptGuidelines"] = binding.spec.PromptGuidelines
		}
		out = append(out, info)
	}
	return out
}

func (s *Session) pluginToolExposure(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if binding, ok := s.pluginToolByName[name]; ok {
		return binding.spec.ToolExposure()
	}
	if exposure, ok := s.toolExposure[name]; ok {
		return string(exposure)
	}
	return "direct"
}

func (s *Session) toolSource(name string) string {
	if _, ok := s.pluginToolByName[name]; ok {
		return "plugin"
	}
	if _, ok := s.toolExposure[name]; ok {
		return "mcp"
	}
	return "builtin"
}

func (s *Session) commandInfos() []map[string]any {
	var out []map[string]any
	for _, cmd := range s.PluginCommands() {
		out = append(out, map[string]any{
			"name":         cmd.Name,
			"description":  cmd.Description,
			"argumentHint": cmd.ArgumentHint,
			"source":       "plugin",
		})
	}
	for _, skill := range s.skills {
		out = append(out, map[string]any{
			"name":        "skill:" + skill.Name,
			"description": skill.Description,
			"source":      "skill",
		})
	}
	return out
}

func (s *Session) contextUsage() map[string]any {
	model := s.Model()
	window := model.ContextWindow
	if window <= 0 {
		window = 128000
	}
	tokens := 0
	for _, message := range s.manager.Project().Messages {
		tokens += session.EstimateTokens(message)
	}
	var pct *float64
	var tokenPtr *int
	if tokens > 0 {
		value := float64(tokens) / float64(window) * 100
		pct = &value
		tokenPtr = &tokens
	}
	return map[string]any{"tokens": tokenPtr, "contextWindow": window, "percent": pct}
}

func (s *Session) pluginSendMessage(arg map[string]any) error {
	message, _ := arg["message"].(map[string]any)
	customType, _ := message["customType"].(string)
	content, _ := message["content"]
	display, _ := message["display"].(bool)
	details, _ := message["details"]
	rawContent, err := json.Marshal(content)
	if err != nil {
		return err
	}
	rawDetails, _ := json.Marshal(details)
	_, err = s.manager.AppendCustomMessage(customType, rawContent, display, rawDetails)
	if err != nil {
		return err
	}
	options, _ := arg["options"].(map[string]any)
	if trigger, _ := options["triggerTurn"].(bool); trigger {
		text := customContentText(content)
		if text != "" {
			return s.FollowUp(context.Background(), ai.UserMessage(text, time.Now().UnixMilli()))
		}
	}
	return nil
}

func customContentText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var parts []string
		for _, item := range value {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func (s *Session) pluginSendUserMessage(ctx context.Context, arg map[string]any) error {
	text := customContentText(arg["content"])
	if text == "" {
		return nil
	}
	s.mu.Lock()
	streaming := s.streaming
	s.mu.Unlock()
	options, _ := arg["options"].(map[string]any)
	deliver, _ := options["deliverAs"].(string)
	if streaming {
		message := ai.UserMessage(text, time.Now().UnixMilli())
		if deliver == "steer" {
			return s.Steer(ctx, message)
		}
		return s.FollowUp(ctx, message)
	}
	if deliver == "steer" || deliver == "followUp" {
		return s.FollowUp(ctx, ai.UserMessage(text, time.Now().UnixMilli()))
	}
	_, err := s.Send(ctx, text, PromptOptions{Source: "extension"})
	return err
}

func (s *AgentSessionServices) pluginExec(ctx context.Context, arg map[string]any) (ExecResult, error) {
	command := stringArg(arg, "command")
	args := stringSliceArg(arg, "args")
	if command == "" {
		return ExecResult{}, fmt.Errorf("exec: command is required")
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = s.CWD
	if options, ok := arg["options"].(map[string]any); ok {
		if cwd := stringArg(options, "cwd"); cwd != "" {
			cmd.Dir = cwd
		}
		if env, ok := options["env"].(map[string]any); ok {
			for key, value := range env {
				cmd.Env = append(cmd.Env, key+"="+fmt.Sprint(value))
			}
		}
		if stdin := stringArg(options, "stdin"); stdin != "" {
			cmd.Stdin = strings.NewReader(stdin)
		}
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *AgentSessionServices) registerPluginProvider(arg map[string]any) error {
	name := stringArg(arg, "name")
	if name == "" {
		return fmt.Errorf("registerProvider: name is required")
	}
	configRaw, err := json.Marshal(arg["config"])
	if err != nil {
		return err
	}
	var config ai.ProviderConfig
	if err := json.Unmarshal(configRaw, &config); err != nil {
		return fmt.Errorf("registerProvider %s: %w", name, err)
	}
	s.providersMu.Lock()
	s.dynamicProviders[name] = config
	s.providersMu.Unlock()
	return nil
}

func (s *AgentSessionServices) unregisterPluginProvider(name string) {
	s.providersMu.Lock()
	delete(s.dynamicProviders, name)
	s.providersMu.Unlock()
}

func (s *AgentSessionServices) providerSnapshot() ai.Providers {
	fileProviders, _ := ai.LoadProviders(s.AgentDir)
	out := ai.Providers{}
	for name, provider := range fileProviders {
		out[name] = provider
	}
	s.providersMu.Lock()
	for name, provider := range s.dynamicProviders {
		out[name] = provider
	}
	s.providersMu.Unlock()
	return out
}

func (s *AgentSessionServices) findModelByProvider(provider string) (ai.Model, error) {
	providers := s.providerSnapshot()
	config, ok := providers[provider]
	if !ok || len(config.Models) == 0 {
		return ai.Model{}, fmt.Errorf("model not found: %s", provider)
	}
	return providers.FindModel(provider + "/" + config.Models[0].ID)
}

func (s *AgentSessionServices) availableModels() []map[string]any {
	providers := s.providerSnapshot()
	var out []map[string]any
	for name, config := range providers {
		for _, entry := range config.Models {
			model, err := providers.FindModel(name + "/" + entry.ID)
			if err != nil {
				continue
			}
			out = append(out, modelJSON(model))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprint(out[i]["provider"], "/", out[i]["id"]) < fmt.Sprint(out[j]["provider"], "/", out[j]["id"])
	})
	return out
}

func (s *AgentSessionServices) registerPluginMCP(name string, raw any) error {
	if name == "" {
		return fmt.Errorf("registerMcpServer: name is required")
	}
	if !mcp.ValidServerName(name) {
		return fmt.Errorf("mcp server name %q is invalid", name)
	}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var config mcp.ServerConfig
	if err := json.Unmarshal(rawJSON, &config); err != nil {
		return fmt.Errorf("mcp server %s: %w", name, err)
	}
	s.pluginMu.Lock()
	if s.pluginMCP == nil {
		s.pluginMCP = map[string]mcp.ServerConfig{}
	}
	_, exists := s.pluginMCP[name]
	sess := s.pluginSession
	s.pluginMu.Unlock()
	if exists && sess != nil {
		_ = sess.unregisterPluginMCP(name)
	}
	s.pluginMu.Lock()
	s.pluginMCP[name] = config
	s.pluginMu.Unlock()
	if sess != nil {
		if err := sess.addPluginMCP(name, config); err != nil {
			return err
		}
		s.emitMcpServersChange()
	}
	return nil
}

func (s *AgentSessionServices) unregisterPluginMCPConfig(name string) {
	s.pluginMu.Lock()
	delete(s.pluginMCP, name)
	s.pluginMu.Unlock()
}

func (s *AgentSessionServices) pluginMCPList() []map[string]any {
	s.pluginMu.Lock()
	defer s.pluginMu.Unlock()
	var out []map[string]any
	for name, config := range s.pluginMCP {
		out = append(out, map[string]any{"name": name, "config": config})
	}
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]["name"]) < fmt.Sprint(out[j]["name"]) })
	return out
}

func (s *AgentSessionServices) registerVirtualModel(raw any) error {
	data, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("registerVirtualModel: model object is required")
	}
	provider, _ := data["provider"].(string)
	id, _ := data["id"].(string)
	if provider == "" || id == "" {
		return fmt.Errorf("registerVirtualModel: provider and id are required")
	}
	s.providersMu.Lock()
	s.virtualModels[provider+"/"+id] = data
	s.providersMu.Unlock()
	return nil
}

func (s *AgentSessionServices) unregisterVirtualModel(provider, id string) error {
	s.providersMu.Lock()
	delete(s.virtualModels, provider+"/"+id)
	s.providersMu.Unlock()
	return nil
}

func (s *AgentSessionServices) emitBusEvent(name string, data any) {
	s.busMu.Lock()
	subs := append([]func(any){}, s.busSubscribers[name]...)
	s.busMu.Unlock()
	for _, fn := range subs {
		fn(data)
	}
	if s.plugins == nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{"event": name, "data": data})
	for _, inst := range s.plugins.Instances() {
		_, _ = inst.Call(context.Background(), plugin.Invoke{Type: "host", Name: "events.deliver", Input: payload})
	}
}

// ExecResult is the result of a plugin-requested shell command.
type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

// OnPluginEvent subscribes the host to a plugin event-bus event.
func (s *AgentSessionServices) OnPluginEvent(name string, fn func(any)) func() {
	s.busMu.Lock()
	defer s.busMu.Unlock()
	s.busSubscribers[name] = append(s.busSubscribers[name], fn)
	index := len(s.busSubscribers[name]) - 1
	return func() {
		s.busMu.Lock()
		defer s.busMu.Unlock()
		if index < len(s.busSubscribers[name]) {
			s.busSubscribers[name][index] = nil
		}
	}
}

func (s *AgentSessionServices) emitMcpServersChange() {
	if sess := s.boundPluginSession(); sess != nil {
		sess.emitPlugin(context.Background(), "mcp_servers_change", map[string]any{
			"type":    "mcp_servers_change",
			"servers": s.pluginMCPList(),
		})
	}
}

// pluginToolBinding is one plugin tool in a session.
type pluginToolBinding struct {
	instance *plugin.Instance
	spec     plugin.ToolSpec
}

func (s *Session) bindPluginTools() {
	if s.services == nil || s.services.plugins == nil {
		return
	}
	for _, inst := range s.services.plugins.Instances() {
		for _, spec := range inst.Snapshot().Tools {
			if _, exists := s.pluginToolByName[spec.Name]; exists {
				s.services.addPluginDiagnostic(PluginDiagnostic{Path: inst.Path, Plugin: inst.Registration.Name, Message: "tool " + spec.Name + " is already registered"})
				continue
			}
			for _, existing := range s.catalog {
				if existing.Name == spec.Name {
					s.services.addPluginDiagnostic(PluginDiagnostic{Path: inst.Path, Plugin: inst.Registration.Name, Message: "tool " + spec.Name + " conflicts with an existing tool"})
					goto nextTool
				}
			}
			{
				binding := pluginToolBinding{instance: inst, spec: spec}
				s.pluginToolByName[spec.Name] = binding
				tool := s.pluginAgentTool(binding)
				if spec.ToolExposure() != "hidden" {
					s.catalog = append(s.catalog, tool)
					if spec.ToolActive() {
						s.tools = append(s.tools, tool)
					}
				}
			}
		nextTool:
		}
	}
}

func (s *Session) pluginAgentTool(binding pluginToolBinding) agent.AgentTool {
	spec := binding.spec
	parameters := spec.Parameters
	if len(parameters) == 0 {
		parameters = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return agent.AgentTool{
		Tool:          ai.Tool{Name: spec.Name, Description: spec.Description, Parameters: parameters},
		Label:         spec.Label,
		ExecutionMode: spec.ExecutionMode,
		Execute: func(ctx context.Context, toolCallID string, args map[string]any) (agent.ToolResult, error) {
			result, err := binding.instance.ExecuteTool(ctx, toolCallID, spec.Name, args, s.pluginContextData(""))
			if err != nil {
				return agent.ToolResult{}, err
			}
			return pluginAgentResult(result), nil
		},
	}
}

func pluginAgentResult(result plugin.ToolResult) agent.ToolResult {
	blocks := make([]ai.ContentBlock, 0, len(result.Content))
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			blocks = append(blocks, ai.TextBlock(block.Text))
		case "image":
			blocks = append(blocks, ai.ImageBlock(block.Data, block.MIMEType))
		default:
			raw, _ := json.Marshal(block)
			var converted ai.ContentBlock
			if json.Unmarshal(raw, &converted) == nil {
				blocks = append(blocks, converted)
			}
		}
	}
	return agent.ToolResult{Content: blocks, Details: result.Details, IsError: result.IsError, Terminate: result.Terminate}
}

func (s *Session) addPluginMCP(name string, config mcp.ServerConfig) error {
	for _, server := range s.servers {
		if server.Name == name {
			return fmt.Errorf("mcp server %q is already connected", name)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connected := mcp.ConnectAll(ctx, map[string]mcp.ServerConfig{name: config})
	if len(connected) == 0 {
		return fmt.Errorf("mcp server %q failed to connect", name)
	}
	for _, server := range connected {
		s.mu.Lock()
		s.mcp = append(s.mcp, server.Client)
		s.servers = append(s.servers, server)
		for _, entry := range server.Tools {
			s.toolExposure[entry.Tool.Name] = entry.Exposure
			s.toolNamespace[entry.Tool.Name] = mcp.Namespace(server.Name)
			switch entry.Exposure {
			case mcp.ExposureHidden:
			case mcp.ExposureDirect:
				s.tools = append(s.tools, entry.Tool)
				s.catalog = append(s.catalog, entry.Tool)
			case mcp.ExposureDeferred, mcp.ExposureCodemode:
				s.catalog = append(s.catalog, entry.Tool)
			}
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *Session) unregisterPluginMCP(name string) error {
	s.mu.Lock()
	var closed *mcp.Client
	var removed []string
	servers := s.servers[:0]
	for _, server := range s.servers {
		if server.Name == name {
			closed = server.Client
			for _, entry := range server.Tools {
				removed = append(removed, entry.Tool.Name)
			}
			continue
		}
		servers = append(servers, server)
	}
	s.servers = servers
	if closed != nil {
		s.mcp = removeMCPClient(s.mcp, closed)
	}
	if len(removed) > 0 {
		remove := map[string]bool{}
		for _, tool := range removed {
			remove[tool] = true
			delete(s.toolExposure, tool)
			delete(s.toolNamespace, tool)
		}
		s.tools = filterOutTools(s.tools, remove)
		s.catalog = filterOutTools(s.catalog, remove)
	}
	s.mu.Unlock()
	if closed != nil {
		closed.Close()
	}
	s.services.unregisterPluginMCPConfig(name)
	s.services.emitMcpServersChange()
	return nil
}

func filterOutTools(tools []agent.AgentTool, remove map[string]bool) []agent.AgentTool {
	out := tools[:0]
	for _, tool := range tools {
		if !remove[tool.Name] {
			out = append(out, tool)
		}
	}
	return out
}

func removeMCPClient(clients []*mcp.Client, target *mcp.Client) []*mcp.Client {
	out := clients[:0]
	for _, client := range clients {
		if client != target {
			out = append(out, client)
		}
	}
	return out
}
