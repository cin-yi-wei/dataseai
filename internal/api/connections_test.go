package api

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/conray/mysqlweb/internal/crypto"
	"github.com/conray/mysqlweb/internal/store"
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
	r := NewRouter(Deps{Version: "test", Store: s, Cipher: c, Registration: "open"})
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
