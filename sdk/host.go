package sdk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/mcp"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
	"github.com/xiaokuang95/nulo/sdk/internal/skills"
)

// Messages returns the active branch's projected messages, including the
// system prompt. The returned slice and messages are copies: changing them
// does not affect the session.
func (s *Session) Messages() []Message {
	if s == nil {
		return nil
	}
	projection := s.manager.Project()
	out := make([]Message, len(projection.Messages))
	copy(out, projection.Messages)
	return out
}

// File is the JSONL path the session writes to. It is set before the first
// user or assistant message creates the file.
func (s *Session) File() string {
	if s == nil {
		return ""
	}
	return s.manager.File()
}

// ID is the session's stable identifier.
func (s *Session) ID() string {
	if s == nil {
		return ""
	}
	return s.manager.ID()
}

// SkillNames returns the names of the skills discovered for this session.
func (s *Session) SkillNames() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.skills))
	for _, skill := range s.skills {
		out = append(out, skill.Name)
	}
	return out
}

// PluginNames returns the native plugins loaded for this session.
func (s *Session) PluginNames() []string {
	if s == nil || s.services == nil || s.services.plugins == nil {
		return nil
	}
	instances := s.services.plugins.Instances()
	out := make([]string, 0, len(instances))
	for _, instance := range instances {
		name := instance.Registration.Name
		if name == "" {
			name = filepath.Base(instance.Path)
		}
		out = append(out, name)
	}
	return out
}

// MCPServerNames returns the MCP servers connected to this session.
func (s *Session) MCPServerNames() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.servers))
	for _, server := range s.servers {
		out = append(out, server.Name)
	}
	return out
}

// SessionFile is Manager().File() without exposing the internal manager type.
func (s *Session) SessionFile() string { return s.File() }

// Branch returns the entries on the active branch, root first.
func (s *Session) Branch() []SessionEntry {
	if s == nil {
		return nil
	}
	return s.manager.Branch()
}

// LeafID is the active branch's leaf entry.
func (s *Session) LeafID() string {
	if s == nil {
		return ""
	}
	return s.manager.LeafID()
}

// ListAllSessions lists every stored session under the agent directory's
// sessions root, newest activity first. Unlike Runtime.ListSessions it spans
// all working directories, so a host can build a project switcher before it
// knows which project the user will open.
func ListAllSessions(agentDir string) ([]SessionInfo, error) {
	root := DefaultSessionDir(agentDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionInfo{}, nil
		}
		return nil, err
	}
	var out []SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		infos, err := session.List(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		out = append(out, infos...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Modified.After(out[j].Modified)
	})
	return out, nil
}

// SkillInfo describes one discovered skill without exposing its body.
type SkillInfo struct {
	Name                   string `json:"name"`
	Description            string `json:"description,omitempty"`
	Path                   string `json:"path,omitempty"`
	Source                 string `json:"source,omitempty"`
	DisableModelInvocation bool   `json:"disableModelInvocation,omitempty"`
}

// ListSkills discovers every skill a session would see, including ones the
// host disabled for model invocation.
func ListSkills(agentDir, cwd string, projectTrusted bool) []SkillInfo {
	var paths []string
	if manager := NewSettingsManager(cwd, agentDir); manager != nil {
		paths = manager.GetSkillPaths()
	}
	loaded := skills.LoadSkills(skills.LoadOptions{
		Cwd:             cwd,
		AgentDir:        agentDir,
		ProjectTrusted:  projectTrusted,
		IncludeDefaults: true,
		SkillPaths:      paths,
	})
	out := make([]SkillInfo, 0, len(loaded.Skills))
	for _, skill := range loaded.Skills {
		out = append(out, SkillInfo{
			Name: skill.Name, Description: skill.Description, Path: skill.FilePath,
			Source: skill.Source, DisableModelInvocation: skill.DisableModelInvocation,
		})
	}
	return out
}

// MCPServerInfo describes one configured MCP server without exposing its
// command, environment, or headers.
type MCPServerInfo struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
}

// ListMCPServers lists the merged global and project mcp.json entries,
// including disabled servers, so a host can show an enable switch.
func ListMCPServers(agentDir, cwd string, projectTrusted bool) []MCPServerInfo {
	cfg := mcp.Load(agentDir, cwd, projectTrusted)
	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]MCPServerInfo, 0, len(names))
	for _, name := range names {
		server := cfg.Servers[name]
		enabled := server.Enabled == nil || *server.Enabled
		out = append(out, MCPServerInfo{Name: name, Enabled: enabled, Description: server.Description})
	}
	return out
}

