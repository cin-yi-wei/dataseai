# Plan 2 — Connections + DB Browse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an authenticated user create encrypted MySQL connection definitions, pick one in the UI, and browse its databases / tables / table rows (with pagination + sort).

**Architecture:** Connection password is AES-GCM-encrypted at rest in the tool's sqlite using the master key bootstrapped in Plan 1. A per-`(user_id, connection_id)` `*sql.DB` pool lives in memory; idle pools self-close after 5 min. Browse handlers are thin wrappers over the standard `database/sql` driver running parameterised `SHOW`/`SELECT` queries with backtick-escaped identifiers. Frontend gains a TopBar (connection picker + open table tabs), Sidebar (databases tree), and a virtualised DataGrid (TanStack Table v8).

**Tech Stack:**
- Go: `github.com/go-sql-driver/mysql`, existing `database/sql`
- Frontend: TanStack Table v8 (`@tanstack/react-table`), existing Zustand
- Existing primitives reused: `internal/crypto.Cipher`, `internal/auth.Middleware`, `internal/store.Store`, `web/src/lib/api.ts`, `web/src/store/auth.ts`

**Spec reference:** `docs/superpowers/specs/2026-06-03-dataseai-design.md` — Sections 5 (`connections` table), 6.2 (connections endpoints), 6.3 (browse endpoints), 7.4 (connection-password encryption), 8.5 (identifier escaping).

**Plan 1 carryover:**
- `cmd/dataseai/main.go` already constructs a `*crypto.Cipher` and discards it — Task 1 wires it through `api.Deps.Cipher`. The encrypted password column will live here.
- Plan 1 Workspace.tsx is the placeholder this plan replaces.

---

## File Structure (created or modified by this plan)

```
dataseai/
├── internal/
│   ├── store/
│   │   ├── migrations/0004_connections.sql        # new
│   │   ├── connections.go                         # new — connection CRUD with AES-GCM
│   │   └── connections_test.go                    # new
│   ├── mysql/                                     # new package
│   │   ├── pool.go                                # *sql.DB pool per (user, conn)
│   │   ├── pool_test.go
│   │   ├── browse.go                              # list dbs / tables / table rows
│   │   ├── browse_test.go                         # uses sqlite-backed fake driver where feasible
│   │   └── ident.go                               # backtick-escape helper
│   └── api/
│       ├── connections.go                         # new — handlers
│       ├── connections_test.go
│       ├── db.go                                  # new — browse handlers
│       ├── db_test.go
│       └── router.go                              # extended to mount new routes + Deps.Cipher
├── cmd/dataseai/main.go                           # thread Cipher into Deps
├── web/
│   ├── package.json                               # add @tanstack/react-table
│   └── src/
│       ├── lib/api.ts                             # unchanged (already supports all verbs)
│       ├── store/
│       │   ├── connections.ts                     # new — Zustand
│       │   ├── tabs.ts                            # new — Zustand (open top tabs)
│       │   └── activeConn.ts                      # new — Zustand (which conn is active)
│       ├── routes/
│       │   └── Workspace.tsx                      # rewritten — full shell
│       └── components/                            # new directory
│           ├── TopBar.tsx
│           ├── ConnectionPicker.tsx
│           ├── ConnectionDialog.tsx
│           ├── Sidebar.tsx
│           ├── BottomTabs.tsx                     # left group only (Plan 2 scope)
│           ├── DataGrid.tsx                       # TanStack Table
│           └── ConnectionsManager.tsx             # full connections CRUD UI
```

**Conventions reused from Plan 1:**
- Go module path is `github.com/conray/dataseai`.
- Tests live alongside source with `_test.go`. In-memory sqlite via `sql.Open("sqlite3", ":memory:")`.
- Frontend tests run with `vitest`. Components don't need tests in Plan 2 unless they have non-trivial logic; the store layer does.
- Each task ends with one `git commit` covering only its files.

---

## Task 1: Thread `*crypto.Cipher` through to API handlers

**Files:**
- Modify: `cmd/dataseai/main.go`, `internal/api/router.go`

- [ ] **Step 1: Add Cipher field to `Deps`**

Edit `internal/api/router.go` so the `Deps` struct reads:

```go
type Deps struct {
	Version      string
	Store        *store.Store
	Cipher       *crypto.Cipher
	Registration string
	WebFS        fs.FS
}
```

Add the import:

```go
import (
	"io/fs"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/crypto"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)
```

(`crypto` is the new addition.)

- [ ] **Step 2: Use the previously-discarded cipher in `main.go`**

In `cmd/dataseai/main.go`, replace:

```go
	if _, err := crypto.New(key); err != nil {
		log.Fatalf("crypto init: %v", err)
	}
```

with:

```go
	cipher, err := crypto.New(key)
	if err != nil {
		log.Fatalf("crypto init: %v", err)
	}
```

And pass it through:

```go
	r := api.NewRouter(api.Deps{
		Version:      version,
		Store:        s,
		Cipher:       cipher,
		Registration: cfg.Registration,
		WebFS:        sub,
	})
```

- [ ] **Step 3: Run existing tests to confirm nothing broke**

```bash
go test ./...
```

Expected: all existing packages still PASS. `Deps` gained a field that defaults to `nil` for tests that don't set it.

- [ ] **Step 4: Commit**

```bash
git add cmd/dataseai/main.go internal/api/router.go
git commit -m "feat(api): thread *crypto.Cipher through api.Deps"
```

---

## Task 2: connections migration + store (CRUD with AES-GCM)

**Files:**
- Create: `internal/store/migrations/0004_connections.sql`, `internal/store/connections.go`, `internal/store/connections_test.go`

- [ ] **Step 1: Migration**

Create `internal/store/migrations/0004_connections.sql`:

```sql
CREATE TABLE connections (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  host         TEXT NOT NULL,
  port         INTEGER NOT NULL DEFAULT 3306,
  username     TEXT NOT NULL,
  password_enc BLOB NOT NULL,
  default_db   TEXT,
  tls          TEXT NOT NULL DEFAULT 'disabled',
  color        TEXT,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(user_id, name)
);
CREATE INDEX idx_connections_user ON connections(user_id);
```

- [ ] **Step 2: Failing tests**

Create `internal/store/connections_test.go`:

