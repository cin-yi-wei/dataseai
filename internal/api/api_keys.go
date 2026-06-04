package api

import (
	"encoding/json"
	"net/http"

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
			"anthropic": map[string]any{"set": keys.Anthropic != "", "masked": maskKey(keys.Anthropic)},
			"openai":    map[string]any{"set": keys.OpenAI != "", "masked": maskKey(keys.OpenAI)},
			"gemini":    map[string]any{"set": keys.Gemini != "", "masked": maskKey(keys.Gemini)},
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
		if body.Provider != "anthropic" && body.Provider != "openai" && body.Provider != "gemini" {
			writeError(w, http.StatusBadRequest, "unknown provider")
			return
		}
		if err := d.Store.SetUserAPIKey(d.Cipher, u.ID, body.Provider, body.Key); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
