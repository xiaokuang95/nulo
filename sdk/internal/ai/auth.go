package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func LookupAPIKey(provider, agentDir string, opts StreamOptions) string {
	if opts.APIKey != "" {
		return opts.APIKey
	}
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

func LoadModels(agentDir string) []Model {
	if agentDir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(agentDir, "models.json"))
	if err != nil {
		return nil
	}
	var wrap struct {
		Models []Model `json:"models"`
	}
	if err := json.Unmarshal(b, &wrap); err == nil && len(wrap.Models) > 0 {
		return wrap.Models
	}
	var list []Model
	if err := json.Unmarshal(b, &list); err == nil {
		return list
	}
	return nil
}

func FindModel(spec string, models []Model) (Model, bool) {
	spec = strings.TrimSpace(spec)
	provider, id, ok := strings.Cut(spec, "/")
	if !ok {
		id = spec
		provider = ""
	}
	for _, m := range models {
		if (provider == "" || m.Provider == provider) && m.ID == id {
			return m, true
		}
		if spec == m.Provider+"/"+m.ID {
			return m, true
		}
	}
	return Model{}, false
}