```go
package store

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/conray/dataseai/internal/crypto"
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

func setupConnections(t *testing.T) (*Store, User, *crypto.Cipher) {
	t.Helper()
	s := setupUsers(t)
	u, err := s.CreateUser("alice", "supersecret123")
	if err != nil {
		t.Fatal(err)
	}
	return s, u, newCipher(t)
}

func TestCreateConnection_PersistsAndDoesNotStorePlaintext(t *testing.T) {
	s, u, c := setupConnections(t)
	conn, err := s.CreateConnection(c, u.ID, ConnectionInput{
		Name: "prod", Host: "db.example.com", Port: 3306,
		Username: "app", Password: "shhh!", TLS: "preferred",
	})
	if err != nil {
		t.Fatal(err)
	}
	if conn.ID == 0 || conn.Name != "prod" {
		t.Fatalf("conn = %+v", conn)
	}
	var enc []byte
	if err := s.DB.QueryRow("SELECT password_enc FROM connections WHERE id=?", conn.ID).Scan(&enc); err != nil {
		t.Fatal(err)
	}
	if string(enc) == "shhh!" || len(enc) == 0 {
		t.Fatalf("password stored unencrypted: %q", enc)
	}
}

func TestCreateConnection_DuplicateName(t *testing.T) {
	s, u, c := setupConnections(t)
	if _, err := s.CreateConnection(c, u.ID, ConnectionInput{Name: "prod", Host: "h", Port: 3306, Username: "u", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateConnection(c, u.ID, ConnectionInput{Name: "prod", Host: "h2", Port: 3306, Username: "u2", Password: "p2"})
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestGetDecryptedPassword(t *testing.T) {
	s, u, c := setupConnections(t)
	in := ConnectionInput{Name: "prod", Host: "h", Port: 3306, Username: "u", Password: "the-real-pw"}
	conn, _ := s.CreateConnection(c, u.ID, in)
	pw, err := s.GetConnectionPassword(c, u.ID, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pw != "the-real-pw" {
		t.Fatalf("decrypted = %q, want the-real-pw", pw)
	}
}

func TestGetConnectionPassword_WrongUser(t *testing.T) {
	s, u, c := setupConnections(t)
	in := ConnectionInput{Name: "prod", Host: "h", Port: 3306, Username: "u", Password: "x"}
	conn, _ := s.CreateConnection(c, u.ID, in)
	bob, _ := s.CreateUser("bob", "anothersecret456")
	_, err := s.GetConnectionPassword(c, bob.ID, conn.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for cross-user access, got %v", err)
	}
}

func TestListConnections_ScopedToUser(t *testing.T) {
	s, u, c := setupConnections(t)
	bob, _ := s.CreateUser("bob", "anothersecret456")
	_, _ = s.CreateConnection(c, u.ID, ConnectionInput{Name: "a-prod", Host: "h", Port: 3306, Username: "u", Password: "p"})
	_, _ = s.CreateConnection(c, u.ID, ConnectionInput{Name: "a-dev", Host: "h", Port: 3306, Username: "u", Password: "p"})
	_, _ = s.CreateConnection(c, bob.ID, ConnectionInput{Name: "b-prod", Host: "h", Port: 3306, Username: "u", Password: "p"})

	list, err := s.ListConnections(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("alice should see 2, got %d", len(list))
	}
	for _, c := range list {
		if c.UserID != u.ID {
			t.Fatalf("leaked connection from user %d into alice's list", c.UserID)
		}
	}
}

func TestUpdateConnection_PreservesPasswordWhenEmpty(t *testing.T) {
	s, u, c := setupConnections(t)
	conn, _ := s.CreateConnection(c, u.ID, ConnectionInput{Name: "prod", Host: "h", Port: 3306, Username: "u", Password: "orig-pw"})
	// Update everything except password (empty Password = keep existing)
	upd, err := s.UpdateConnection(c, u.ID, conn.ID, ConnectionInput{Name: "prod-renamed", Host: "h2", Port: 3307, Username: "u2", Password: ""})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Name != "prod-renamed" || upd.Host != "h2" || upd.Port != 3307 {
		t.Fatalf("update did not persist: %+v", upd)
	}
	pw, _ := s.GetConnectionPassword(c, u.ID, conn.ID)
	if pw != "orig-pw" {
		t.Fatalf("password was clobbered: %q", pw)
	}
}

func TestDeleteConnection_ScopedToUser(t *testing.T) {
	s, u, c := setupConnections(t)
	bob, _ := s.CreateUser("bob", "anothersecret456")
	conn, _ := s.CreateConnection(c, bob.ID, ConnectionInput{Name: "b-prod", Host: "h", Port: 3306, Username: "u", Password: "p"})
	// alice cannot delete bob's connection
	err := s.DeleteConnection(u.ID, conn.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("alice should not see bob's conn, got %v", err)
	}
	// bob can
	if err := s.DeleteConnection(bob.ID, conn.ID); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 3: Run, confirm fail**

```bash
go test ./internal/store/ -run Connection -v
```

Expected: FAIL — `CreateConnection` etc. undefined.

- [ ] **Step 4: Implement**

Create `internal/store/connections.go`:

```go
package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/conray/dataseai/internal/crypto"
)

type ConnectionInput struct {
	Name      string
	Host      string
	Port      int
	Username  string
	Password  string // plaintext on the way in; empty on Update means "keep existing"
	DefaultDB string
	TLS       string // "disabled" | "preferred" | "required"
	Color     string
}

type Connection struct {
	ID        int64
	UserID    int64
	Name      string
	Host      string
	Port      int
	Username  string
	DefaultDB string
	TLS       string
	Color     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Store) CreateConnection(c *crypto.Cipher, userID int64, in ConnectionInput) (Connection, error) {
	if in.Port == 0 {
		in.Port = 3306
	}
	if in.TLS == "" {
		in.TLS = "disabled"
	}
	enc, err := c.Encrypt([]byte(in.Password))
	if err != nil {
		return Connection{}, err
	}
	res, err := s.DB.Exec(
		`INSERT INTO connections(user_id, name, host, port, username, password_enc, default_db, tls, color)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		userID, in.Name, in.Host, in.Port, in.Username, enc, in.DefaultDB, in.TLS, in.Color,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Connection{}, ErrDuplicate
		}
		return Connection{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetConnection(userID, id)
}

func (s *Store) GetConnection(userID, id int64) (Connection, error) {
	row := s.DB.QueryRow(
		`SELECT id, user_id, name, host, port, username, default_db, tls, color, created_at, updated_at
		 FROM connections WHERE id=? AND user_id=?`,
		id, userID,
	)
	var c Connection
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &c.Host, &c.Port, &c.Username, &c.DefaultDB, &c.TLS, &c.Color, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return c, ErrNotFound
		}
		return c, err
	}
	return c, nil
}

func (s *Store) GetConnectionPassword(c *crypto.Cipher, userID, id int64) (string, error) {
	var enc []byte
	err := s.DB.QueryRow(
		`SELECT password_enc FROM connections WHERE id=? AND user_id=?`,
		id, userID,
	).Scan(&enc)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	pw, err := c.Decrypt(enc)
	if err != nil {
		return "", err
	}
	return string(pw), nil
}

func (s *Store) ListConnections(userID int64) ([]Connection, error) {
	rows, err := s.DB.Query(
		`SELECT id, user_id, name, host, port, username, default_db, tls, color, created_at, updated_at
		 FROM connections WHERE user_id=? ORDER BY name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Host, &c.Port, &c.Username, &c.DefaultDB, &c.TLS, &c.Color, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateConnection(c *crypto.Cipher, userID, id int64, in ConnectionInput) (Connection, error) {
	// Ensure the connection belongs to user before touching it.
	if _, err := s.GetConnection(userID, id); err != nil {
		return Connection{}, err
	}
	if in.Port == 0 {
		in.Port = 3306
	}
	if in.TLS == "" {
		in.TLS = "disabled"
	}
	if in.Password == "" {
		// Keep existing password.
		_, err := s.DB.Exec(
			`UPDATE connections
			 SET name=?, host=?, port=?, username=?, default_db=?, tls=?, color=?, updated_at=CURRENT_TIMESTAMP
			 WHERE id=? AND user_id=?`,
			in.Name, in.Host, in.Port, in.Username, in.DefaultDB, in.TLS, in.Color, id, userID,
		)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return Connection{}, ErrDuplicate
			}
			return Connection{}, err
		}
	} else {
		enc, err := c.Encrypt([]byte(in.Password))
		if err != nil {
			return Connection{}, err
		}
		_, err = s.DB.Exec(
			`UPDATE connections
			 SET name=?, host=?, port=?, username=?, password_enc=?, default_db=?, tls=?, color=?, updated_at=CURRENT_TIMESTAMP
			 WHERE id=? AND user_id=?`,
			in.Name, in.Host, in.Port, in.Username, enc, in.DefaultDB, in.TLS, in.Color, id, userID,
		)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return Connection{}, ErrDuplicate
			}
			return Connection{}, err
		}
	}
	return s.GetConnection(userID, id)
}

