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

// newTestRouterWithSqliteAsMySQL wires the Pool to open in-memory sqlite
// instead of MySQL, so tests can run without a MySQL server.
func newTestRouterWithSqliteAsMySQL(t *testing.T) (http.Handler, *store.Store) {
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
	r := NewRouter(Deps{Version: "test", Store: s, Cipher: c, Pool: pool, Registration: "open"})
	return r, s
}

func TestListDatabases_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := get(t, r, "/api/db/1/databases", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestListDatabases_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := get(t, r, "/api/db/999/databases", tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestListTables_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := get(t, r, "/api/db/1/databases/x/tables", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestListTables_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := get(t, r, "/api/db/999/databases/x/tables", tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestFetchTableRows_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := get(t, r, "/api/db/1/databases/x/tables/users/data", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestFetchTableRows_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := get(t, r, "/api/db/999/databases/x/tables/users/data", tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestListDatabases_DecodesShape(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{
		"name": "x", "host": "h", "port": 3306, "username": "u", "password": "p",
	}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	rec = get(t, r, "/api/db/"+itoa(created.Connection.ID)+"/databases", tok)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 (sqlite lacks information_schema), got %d body=%s", rec.Code, rec.Body.String())
	}
}
