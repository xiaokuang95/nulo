package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// models.json, mirroring Pi's shape: a "providers" map where each provider holds
// the endpoint, credentials and API for its models, and each model inherits what
// it does not set itself.

// Defaults Pi applies to a model that does not state them.
const (
	defaultContextWindow = 128000
	defaultMaxTokens     = 16384
)

// ProviderConfig is one entry under "providers".
type ProviderConfig struct {
	Name    string                `json:"name,omitempty"`
	BaseURL string                `json:"baseUrl,omitempty"`
	APIKey  string                `json:"apiKey,omitempty"`
	API     string                `json:"api,omitempty"`
	Headers map[string]string     `json:"headers,omitempty"`
	Compat  *Compat               `json:"compat,omitempty"`
	Models  []ProviderModelConfig `json:"models,omitempty"`
}

// ProviderModelConfig adds a model to a provider. Fields it leaves empty are
// inherited from the provider, except Compat, which merges field by field.
type ProviderModelConfig struct {
	ID            string            `json:"id"`
	Name          string            `json:"name,omitempty"`
	API           string            `json:"api,omitempty"`
	BaseURL       string            `json:"baseUrl,omitempty"`
	Reasoning     *bool             `json:"reasoning,omitempty"`
	Input         []string          `json:"input,omitempty"`
	ContextWindow int               `json:"contextWindow,omitempty"`
	MaxTokens     int               `json:"maxTokens,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	Compat        *Compat           `json:"compat,omitempty"`
	Cost          *Cost             `json:"cost,omitempty"`
}

// Providers is the parsed "providers" map, keyed by provider ID.
type Providers map[string]ProviderConfig

type modelsFile struct {
	Providers map[string]ProviderConfig `json:"providers"`
	// Legacy is read only to detect the shape this parser replaced.
	Legacy json.RawMessage `json:"models"`
}

// errLegacyModelsShape explains the migration for the flat array nulo used before
// the provider layer existed.
const errLegacyModelsShape = `models.json: the flat "models" array was replaced by Pi's "providers" map.
Move the shared fields up into a provider and keep only what differs per model:

  {"providers": {
     "tokenharbor": {
       "baseUrl": "https://example.com/v1",
       "api": "openai-completions",
       "apiKey": "$TOKENHARBOR_API_KEY",
       "models": [{"id": "deepseek-v4.1-flash", "cost": {...}}]
     }
  }}`

// LoadProviders reads <agentDir>/models.json. A missing file is not an error.
func LoadProviders(agentDir string) (Providers, error) {
	if agentDir == "" {
		return nil, nil
	}
	path := filepath.Join(agentDir, "models.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if trimmed := strings.TrimSpace(string(b)); strings.HasPrefix(trimmed, "[") {
		return nil, errors.New(errLegacyModelsShape)
	}
	var f modelsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(f.Providers) == 0 && len(f.Legacy) > 0 {
		return nil, errors.New(errLegacyModelsShape)
	}
	return f.Providers, nil
}

// FindModel resolves "provider/id", or a bare id searched across providers.
func (p Providers) FindModel(spec string) (Model, error) {
	spec = strings.TrimSpace(spec)
	providerID, modelID, hasProvider := strings.Cut(spec, "/")
	if !hasProvider {
		providerID, modelID = "", spec
	}
	for _, name := range p.sortedIDs() {
		if providerID != "" && name != providerID {
			continue
		}
		cfg := p[name]
		for _, m := range cfg.Models {
			if m.ID == modelID {
				return cfg.resolve(name, m)
			}
		}
	}
	return Model{}, fmt.Errorf("unknown model %q", spec)
}

// sortedIDs keeps a bare-id lookup deterministic when several providers offer it.
func (p Providers) sortedIDs() []string {
	names := make([]string, 0, len(p))
	for name := range p {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolve folds one provider and model entry into a ready-to-use Model.
func (c ProviderConfig) resolve(providerID string, m ProviderModelConfig) (Model, error) {
	api := m.API
	if api == "" {
		api = c.API
	}
	if api == "" {
		return Model{}, fmt.Errorf("provider %q, model %q: no \"api\" specified; set it at provider or model level", providerID, m.ID)
	}
	baseURL := m.BaseURL
	if baseURL == "" {
		baseURL = c.BaseURL
	}
	var contextWindow, maxTokens int
	if contextWindow = m.ContextWindow; contextWindow == 0 {
		contextWindow = defaultContextWindow
	}
	if maxTokens = m.MaxTokens; maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}
	cost := Cost{}
	if m.Cost != nil {
		cost = *m.Cost
	}
	compat := Compat{}
	if c.Compat != nil {
		compat = *c.Compat
	}
	if m.Compat != nil {
		compat = compat.Override(*m.Compat)
	}
	headers, err := ResolveHeaders(mergeHeaders(c.Headers, m.Headers))
	if err != nil {
		return Model{}, fmt.Errorf("provider %q, model %q: %w", providerID, m.ID, err)
	}
	// A provider without an apiKey falls back to auth.json and the environment at
	// request time, so an empty value here is legitimate.
	apiKey := ""
	if c.APIKey != "" {
		if apiKey, err = ResolveConfigValue(c.APIKey); err != nil {
			return Model{}, fmt.Errorf("provider %q: apiKey: %w", providerID, err)
		}
	}
	name := m.Name
	if name == "" {
		name = m.ID
	}
	return Model{
		ID:            m.ID,
		Name:          name,
		API:           api,
		Provider:      providerID,
		BaseURL:       baseURL,
		Reasoning:     m.Reasoning != nil && *m.Reasoning,
		Input:         m.Input,
		ContextWindow: contextWindow,
		MaxTokens:     maxTokens,
		Headers:       headers,
		Compat:        compat,
		Cost:          cost,
		APIKey:        apiKey,
	}, nil
}

func mergeHeaders(base, over map[string]string) map[string]string {
	if len(base) == 0 && len(over) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// RequestHeaders merges the model's headers with per-request overrides, which win.
func RequestHeaders(model Model, opts StreamOptions) map[string]string {
	return mergeHeaders(model.Headers, opts.Headers)
}

var (
	configEnvBraced = regexp.MustCompile(`^\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	configEnvBare   = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)`)
)

// ResolveConfigValue resolves a models.json string the way Pi does: a leading "!"
// runs the rest as a shell command and uses its trimmed stdout; "$NAME" and
// "${NAME}" interpolate the environment; "$$" and "$!" escape a literal "$" and
// "!"; anything else is a literal.
//
// Values are resolved when the model is built, not per request, so a command runs
// once per BuildModel rather than once per API call.
func ResolveConfigValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if strings.HasPrefix(raw, "!") {
		return runConfigCommand(raw[1:])
	}
	var b strings.Builder
	for i := 0; i < len(raw); {
		if raw[i] != '$' || i+1 >= len(raw) {
			b.WriteByte(raw[i])
			i++
			continue
		}
		switch next := raw[i+1]; {
		case next == '$' || next == '!':
			b.WriteByte(next)
			i += 2
		case configEnvBraced.MatchString(raw[i+1:]):
			name := configEnvBraced.FindStringSubmatch(raw[i+1:])[1]
			b.WriteString(os.Getenv(name))
			i += 1 + len(name) + 2 // "$" + "{" + name + "}"
		default:
			m := configEnvBare.FindStringSubmatch(raw[i+1:])
			if m == nil {
				b.WriteByte('$')
				i++
				continue
			}
			b.WriteString(os.Getenv(m[1]))
			i += 1 + len(m[1])
		}
	}
	return b.String(), nil
}

// ResolveHeaders resolves every header value, dropping those that resolve to
// nothing, as Pi does.
func ResolveHeaders(headers map[string]string) (map[string]string, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		resolved, err := ResolveConfigValue(v)
		if err != nil {
			return nil, fmt.Errorf("header %q: %w", k, err)
		}
		if resolved != "" {
			out[k] = resolved
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// configCommandTimeout bounds a "!command" value, matching Pi's 10s.
const configCommandTimeout = 10 * time.Second

func runConfigCommand(command string) (string, error) {
	shell, args := "sh", []string{"-c"}
	if runtime.GOOS == "windows" {
		shell, args = "cmd", []string{"/c"}
	} else if bash, err := exec.LookPath("bash"); err == nil {
		shell = bash
	}
	ctx, cancel := context.WithTimeout(context.Background(), configCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, append(args, command)...)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("command %q: %w", command, err)
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("command %q produced no output", command)
	}
	return value, nil
}