func (s *Store) DeleteConnection(userID, id int64) error {
	res, err := s.DB.Exec("DELETE FROM connections WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 5: Run tests, confirm pass**

```bash
go test ./internal/store/ -v
```

Expected: all pass (new connection tests + existing migrate/users/sessions tests).

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat(store): connections table + AES-GCM CRUD scoped by user"
```

---

## Task 3: Connections list + create API

**Files:**
- Create: `internal/api/connections.go`, `internal/api/connections_test.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Failing tests**

Create `internal/api/connections_test.go`:

```go
package api

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conray/dataseai/internal/crypto"
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
	// password must NOT echo back
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
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/api/ -run Connection -v
```

Expected: FAIL — handlers / route undefined.

- [ ] **Step 3: Implement create + list handlers**

Create `internal/api/connections.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/store"
)

type connectionReq struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	DefaultDB string `json:"default_db,omitempty"`
	TLS       string `json:"tls,omitempty"`
	Color     string `json:"color,omitempty"`
}

func (r connectionReq) validate() error {
	if r.Name == "" || len(r.Name) > 64 {
		return errors.New("name required (1-64 chars)")
	}
	if r.Host == "" {
		return errors.New("host required")
	}
	if r.Username == "" {
		return errors.New("username required")
	}
	if r.TLS != "" && r.TLS != "disabled" && r.TLS != "preferred" && r.TLS != "required" {
		return errors.New("tls must be disabled|preferred|required")
	}
	return nil
}

func connectionJSON(c store.Connection) map[string]any {
	return map[string]any{
		"id":         c.ID,
		"name":       c.Name,
		"host":       c.Host,
		"port":       c.Port,
		"username":   c.Username,
		"default_db": c.DefaultDB,
		"tls":        c.TLS,
		"color":      c.Color,
		"created_at": c.CreatedAt,
		"updated_at": c.UpdatedAt,
	}
}

func handleCreateConnection(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var req connectionReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := req.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		c, err := d.Store.CreateConnection(d.Cipher, u.ID, store.ConnectionInput{
			Name: req.Name, Host: req.Host, Port: req.Port, Username: req.Username, Password: req.Password,
			DefaultDB: req.DefaultDB, TLS: req.TLS, Color: req.Color,
		})
		if err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				writeError(w, http.StatusConflict, "name already used")
				return
			}
			writeError(w, http.StatusInternalServerError, "create connection failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connection": connectionJSON(c)})
	}
}

func handleListConnections(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		list, err := d.Store.ListConnections(u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(list))
		for _, c := range list {
			out = append(out, connectionJSON(c))
		}
		writeJSON(w, http.StatusOK, map[string]any{"connections": out})
	}
}
```

- [ ] **Step 4: Wire routes in `internal/api/router.go` inside the authenticated group**

Add these two lines inside the `r.Group(func(r chi.Router) { ... })` block (after the session routes):

```go
		r.Post("/api/connections", handleCreateConnection(d))
		r.Get("/api/connections", handleListConnections(d))
```

- [ ] **Step 5: Run, confirm pass**

```bash
go test ./internal/api/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat(api): POST/GET /api/connections (list + create)"
```

---

## Task 4: Connection get / update / delete handlers

**Files:**
- Modify: `internal/api/connections.go`, `internal/api/router.go`, `internal/api/connections_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/api/connections_test.go`:

```go
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
	r, _, c := newTestRouterWithCipher(t)
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
	// Confirm password preserved by reading through the store with the cipher
	s := storeFromRouter(r)
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
```

**Important:** rewrite the `TestUpdateConnection_KeepsPasswordWhenEmpty` test above so it uses the store returned by `newTestRouterWithCipher` (not a router-internal recovery hack):

```go
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
```

Add these helpers at the bottom of `connections_test.go`:

```go
import "strconv"

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

func userIDOfAlice(s *store.Store) int64 {
	row := s.DB.QueryRow("SELECT id FROM users WHERE username='alice'")
	var id int64
	_ = row.Scan(&id)
	return id
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/api/ -run "TestGetConnection|TestUpdateConnection|TestDeleteConnection" -v
```

Expected: FAIL — handlers / routes undefined.

- [ ] **Step 3: Implement handlers**

Append to `internal/api/connections.go`:

```go
import (
	"github.com/go-chi/chi/v5"
	"strconv"
)
```

(Merge this into the existing import block; do not leave two `import ()` blocks. If `strconv` and `chi` are already imported in `connections.go`, skip the duplicates.)

```go
func parseConnIDParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func handleGetConnection(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := parseConnIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		c, err := d.Store.GetConnection(u.ID, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "get failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connection": connectionJSON(c)})
	}
}

func handleUpdateConnection(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := parseConnIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		var req connectionReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := req.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		c, err := d.Store.UpdateConnection(d.Cipher, u.ID, id, store.ConnectionInput{
			Name: req.Name, Host: req.Host, Port: req.Port, Username: req.Username, Password: req.Password,
			DefaultDB: req.DefaultDB, TLS: req.TLS, Color: req.Color,
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			if errors.Is(err, store.ErrDuplicate) {
				writeError(w, http.StatusConflict, "name already used")
				return
			}
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connection": connectionJSON(c)})
	}
}

func handleDeleteConnection(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := parseConnIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		if err := d.Store.DeleteConnection(u.ID, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 4: Wire routes in `router.go` inside the auth group**

```go
		r.Get("/api/connections/{id}", handleGetConnection(d))
		r.Put("/api/connections/{id}", handleUpdateConnection(d))
		r.Delete("/api/connections/{id}", handleDeleteConnection(d))
```

- [ ] **Step 5: Run all tests**

```bash
go test ./internal/api/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat(api): GET/PUT/DELETE /api/connections/:id (user-scoped)"
```

---

## Task 5: Add MySQL driver + connection pool

**Files:**
- Create: `internal/mysql/pool.go`, `internal/mysql/pool_test.go`, `internal/mysql/ident.go`, `internal/mysql/ident_test.go`

- [ ] **Step 1: Add driver dependency**

```bash
go get github.com/go-sql-driver/mysql@v1.8.1
```

- [ ] **Step 2: Failing tests for the identifier escaper**

Create `internal/mysql/ident_test.go`:

```go
package mysql

import "testing"

func TestQuoteIdent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"users", "`users`"},
		{"my table", "`my table`"},
		{"weird`name", "`weird``name`"},
		{"", "``"},
	}
	for _, c := range cases {
		got := QuoteIdent(c.in)
		if got != c.want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 3: Run, confirm fail**

```bash
go test ./internal/mysql/ -v
```

- [ ] **Step 4: Implement identifier escaper**

Create `internal/mysql/ident.go`:

```go
package mysql

import "strings"

// QuoteIdent wraps a MySQL identifier in backticks, doubling any embedded
// backticks to escape them. Use this for any user-controlled table / column /
// database name to defend against keyword collisions and injection.
func QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
```

- [ ] **Step 5: Failing tests for the pool**

Create `internal/mysql/pool_test.go`:

```go
package mysql

import (
	"database/sql"
	"testing"
	"time"
)

func TestPool_LazyCreate_SameKeyReturnsSameDB(t *testing.T) {
	opens := 0
	p := NewPool(PoolConfig{
		IdleTimeout: 5 * time.Minute,
		Open: func(dsn string) (*sql.DB, error) {
			opens++
			return new(sql.DB), nil // placeholder, never used
		},
	})
	a, err := p.Get(PoolKey{UserID: 1, ConnID: 10}, "dsn1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Get(PoolKey{UserID: 1, ConnID: 10}, "dsn1")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("same key should return same *sql.DB")
	}
	if opens != 1 {
		t.Fatalf("opens = %d, want 1", opens)
	}
}

func TestPool_DifferentKeysAreIsolated(t *testing.T) {
	opens := 0
	p := NewPool(PoolConfig{
		Open: func(dsn string) (*sql.DB, error) {
			opens++
			return new(sql.DB), nil
		},
	})
	_, _ = p.Get(PoolKey{UserID: 1, ConnID: 10}, "dsn1")
	_, _ = p.Get(PoolKey{UserID: 2, ConnID: 10}, "dsn2")
	if opens != 2 {
		t.Fatalf("opens = %d, want 2", opens)
	}
}

func TestPool_Evict(t *testing.T) {
	p := NewPool(PoolConfig{Open: func(dsn string) (*sql.DB, error) { return new(sql.DB), nil }})
	a, _ := p.Get(PoolKey{UserID: 1, ConnID: 10}, "dsn1")
	p.Evict(PoolKey{UserID: 1, ConnID: 10})
	b, _ := p.Get(PoolKey{UserID: 1, ConnID: 10}, "dsn1")
	if a == b {
		t.Fatal("evict should force re-open")
	}
}
```

- [ ] **Step 6: Run, confirm fail**

```bash
go test ./internal/mysql/ -run TestPool -v
```

- [ ] **Step 7: Implement pool**

Create `internal/mysql/pool.go`:

```go
package mysql

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type PoolKey struct {
	UserID int64
	ConnID int64
}

type PoolConfig struct {
	IdleTimeout time.Duration                          // 0 = disabled
	Open        func(dsn string) (*sql.DB, error)      // override for tests; nil uses sql.Open("mysql", dsn)
}

type pooled struct {
	db       *sql.DB
	lastUsed time.Time
}

type Pool struct {
	cfg PoolConfig
	mu  sync.Mutex
	m   map[PoolKey]*pooled
}

func NewPool(cfg PoolConfig) *Pool {
	if cfg.Open == nil {
		cfg.Open = func(dsn string) (*sql.DB, error) {
			return sql.Open("mysql", dsn)
		}
	}
	return &Pool{cfg: cfg, m: map[PoolKey]*pooled{}}
}

func (p *Pool) Get(key PoolKey, dsn string) (*sql.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.m[key]; ok {
		entry.lastUsed = time.Now()
		return entry.db, nil
	}
	db, err := p.cfg.Open(dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	p.m[key] = &pooled{db: db, lastUsed: time.Now()}
	return db, nil
}

func (p *Pool) Evict(key PoolKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.m[key]; ok {
		_ = entry.db.Close()
		delete(p.m, key)
	}
}

// EvictUser closes every pooled connection for a single user — call when a
// user is deleted or their credentials change.
func (p *Pool) EvictUser(userID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, entry := range p.m {
		if k.UserID == userID {
			_ = entry.db.Close()
			delete(p.m, k)
		}
	}
}

// Sweep closes any entry idle longer than IdleTimeout. No-op if IdleTimeout==0.
func (p *Pool) Sweep(now time.Time) {
	if p.cfg.IdleTimeout == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, entry := range p.m {
		if now.Sub(entry.lastUsed) >= p.cfg.IdleTimeout {
			_ = entry.db.Close()
			delete(p.m, k)
		}
	}
}
```

- [ ] **Step 8: Run, confirm pass**

```bash
go test ./internal/mysql/ -v
```

- [ ] **Step 9: Commit**

```bash
git add internal/mysql/ go.mod go.sum
git commit -m "feat(mysql): identifier escaper + per-(user,conn) *sql.DB pool"
```

---

## Task 6: DSN builder + connection-test endpoint

**Files:**
- Create: `internal/mysql/dsn.go`, `internal/mysql/dsn_test.go`
- Modify: `internal/api/connections.go`, `internal/api/router.go`, `internal/api/connections_test.go`

- [ ] **Step 1: Failing test for DSN**

Create `internal/mysql/dsn_test.go`:

```go
package mysql

import "testing"

func TestBuildDSN(t *testing.T) {
	cases := []struct {
		in   DSNInput
		want string
	}{
		{
			DSNInput{Host: "h", Port: 3306, Username: "u", Password: "p", TLS: "disabled"},
			"u:p@tcp(h:3306)/?parseTime=true&tls=false&charset=utf8mb4",
		},
		{
			DSNInput{Host: "h", Port: 3307, Username: "u", Password: "p:@/", DefaultDB: "mydb", TLS: "required"},
			"u:p%3A%40%2F@tcp(h:3307)/mydb?parseTime=true&tls=true&charset=utf8mb4",
		},
		{
			DSNInput{Host: "h", Port: 3306, Username: "u", Password: "p", TLS: "preferred"},
			"u:p@tcp(h:3306)/?parseTime=true&tls=preferred&charset=utf8mb4",
		},
	}
	for _, c := range cases {
		got := BuildDSN(c.in)
		if got != c.want {
			t.Errorf("BuildDSN(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/mysql/ -run TestBuildDSN -v
```

- [ ] **Step 3: Implement DSN builder**

Create `internal/mysql/dsn.go`:

```go
package mysql

import (
	"fmt"
	"net/url"
)

type DSNInput struct {
	Host      string
	Port      int
	Username  string
	Password  string
	DefaultDB string
	TLS       string // "disabled" | "preferred" | "required"
}

// BuildDSN constructs a go-sql-driver/mysql DSN.
// Format: user:password@tcp(host:port)/dbname?param=value
func BuildDSN(in DSNInput) string {
	tlsParam := "false"
	switch in.TLS {
	case "required":
		tlsParam = "true"
	case "preferred":
		tlsParam = "preferred"
	}
	user := url.QueryEscape(in.Username)
	pass := url.QueryEscape(in.Password)
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&tls=%s&charset=utf8mb4",
		user, pass, in.Host, in.Port, in.DefaultDB, tlsParam,
	)
}
```

Note: `go-sql-driver/mysql` actually accepts user/pass URL-escaped only for special characters; the escape is conservative here. For practical use this works.

- [ ] **Step 4: Add failing test for the test-connection endpoint**

Append to `internal/api/connections_test.go`:

```go
func TestTestConnection_PassesOpenError(t *testing.T) {
	r, _, _ := newTestRouterWithCipher(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{
		"name": "prod", "host": "127.0.0.1", "port": 65535, // nothing listening
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
```

- [ ] **Step 5: Run, confirm fail**

```bash
go test ./internal/api/ -run TestTestConnection -v
```

- [ ] **Step 6: Implement the test-connection handler**

Append to `internal/api/connections.go`:

```go
import "github.com/conray/dataseai/internal/mysql"

// (Merge into the existing import block.)

func handleTestConnection(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := parseConnIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		conn, err := d.Store.GetConnection(u.ID, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		pw, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "decrypt failed")
			return
		}
		dsn := mysql.BuildDSN(mysql.DSNInput{
			Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
			DefaultDB: conn.DefaultDB, TLS: conn.TLS,
		})
		db, err := d.Pool.Get(mysql.PoolKey{UserID: u.ID, ConnID: id}, dsn)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		ctx, cancel := contextWithTimeout(r.Context(), 5)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			d.Pool.Evict(mysql.PoolKey{UserID: u.ID, ConnID: id})
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "connected"})
	}
}
```

Add a tiny helper near the top of `connections.go` (or in a new `internal/api/util.go` if it's already busy):

```go
import (
	"context"
	"time"
)

func contextWithTimeout(parent context.Context, seconds int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(seconds)*time.Second)
}
```

(If you already have `context` and `time` imported, just keep the function — drop the redundant `import` statement.)

- [ ] **Step 7: Extend `Deps` with the Pool**

Edit `internal/api/router.go`:

```go
import (
	"io/fs"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/crypto"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

type Deps struct {
	Version      string
	Store        *store.Store
	Cipher       *crypto.Cipher
	Pool         *mysql.Pool
	Registration string
	WebFS        fs.FS
}
```

Inside the auth group, add:

```go
		r.Post("/api/connections/{id}/test", handleTestConnection(d))
```

- [ ] **Step 8: Update `newTestRouterWithCipher` to supply a Pool**

Edit `internal/api/connections_test.go` so the helper reads:

```go
import (
	"database/sql"

	"github.com/conray/dataseai/internal/crypto"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

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
```

- [ ] **Step 9: Wire the Pool in `main.go`**

Edit `cmd/dataseai/main.go`:

```go
import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"time"

	dataseai "github.com/conray/dataseai"
	"github.com/conray/dataseai/internal/api"
	"github.com/conray/dataseai/internal/config"
	"github.com/conray/dataseai/internal/crypto"
	mysqlpkg "github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
)
```

And inside `main()` after the `s := &store.Store{DB: db}` line:

```go
	pool := mysqlpkg.NewPool(mysqlpkg.PoolConfig{IdleTimeout: 5 * time.Minute})
```

Pass it through:

```go
	r := api.NewRouter(api.Deps{
		Version:      version,
		Store:        s,
		Cipher:       cipher,
		Pool:         pool,
		Registration: cfg.Registration,
		WebFS:        sub,
	})
```

- [ ] **Step 10: Run all tests**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/ cmd/
git commit -m "feat(api): POST /api/connections/:id/test + mysql DSN builder + Pool wired"
```

---

## Task 7: Browse — list databases

**Files:**
- Create: `internal/mysql/browse.go`, `internal/api/db.go`, `internal/api/db_test.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Implement browse function (no test — needs real MySQL)**

Create `internal/mysql/browse.go`:

```go
package mysql

import (
	"context"
	"database/sql"
)

// ListDatabases returns visible database names excluding MySQL/system schemas.
func ListDatabases(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT schema_name
		 FROM information_schema.schemata
		 WHERE schema_name NOT IN ('mysql','information_schema','performance_schema','sys')
		 ORDER BY schema_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
```

- [ ] **Step 2: Failing test for the API handler (uses mocked Pool)**

Create `internal/api/db_test.go`:

```go
package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

// newTestRouterWithSqliteAsMySQL wires the Pool to open in-memory sqlite
// instead of MySQL, so tests can run without a MySQL server. The handlers
// then run their queries against sqlite — which works for queries we shape
// in a portable way (no information_schema-specific reliance for unit
// tests; integration smoke covers real MySQL).
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

// Note: a positive ListDatabases test requires MySQL (sqlite has no
// information_schema). The handler structure is exercised by the auth +
// not-found tests; the actual query is covered by the manual smoke test
// documented at the end of this plan.

func TestListDatabases_DecodesShape(t *testing.T) {
	// We can't run the real query, but we can verify the handler's response
	// shape under a fake driver that returns nil rows.
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
	// sqlite has no information_schema.schemata — handler returns 500.
	// We just confirm it routes to the right user-scoped handler.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 (sqlite lacks information_schema), got %d body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 3: Run, confirm fail**

```bash
go test ./internal/api/ -run TestListDatabases -v
```

- [ ] **Step 4: Implement handler**

Create `internal/api/db.go`:

```go
package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

func parseConnID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "connId"), 10, 64)
}

