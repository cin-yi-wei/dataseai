package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// claudeCreds matches the layout of ~/.claude/.credentials.json that Claude
// Code writes after a successful `claude login`.
type claudeCreds struct {
	ClaudeAiOauth struct {
		AccessToken      string `json:"accessToken"`
		RefreshToken     string `json:"refreshToken"`
		ExpiresAt        int64  `json:"expiresAt"` // Unix millis
		SubscriptionType string `json:"subscriptionType"`
	} `json:"claudeAiOauth"`
}

// ClaudeCodeCredentialsPath is the file Claude Code stores its OAuth tokens
// in. Override the function for tests.
var ClaudeCodeCredentialsPath = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", ".credentials.json"), nil
}

// LoadClaudeCodeToken reads the OAuth access token Claude Code stores locally
// and returns it ready to use as a bearer credential against
// api.anthropic.com. Expired tokens come back as a clear error so the user
// knows to run `claude` again.
func LoadClaudeCodeToken() (string, error) {
	path, err := ClaudeCodeCredentialsPath()
	if err != nil {
		return "", fmt.Errorf("resolve credentials path: %w", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w (run `claude` to log in)", path, err)
	}
	var c claudeCreds
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	if c.ClaudeAiOauth.AccessToken == "" {
		return "", errors.New("no claudeAiOauth.accessToken in credentials file")
	}
	if c.ClaudeAiOauth.ExpiresAt > 0 {
		exp := time.UnixMilli(c.ClaudeAiOauth.ExpiresAt)
		if time.Now().After(exp) {
			return "", fmt.Errorf("claude code token expired at %s; run `claude` to refresh", exp.Format(time.RFC3339))
		}
	}
	return c.ClaudeAiOauth.AccessToken, nil
}
