package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

type registerReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func validatePassword(p string) error {
	if len(p) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	var hasLetter, hasDigit bool
	for _, r := range p {
		switch {
		case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z':
			hasLetter = true
		case '0' <= r && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errors.New("password must contain letters and digits")
	}
	return nil
}

func handleRegister(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Registration != "open" {
			writeError(w, http.StatusForbidden, "registration is closed")
			return
		}
		var req registerReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if !usernameRE.MatchString(req.Username) {
			writeError(w, http.StatusBadRequest, "username must be 3-32 chars [A-Za-z0-9_.-]")
			return
		}
		if err := validatePassword(req.Password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		u, err := d.Store.CreateUser(req.Username, req.Password)
		if err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				writeError(w, http.StatusConflict, "username already exists")
				return
			}
			writeError(w, http.StatusInternalServerError, "create user failed")
			return
		}
		sess, err := d.Store.CreateSession(u.ID, r.UserAgent(), auth.SessionTTL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create session failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"token": sess.Token,
			"user":  map[string]any{"id": u.ID, "username": u.Username},
		})
	}
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func handleLogin(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		u, err := d.Store.VerifyPassword(req.Username, req.Password)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		sess, err := d.Store.CreateSession(u.ID, r.UserAgent(), auth.SessionTTL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create session failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"token": sess.Token,
			"user":  map[string]any{"id": u.ID, "username": u.Username},
		})
	}
}

func handleMe(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"user": map[string]any{"id": u.ID, "username": u.Username, "is_admin": u.IsAdmin},
		})
	}
}

func handleLogout(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := auth.SessionFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err := d.Store.DeleteSession(sess.Token); err != nil {
			writeError(w, http.StatusInternalServerError, "delete session failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type passwordReq struct {
	Old string `json:"old"`
	New string `json:"new"`
}

func handlePasswordChange(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		sess, _ := auth.SessionFromContext(r.Context())
		var req passwordReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if _, err := d.Store.VerifyPassword(u.Username, req.Old); err != nil {
			writeError(w, http.StatusUnauthorized, "old password incorrect")
			return
		}
		if err := validatePassword(req.New); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := d.Store.UpdatePassword(u.ID, req.New); err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		if err := d.Store.DeleteUserSessionsExcept(u.ID, sess.Token); err != nil {
			writeError(w, http.StatusInternalServerError, "session cleanup failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type forgotPasswordReq struct {
	Username string `json:"username"`
	New      string `json:"new"`
}

// handleForgotPassword 無條件重設密碼（Phase 1）：輸入帳號 + 新密碼即直接改，
// 不做任何身分驗證。適用 GUI 單機自用場景。
// Phase 2 將改為需 email 驗證碼才能重設。
func handleForgotPassword(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req forgotPasswordReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" {
			writeError(w, http.StatusBadRequest, "username required")
			return
		}
		if err := validatePassword(req.New); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		u, err := d.Store.ResetPassword(req.Username, req.New)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "reset failed")
			return
		}
		// 重設後撤銷該帳號所有既有 session，強制重新登入。
		if err := d.Store.DeleteUserSessionsExcept(u.ID, ""); err != nil {
			writeError(w, http.StatusInternalServerError, "session cleanup failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func tokenPrefix(t string) string {
	if len(t) <= 8 {
		return t
	}
	return t[:8]
}

func handleListSessions(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		me, _ := auth.SessionFromContext(r.Context())
		list, err := d.Store.ListSessionsByUser(u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(list))
		for _, s := range list {
			out = append(out, map[string]any{
				"id":           tokenPrefix(s.Token),
				"user_agent":   s.UserAgent,
				"created_at":   s.CreatedAt,
				"last_used_at": s.LastUsedAt,
				"expires_at":   s.ExpiresAt,
				"current":      s.Token == me.Token,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
	}
}

func handleRevokeSession(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id := chi.URLParam(r, "id")
		list, err := d.Store.ListSessionsByUser(u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		var target string
		for _, s := range list {
			if strings.HasPrefix(s.Token, id) {
				target = s.Token
				break
			}
		}
		if target == "" {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		if err := d.Store.DeleteSession(target); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