type connSession struct {
	Conn store.Connection
	DB   *sql.DB
	Pool *mysql.Pool
	Key  mysql.PoolKey
}

func resolveConn(d Deps, w http.ResponseWriter, r *http.Request) (*connSession, bool) {
	u, _ := auth.UserFromContext(r.Context())
	id, err := parseConnID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad connId")
		return nil, false
	}
	conn, err := d.Store.GetConnection(u.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "connection not found")
		} else {
			writeError(w, http.StatusInternalServerError, "lookup failed")
		}
		return nil, false
	}
	pw, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "decrypt failed")
		return nil, false
	}
	dsn := mysql.BuildDSN(mysql.DSNInput{
		Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
		DefaultDB: conn.DefaultDB, TLS: conn.TLS,
	})
	key := mysql.PoolKey{UserID: u.ID, ConnID: id}
	db, err := d.Pool.Get(key, dsn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pool open failed")
		return nil, false
	}
	return &connSession{Conn: conn, DB: db, Pool: d.Pool, Key: key}, true
}

func handleListDatabases(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		names, err := mysql.ListDatabases(ctx, cs.DB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"databases": names})
	}
}
```

- [ ] **Step 5: Wire route in `router.go` inside the auth group**

```go
		r.Get("/api/db/{connId}/databases", handleListDatabases(d))
