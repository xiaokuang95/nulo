package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// envAPIKeyVars maps the providers Pi knows to the environment variable that
// holds their API key. An unknown provider has no ambient key: Pi returns
// nothing rather than guessing, so a gateway never receives an unrelated key.
var envAPIKeyVars = map[string]string{
	"ant-ling":              "ANT_LING_API_KEY",
	"openai":                "OPENAI_API_KEY",
	"azure":                 "AZURE_OPENAI_API_KEY",
	"nvidia":                "NVIDIA_API_KEY",
	"deepseek":              "DEEPSEEK_API_KEY",
	"google":                "GEMINI_API_KEY",
	"google-vertex":         "GOOGLE_CLOUD_API_KEY",
	"groq":                  "GROQ_API_KEY",
	"cerebras":              "CEREBRAS_API_KEY",
	"xai":                   "XAI_API_KEY",
	"radius":                "RADIUS_API_KEY",
	"openrouter":            "OPENROUTER_API_KEY",
	"zai":                   "ZAI_API_KEY",
	"mistral":               "MISTRAL_API_KEY",
	"minimax":               "MINIMAX_API_KEY",
	"moonshotai":            "MOONSHOT_API_KEY",
	"huggingface":           "HF_TOKEN",
	"fireworks":             "FIREWORKS_API_KEY",
	"together":              "TOGETHER_API_KEY",
	"baseten":               "BASETEN_API_KEY",
	"opencode":              "OPENCODE_API_KEY",
	"kimi-coding":           "KIMI_API_KEY",
	"meta":                  "META_API_KEY",
	"cloudflare-workers-ai": "CLOUDFLARE_API_KEY",
}

// LookupAPIKey resolves a provider's key in Pi's order: an explicit per-request
// key, then a stored auth.json credential (API key or OAuth token, refreshed
// when expired), then the provider's models.json apiKey (resolved now, so a
// "!command" rotates between requests), and finally the provider's environment
// variable.
func LookupAPIKey(model Model, agentDir string, opts StreamOptions) (string, error) {
	if opts.APIKey != "" {
		return opts.APIKey, nil
	}
	if agentDir != "" {
		if credential, ok := credentialFromFile(model.Provider, agentDir); ok {
			switch credential.Type {
			case "", "api_key":
				if credential.Key != "" {
					return credential.Key, nil
				}
			case "oauth":
				key, err := oauthAccessToken(model, agentDir, credential)
				if err != nil {
					return "", err
				}
				if key != "" {
					return key, nil
				}
			}
		}
	}
	if model.APIKey != "" {
		return resolveConfigValueE(model.APIKey, `API key for provider "`+model.Provider+`"`, opts.Env)
	}
	return keyFromEnv(model.Provider), nil
}

func keyFromEnv(provider string) string {
	if provider == "anthropic" {
		// ANTHROPIC_AUTH_TOKEN is a bearer header, not an API key.
		return os.Getenv("ANTHROPIC_API_KEY")
	}
	if name, ok := envAPIKeyVars[provider]; ok {
		return os.Getenv(name)
	}
	return ""
}

// storedCredential is one auth.json entry: an API key or an OAuth token set.
type storedCredential struct {
	Type    string `json:"type"`
	Key     string `json:"key,omitempty"`
	Access  string `json:"access,omitempty"`
	Refresh string `json:"refresh,omitempty"`
	Expires int64  `json:"expires,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

func credentialFromFile(provider, agentDir string) (storedCredential, bool) {
	if agentDir == "" {
		return storedCredential{}, false
	}
	b, err := os.ReadFile(filepath.Join(agentDir, "auth.json"))
	if err != nil {
		return storedCredential{}, false
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		return storedCredential{}, false
	}
	raw, ok := root[provider]
	if !ok {
		return storedCredential{}, false
	}
	var credential storedCredential
	if err := json.Unmarshal(raw, &credential); err != nil {
		return storedCredential{}, false
	}
	return credential, true
}
