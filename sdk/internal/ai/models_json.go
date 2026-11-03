package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
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
//
// Header values are pointers so a model can null out a header it inherits,
// as Pi's schema allows.
type ProviderConfig struct {
	Name       string                `json:"name,omitempty"`
	BaseURL    string                `json:"baseUrl,omitempty"`
	APIKey     string                `json:"apiKey,omitempty"`
	API        string                `json:"api,omitempty"`
	OAuth      string                `json:"oauth,omitempty"`
	AuthHeader *bool                 `json:"authHeader,omitempty"`
	Headers    map[string]*string    `json:"headers,omitempty"`
	Compat     *Compat               `json:"compat,omitempty"`
	Models     []ProviderModelConfig `json:"models,omitempty"`
	// ModelOverrides changes models without replacing the provider's list. Pi
	// applies them to built-in and extension models; nulo has no built-in
	// catalog, so it applies them to this provider's own models.
	ModelOverrides map[string]ModelOverride `json:"modelOverrides,omitempty"`
}

// ProviderModelConfig adds a model to a provider. Fields it leaves empty are
// inherited from the provider, except Compat, which merges field by field.
type ProviderModelConfig struct {
	ID                            string                    `json:"id"`
	Name                          string                    `json:"name,omitempty"`
	API                           string                    `json:"api,omitempty"`
	BaseURL                       string                    `json:"baseUrl,omitempty"`
	Reasoning                     *bool                     `json:"reasoning,omitempty"`
	ThinkingLevelMap              map[string]*string        `json:"thinkingLevelMap,omitempty"`
	Input                         []string                  `json:"input,omitempty"`
	InputLimits                   *InputLimits              `json:"inputLimits,omitempty"`
	PromptCache                   map[string]float64        `json:"promptCache,omitempty"`
	SamplingParams                map[string]any            `json:"samplingParams,omitempty"`
	SamplingParamsByThinkingLevel map[string]map[string]any `json:"samplingParamsByThinkingLevel,omitempty"`
	ContextWindow                 int                       `json:"contextWindow,omitempty"`
	MaxTokens                     int                       `json:"maxTokens,omitempty"`
	Headers                       map[string]*string        `json:"headers,omitempty"`
	Compat                        *Compat                   `json:"compat,omitempty"`
	Cost                          *Cost                     `json:"cost,omitempty"`
}

// ModelOverride changes one existing model's metadata, field by field for
// object values.
type ModelOverride struct {
	Name                          string                    `json:"name,omitempty"`
	Reasoning                     *bool                     `json:"reasoning,omitempty"`
	ThinkingLevelMap              map[string]*string        `json:"thinkingLevelMap,omitempty"`
	Input                         *[]string                 `json:"input,omitempty"`
	InputLimits                   *InputLimits              `json:"inputLimits,omitempty"`
	PromptCache                   map[string]float64        `json:"promptCache,omitempty"`
	SamplingParams                map[string]any            `json:"samplingParams,omitempty"`
	SamplingParamsByThinkingLevel map[string]map[string]any `json:"samplingParamsByThinkingLevel,omitempty"`
	ContextWindow                 *int                      `json:"contextWindow,omitempty"`
	MaxTokens                     *int                      `json:"maxTokens,omitempty"`
	Headers                       map[string]*string        `json:"headers,omitempty"`
	Compat                        *Compat                   `json:"compat,omitempty"`
	Cost                          *CostOverride             `json:"cost,omitempty"`
}

