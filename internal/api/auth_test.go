package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conray/dataseai/internal/store"
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

// newForgotRouter is a test router with the self-serve reset endpoint enabled.
func newForgotRouter(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	db, _ := sql.Open("sqlite3", ":memory:")
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{DB: db}
	return NewRouter(Deps{Version: "test", Store: s, Registration: "open", ForgotPasswordEnabled: true}), s
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

func putJSON(t *testing.T, h http.Handler, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestChangePassword_HappyPath_RevokesOtherSessions(t *testing.T) {
	r, s := newTestRouter(t)
	tok1 := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/auth/login", map[string]string{"username": "alice", "password": "supersecret123"}, "")
	var body struct{ Token string }
	_ = json.NewDecoder(rec.Body).Decode(&body)
	tok2 := body.Token

	rec = putJSON(t, r, "/api/auth/password", map[string]string{
		"old": "supersecret123",
		"new": "anothersecret456",
	}, tok1)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	if rc := get(t, r, "/api/auth/me", tok1).Code; rc != http.StatusOK {
		t.Fatalf("tok1 me = %d (should still work)", rc)
	}
	if rc := get(t, r, "/api/auth/me", tok2).Code; rc != http.StatusUnauthorized {
		t.Fatalf("tok2 me = %d (should be revoked)", rc)
	}
	rec = post(t, r, "/api/auth/login", map[string]string{"username": "alice", "password": "anothersecret456"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("relogin code = %d", rec.Code)
	}
	_ = s
}

func TestChangePassword_RejectsWrongOld(t *testing.T) {
	r, _ := newTestRouter(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := putJSON(t, r, "/api/auth/password", map[string]string{
		"old": "wrongone1",
		"new": "anothersecret456",
	}, tok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestChangePassword_RejectsWeakNew(t *testing.T) {
	r, _ := newTestRouter(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := putJSON(t, r, "/api/auth/password", map[string]string{
		"old": "supersecret123",
		"new": "weak",
	}, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func delete_(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestListSessions(t *testing.T) {
	r, _ := newTestRouter(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	_ = post(t, r, "/api/auth/login", map[string]string{"username": "alice", "password": "supersecret123"}, "")
	rec := get(t, r, "/api/auth/sessions", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var body struct {
		Sessions []map[string]any `json:"sessions"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.Sessions) != 2 {
		t.Fatalf("got %d sessions", len(body.Sessions))
	}
	for _, s := range body.Sessions {
		if _, ok := s["token"]; ok {
			t.Fatalf("session leaked full token: %+v", s)
		}
	}
}

func TestRevokeSession(t *testing.T) {
	r, _ := newTestRouter(t)
	tok1 := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/auth/login", map[string]string{"username": "alice", "password": "supersecret123"}, "")
	var body struct{ Token string }
	_ = json.NewDecoder(rec.Body).Decode(&body)
	tok2 := body.Token

	rec = get(t, r, "/api/auth/sessions", tok1)
	var list struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&list)
	var tok2ID string
	for _, s := range list.Sessions {
		if len(s.ID) >= 8 && s.ID == tok2[:len(s.ID)] {
			tok2ID = s.ID
		}
	}
	if tok2ID == "" {
		t.Fatalf("tok2 not found in list")
	}
	rec = delete_(t, r, "/api/auth/sessions/"+tok2ID, tok1)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke code = %d", rec.Code)
	}
	if rc := get(t, r, "/api/auth/me", tok2).Code; rc != http.StatusUnauthorized {
		t.Fatalf("tok2 should be revoked, got %d", rc)
	}
}

func TestForgotPassword_ResetsAndLogsIn(t *testing.T) {
	r, _ := newForgotRouter(t)
	if rc := post(t, r, "/api/auth/register", map[string]string{"username": "adam", "password": "oldpass123"}, "").Code; rc != http.StatusOK {
		t.Fatalf("register code = %d", rc)
	}
	// 無條件重設，不帶舊密碼
	rec := post(t, r, "/api/auth/forgot-password", map[string]string{"username": "adam", "new": "newpass456"}, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("forgot code = %d body=%s", rec.Code, rec.Body.String())
	}
	// 舊密碼失效
	if rc := post(t, r, "/api/auth/login", map[string]string{"username": "adam", "password": "oldpass123"}, "").Code; rc != http.StatusUnauthorized {
		t.Fatalf("old password should fail, got %d", rc)
	}
	// 新密碼可登入
	if rc := post(t, r, "/api/auth/login", map[string]string{"username": "adam", "password": "newpass456"}, "").Code; rc != http.StatusOK {
		t.Fatalf("new password should log in, got %d", rc)
	}
}

func TestForgotPassword_DisabledByDefault(t *testing.T) {
	// Default router (flag off) must not expose the reset endpoint at all.
	r, _ := newTestRouter(t)
	_ = post(t, r, "/api/auth/register", map[string]string{"username": "adam", "password": "oldpass123"}, "")
	rec := post(t, r, "/api/auth/forgot-password", map[string]string{"username": "adam", "new": "newpass456"}, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled forgot-password should 404, got %d", rec.Code)
	}
}

func TestForgotPassword_UnknownUser(t *testing.T) {
	r, _ := newForgotRouter(t)
	rec := post(t, r, "/api/auth/forgot-password", map[string]string{"username": "nobody", "new": "newpass456"}, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestForgotPassword_RejectsWeakPassword(t *testing.T) {
	r, _ := newForgotRouter(t)
	_ = post(t, r, "/api/auth/register", map[string]string{"username": "adam", "password": "oldpass123"}, "")
	rec := post(t, r, "/api/auth/forgot-password", map[string]string{"username": "adam", "new": "short"}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestForgotPassword_InvalidatesExistingSessions(t *testing.T) {
	r, _ := newForgotRouter(t)
	reg := post(t, r, "/api/auth/register", map[string]string{"username": "adam", "password": "oldpass123"}, "")
	var body struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(reg.Body).Decode(&body)
	if rc := get(t, r, "/api/auth/me", body.Token).Code; rc != http.StatusOK {
		t.Fatalf("session should be valid pre-reset, got %d", rc)
	}
	_ = post(t, r, "/api/auth/forgot-password", map[string]string{"username": "adam", "new": "newpass456"}, "")
	if rc := get(t, r, "/api/auth/me", body.Token).Code; rc != http.StatusUnauthorized {
		t.Fatalf("session should be revoked post-reset, got %d", rc)
	}
}
