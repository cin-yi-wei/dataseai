package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

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

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// handleGetEmail returns the logged-in user's stored email (for Settings).
func handleGetEmail(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		email, err := d.Store.EmailByID(u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"email": email})
	}
}

type setEmailReq struct {
	Email string `json:"email"`
}

// handleSetEmail sets/clears the logged-in user's email (used for reset codes).
func handleSetEmail(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var req setEmailReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.Email = strings.TrimSpace(req.Email)
		if req.Email != "" && !emailRE.MatchString(req.Email) {
			writeError(w, http.StatusBadRequest, "invalid email")
			return
		}
		if err := d.Store.SetEmail(u.ID, req.Email); err != nil {
			writeError(w, http.StatusInternalServerError, "save failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
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

// handleAuthConfig exposes non-secret feature flags the login UI needs before
// the user is authenticated: whether to show the "forgot password" link, and
// whether reset uses the email-code flow (true) or the unconditional local
// flow (false, single-user desktop GUI).
func handleAuthConfig(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"forgot_password": d.ForgotPasswordEnabled,
			"email_reset":     d.Mailer != nil,
		})
	}
}

const resetCodeTTL = 15 * time.Minute

// genResetCode returns a random 6-digit numeric code.
func genResetCode() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	n := binary.BigEndian.Uint32(b[:]) % 1000000
	return fmt.Sprintf("%06d", n)
}

// hashResetCode is the at-rest form of a reset code (never store the code).
func hashResetCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

type forgotPasswordReq struct {
	// Username identifies the account. In the email-code flow it may also be
	// an email address (LookupForReset accepts either).
	Username string `json:"username"`
	New      string `json:"new"` // used only by the unconditional (GUI) flow
}

// handleForgotPassword serves step 1 of self-serve reset. With a mailer
// configured it issues a one-time code by email (never revealing whether the
// account exists). Without a mailer (single-user desktop GUI) it falls back to
// the unconditional reset: username + new password, no verification.
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

		// Email-code flow.
		if d.Mailer != nil {
			userID, email, ok := d.Store.LookupForReset(req.Username)
			if ok {
				code := genResetCode()
				now := time.Now()
				if err := d.Store.CreateResetCode(userID, hashResetCode(code), now, resetCodeTTL); err == nil {
					body := fmt.Sprintf("Your DataseAI password reset code is %s\nIt expires in 15 minutes. If you didn't request this, ignore this email.", code)
					// Best-effort send; failures are logged server-side but the
					// response stays uniform so accounts can't be enumerated.
					_ = d.Mailer.Send(r.Context(), email, "DataseAI password reset code", body)
				}
			}
			// Always 204 regardless of whether the account/email existed.
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Unconditional flow (desktop GUI, no mailer).
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
		if err := d.Store.DeleteUserSessionsExcept(u.ID, ""); err != nil {
			writeError(w, http.StatusInternalServerError, "session cleanup failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type resetPasswordReq struct {
	Username string `json:"username"`
	Code     string `json:"code"`
	New      string `json:"new"`
}

// handleResetPassword serves step 2 of the email-code flow: verify the code and
// set the new password. Mounted only when a mailer is configured.
func handleResetPassword(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req resetPasswordReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		req.Code = strings.TrimSpace(req.Code)
		if req.Username == "" || req.Code == "" {
			writeError(w, http.StatusBadRequest, "username and code required")
			return
		}
		if err := validatePassword(req.New); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		userID, _, ok := d.Store.LookupForReset(req.Username)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid or expired code")
			return
		}
		now := time.Now()
		if err := d.Store.UseResetCode(userID, hashResetCode(req.Code), now); err != nil {
			writeError(w, http.StatusBadRequest, "invalid or expired code")
			return
		}
		if err := d.Store.UpdatePassword(userID, req.New); err != nil {
			writeError(w, http.StatusInternalServerError, "reset failed")
			return
		}
		// Revoke all existing sessions after a reset.
		if err := d.Store.DeleteUserSessionsExcept(userID, ""); err != nil {
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