// CostOverride merges into a model's cost field by field.
type CostOverride struct {
	Input      *float64    `json:"input,omitempty"`
	Output     *float64    `json:"output,omitempty"`
	CacheRead  *float64    `json:"cacheRead,omitempty"`
	CacheWrite *float64    `json:"cacheWrite,omitempty"`
	Tiers      *[]CostTier `json:"tiers,omitempty"`
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
	clean := stripJSONComments(string(b))
	if trimmed := strings.TrimSpace(clean); strings.HasPrefix(trimmed, "[") {
		return nil, errors.New(errLegacyModelsShape)
	}
	var f modelsFile
	if err := json.Unmarshal([]byte(clean), &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(f.Providers) == 0 && len(f.Legacy) > 0 {
		return nil, errors.New(errLegacyModelsShape)
	}
	return f.Providers, nil
}

// FindModel resolves "provider/id". Like Pi, a spec without a provider uses
// "openai"; within a provider an entry whose id repeats earlier ones replaces
// them, so the last definition wins.
func (p Providers) FindModel(spec string) (Model, error) {
	spec = strings.TrimSpace(spec)
	providerID, modelID, hasProvider := strings.Cut(spec, "/")
	if !hasProvider {
		providerID, modelID = "openai", spec
	}
	cfg, ok := p[providerID]
	if !ok {
		return Model{}, fmt.Errorf("model not found: %s/%s", providerID, modelID)
	}
	var found *ProviderModelConfig
	for i := range cfg.Models {
		if cfg.Models[i].ID == modelID {
			found = &cfg.Models[i]
		}
	}
	if found == nil {
		return Model{}, fmt.Errorf("model not found: %s/%s", providerID, modelID)
	}
	model, err := cfg.resolve(providerID, *found)
	if err != nil {
		return Model{}, err
	}
	if override, ok := cfg.ModelOverrides[modelID]; ok {
		model = applyModelOverride(model, override)
	}
	return model, nil
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
	if c.OAuth != "" && c.OAuth != "radius" {
		return Model{}, fmt.Errorf("provider %q: unsupported oauth flow %q", providerID, c.OAuth)
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
	headers := mergeHeaders(c.Headers, m.Headers)
	input := m.Input
	if input == nil {
		input = []string{"text"}
	}
	name := m.Name
	if name == "" {
		name = m.ID
	}
	return Model{
		ID:                            m.ID,
		Name:                          name,
		API:                           api,
		Provider:                      providerID,
		BaseURL:                       baseURL,
		Reasoning:                     m.Reasoning != nil && *m.Reasoning,
		Input:                         input,
		ContextWindow:                 contextWindow,
		MaxTokens:                     maxTokens,
		ThinkingLevelMap:              thinkingLevelMap(m.ThinkingLevelMap),
		SamplingParams:                m.SamplingParams,
		SamplingParamsByThinkingLevel: samplingParamsByLevel(m.SamplingParamsByThinkingLevel),
		InputLimits:                   m.InputLimits,
		PromptCache:                   m.PromptCache,
		Headers:                       headers,
		Compat:                        compat,
		Cost:                          cost,
		AuthHeader:                    c.AuthHeader != nil && *c.AuthHeader,
		OAuth:                         c.OAuth,
		// apiKey and header values are templates; the request resolves them,
		// as Pi does, so a "!command" key can rotate between requests.
		APIKey: c.APIKey,
	}, nil
}

// applyModelOverride merges one modelOverrides entry into the resolved model.
// Object fields merge field by field; every other field replaces.
func applyModelOverride(model Model, o ModelOverride) Model {
	if o.Name != "" {
		model.Name = o.Name
	}
	if o.Reasoning != nil {
		model.Reasoning = *o.Reasoning
	}
	model.ThinkingLevelMap = mergeThinkingLevelMaps(model.ThinkingLevelMap, thinkingLevelMap(o.ThinkingLevelMap))
	if o.SamplingParams != nil {
		if model.SamplingParams == nil {
			model.SamplingParams = map[string]any{}
		}
		maps.Copy(model.SamplingParams, o.SamplingParams)
	}
	model.SamplingParamsByThinkingLevel = mergeSamplingParamsByLevel(model.SamplingParamsByThinkingLevel, samplingParamsByLevel(o.SamplingParamsByThinkingLevel))
	model.InputLimits = mergeInputLimits(model.InputLimits, o.InputLimits)
	if o.Input != nil {
		model.Input = slices.Clone(*o.Input)
	}
	if o.ContextWindow != nil {
		model.ContextWindow = *o.ContextWindow
	}
	if o.MaxTokens != nil {
		model.MaxTokens = *o.MaxTokens
	}
	if o.Cost != nil {
		if o.Cost.Input != nil {
			model.Cost.Input = *o.Cost.Input
		}
		if o.Cost.Output != nil {
			model.Cost.Output = *o.Cost.Output
		}
		if o.Cost.CacheRead != nil {
			model.Cost.CacheRead = *o.Cost.CacheRead
		}
		if o.Cost.CacheWrite != nil {
			model.Cost.CacheWrite = *o.Cost.CacheWrite
		}
		if o.Cost.Tiers != nil {
			model.Cost.Tiers = slices.Clone(*o.Cost.Tiers)
		}
	}
	if o.PromptCache != nil {
		if model.PromptCache == nil {
			model.PromptCache = map[string]float64{}
		}
		maps.Copy(model.PromptCache, o.PromptCache)
	}
	model.Headers = mergeResolvedHeaders(model.Headers, o.Headers)
	if o.Compat != nil {
		model.Compat = model.Compat.Override(*o.Compat)
	}
	return model
}

// thinkingLevelMap converts the JSON form, keeping null as an unsupported
// marker and dropping names that are not thinking levels.
func thinkingLevelMap(raw map[string]*string) map[ThinkingLevel]*string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[ThinkingLevel]*string, len(raw))
	for name, value := range raw {
		level := ThinkingLevel(name)
		if !slices.Contains(allThinkingLevels, level) {
			continue
		}
		out[level] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func samplingParamsByLevel(raw map[string]map[string]any) map[ThinkingLevel]map[string]any {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[ThinkingLevel]map[string]any, len(raw))
	for name, params := range raw {
		level := ThinkingLevel(name)
		if !slices.Contains(allThinkingLevels, level) || params == nil {
			continue
		}
		out[level] = params
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeThinkingLevelMaps(base, override map[ThinkingLevel]*string) map[ThinkingLevel]*string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[ThinkingLevel]*string, len(base)+len(override))
	for level, value := range base {
		out[level] = value
	}
	for level, value := range override {
		if value == nil {
			out[level] = nil
			continue
		}
		copy := *value
		out[level] = &copy
	}
	return out
}

func mergeSamplingParamsByLevel(base, override map[ThinkingLevel]map[string]any) map[ThinkingLevel]map[string]any {
	if override == nil {
		return base
	}
	out := make(map[ThinkingLevel]map[string]any, len(base)+len(override))
	for level, params := range base {
		out[level] = params
	}
	for level, params := range override {
		next := make(map[string]any, len(base[level])+len(params))
		maps.Copy(next, base[level])
		maps.Copy(next, params)
		out[level] = next
	}
	return out
}

// mergeInputLimits merges an override over the base, keeping a zero field as
// "not overridden", as Pi's mergeInputLimits.
func mergeInputLimits(base, override *InputLimits) *InputLimits {
	if base == nil && override == nil {
		return nil
	}
	out := &InputLimits{}
	if base != nil {
		*out = *base
	}
	if override == nil {
		return out
	}
	if override.MaxRequestBytes != 0 {
		out.MaxRequestBytes = override.MaxRequestBytes
	}
	if override.Images != nil {
		images := ImageInputLimits{}
		if out.Images != nil {
			images = *out.Images
		}
		if override.Images.MaxPerMessage != 0 {
			images.MaxPerMessage = override.Images.MaxPerMessage
		}
		if override.Images.MaxPerRequest != 0 {
			images.MaxPerRequest = override.Images.MaxPerRequest
		}
		if override.Images.Resize != nil {
			resize := ImageResizeOptions{}
			if images.Resize != nil {
				resize = *images.Resize
			}
			if override.Images.Resize.MaxWidth != 0 {
				resize.MaxWidth = override.Images.Resize.MaxWidth
			}
			if override.Images.Resize.MaxHeight != 0 {
				resize.MaxHeight = override.Images.Resize.MaxHeight
			}
			if override.Images.Resize.MaxBytes != 0 {
				resize.MaxBytes = override.Images.Resize.MaxBytes
			}
			if override.Images.Resize.JPEGQuality != 0 {
				resize.JPEGQuality = override.Images.Resize.JPEGQuality
			}
			images.Resize = &resize
		}
		out.Images = &images
	}
	return out
}

// mergeHeaders folds the provider headers and then the model headers over them,
// case-insensitively. A null model value removes the inherited header.
func mergeHeaders(base, over map[string]*string) map[string]string {
	if len(base) == 0 && len(over) == 0 {
		return nil
	}
	type header struct{ name, value string }
	merged := map[string]header{}
	apply := func(headers map[string]*string) {
		for name, value := range headers {
			key := strings.ToLower(name)
			if value == nil {
				delete(merged, key)
				continue
			}
			merged[key] = header{name, *value}
		}
	}
	apply(base)
	apply(over)
	if len(merged) == 0 {
		return nil
	}
	out := make(map[string]string, len(merged))
	for _, h := range merged {
		out[h.name] = h.value
	}
	return out
}

// mergeResolvedHeaders applies nullable override headers over already-merged
// model headers, case-insensitively; a null removes the inherited spelling.
func mergeResolvedHeaders(base map[string]string, over map[string]*string) map[string]string {
	if len(over) == 0 {
		return base
	}
	basePointers := make(map[string]*string, len(base))
	for name, value := range base {
		value := value
		basePointers[name] = &value
	}
	return mergeHeaders(basePointers, over)
}

// RequestHeaders resolves the model's header templates with the request's
// environment overrides and merges the per-request headers, which win.
func RequestHeaders(model Model, opts StreamOptions) (map[string]string, error) {
	resolved, err := resolveHeaders(model.Headers, `model "`+model.Provider+"/"+model.ID+`"`, opts.Env)
	if err != nil {
		return nil, err
	}
	if len(opts.Headers) == 0 {
		return resolved, nil
	}
	if resolved == nil {
		resolved = make(map[string]string, len(opts.Headers))
	}
	for name, value := range opts.Headers {
		for existing := range resolved {
			if strings.EqualFold(existing, name) {
				delete(resolved, existing)
			}
		}
		resolved[name] = value
	}
	return resolved, nil
}

var (
	configEnvName       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	configEnvNamePrefix = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
)

// configPart is one segment of a models.json value: literal text or an
// environment-variable reference, as Pi's resolve-config-value.ts parses it.
type configPart struct {
	isEnv bool
	value string
}

// parseConfigTemplate splits raw into literal and "$NAME" / "${NAME}" parts.
// "$$" and "$!" escape a literal "$" and "!"; a "$" that starts no name stays
// literal, and "${...}" that is not a valid name stays literal too.
func parseConfigTemplate(raw string) []configPart {
	var parts []configPart
	literal := func(s string) {
		if s == "" {
			return
		}
		if n := len(parts); n > 0 && !parts[n-1].isEnv {
			parts[n-1].value += s
			return
		}
		parts = append(parts, configPart{value: s})
	}
	for i := 0; i < len(raw); {
		rel := strings.IndexByte(raw[i:], '$')
		if rel < 0 {
			literal(raw[i:])
			break
		}
		dollar := i + rel
		literal(raw[i:dollar])
		var next byte
		if dollar+1 < len(raw) {
			next = raw[dollar+1]
		}
		switch {
		case next == '$' || next == '!':
			literal(string(next))
			i = dollar + 2
		case next == '{':
			rel := strings.IndexByte(raw[dollar+2:], '}')
			if rel < 0 {
				literal("$")
				i = dollar + 1
				continue
			}
			end := dollar + 2 + rel
			name := raw[dollar+2 : end]
			if configEnvName.MatchString(name) {
				parts = append(parts, configPart{isEnv: true, value: name})
			} else {
				literal(raw[dollar : end+1])
			}
			i = end + 1
		default:
			name := configEnvNamePrefix.FindString(raw[dollar+1:])
			if name == "" {
				literal("$")
				i = dollar + 1
				continue
			}
			parts = append(parts, configPart{isEnv: true, value: name})
			i = dollar + 1 + len(name)
		}
	}
	return parts
}

// resolveTemplate concatenates the parts. A referenced variable that is unset
// or empty makes the whole value undefined, which Pi collapses to "".
// Provider-scoped entries in env win over the process environment.
func resolveTemplate(parts []configPart, env map[string]string) (string, bool) {
	var b strings.Builder
	for _, p := range parts {
		if !p.isEnv {
			b.WriteString(p.value)
			continue
		}
		value := ""
		if env != nil {
			value = env[p.value]
		}
		if value == "" {
			value = os.Getenv(p.value)
		}
		if value == "" {
			return "", false
		}
		b.WriteString(value)
	}
	return b.String(), true
}

func templateEnvNames(parts []configPart) []string {
	var names []string
	for _, p := range parts {
		if !p.isEnv {
			continue
		}
		seen := false
		for _, name := range names {
			if name == p.value {
				seen = true
				break
			}
		}
		if !seen {
			names = append(names, p.value)
		}
	}
	return names
}

func missingEnvNames(parts []configPart, env map[string]string) []string {
	var missing []string
	for _, name := range templateEnvNames(parts) {
		value := ""
		if env != nil {
			value = env[name]
		}
		if value == "" {
			value = os.Getenv(name)
		}
		if value == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

// resolveConfigValue is Pi's resolveConfigValue: a command or a missing variable
// resolves to "", the undefined value.
func resolveConfigValue(raw string, env map[string]string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "!") {
		value, _ := runConfigCommand(raw)
		return value
	}
	value, _ := resolveTemplate(parseConfigTemplate(raw), env)
	return value
}

// resolveConfigValueE is Pi's resolveConfigValueOrError: request-time
// credentials and headers report why they could not be resolved.
func resolveConfigValueE(raw, description string, env map[string]string) (string, error) {
	if strings.HasPrefix(raw, "!") {
		value, ok := runConfigCommand(raw)
		if !ok {
			return "", fmt.Errorf("Failed to resolve %s from shell command: %s", description, strings.TrimPrefix(raw, "!"))
		}
		return value, nil
	}
	parts := parseConfigTemplate(raw)
	if value, ok := resolveTemplate(parts, env); ok {
		return value, nil
	}
	missing := missingEnvNames(parts, env)
	switch len(missing) {
	case 0:
		return "", fmt.Errorf("Failed to resolve %s", description)
	case 1:
		return "", fmt.Errorf("Failed to resolve %s from environment variable: %s", description, missing[0])
	default:
		return "", fmt.Errorf("Failed to resolve %s from environment variables: %s", description, strings.Join(missing, ", "))
	}
}

// resolveHeaders resolves every template and drops entries that resolve to
// nothing, as Pi does.
func resolveHeaders(headers map[string]string, description string, env map[string]string) (map[string]string, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(headers))
	for name, raw := range headers {
		value, err := resolveConfigValueE(raw, fmt.Sprintf(`%s header "%s"`, description, name), env)
		if err != nil {
			return nil, err
		}
		if value != "" {
			out[name] = value
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// configCommandTimeout bounds a "!command" value, matching Pi's 10s.
const configCommandTimeout = 10 * time.Second

// runConfigCommand executes a "!command" value and reports whether it ran; an
// empty result is a successful but empty value, as Pi's execSync result is.
func runConfigCommand(command string) (string, bool) {
	shell, args := "sh", []string{"-c"}
	if runtime.GOOS == "windows" {
		shell, args = "cmd", []string{"/c"}
	} else if bash, err := exec.LookPath("bash"); err == nil {
		shell = bash
	}
	ctx, cancel := context.WithTimeout(context.Background(), configCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, append(args, command[1:])...)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// stripJSONComments removes // line comments and trailing commas from
// models.json, leaving string literals untouched, as Pi's stripJsonComments.
func stripJSONComments(input string) string {
	var b strings.Builder
	for i := 0; i < len(input); {
		if input[i] == '"' {
			if end, ok := jsonStringLiteralEnd(input, i); ok {
				b.WriteString(input[i:end])
				i = end
				continue
			}
		}
		if input[i] == '/' && i+1 < len(input) && input[i+1] == '/' {
			end := strings.IndexByte(input[i:], '\n')
			if end < 0 {
				break
			}
			i += end
			continue
		}
		b.WriteByte(input[i])
		i++
	}
	withoutComments := b.String()

	b.Reset()
	for i := 0; i < len(withoutComments); {
		if withoutComments[i] == '"' {
			if end, ok := jsonStringLiteralEnd(withoutComments, i); ok {
				b.WriteString(withoutComments[i:end])
				i = end
				continue
			}
		}
		if withoutComments[i] == ',' {
			next := i + 1
			for next < len(withoutComments) && strings.ContainsRune(" \t\r\n", rune(withoutComments[next])) {
				next++
			}
			if next < len(withoutComments) && (withoutComments[next] == '}' || withoutComments[next] == ']') {
				i++
				continue
			}
		}
		b.WriteByte(withoutComments[i])
		i++
	}
	return b.String()
}

// jsonStringLiteralEnd matches Pi's JSON string literal at start: bytes of a
// JSON string literal, with a backslash escaping the next character.
func jsonStringLiteralEnd(input string, start int) (int, bool) {
	for i := start + 1; i < len(input); {
		switch input[i] {
		case '"':
			return i + 1, true
		case '\\':
			if i+1 >= len(input) {
				return 0, false
			}
			i += 2
		default:
			i++
		}
	}
	return 0, false
}
