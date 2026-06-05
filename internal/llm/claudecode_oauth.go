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
	ClaudeCodeClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	ClaudeCodeAuthorizeURL = "https://claude.ai/oauth/authorize"
	ClaudeCodeTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	ClaudeCodeRedirectURI  = "https://console.anthropic.com/oauth/code/callback"
	ClaudeCodeScopes       = "org:create_api_key user:profile user:inference"
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
// parameter.
func RandomState() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// BuildAuthorizeURL composes the Claude Code authorization URL with the given
// PKCE challenge + state.
func BuildAuthorizeURL(challenge, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", ClaudeCodeClientID)
	q.Set("redirect_uri", ClaudeCodeRedirectURI)
	q.Set("scope", ClaudeCodeScopes)
	q.Set("code", "true") // Anthropic-specific: hints the callback page to display the code
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	return ClaudeCodeAuthorizeURL + "?" + q.Encode()
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
