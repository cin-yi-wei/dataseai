package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conray/mysqlweb/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

func newTestRouter(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	db, _ := sql.Open("sqlite3", ":memory:")
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{DB: db}
	return NewRouter(Deps{Version: "test", Store: s, Registration: "open"}), s
}

func post(t *testing.T, h http.Handler, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegister_HappyPath(t *testing.T) {
	r, _ := newTestRouter(t)
	rec := post(t, r, "/api/auth/register", map[string]string{
		"username": "alice", "password": "supersecret123",
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Token string `json:"token"`
		User  struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.Token == "" || body.User.Username != "alice" {
		t.Fatalf("body = %+v", body)
	}
}

func TestRegister_RejectsShortPassword(t *testing.T) {
	r, _ := newTestRouter(t)
	rec := post(t, r, "/api/auth/register", map[string]string{
		"username": "alice", "password": "short",
	}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	r, _ := newTestRouter(t)
	_ = post(t, r, "/api/auth/register", map[string]string{"username": "alice", "password": "supersecret123"}, "")
	rec := post(t, r, "/api/auth/register", map[string]string{"username": "alice", "password": "supersecret123"}, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestRegister_DisabledWhenClosed(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	t.Cleanup(func() { db.Close() })
	_ = store.Migrate(db)
	r := NewRouter(Deps{Version: "test", Store: &store.Store{DB: db}, Registration: "closed"})
	rec := post(t, r, "/api/auth/register", map[string]string{"username": "alice", "password": "supersecret123"}, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestLogin_HappyPath(t *testing.T) {
	r, _ := newTestRouter(t)
	_ = post(t, r, "/api/auth/register", map[string]string{"username": "alice", "password": "supersecret123"}, "")
	rec := post(t, r, "/api/auth/login", map[string]string{"username": "alice", "password": "supersecret123"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.Token == "" {
		t.Fatal("empty token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	r, _ := newTestRouter(t)
	_ = post(t, r, "/api/auth/register", map[string]string{"username": "alice", "password": "supersecret123"}, "")
	rec := post(t, r, "/api/auth/login", map[string]string{"username": "alice", "password": "wrongpw9999"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	r, _ := newTestRouter(t)
	rec := post(t, r, "/api/auth/login", map[string]string{"username": "ghost", "password": "supersecret123"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func registerAndLogin(t *testing.T, r http.Handler, username, password string) string {
	t.Helper()
	rec := post(t, r, "/api/auth/register", map[string]string{"username": username, "password": password}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("register failed: %d %s", rec.Code, rec.Body.String())
	}
	var body struct{ Token string }
	_ = json.NewDecoder(rec.Body).Decode(&body)
	return body.Token
}

func get(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMe_HappyPath(t *testing.T) {
	r, _ := newTestRouter(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := get(t, r, "/api/auth/me", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var body struct {
		User struct{ Username string }
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.User.Username != "alice" {
		t.Fatalf("body = %+v", body)
	}
}

func TestMe_RejectsNoToken(t *testing.T) {
	r, _ := newTestRouter(t)
	rec := get(t, r, "/api/auth/me", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestLogout_RevokesToken(t *testing.T) {
	r, _ := newTestRouter(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/auth/logout", nil, tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d", rec.Code)
	}
	rec = get(t, r, "/api/auth/me", tok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout /me code = %d", rec.Code)
	}
}