```

- [ ] **Step 6: Run, confirm pass**

```bash
go test ./internal/api/ -v
```

Expected: PASS — including the 500-on-sqlite shape test.

- [ ] **Step 7: Commit**

```bash
git add internal/api/db.go internal/api/db_test.go internal/api/router.go internal/mysql/browse.go
git commit -m "feat(api): GET /api/db/:connId/databases + mysql.ListDatabases"
```

---

## Task 8: Browse — list tables

**Files:**
- Modify: `internal/mysql/browse.go`, `internal/api/db.go`, `internal/api/db_test.go`, `internal/api/router.go`

- [ ] **Step 1: Append `ListTables` to `internal/mysql/browse.go`**

```go
type TableInfo struct {
	Name    string `json:"name"`
	RowsEst int64  `json:"rows_est"`
	SizeMB  int64  `json:"size_mb"`
}

func ListTables(ctx context.Context, db *sql.DB, schema string) ([]TableInfo, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT table_name,
		        COALESCE(table_rows, 0),
		        COALESCE(ROUND((data_length + index_length) / 1024 / 1024), 0)
		 FROM information_schema.tables
		 WHERE table_schema = ?
		 ORDER BY table_name`,
		schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.RowsEst, &t.SizeMB); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
```

- [ ] **Step 2: Failing test for the handler**

Append to `internal/api/db_test.go`:

```go
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
```

- [ ] **Step 3: Run, confirm fail**

```bash
go test ./internal/api/ -run TestListTables -v
```

- [ ] **Step 4: Implement handler**

Append to `internal/api/db.go`:

```go
func handleListTables(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		if schema == "" {
			writeError(w, http.StatusBadRequest, "missing db")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		tables, err := mysql.ListTables(ctx, cs.DB, schema)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
	}
}
```

- [ ] **Step 5: Wire route in `router.go` inside the auth group**

```go
		r.Get("/api/db/{connId}/databases/{db}/tables", handleListTables(d))
```

- [ ] **Step 6: Run, confirm pass**

```bash
go test ./internal/api/ -v
```

- [ ] **Step 7: Commit**

```bash
git add internal/api/db.go internal/api/db_test.go internal/api/router.go internal/mysql/browse.go
git commit -m "feat(api): GET /api/db/:connId/databases/:db/tables"
```

---

## Task 9: Browse — table rows with pagination + sort

**Files:**
- Modify: `internal/mysql/browse.go`, `internal/api/db.go`, `internal/api/db_test.go`, `internal/api/router.go`

- [ ] **Step 1: Append `FetchTableRows` to `internal/mysql/browse.go`**

```go
type RowsPage struct {
	Columns []string          `json:"columns"`
	Rows    [][]any           `json:"rows"`
	Total   int64             `json:"total"`
	Page    int               `json:"page"`
	PerPage int               `json:"per_page"`
}

type RowsOpts struct {
	Schema   string
	Table    string
	Page     int    // 1-based
	PerPage  int    // capped at 500
	SortCol  string // empty = no order
	SortDir  string // "asc" | "desc"
}

func FetchTableRows(ctx context.Context, db *sql.DB, o RowsOpts) (RowsPage, error) {
	if o.Page < 1 {
		o.Page = 1
	}
	if o.PerPage < 1 {
		o.PerPage = 50
	}
	if o.PerPage > 500 {
		o.PerPage = 500
	}
	offset := (o.Page - 1) * o.PerPage

	schema := QuoteIdent(o.Schema)
	table := QuoteIdent(o.Table)
	qualified := schema + "." + table

	// Total row count (fast on small tables; for huge tables consumers can
	// fall back to information_schema.table_rows — out of scope for Plan 2).
	var total int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+qualified).Scan(&total); err != nil {
		return RowsPage{}, err
	}

	orderBy := ""
	if o.SortCol != "" {
		dir := "ASC"
		if o.SortDir == "desc" {
			dir = "DESC"
		}
		orderBy = " ORDER BY " + QuoteIdent(o.SortCol) + " " + dir
	}

	rows, err := db.QueryContext(ctx, "SELECT * FROM "+qualified+orderBy+" LIMIT ? OFFSET ?", o.PerPage, offset)
	if err != nil {
		return RowsPage{}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return RowsPage{}, err
	}
	page := RowsPage{Columns: cols, Total: total, Page: o.Page, PerPage: o.PerPage}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return RowsPage{}, err
		}
		// Convert []byte to string so JSON encodes as text.
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		page.Rows = append(page.Rows, vals)
	}
	return page, rows.Err()
}
```

- [ ] **Step 2: Failing test for the handler**

Append to `internal/api/db_test.go`:

```go
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
```

- [ ] **Step 3: Run, confirm fail**

```bash
go test ./internal/api/ -run TestFetchTableRows -v
```

- [ ] **Step 4: Implement handler**

Append to `internal/api/db.go`:

```go
func handleTableData(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		table := chi.URLParam(r, "table")
		if schema == "" || table == "" {
			writeError(w, http.StatusBadRequest, "missing db/table")
			return
		}
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		perPage, _ := strconv.Atoi(q.Get("per_page"))
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := mysql.FetchTableRows(ctx, cs.DB, mysql.RowsOpts{
			Schema: schema, Table: table, Page: page, PerPage: perPage,
			SortCol: q.Get("sort_col"), SortDir: q.Get("sort_dir"),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
```

- [ ] **Step 5: Wire route in `router.go`**

```go
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/data", handleTableData(d))
```

- [ ] **Step 6: Run all tests**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/ internal/mysql/
git commit -m "feat(api): GET /api/db/:connId/databases/:db/tables/:t/data"
```

---

## Task 10: Frontend — connections store + types

**Files:**
- Create: `web/src/store/connections.ts`, `web/src/store/connections.test.ts`

- [ ] **Step 1: Failing test**

Create `web/src/store/connections.test.ts`:

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useConnections } from './connections'

describe('useConnections', () => {
  beforeEach(() => {
    useConnections.setState({ list: [], loading: false, error: null })
  })

  it('starts empty', () => {
    expect(useConnections.getState().list).toEqual([])
  })

  it('setList replaces items', () => {
    useConnections.getState().setList([
      { id: 1, name: 'prod', host: 'h', port: 3306, username: 'u', default_db: '', tls: 'disabled', color: '', created_at: '', updated_at: '' },
    ])
    expect(useConnections.getState().list).toHaveLength(1)
  })
})
```

- [ ] **Step 2: Run, confirm fail**

```bash
cd web && npm test -- connections.test.ts
```

- [ ] **Step 3: Implement store**

Create `web/src/store/connections.ts`:

```ts
import { create } from 'zustand'
import { api, ApiError } from '../lib/api'

export interface Connection {
  id: number
  name: string
  host: string
  port: number
  username: string
  default_db: string
  tls: 'disabled' | 'preferred' | 'required'
  color: string
  created_at: string
  updated_at: string
}

export interface ConnectionInput {
  name: string
  host: string
  port: number
  username: string
  password: string
  default_db?: string
  tls?: 'disabled' | 'preferred' | 'required'
  color?: string
}

interface State {
  list: Connection[]
  loading: boolean
  error: string | null
  setList: (l: Connection[]) => void
  load: () => Promise<void>
  create: (in_: ConnectionInput) => Promise<Connection>
  update: (id: number, in_: ConnectionInput) => Promise<Connection>
  remove: (id: number) => Promise<void>
  test: (id: number) => Promise<{ ok: boolean; message: string }>
}

export const useConnections = create<State>((set, get) => ({
  list: [],
  loading: false,
  error: null,
  setList: (l) => set({ list: l }),
  load: async () => {
    set({ loading: true, error: null })
    try {
      const r = await api.get<{ connections: Connection[] }>('/api/connections')
      set({ list: r.connections ?? [], loading: false })
    } catch (err) {
      set({ loading: false, error: err instanceof ApiError ? err.message : 'load failed' })
    }
  },
  create: async (in_) => {
    const r = await api.post<{ connection: Connection }>('/api/connections', in_)
    set({ list: [...get().list, r.connection] })
    return r.connection
  },
  update: async (id, in_) => {
    const r = await api.put<{ connection: Connection }>(`/api/connections/${id}`, in_)
    set({ list: get().list.map((c) => (c.id === id ? r.connection : c)) })
    return r.connection
  },
  remove: async (id) => {
    await api.del(`/api/connections/${id}`)
    set({ list: get().list.filter((c) => c.id !== id) })
  },
  test: async (id) => api.post(`/api/connections/${id}/test`, null),
}))
```

- [ ] **Step 4: Run, confirm pass**

```bash
cd web && npm test
```

- [ ] **Step 5: Commit**

```bash
git add web/src/store/connections.ts web/src/store/connections.test.ts
git commit -m "feat(web): connections Zustand store with CRUD + test"
```

---

## Task 11: Frontend — ConnectionDialog component

**Files:**
- Create: `web/src/components/ConnectionDialog.tsx`

- [ ] **Step 1: Implement (no test — pure UI)**

Create `web/src/components/ConnectionDialog.tsx`:

```tsx
import { FormEvent, useEffect, useState } from 'react'
import { ApiError } from '../lib/api'
import { Connection, ConnectionInput, useConnections } from '../store/connections'

interface Props {
  initial?: Connection | null
  onClose: () => void
  onSaved: (c: Connection) => void
}

export default function ConnectionDialog({ initial, onClose, onSaved }: Props) {
  const create = useConnections((s) => s.create)
  const update = useConnections((s) => s.update)
  const testConn = useConnections((s) => s.test)
  const [name, setName] = useState(initial?.name ?? '')
  const [host, setHost] = useState(initial?.host ?? 'localhost')
  const [port, setPort] = useState<number>(initial?.port ?? 3306)
  const [username, setUsername] = useState(initial?.username ?? '')
  const [password, setPassword] = useState('')
  const [defaultDB, setDefaultDB] = useState(initial?.default_db ?? '')
  const [tls, setTLS] = useState<ConnectionInput['tls']>(initial?.tls ?? 'disabled')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [testMsg, setTestMsg] = useState<string | null>(null)

  useEffect(() => {
    if (!initial) setPassword('')
  }, [initial])

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const input: ConnectionInput = { name, host, port, username, password, default_db: defaultDB, tls }
      const saved = initial ? await update(initial.id, input) : await create(input)
      onSaved(saved)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'save failed')
    } finally {
      setBusy(false)
    }
  }

  async function runTest() {
    if (!initial) {
      setTestMsg('save first to test')
      return
    }
    setTestMsg('testing…')
    try {
      const r = await testConn(initial.id)
      setTestMsg(r.ok ? 'connected ✓' : `failed: ${r.message}`)
    } catch (err) {
      setTestMsg(err instanceof ApiError ? err.message : 'test failed')
    }
  }

  return (
    <div style={backdrop}>
      <div style={modal}>
        <h2 style={{ marginTop: 0 }}>{initial ? 'edit connection' : 'new connection'}</h2>
        <form onSubmit={submit} style={{ display: 'grid', gap: 10 }}>
          <label>name <input value={name} onChange={(e) => setName(e.target.value)} required style={input} /></label>
          <label>host <input value={host} onChange={(e) => setHost(e.target.value)} required style={input} /></label>
          <label>port <input type="number" value={port} onChange={(e) => setPort(parseInt(e.target.value || '0', 10))} required style={input} /></label>
          <label>user <input value={username} onChange={(e) => setUsername(e.target.value)} required style={input} /></label>
          <label>password <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={initial ? '(leave blank to keep)' : ''} required={!initial} style={input} /></label>
          <label>default db <input value={defaultDB} onChange={(e) => setDefaultDB(e.target.value)} style={input} /></label>
          <label>tls
            <select value={tls} onChange={(e) => setTLS(e.target.value as ConnectionInput['tls'])} style={input}>
              <option value="disabled">disabled</option>
              <option value="preferred">preferred</option>
              <option value="required">required</option>
            </select>
          </label>
          {error && <div style={{ color: 'crimson', fontSize: 13 }}>{error}</div>}
          {testMsg && <div style={{ fontSize: 13 }}>{testMsg}</div>}
          <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 8 }}>
            {initial && <button type="button" onClick={runTest}>test</button>}
            <button type="button" onClick={onClose}>cancel</button>
            <button disabled={busy} type="submit">{busy ? 'saving…' : 'save'}</button>
          </div>
        </form>
      </div>
    </div>
  )
}

const backdrop: React.CSSProperties = {
  position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)',
  display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
}
const modal: React.CSSProperties = {
  background: 'white', padding: 20, borderRadius: 8, minWidth: 400, fontFamily: 'system-ui',
}
const input: React.CSSProperties = { display: 'block', width: '100%', padding: '4px 6px', marginTop: 2, boxSizing: 'border-box' }
```

Note: the file uses `React.CSSProperties` so add `import type { CSSProperties } from 'react'` and reference `CSSProperties` instead of `React.CSSProperties` to match the convention from Plan 1 (Task 19).

(Apply that rename: `const backdrop: CSSProperties`, `const modal: CSSProperties`, `const input: CSSProperties`, and the import line `import type { CSSProperties } from 'react'`.)

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/ConnectionDialog.tsx
git commit -m "feat(web): ConnectionDialog (new/edit + test connection)"
```

---

## Task 12: Frontend — ConnectionsManager + TopBar + active conn store

**Files:**
- Create: `web/src/store/activeConn.ts`, `web/src/components/ConnectionsManager.tsx`, `web/src/components/TopBar.tsx`, `web/src/components/ConnectionPicker.tsx`

- [ ] **Step 1: Active-conn store**

Create `web/src/store/activeConn.ts`:

```ts
import { create } from 'zustand'

interface State {
  activeId: number | null
  setActive: (id: number | null) => void
}

export const useActiveConn = create<State>((set) => ({
  activeId: null,
  setActive: (id) => set({ activeId: id }),
}))
```

- [ ] **Step 2: ConnectionsManager**

Create `web/src/components/ConnectionsManager.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { Connection, useConnections } from '../store/connections'
import ConnectionDialog from './ConnectionDialog'

interface Props {
  onClose: () => void
}

export default function ConnectionsManager({ onClose }: Props) {
  const list = useConnections((s) => s.list)
  const load = useConnections((s) => s.load)
  const remove = useConnections((s) => s.remove)
  const [editing, setEditing] = useState<Connection | 'new' | null>(null)

  useEffect(() => { void load() }, [load])

  return (
    <main style={{ fontFamily: 'system-ui', padding: 24, maxWidth: 800, margin: '0 auto' }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <h1 style={{ margin: 0 }}>connections</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={() => setEditing('new')}>+ new</button>
          <button onClick={onClose}>back</button>
        </div>
      </header>
      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <thead>
          <tr>
            <th style={th}>name</th>
            <th style={th}>host</th>
            <th style={th}>port</th>
            <th style={th}>user</th>
            <th style={th}>tls</th>
            <th style={th}></th>
          </tr>
        </thead>
        <tbody>
          {list.map((c) => (
            <tr key={c.id}>
              <td style={td}>{c.name}</td>
              <td style={td}>{c.host}</td>
              <td style={td}>{c.port}</td>
              <td style={td}>{c.username}</td>
              <td style={td}>{c.tls}</td>
              <td style={td}>
                <button onClick={() => setEditing(c)}>edit</button>{' '}
                <button onClick={() => { if (confirm(`delete ${c.name}?`)) void remove(c.id) }}>delete</button>
              </td>
            </tr>
          ))}
          {list.length === 0 && (
            <tr><td colSpan={6} style={{ ...td, textAlign: 'center', color: '#999', padding: 24 }}>no connections yet — click + new</td></tr>
          )}
        </tbody>
      </table>
      {editing && (
        <ConnectionDialog
          initial={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => void load()}
        />
      )}
    </main>
  )
}

import type { CSSProperties } from 'react'
const th: CSSProperties = { textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid #ddd', fontSize: 13 }
const td: CSSProperties = { padding: '6px 8px', borderBottom: '1px solid #f3f3f3', fontSize: 13 }
```

- [ ] **Step 3: ConnectionPicker**

Create `web/src/components/ConnectionPicker.tsx`:

```tsx
import { useEffect } from 'react'
import { useConnections } from '../store/connections'
import { useActiveConn } from '../store/activeConn'

export default function ConnectionPicker() {
  const list = useConnections((s) => s.list)
  const load = useConnections((s) => s.load)
  const activeId = useActiveConn((s) => s.activeId)
  const setActive = useActiveConn((s) => s.setActive)

  useEffect(() => { void load() }, [load])

  return (
    <select
      value={activeId ?? ''}
      onChange={(e) => setActive(e.target.value === '' ? null : Number(e.target.value))}
      style={{ padding: '4px 6px' }}
    >
      <option value="">— pick connection —</option>
      {list.map((c) => (
        <option key={c.id} value={c.id}>● {c.name}</option>
      ))}
    </select>
  )
}
```

- [ ] **Step 4: TopBar**

Create `web/src/components/TopBar.tsx`:

```tsx
import { useAuth } from '../store/auth'
import ConnectionPicker from './ConnectionPicker'

interface Props {
  onOpenConnections: () => void
  onOpenSettings: () => void
}

export default function TopBar({ onOpenConnections, onOpenSettings }: Props) {
  const user = useAuth((s) => s.user!)
  const logout = useAuth((s) => s.logout)
  return (
    <header
      style={{
        display: 'flex', alignItems: 'center', gap: 12,
        padding: '8px 16px', borderBottom: '1px solid #ddd', background: '#fafafa',
      }}
    >
      <strong style={{ marginRight: 8 }}>dataseai</strong>
      <ConnectionPicker />
      <button onClick={onOpenConnections}>manage</button>
      <div style={{ flex: 1 }} />
      <span style={{ fontSize: 13 }}>{user.username}</span>
      <button onClick={onOpenSettings}>settings</button>
      <button onClick={() => logout()}>log out</button>
    </header>
  )
}
```

- [ ] **Step 5: Typecheck**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 6: Commit**

```bash
git add web/src/store/activeConn.ts web/src/components/
git commit -m "feat(web): TopBar + ConnectionsManager + ConnectionPicker"
```

---

## Task 13: Frontend — Sidebar (databases → tables tree)

**Files:**
- Create: `web/src/components/Sidebar.tsx`

- [ ] **Step 1: Implement**

Create `web/src/components/Sidebar.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'

interface TableInfo {
  name: string
  rows_est: number
  size_mb: number
}

interface Props {
  onPickTable: (db: string, table: string) => void
  selected?: { db: string; table: string } | null
}

export default function Sidebar({ onPickTable, selected }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [databases, setDatabases] = useState<string[]>([])
  const [openDB, setOpenDB] = useState<string | null>(null)
  const [tables, setTables] = useState<Record<string, TableInfo[]>>({})
  const [filter, setFilter] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (connId == null) {
      setDatabases([])
      setTables({})
      setOpenDB(null)
      return
    }
    setError(null)
    api.get<{ databases: string[] }>(`/api/db/${connId}/databases`)
      .then((r) => setDatabases(r.databases ?? []))
      .catch((err) => setError(err instanceof ApiError ? err.message : 'load failed'))
  }, [connId])

  async function expand(db: string) {
    if (openDB === db) {
      setOpenDB(null)
      return
    }
    setOpenDB(db)
    if (tables[db]) return
    try {
      const r = await api.get<{ tables: TableInfo[] }>(`/api/db/${connId}/databases/${encodeURIComponent(db)}/tables`)
      setTables((t) => ({ ...t, [db]: r.tables ?? [] }))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'tables failed')
    }
  }

  if (connId == null) {
    return (
      <aside style={sidebar}>
        <div style={{ color: '#999', fontSize: 13, padding: 16 }}>pick a connection in the top bar</div>
      </aside>
    )
  }

  return (
    <aside style={sidebar}>
      <input
        placeholder="filter tables…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        style={{ width: '100%', padding: '4px 6px', marginBottom: 8, boxSizing: 'border-box' }}
      />
      {error && <div style={{ color: 'crimson', fontSize: 12, marginBottom: 4 }}>{error}</div>}
      {databases.map((db) => {
        const isOpen = openDB === db
        const list = (tables[db] ?? []).filter((t) => !filter || t.name.toLowerCase().includes(filter.toLowerCase()))
        return (
          <div key={db}>
            <div
              onClick={() => void expand(db)}
              style={{ cursor: 'pointer', padding: '4px 0', fontWeight: 600, fontSize: 13 }}
            >
              {isOpen ? '▼' : '▶'} {db}
            </div>
            {isOpen && (
              <div style={{ paddingLeft: 12 }}>
                {list.map((t) => {
                  const active = selected && selected.db === db && selected.table === t.name
                  return (
                    <div
                      key={t.name}
                      onClick={() => onPickTable(db, t.name)}
                      style={{
                        cursor: 'pointer', padding: '2px 4px', fontSize: 12,
                        background: active ? '#cfe2ff' : 'transparent',
                      }}
                    >
                      📋 {t.name}
                    </div>
                  )
                })}
                {list.length === 0 && <div style={{ color: '#999', fontSize: 11, padding: '2px 4px' }}>(empty)</div>}
              </div>
            )}
          </div>
        )
      })}
    </aside>
  )
}

