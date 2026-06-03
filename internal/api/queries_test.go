package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/conray/mysqlweb/internal/mysql"
	"github.com/conray/mysqlweb/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

func newTestRouterWithRegistry(t *testing.T, registry *mysql.Registry) (http.Handler, *store.Store) {
	t.Helper()
	db, _ := sql.Open("sqlite3", ":memory:")
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{DB: db}
	c := newCipher(t)
	pool := mysql.NewPool(mysql.PoolConfig{
		Open: func(dsn string) (*sql.DB, error) {
			return sql.Open("sqlite3", ":memory:")
		},
	})
	r := NewRouter(Deps{Version: "test", Store: s, Cipher: c, Pool: pool, QueryRegistry: registry, Registration: "open"})
	return r, s
}

func TestActiveQueries_Empty(t *testing.T) {
	r, _ := newTestRouterWithRegistry(t, mysql.NewRegistry())
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := get(t, r, "/api/queries/active", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var body struct {
		Queries []map[string]any `json:"queries"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.Queries) != 0 {
		t.Fatalf("expected 0, got %d", len(body.Queries))
	}
}

func TestActiveQueries_PerUser(t *testing.T) {
	reg := mysql.NewRegistry()
	r, s := newTestRouterWithRegistry(t, reg)
	aliceTok := registerAndLogin(t, r, "alice", "supersecret123")
	_ = registerAndLogin(t, r, "bob", "anothersecret456")
	aliceID := userIDOfAlice(s)
	var bobID int64
	_ = s.DB.QueryRow("SELECT id FROM users WHERE username='bob'").Scan(&bobID)

	reg.Register("q1", 101, "SELECT alice", aliceID, 10)
	reg.Register("q2", 202, "SELECT bob", bobID, 11)

	rec := get(t, r, "/api/queries/active", aliceTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var body struct {
		Queries []map[string]any `json:"queries"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.Queries) != 1 || body.Queries[0]["query_id"] != "q1" {
		t.Fatalf("alice sees %+v", body.Queries)
	}
}
