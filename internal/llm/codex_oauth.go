package llm

// Codex / ChatGPT OAuth flow. Mirrors what `codex login` does locally —
// constants extracted from a live run of the CLI's launch URL. Same caveats
// as Claude Code: this impersonates the Codex CLI's OAuth client, so it's
// suitable for personal / small-team use only.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	CodexClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	CodexTokenURL     = "https://auth.openai.com/oauth/token"
	CodexRedirectURI  = "http://localhost:1455/auth/callback"
	CodexScopes       = "openid profile email offline_access api.connectors.read api.connectors.invoke"
)

// BuildCodexAuthorizeURL composes the Codex authorization URL with PKCE.
func BuildCodexAuthorizeURL(challenge, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", CodexClientID)
	q.Set("redirect_uri", CodexRedirectURI)
	q.Set("scope", CodexScopes)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("state", state)
	q.Set("originator", "codex_cli_rs")
	return CodexAuthorizeURL + "?" + q.Encode()
}

// ExchangeCodexCode trades the auth code for tokens. The simplified-flow flag
// gets us an api-scoped access_token directly without a second token-exchange
// hop.
func ExchangeCodexCode(httpClient *http.Client, code, verifier, state string) (TokenResponse, error) {
	if i := strings.Index(code, "#"); i > 0 {
		code = code[:i]
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(code))
	form.Set("redirect_uri", CodexRedirectURI)
	form.Set("client_id", CodexClientID)
	form.Set("code_verifier", verifier)
	form.Set("state", state)
	return postCodexToken(httpClient, form)
}

func RefreshCodexTokens(httpClient *http.Client, refreshToken string) (TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", CodexClientID)
	form.Set("scope", CodexScopes)
	return postCodexToken(httpClient, form)
}

func postCodexToken(httpClient *http.Client, form url.Values) (TokenResponse, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequest("POST", CodexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("codex oauth %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var out TokenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return TokenResponse{}, fmt.Errorf("codex oauth parse: %w (body=%s)", err, truncate(string(raw), 200))
	}
	if out.AccessToken == "" {
		return TokenResponse{}, errors.New("codex oauth: no access_token in response")
	}
	return out, nil
}
