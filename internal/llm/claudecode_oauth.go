package llm

// Claude Code OAuth flow — based on the public PKCE flow Claude Code's CLI
// uses. Anthropic does not officially support third-party OAuth clients;
// the constants below are extracted from the open-source CLI surface and are
// known to drive a working sign-in. If Anthropic changes them, this provider
// will need to be updated.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	// Constants extracted from `claude auth login` in Claude Code 2.1.165.
	// Anthropic shifted the OAuth host to claude.com/cai and added a hosted
	// callback page on platform.claude.com that displays the code post-auth
	// (no localhost failed-page anymore). Token endpoint also moved.
	ClaudeCodeClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	ClaudeCodeAuthorizeURL = "https://claude.com/cai/oauth/authorize"
	ClaudeCodeTokenURL     = "https://platform.claude.com/oauth/token"
	ClaudeCodeRedirectURI  = "https://platform.claude.com/oauth/code/callback"
	ClaudeCodeScopes       = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
)

// PKCEPair is the verifier/challenge couple required by the auth code flow.
type PKCEPair struct {
	Verifier  string `json:"verifier"`
	Challenge string `json:"challenge"`
}

// NewPKCEPair produces a fresh PKCE verifier + S256 challenge.
func NewPKCEPair() (PKCEPair, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return PKCEPair{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return PKCEPair{Verifier: verifier, Challenge: challenge}, nil
}

// RandomState returns a URL-safe random string suitable for the OAuth `state`
// parameter. 32 bytes (43 base64url chars) matches Claude Code CLI's output;
// Anthropic's authorize endpoint rejects shorter state values with
// "Invalid request format".
func RandomState() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// BuildAuthorizeURL composes the Claude Code authorization URL. The `code=true`
// flag opts into the hosted callback page (platform.claude.com/oauth/code/
// callback) that displays the code visibly after sign-in instead of redirecting
// to a localhost address.
//
// Query parameter ORDER matches Claude Code 2.1's CLI output byte-for-byte:
// claude.com's authorize endpoint rejects requests whose params arrive in a
// different order with "Invalid request format". url.Values.Encode() sorts
// keys alphabetically, so we manually build the query string instead.
func BuildAuthorizeURL(challenge, state string) string {
	params := []struct{ k, v string }{
		{"code", "true"},
		{"client_id", ClaudeCodeClientID},
		{"response_type", "code"},
		{"redirect_uri", ClaudeCodeRedirectURI},
		{"scope", ClaudeCodeScopes},
		{"code_challenge", challenge},
		{"code_challenge_method", "S256"},
		{"state", state},
	}
	var b strings.Builder
	b.WriteString(ClaudeCodeAuthorizeURL)
	b.WriteByte('?')
	for i, p := range params {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(p.k)
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(p.v))
	}
	return b.String()
}

// ExtractCodeFromPaste accepts either:
//   - the bare code (anything not starting with `http`)
//   - a full callback URL like `http://localhost/callback?code=...&state=...`
//
// Returns (code, state) — state may be empty if not present.
func ExtractCodeFromPaste(input string) (code, state string) {
	s := strings.TrimSpace(input)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		if u, err := url.Parse(s); err == nil {
			return u.Query().Get("code"), u.Query().Get("state")
		}
	}
	// Anthropic's older flow shows `<code>#<state>` on the callback page; if
	// the user copies that, split it.
	if i := strings.Index(s, "#"); i > 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// TokenResponse mirrors the JSON returned by /v1/oauth/token.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// ExchangeCode swaps the authorization code (possibly suffixed with `#state`
// as Anthropic's callback page shows it) plus the original PKCE verifier for
// access + refresh tokens.
func ExchangeCode(httpClient *http.Client, code, verifier, state string) (TokenResponse, error) {
	// Anthropic's callback shows `<code>#<state>`; users sometimes paste the
	// whole thing. Split it back into the bare code if present.
	if i := strings.Index(code, "#"); i > 0 {
		code = code[:i]
	}
	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          strings.TrimSpace(code),
		"redirect_uri":  ClaudeCodeRedirectURI,
		"client_id":     ClaudeCodeClientID,
		"code_verifier": verifier,
		"state":         state,
	}
	return postToken(httpClient, body)
}

// RefreshTokens uses a refresh_token to mint a fresh access_token. Anthropic
// rotates the refresh token in most responses, so callers must persist the
// returned RefreshToken too.
func RefreshTokens(httpClient *http.Client, refreshToken string) (TokenResponse, error) {
	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     ClaudeCodeClientID,
	}
	return postToken(httpClient, body)
}

func postToken(httpClient *http.Client, body map[string]string) (TokenResponse, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	bs, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", ClaudeCodeTokenURL, strings.NewReader(string(bs)))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("claudecode oauth %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var out TokenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return TokenResponse{}, fmt.Errorf("claudecode oauth parse: %w (body=%s)", err, truncate(string(raw), 200))
	}
	if out.AccessToken == "" {
		return TokenResponse{}, errors.New("claudecode oauth: no access_token in response")
	}
	return out, nil
}
