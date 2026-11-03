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
	path := filepath.Join(agentDir, "auth.json")
	root := map[string]json.RawMessage{}
	if body, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(body, &root); err != nil {
			return err
		}
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	root[provider] = encoded
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
	return os.Rename(name, path)
}
