package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/conray/dataseai/internal/auth"
)

// maskKey reveals only the last 4 chars of the key (or empty).
func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 4 {
		return "****"
	}
	return "****" + k[len(k)-4:]
}

func handleGetAPIKeys(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		keys, err := d.Store.GetUserAPIKeys(d.Cipher, u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Don't return the raw keys — return whether each is set + masked tail.
		writeJSON(w, http.StatusOK, map[string]any{
			"anthropic":  map[string]any{"set": keys.Anthropic != "", "masked": maskKey(keys.Anthropic)},
			"openai":     map[string]any{"set": keys.OpenAI != "", "masked": maskKey(keys.OpenAI)},
			"gemini":     map[string]any{"set": keys.Gemini != "", "masked": maskKey(keys.Gemini)},
			"claudecode": map[string]any{"set": keys.ClaudeCode != "", "masked": maskKey(keys.ClaudeCode)},
		})
	}
}

func handlePutAPIKey(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var body struct {
			Provider string `json:"provider"`
			Key      string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "bad body")
			return
		}
		if body.Provider != "anthropic" && body.Provider != "openai" && body.Provider != "gemini" && body.Provider != "claudecode" {
			writeError(w, http.StatusBadRequest, "unknown provider")
			return
		}
		// For Claude Code: accept either the bare accessToken or the full
		// contents of ~/.claude/.credentials.json — extract the token in the
		// latter case so the user can paste the file verbatim.
		key := body.Key
		if body.Provider == "claudecode" && key != "" {
			if extracted, err := extractClaudeCodeToken(key); err == nil {
				key = extracted
			} else {
				writeError(w, http.StatusBadRequest, "not a Claude Code OAuth token: "+err.Error())
				return
			}
		}
		if err := d.Store.SetUserAPIKey(d.Cipher, u.ID, body.Provider, key); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// extractClaudeCodeToken accepts either:
//   - a bare access token starting with `sk-ant-oat`
//   - the full JSON contents of ~/.claude/.credentials.json
//
// It returns the bare access token on success.
func extractClaudeCodeToken(input string) (string, error) {
	s := strings.TrimSpace(input)
	if strings.HasPrefix(s, "{") {
		var c struct {
			ClaudeAiOauth struct {
				AccessToken string `json:"accessToken"`
			} `json:"claudeAiOauth"`
		}
		if err := json.Unmarshal([]byte(s), &c); err == nil && c.ClaudeAiOauth.AccessToken != "" {
			return c.ClaudeAiOauth.AccessToken, nil
		}
		return "", errInvalidClaudeCodeToken
	}
	if strings.HasPrefix(s, "sk-ant-oat") {
		return s, nil
	}
	return "", errInvalidClaudeCodeToken
}

var errInvalidClaudeCodeToken = errInvalidClaudeCodeTokenT{}

type errInvalidClaudeCodeTokenT struct{}

func (errInvalidClaudeCodeTokenT) Error() string {
	return "expected sk-ant-oat... or the JSON content of ~/.claude/.credentials.json"
}
