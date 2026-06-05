package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/llm"
	"github.com/conray/dataseai/internal/store"
)

// startResp gives the client everything it needs to open the auth window and
// later post the code back. Verifier + state are stored on the client side so
// the server can stay stateless between /start and /exchange.
type claudeStartResp struct {
	AuthURL  string `json:"auth_url"`
	Verifier string `json:"verifier"`
	State    string `json:"state"`
}

func handleClaudeCodeStart(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = auth.UserFromContext(r.Context())
		pair, err := llm.NewPKCEPair()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		state, err := llm.RandomState()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, claudeStartResp{
			AuthURL:  llm.BuildAuthorizeURL(pair.Challenge, state),
			Verifier: pair.Verifier,
			State:    state,
		})
	}
}

func handleClaudeCodeExchange(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var body struct {
			Code     string `json:"code"`
			Verifier string `json:"verifier"`
			State    string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" || body.Verifier == "" {
			writeError(w, http.StatusBadRequest, "bad body")
			return
		}
		// Accept the full callback URL OR the bare code.
		code, pastedState := llm.ExtractCodeFromPaste(body.Code)
		if code == "" {
			writeError(w, http.StatusBadRequest, "no code in paste")
			return
		}
		if pastedState != "" && pastedState != body.State {
			writeError(w, http.StatusBadRequest, "state mismatch — please restart Connect Claude")
			return
		}
		tokens, err := llm.ExchangeCode(nil, code, body.Verifier, body.State)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		expMs := int64(0)
		if tokens.ExpiresIn > 0 {
			expMs = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).UnixMilli()
		}
		if err := d.Store.SetClaudeCodeTokens(d.Cipher, u.ID, store.ClaudeCodeTokens{
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
			ExpiresAtMs:  expMs,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"expires_at_ms": expMs,
		})
	}
}

func handleClaudeCodeDisconnect(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		if err := d.Store.SetClaudeCodeTokens(d.Cipher, u.ID, store.ClaudeCodeTokens{}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