// TestProviderModel sends a tiny prompt through the configured provider to
// verify that its key, endpoint, and first model work together. It does not
// save a session and waits at most 20 seconds.
func TestProviderModel(agentDir, modelSpec string) error {
	providers, err := ai.LoadProviders(agentDir)
	if err != nil {
		return err
	}
	model, err := providers.FindModel(modelSpec)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream := ai.StreamForDir(agentDir)(ctx, model, ai.Transcript{
		Messages: []ai.Message{ai.UserMessage("Reply with the single word OK.", time.Now().UnixMilli())},
	}, ai.StreamOptions{MaxTokens: 32})
	result := stream.Result()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if result.StopReason == ai.StopError {
		message := strings.TrimSpace(result.ErrorMessage)
		if message == "" {
			message = "provider returned an error"
		}
		if len(message) > 300 {
			message = message[:300] + "…"
		}
		return errors.New(message)
	}
	return nil
}

// ProviderModels is one provider and the models it serves, as read from
// models.json. The provider entry is resolved into ready-to-use Model values,
// so a host can show names, context windows and limits without parsing the
// file itself.
type ProviderModels struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	API     string  `json:"api"`
	BaseURL string  `json:"baseUrl"`
	Models  []Model `json:"models"`
	// APIKeySource is "auth" for auth.json, "models" for a models.json
	// template, "env" for the provider's environment variable, or empty.
	// It reports where a key would come from without exposing the key.
	APIKeySource string `json:"apiKeySource,omitempty"`
}

// ListProviders resolves every provider in <agentDir>/models.json. Models
// defined twice keep the last definition, matching Providers.FindModel.
// Providers and their models are sorted by ID for a stable menu.
func ListProviders(agentDir string) ([]ProviderModels, error) {
	providers, err := ai.LoadProviders(agentDir)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderModels, 0, len(providers))
	for providerID, config := range providers {
		entry := ProviderModels{
			ID:           providerID,
			Name:         config.Name,
			API:          config.API,
			BaseURL:      config.BaseURL,
			APIKeySource: ai.APIKeySource(agentDir, providerID, config.APIKey),
		}
		if entry.Name == "" {
			entry.Name = providerID
		}
		last := make(map[string]int, len(config.Models))
		for i, model := range config.Models {
			last[model.ID] = i
		}
		for i, model := range config.Models {
			if last[model.ID] != i {
				continue
			}
			resolved, err := providers.FindModel(providerID + "/" + model.ID)
			if err != nil {
				continue
			}
			entry.Models = append(entry.Models, resolved)
		}
		if entry.API == "" && len(entry.Models) > 0 {
			entry.API = entry.Models[0].API
		}
		if entry.BaseURL == "" && len(entry.Models) > 0 {
			entry.BaseURL = entry.Models[0].BaseURL
		}
		sort.SliceStable(entry.Models, func(i, j int) bool {
			return entry.Models[i].ID < entry.Models[j].ID
		})
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ToolApprovalRequest describes one tool call waiting for host permission.
type ToolApprovalRequest struct {
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Args       map[string]any `json:"args"`
	CWD        string         `json:"cwd"`
}

// ToolApprovalDecision lets a host allow or block one tool call. A blocked
// call becomes a tool error with Reason.
type ToolApprovalDecision struct {
	Allow  bool
	Reason string
}

// ToolApprovalFunc is consulted before a tool executes. It may block until a
// person answers; the agent turn waits in the meantime. Returning an error
// aborts the turn.
type ToolApprovalFunc func(ctx context.Context, request ToolApprovalRequest) (ToolApprovalDecision, error)

// SetProviderAPIKey stores an API key for provider in the agent's auth.json.
// The next request picks it up without restarting the runtime. The file is
// rewritten atomically with owner-only permissions.
func SetProviderAPIKey(agentDir, provider, key string) error {
	return ai.SaveAPIKey(agentDir, provider, key)
}

// ClearProviderAPIKey removes a stored auth.json credential. It does not
// remove a models.json apiKey or environment fallback.
func ClearProviderAPIKey(agentDir, provider string) error {
	return ai.RemoveAPIKey(agentDir, provider)
}
