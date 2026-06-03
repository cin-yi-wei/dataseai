package auth

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/conray/mysqlweb/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

func newStore(t *testing.T) *store.Store {
	db, _ := sql.Open("sqlite3", ":memory:")
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return &store.Store{DB: db}
}

func handlerThatReadsUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFromContext(r.Context())
		if !ok {
			http.Error(w, "no user", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(u.Username))
	})
}

func TestMiddleware_RejectsMissingHeader(t *testing.T) {
	s := newStore(t)
	mw := Middleware(s)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mw(handlerThatReadsUser()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestMiddleware_RejectsBadToken(t *testing.T) {
	s := newStore(t)
	mw := Middleware(s)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	mw(handlerThatReadsUser()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestMiddleware_AcceptsValidToken_InjectsUser(t *testing.T) {
	s := newStore(t)
	u, _ := s.CreateUser("alice", "supersecret123")
	sess, _ := s.CreateSession(u.ID, "ua", time.Hour)
	mw := Middleware(s)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	mw(handlerThatReadsUser()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "alice" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}
