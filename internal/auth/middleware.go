package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/conray/mysqlweb/internal/store"
)

type ctxKey string

const userKey ctxKey = "user"
const sessionKey ctxKey = "session"

const SessionTTL = 30 * 24 * time.Hour

type UserCtx struct {
	ID       int64
	Username string
}

func UserFromContext(ctx context.Context) (UserCtx, bool) {
	u, ok := ctx.Value(userKey).(UserCtx)
	return u, ok
}

func SessionFromContext(ctx context.Context) (store.Session, bool) {
	s, ok := ctx.Value(sessionKey).(store.Session)
	return s, ok
}

func Middleware(s *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			tok := strings.TrimPrefix(h, "Bearer ")
			sess, err := s.GetSession(tok)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrSessionExpired) {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			u, err := s.GetUserByID(sess.UserID)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_ = s.RefreshSession(tok, SessionTTL)
			ctx := context.WithValue(r.Context(), userKey, UserCtx{ID: u.ID, Username: u.Username})
			ctx = context.WithValue(ctx, sessionKey, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
