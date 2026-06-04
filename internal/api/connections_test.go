package api

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/conray/dataseai/internal/crypto"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

func newCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	c, err := crypto.New(key)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func newTestRouterWithCipher(t *testing.T) (http.Handler, *store.Store, *crypto.Cipher) {
	t.Helper()
	db, _ := sql.Open("sqlite3", ":memory:")
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{DB: db}
	c := newCipher(t)
	pool := mysql.NewPool(mysql.PoolConfig{})
	r := NewRouter(Deps{Version: "test", Store: s, Cipher: c, Pool: pool, Registration: "open"})
	return r, s, c
}

func TestCreateConnection_HappyPath(t *testing.T) {
	r, _, _ := newTestRouterWithCipher(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	body := map[string]any{
		"name": "prod", "host": "db.example.com", "port": 3306,
		"username": "app", "password": "shhh!",
	}
	rec := post(t, r, "/api/connections", body, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Connection struct {
			ID   int64
			Name string
			Host string
		} `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if got.Connection.Name != "prod" || got.Connection.Host != "db.example.com" {
		t.Fatalf("body = %+v", got)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("shhh!")) {
		t.Fatal("plaintext password leaked in response")
	}
}

func TestListConnections_ScopedToUser(t *testing.T) {
	r, _, _ := newTestRouterWithCipher(t)
	aliceTok := registerAndLogin(t, r, "alice", "supersecret123")
	bobTok := registerAndLogin(t, r, "bob", "anothersecret456")
	_ = post(t, r, "/api/connections", map[string]any{"name": "a-prod", "host": "h", "port": 3306, "username": "u", "password": "p"}, aliceTok)
	_ = post(t, r, "/api/connections", map[string]any{"name": "b-prod", "host": "h", "port": 3306, "username": "u", "password": "p"}, bobTok)
	rec := get(t, r, "/api/connections", aliceTok)
	var got struct {
		Connections []struct{ Name string } `json:"connections"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if len(got.Connections) != 1 || got.Connections[0].Name != "a-prod" {
		t.Fatalf("alice sees: %+v", got.Connections)
	}
}

func TestCreateConnection_RequiresAuth(t *testing.T) {
	r, _, _ := newTestRouterWithCipher(t)
	rec := post(t, r, "/api/connections", map[string]any{"name": "x", "host": "h", "port": 3306, "username": "u", "password": "p"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestCreateConnection_DuplicateName(t *testing.T) {
	r, _, _ := newTestRouterWithCipher(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	body := map[string]any{"name": "prod", "host": "h", "port": 3306, "username": "u", "password": "p"}
	_ = post(t, r, "/api/connections", body, tok)
	rec := post(t, r, "/api/connections", body, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestGetConnection(t *testing.T) {
	r, _, _ := newTestRouterWithCipher(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{"name": "prod", "host": "h", "port": 3306, "username": "u", "password": "p"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	rec = get(t, r, "/api/connections/"+itoa(created.Connection.ID), tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetConnection_CrossUserHidden(t *testing.T) {
	r, _, _ := newTestRouterWithCipher(t)
	aliceTok := registerAndLogin(t, r, "alice", "supersecret123")
	bobTok := registerAndLogin(t, r, "bob", "anothersecret456")
	rec := post(t, r, "/api/connections", map[string]any{"name": "a-prod", "host": "h", "port": 3306, "username": "u", "password": "p"}, aliceTok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	rec = get(t, r, "/api/connections/"+itoa(created.Connection.ID), bobTok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d (bob must not see alice's conn)", rec.Code)
	}
}

func TestUpdateConnection_KeepsPasswordWhenEmpty(t *testing.T) {
	r, s, c := newTestRouterWithCipher(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{"name": "prod", "host": "h", "port": 3306, "username": "u", "password": "orig-pw"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	rec = putJSON(t, r, "/api/connections/"+itoa(created.Connection.ID), map[string]any{"name": "prod", "host": "h2", "port": 3306, "username": "u", "password": ""}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	pw, err := s.GetConnectionPassword(c, userIDOfAlice(s), created.Connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pw != "orig-pw" {
		t.Fatalf("password clobbered: %q", pw)
	}
}

func TestDeleteConnection(t *testing.T) {
	r, _, _ := newTestRouterWithCipher(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{"name": "prod", "host": "h", "port": 3306, "username": "u", "password": "p"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	rec = delete_(t, r, "/api/connections/"+itoa(created.Connection.ID), tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d", rec.Code)
	}
	rec = get(t, r, "/api/connections/"+itoa(created.Connection.ID), tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("post-delete code = %d", rec.Code)
	}
}

func TestTestConnection_PassesOpenError(t *testing.T) {
	r, _, _ := newTestRouterWithCipher(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{
		"name": "prod", "host": "127.0.0.1", "port": 65535,
		"username": "u", "password": "p",
	}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	rec = post(t, r, "/api/connections/"+itoa(created.Connection.ID)+"/test", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("test endpoint should always return 200 even on failure, got %d", rec.Code)
	}
	var got struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if got.OK {
		t.Fatalf("expected ok=false for unreachable host, got %+v", got)
	}
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

func userIDOfAlice(s *store.Store) int64 {
	row := s.DB.QueryRow("SELECT id FROM users WHERE username='alice'")
	var id int64
	_ = row.Scan(&id)
	return id
}