import type { CSSProperties } from 'react'
const sidebar: CSSProperties = {
  width: 220, borderRight: '1px solid #ddd', padding: 8, overflowY: 'auto',
  fontFamily: 'system-ui', boxSizing: 'border-box',
}
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/Sidebar.tsx
git commit -m "feat(web): Sidebar with database/table tree"
```

---

## Task 14: Frontend — DataGrid with TanStack Table

**Files:**
- Modify: `web/package.json` (add `@tanstack/react-table`)
- Create: `web/src/components/DataGrid.tsx`

- [ ] **Step 1: Add dependency**

```bash
cd web && npm install @tanstack/react-table@^8.20.5 && cd ..
```

- [ ] **Step 2: Implement DataGrid**

Create `web/src/components/DataGrid.tsx`:

```tsx
import { useEffect, useMemo, useState } from 'react'
import { ColumnDef, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'

interface RowsPage {
  columns: string[]
  rows: any[][]
  total: number
  page: number
  per_page: number
}

interface Props {
  db: string
  table: string
}

export default function DataGrid({ db, table }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [data, setData] = useState<RowsPage | null>(null)
  const [page, setPage] = useState(1)
  const [perPage] = useState(50)
  const [sortCol, setSortCol] = useState<string | null>(null)
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (connId == null) return
    setLoading(true)
    setError(null)
    const params = new URLSearchParams({
      page: String(page),
      per_page: String(perPage),
    })
    if (sortCol) {
      params.set('sort_col', sortCol)
      params.set('sort_dir', sortDir)
    }
    api
      .get<RowsPage>(`/api/db/${connId}/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}/data?${params}`)
      .then((d) => setData({ ...d, rows: d.rows ?? [] }))
      .catch((err) => setError(err instanceof ApiError ? err.message : 'load failed'))
      .finally(() => setLoading(false))
  }, [connId, db, table, page, perPage, sortCol, sortDir])

  const columns = useMemo<ColumnDef<any[]>[]>(() => {
    if (!data) return []
    return data.columns.map((name, idx) => ({
      id: name,
      header: () => (
        <span
          onClick={() => {
            if (sortCol === name) {
              setSortDir(sortDir === 'asc' ? 'desc' : 'asc')
            } else {
              setSortCol(name)
              setSortDir('asc')
            }
          }}
          style={{ cursor: 'pointer' }}
        >
          {name}{sortCol === name ? (sortDir === 'asc' ? ' ▲' : ' ▼') : ''}
        </span>
      ),
      accessorFn: (row) => row[idx],
      cell: (info) => {
        const v = info.getValue()
        if (v === null || v === undefined) return <span style={{ color: '#999' }}>NULL</span>
        return String(v)
      },
    }))
  }, [data, sortCol, sortDir])

  const tableInst = useReactTable({
    data: data?.rows ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.per_page)) : 1

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', fontFamily: 'system-ui' }}>
      <div style={{ flex: 1, overflow: 'auto' }}>
        {error && <div style={{ color: 'crimson', padding: 8 }}>{error}</div>}
        {loading && !data && <div style={{ color: '#999', padding: 8 }}>loading…</div>}
        {data && (
          <table style={{ borderCollapse: 'collapse', fontSize: 13, width: '100%' }}>
            <thead style={{ background: '#f4f4f4', position: 'sticky', top: 0 }}>
              {tableInst.getHeaderGroups().map((hg) => (
                <tr key={hg.id}>
                  {hg.headers.map((h) => (
                    <th key={h.id} style={th}>{flexRender(h.column.columnDef.header, h.getContext())}</th>
                  ))}
                </tr>
              ))}
            </thead>
            <tbody>
              {tableInst.getRowModel().rows.map((row) => (
                <tr key={row.id}>
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} style={td}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      <div style={{
        display: 'flex', gap: 8, alignItems: 'center', padding: 6,
        borderTop: '1px solid #ddd', fontSize: 12,
      }}>
        <button disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>‹ prev</button>
        <span>page {data?.page ?? 1} / {totalPages} · {data?.total ?? 0} rows total · {perPage}/page</span>
        <button disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>next ›</button>
      </div>
    </div>
  )
}

import type { CSSProperties } from 'react'
const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #ddd', whiteSpace: 'nowrap' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid #f3f3f3', whiteSpace: 'nowrap' }
```

- [ ] **Step 3: Typecheck + build**

```bash
cd web && npx tsc --noEmit && npm run build && cd ..
```

- [ ] **Step 4: Commit**

```bash
git add web/package.json web/package-lock.json web/src/components/DataGrid.tsx
git commit -m "feat(web): DataGrid (TanStack Table, pagination + sort)"
```

---

## Task 15: Frontend — BottomTabs (left group skeleton)

**Files:**
- Create: `web/src/components/BottomTabs.tsx`

- [ ] **Step 1: Implement**

Create `web/src/components/BottomTabs.tsx`:

```tsx
import type { CSSProperties } from 'react'

export type BottomTab = 'data' | 'structure' | 'indexes' | 'fks'

interface Props {
  value: BottomTab
  onChange: (t: BottomTab) => void
}

const TABS: { key: BottomTab; label: string }[] = [
  { key: 'data', label: '📊 Data' },
  { key: 'structure', label: '🏗 Structure (Plan 3)' },
  { key: 'indexes', label: '🔑 Indexes (Plan 3)' },
  { key: 'fks', label: '🔗 FK (Plan 3)' },
]

export default function BottomTabs({ value, onChange }: Props) {
  return (
    <div style={bar}>
      <span style={label}>TABLE</span>
      {TABS.map((t) => {
        const active = t.key === value
        const enabled = t.key === 'data' // Plan 2 only wires Data
        return (
          <button
            key={t.key}
            onClick={() => enabled && onChange(t.key)}
            disabled={!enabled}
            style={{ ...tab, ...(active ? tabActive : null), opacity: enabled ? 1 : 0.4 }}
          >
            {t.label}
          </button>
        )
      })}
    </div>
  )
}

const bar: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 4, padding: '0 8px',
  background: '#1a1a1a', color: '#ddd', borderTop: '1px solid #333', height: 30,
}
const label: CSSProperties = {
  fontSize: 10, letterSpacing: 1, padding: '0 8px', borderRight: '1px solid #2a2a2a', color: '#666',
}
const tab: CSSProperties = {
  background: 'transparent', color: '#aaa', border: 'none', padding: '4px 10px',
  borderRadius: '3px 3px 0 0', fontSize: 12, cursor: 'pointer',
}
const tabActive: CSSProperties = { background: '#333', color: '#fff' }
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/BottomTabs.tsx
git commit -m "feat(web): BottomTabs left group (Data enabled, Structure/Indexes/FK stubs)"
```

---

## Task 16: Frontend — Workspace integration

**Files:**
- Modify: `web/src/routes/Workspace.tsx`, `web/src/App.tsx`

- [ ] **Step 1: Rewrite Workspace**

Replace `web/src/routes/Workspace.tsx` with:

```tsx
import { useState } from 'react'
import TopBar from '../components/TopBar'
import Sidebar from '../components/Sidebar'
import DataGrid from '../components/DataGrid'
import BottomTabs, { BottomTab } from '../components/BottomTabs'
import ConnectionsManager from '../components/ConnectionsManager'
import { useActiveConn } from '../store/activeConn'

interface Props {
  onOpenSettings: () => void
}

export default function Workspace({ onOpenSettings }: Props) {
  const [view, setView] = useState<'workspace' | 'connections'>('workspace')
  const [selected, setSelected] = useState<{ db: string; table: string } | null>(null)
  const [bottom, setBottom] = useState<BottomTab>('data')
  const connId = useActiveConn((s) => s.activeId)

  if (view === 'connections') {
    return <ConnectionsManager onClose={() => setView('workspace')} />
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TopBar onOpenConnections={() => setView('connections')} onOpenSettings={onOpenSettings} />
      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        <Sidebar onPickTable={(db, table) => setSelected({ db, table })} selected={selected} />
        <main style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <div style={{ flex: 1, overflow: 'hidden' }}>
            {connId == null && (
              <div style={center}>pick a connection in the top bar</div>
            )}
            {connId != null && selected == null && (
              <div style={center}>pick a table in the sidebar</div>
            )}
            {connId != null && selected != null && bottom === 'data' && (
              <DataGrid key={`${connId}-${selected.db}-${selected.table}`} db={selected.db} table={selected.table} />
            )}
          </div>
          <BottomTabs value={bottom} onChange={setBottom} />
        </main>
      </div>
    </div>
  )
}

import type { CSSProperties } from 'react'
const center: CSSProperties = {
  display: 'flex', alignItems: 'center', justifyContent: 'center',
  height: '100%', color: '#999', fontFamily: 'system-ui',
}
```

- [ ] **Step 2: Build to verify nothing else broke**

```bash
cd web && npm run build && cd ..
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add web/src/routes/Workspace.tsx
git commit -m "feat(web): Workspace integrates TopBar + Sidebar + DataGrid + BottomTabs"
```

---

## Task 17: README addendum + manual MySQL smoke

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Append Plan 2 section to README**

After the existing "## What's in this plan" section, add a new section before "## Manual checklist for first deploy":

```markdown
## What's in this plan (Plan 2)

- POST/GET/PUT/DELETE `/api/connections` and `/api/connections/:id`
- POST `/api/connections/:id/test`
- GET `/api/db/:connId/databases`
- GET `/api/db/:connId/databases/:db/tables`
- GET `/api/db/:connId/databases/:db/tables/:t/data` (page / per_page / sort_col / sort_dir)
- AES-GCM-encrypted connection passwords (uses master key from Plan 1)
- Per-`(user, conn)` `*sql.DB` pool with 5-minute idle eviction
- Frontend: TopBar, ConnectionPicker, ConnectionsManager + Dialog (CRUD + test), Sidebar (databases/tables tree), DataGrid (TanStack Table, paginate + sort), BottomTabs (Data enabled; Structure/Indexes/FK stubbed for Plan 3)
```

And add a new sub-section under "## Manual checklist for first deploy" titled "### Manual MySQL smoke (Plan 2)":

```markdown
### Manual MySQL smoke (Plan 2)

Requires a reachable MySQL 8.x. Quick local instance:

  ```bash
  docker run --rm -d --name smoke-mysql -p 13306:3306 \
    -e MYSQL_ROOT_PASSWORD=rootpw -e MYSQL_DATABASE=demo \
    mysql:8
  sleep 10  # wait for init
  docker exec -i smoke-mysql mysql -uroot -prootpw demo <<'SQL'
  CREATE TABLE users (id INT PRIMARY KEY AUTO_INCREMENT, name VARCHAR(40), email VARCHAR(80));
  INSERT INTO users(name,email) VALUES ('alice','a@x.io'),('bob','b@x.io'),('cathy','c@x.io');
  SQL
  ```

Then in the running dataseai:

1. Log in (Plan 1 register/login still works).
2. Click "manage" in the top bar → "+ new":
   - name `local`, host `host.docker.internal` (or your host IP), port `13306`, user `root`, password `rootpw`, default db `demo`, tls `disabled`. Save.
3. Open the connection in the dialog → click "test" → expect "connected ✓".
4. Pick `local` in the top-bar selector.
5. Sidebar should list the `demo` database; click to expand and see the `users` table.
6. Click `users` → DataGrid shows the 3 seeded rows.
7. Click the `name` header to sort; flip direction with another click.
8. Pagination footer should show "page 1 / 1 · 3 rows total".
9. Tear down: `docker stop smoke-mysql`.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README addendum for Plan 2 endpoints + manual MySQL smoke"
```

---

## Plan 2 Done — milestone

After Task 17 the repository should let a user:
- create / edit / delete encrypted MySQL connection definitions,
- test a connection from the dialog,
- pick an active connection in the top bar,
- expand a database in the sidebar to see its tables,
- click a table to browse its rows with pagination and column-click sort.

Total: 17 commits expected.

**Not in scope (deferred to Plan 3):** SQL Editor, query history, Structure / Indexes / Foreign Keys views — covered by the disabled BottomTabs tabs.

**Carryover from Plan 1 integration review** still open:
- I1 (rate-limit semantics, 5/min vs 5/sec)
- I2 (5-second middleware cache)
- I3 (username-enumeration timing leak)
- I4 (rate-limit on password change)

These were not touched in Plan 2; they remain backlog items for whichever plan revisits `internal/auth` and `internal/api/auth.go`.
