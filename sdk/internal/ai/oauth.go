package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Radius OAuth credentials live in auth.json as
// {"type":"oauth","access":"…","refresh":"…","expires":<ms>}. The refresh
// endpoint and client ID mirror Pi's radius.ts.
const (
	radiusOAuthClientID   = "pi-gateway"
	radiusTokenExpirySkew = 60 * time.Second
)

var authFileMu sync.Mutex

// oauthAccessToken returns the stored access token, refreshing an expired
// Radius token at the provider's gateway and writing the new credential back.
func oauthAccessToken(model Model, agentDir string, credential storedCredential) (string, error) {
	now := time.Now().UnixMilli()
	fresh := credential.Expires == 0 || now+radiusTokenExpirySkew.Milliseconds() < credential.Expires
	if fresh || credential.Refresh == "" {
		return credential.Access, nil
	}
	if model.OAuth == "" {
		return credential.Access, nil
	}
	if model.OAuth != "radius" {
		return "", fmt.Errorf("oauth flow %q cannot refresh", model.OAuth)
	}
	refreshed, err := refreshRadiusToken(model, credential)
	if err != nil {
		return "", fmt.Errorf("refresh oauth for provider %q: %w", model.Provider, err)
	}
	if agentDir != "" {
		_ = saveCredential(agentDir, model.Provider, refreshed)
	}
	return refreshed.Access, nil
}

// refreshRadiusToken exchanges the refresh token at the gateway's token
// endpoint, as Pi's RadiusOAuth.RefreshToken.
func refreshRadiusToken(model Model, credential storedCredential) (storedCredential, error) {
	base := strings.TrimRight(model.BaseURL, "/")
	if base == "" {
		return storedCredential{}, errors.New("oauth provider has no baseUrl")
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {radiusOAuthClientID},
		"refresh_token": {credential.Refresh},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return storedCredential{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return storedCredential{}, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return storedCredential{}, fmt.Errorf("Radius OAuth token request failed: %d %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var token struct {
		AccessToken  string  `json:"access_token"`
		RefreshToken string  `json:"refresh_token"`
		ExpiresIn    float64 `json:"expires_in"`
		Scope        string  `json:"scope"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return storedCredential{}, fmt.Errorf("Radius OAuth token response: %w", err)
	}
	if token.AccessToken == "" {
		return storedCredential{}, errors.New("Radius OAuth token response has no access_token")
	}
	next := storedCredential{
		Type:    "oauth",
		Access:  token.AccessToken,
		Refresh: token.RefreshToken,
		Expires: time.Now().Add(time.Duration(token.ExpiresIn * float64(time.Second))).Add(-radiusTokenExpirySkew).UnixMilli(),
		Scope:   token.Scope,
	}
	if next.Refresh == "" {
		next.Refresh = credential.Refresh
	}
	return next, nil
}

// saveCredential rewrites one auth.json entry, keeping every other provider.
func saveCredential(agentDir, provider string, credential storedCredential) error {
	authFileMu.Lock()
	defer authFileMu.Unlock()
	root, err := readAuthRoot(agentDir)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	root[provider] = encoded
	return writeAuthRoot(agentDir, root)
}

// SaveAPIKey stores an API key for a provider in auth.json with owner-only
// permissions. It overrides a models.json key or environment variable.
func SaveAPIKey(agentDir, provider, key string) error {
	agentDir = strings.TrimSpace(agentDir)
	provider = strings.TrimSpace(provider)
	key = strings.TrimSpace(key)
	if agentDir == "" {
		return errors.New("agent directory is required")
	}
	if provider == "" {
		return errors.New("provider is required")
	}
	if key == "" {
		return errors.New("API key is empty")
	}
	return saveCredential(agentDir, provider, storedCredential{Type: "api_key", Key: key})
}

// RemoveAPIKey removes this provider's stored credential from auth.json. It
// leaves other providers and any models.json fallback untouched.
func RemoveAPIKey(agentDir, provider string) error {
	agentDir = strings.TrimSpace(agentDir)
	provider = strings.TrimSpace(provider)
	if agentDir == "" {
		return errors.New("agent directory is required")
	}
	if provider == "" {
		return errors.New("provider is required")
	}
	authFileMu.Lock()
	defer authFileMu.Unlock()
	root, err := readAuthRoot(agentDir)
	if err != nil {
		return err
	}
	if _, ok := root[provider]; !ok {
		return nil
	}
	delete(root, provider)
	return writeAuthRoot(agentDir, root)
}

func readAuthRoot(agentDir string) (map[string]json.RawMessage, error) {
	root := map[string]json.RawMessage{}
	body, err := os.ReadFile(filepath.Join(agentDir, "auth.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return root, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	return root, nil
}

func writeAuthRoot(agentDir string, root map[string]json.RawMessage) error {
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	temp, err := os.CreateTemp(agentDir, ".auth-*.json")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	path := filepath.Join(agentDir, "auth.json")
	if err := os.Rename(name, path); err != nil {
		if runtime.GOOS != "windows" {
			return err
		}
		// Windows cannot replace an existing file with Rename.
		if removeErr := os.Remove(path); removeErr == nil || os.IsNotExist(removeErr) {
			if retryErr := os.Rename(name, path); retryErr != nil {
				return retryErr
			}
			return nil
		}
		return err
	}
	return nil
}
