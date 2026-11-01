package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// LookupAPIKey resolves a provider's key: an explicit per-request key, then the
// provider's apiKey from models.json, then a per-request env override, then
// auth.json, then the process environment.
func LookupAPIKey(model Model, agentDir string, opts StreamOptions) string {
	if opts.APIKey != "" {
		return opts.APIKey
	}
	if model.APIKey != "" {
		return model.APIKey
	}
	provider := model.Provider
	if opts.Env != nil {
		if k := keyFromEnvMap(provider, opts.Env); k != "" {
			return k
		}
	}
	if k := keyFromFile(provider, agentDir); k != "" {
		return k
	}
	return keyFromEnv(provider)
}

func keyFromEnv(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY")
	case "openai", "openai-codex":
		return os.Getenv("OPENAI_API_KEY")
	default:
		if v := os.Getenv(strings.ToUpper(provider) + "_API_KEY"); v != "" {
			return v
		}
		return os.Getenv("OPENAI_API_KEY")
	}
}

func keyFromEnvMap(provider string, env map[string]string) string {
	keys := []string{strings.ToUpper(provider) + "_API_KEY"}
	if provider == "anthropic" {
		keys = append(keys, "ANTHROPIC_API_KEY")
	}
	if provider == "openai" {
		keys = append(keys, "OPENAI_API_KEY")
	}
	for _, k := range keys {
		if v := env[k]; v != "" {
			return v
		}
	}
	return ""
}

func keyFromFile(provider, agentDir string) string {
	if agentDir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(agentDir, "auth.json"))
	if err != nil {
		return ""
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		return ""
	}
	raw, ok := root[provider]
	if !ok {
		return ""
	}
	var rec struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return ""
	}
	return rec.Key
}
