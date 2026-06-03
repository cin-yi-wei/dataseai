# Plan 1 — Foundation + Auth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the mysqlweb Go service + React SPA shell with user registration, login, multi-session management, and a Docker-compose-ready deployment. End state: a user can `docker compose up`, register, log in, see an empty Workspace placeholder, change password, and revoke other sessions.

**Architecture:** Single Go binary (chi router) serving an embedded React/Vite SPA. Tool state in a file-backed sqlite. Sessions are token-based and persisted (multi-device friendly, revocable). AES-GCM crypto package and master-key bootstrap are wired up now (used by later plans). No external MySQL or MCP integration yet.

**Tech Stack:**
- Go 1.22, chi v5 router, `golang.org/x/crypto/bcrypt`, `golang.org/x/time/rate`, `github.com/mattn/go-sqlite3` (CGO sqlite driver)
- React 18 + Vite + TypeScript, Zustand for state, plain `fetch` for API
- Docker multi-stage build, alpine final image

**Spec reference:** `docs/superpowers/specs/2026-06-03-mysqlweb-design.md` (Sections 1, 2, 3, 4, 5, 6.1, 7, 10).

---

## File Structure (created by this plan)

```
mysqlweb/
├── cmd/mysqlweb/main.go                  # entrypoint (wires everything)
├── internal/
│   ├── config/config.go                  # ENV -> Config struct
│   ├── config/config_test.go
│   ├── store/db.go                       # sql.Open + ping
│   ├── store/migrate.go                  # embedded SQL migration runner
│   ├── store/migrate_test.go
│   ├── store/migrations/
│   │   ├── 0001_init.sql                 # schema_migrations table
│   │   ├── 0002_users.sql
│   │   └── 0003_sessions.sql
│   ├── store/users.go
│   ├── store/users_test.go
│   ├── store/sessions.go
│   ├── store/sessions_test.go
│   ├── crypto/aesgcm.go
│   ├── crypto/aesgcm_test.go
│   ├── crypto/masterkey.go               # load/generate master key
│   ├── crypto/masterkey_test.go
│   ├── auth/token.go                     # crypto/rand 32-byte hex token
│   ├── auth/token_test.go
│   ├── auth/middleware.go                # Bearer-token middleware
│   ├── auth/middleware_test.go
│   ├── api/auth.go                       # register/login/logout/me/password/sessions handlers
│   ├── api/auth_test.go
│   ├── api/ratelimit.go                  # in-memory per-IP rate limiter
│   ├── api/ratelimit_test.go
│   ├── api/health.go
│   └── api/router.go                     # builds chi router (test-friendly)
├── web/
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       ├── lib/api.ts
│       ├── store/auth.ts
│       ├── routes/Login.tsx
│       ├── routes/Register.tsx
│       ├── routes/Workspace.tsx
│       └── routes/Settings.tsx
├── embed.go                              # //go:embed web/dist
├── Dockerfile
├── docker-compose.yml
├── .env.example
├── .github/workflows/ci.yml
├── README.md
├── go.mod
└── go.sum
```

**Conventions used throughout this plan:**
- All Go module imports use module path `github.com/conray/mysqlweb` (rename in T1 if you publish under a different org).
- Test files live beside the source file using `_test.go`.
- Each task ends with a single `git commit` covering only the files it changed.

---

## Task 1: Init Go module + chi router skeleton + /api/health

**Files:**
- Create: `go.mod`, `cmd/mysqlweb/main.go`, `internal/api/health.go`, `internal/api/router.go`
- Test: `internal/api/health_test.go`

- [ ] **Step 1: Initialize Go module**

```bash
cd /home/conray/project/mysqlweb
go mod init github.com/conray/mysqlweb
go get github.com/go-chi/chi/v5@v5.1.0
```

- [ ] **Step 2: Write the failing health-endpoint test**

Create `internal/api/health_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth_Returns200WithVersion(t *testing.T) {
	r := NewRouter(Deps{Version: "test-v"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.OK || body.Version != "test-v" {
		t.Fatalf("body = %+v", body)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/api/ -run TestHealth -v
```

Expected: FAIL — `NewRouter` undefined, `Deps` undefined.

- [ ] **Step 4: Implement health handler + router builder**

Create `internal/api/health.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"time"
)

var startedAt = time.Now()

func handleHealth(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        true,
			"version":   version,
			"uptime_s":  int(time.Since(startedAt).Seconds()),
		})
	}
}
```

Create `internal/api/router.go`:

```go
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Deps struct {
	Version string
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", handleHealth(d.Version))
	return r
}
```

- [ ] **Step 5: Run test to verify pass**

```bash
go test ./internal/api/ -run TestHealth -v
```

Expected: PASS.

- [ ] **Step 6: Add minimal main.go**

Create `cmd/mysqlweb/main.go`:

```go
package main

import (
	"log"
	"net/http"

	"github.com/conray/mysqlweb/internal/api"
)

var version = "dev"

func main() {
	r := api.NewRouter(api.Deps{Version: version})
	addr := ":53306"
	log.Printf("mysqlweb listening on %s (version=%s)", addr, version)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 7: Build & sanity boot**

```bash
go build -o /tmp/mysqlweb ./cmd/mysqlweb
/tmp/mysqlweb &
PID=$!
sleep 0.3
curl -s http://localhost:53306/api/health
kill $PID
```

Expected: `{"ok":true,"uptime_s":0,"version":"dev"}` printed.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum cmd/ internal/api/
git commit -m "feat(api): chi router skeleton with /api/health"
```

---

## Task 2: Config package (ENV → Config struct)

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/config/config_test.go`:

```go
package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("MYSQLWEB_PORT", "")
	t.Setenv("MYSQLWEB_DB_PATH", "")
	t.Setenv("MYSQLWEB_MASTER_KEY", "")
	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c.Port != 53306 {
		t.Errorf("Port = %d, want 53306", c.Port)
	}
	if c.DBPath != "/data/mysqlweb.db" {
		t.Errorf("DBPath = %q, want /data/mysqlweb.db", c.DBPath)
	}
	if c.Registration != "open" {
		t.Errorf("Registration = %q, want open", c.Registration)
	}
	if c.HistoryMax != 1000 {
		t.Errorf("HistoryMax = %d, want 1000", c.HistoryMax)
	}
}

func TestLoad_FromEnv(t *testing.T) {
	t.Setenv("MYSQLWEB_PORT", "9999")
	t.Setenv("MYSQLWEB_DB_PATH", "/tmp/x.db")
	t.Setenv("MYSQLWEB_REGISTRATION", "closed")
	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c.Port != 9999 {
		t.Errorf("Port = %d", c.Port)
	}
	if c.DBPath != "/tmp/x.db" {
		t.Errorf("DBPath = %q", c.DBPath)
	}
	if c.Registration != "closed" {
		t.Errorf("Registration = %q", c.Registration)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	t.Setenv("MYSQLWEB_PORT", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/config/ -v
```

Expected: FAIL — `Load`, `Config` undefined.

- [ ] **Step 3: Implement config**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port               int
	DBPath             string
	MasterKeyHex       string // empty means "generate on first launch"
	Registration       string // "open" | "closed"
	HistoryMax         int
	QueryTimeoutS      int
	QueryHTTPMaxMB     int
	LLMDefault         string // "anthropic" | "openai"
	AnthropicAPIKey    string
	OpenAIAPIKey       string
	MCPMySQLURL        string
}

func Load() (Config, error) {
	c := Config{
		Port:           53306,
		DBPath:         "/data/mysqlweb.db",
		Registration:   "open",
		HistoryMax:     1000,
		QueryTimeoutS:  5,
		QueryHTTPMaxMB: 10,
		LLMDefault:     "anthropic",
	}
	if v := os.Getenv("MYSQLWEB_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("MYSQLWEB_PORT: %w", err)
		}
		c.Port = p
	}
	if v := os.Getenv("MYSQLWEB_DB_PATH"); v != "" {
		c.DBPath = v
	}
	if v := os.Getenv("MYSQLWEB_MASTER_KEY"); v != "" {
		c.MasterKeyHex = v
	}
	if v := os.Getenv("MYSQLWEB_REGISTRATION"); v != "" {
		c.Registration = v
	}
	if v := os.Getenv("MYSQLWEB_HISTORY_MAX"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("MYSQLWEB_HISTORY_MAX: %w", err)
		}
		c.HistoryMax = n
	}
	if v := os.Getenv("MYSQLWEB_QUERY_TIMEOUT_S"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("MYSQLWEB_QUERY_TIMEOUT_S: %w", err)
		}
		c.QueryTimeoutS = n
	}
	if v := os.Getenv("MYSQLWEB_QUERY_HTTP_MAX_MB"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("MYSQLWEB_QUERY_HTTP_MAX_MB: %w", err)
		}
		c.QueryHTTPMaxMB = n
	}
	if v := os.Getenv("MYSQLWEB_LLM_DEFAULT"); v != "" {
		c.LLMDefault = v
	}
	c.AnthropicAPIKey = os.Getenv("ANTHROPIC_API_KEY")
	c.OpenAIAPIKey = os.Getenv("OPENAI_API_KEY")
	c.MCPMySQLURL = os.Getenv("MCP_MYSQL_URL")
	return c, nil
}
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/config/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): env-driven Config with sensible defaults"
```

---

## Task 3: Sqlite store + migration runner

**Files:**
- Create: `internal/store/db.go`, `internal/store/migrate.go`, `internal/store/migrations/0001_init.sql`
- Test: `internal/store/migrate_test.go`

- [ ] **Step 1: Add sqlite driver dependency**

```bash
go get github.com/mattn/go-sqlite3@v1.14.22
```

- [ ] **Step 2: Write the failing migration test**

Create `internal/store/migrate_test.go`:

```go
package store

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openMem(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrate_AppliesAllAndIsIdempotent(t *testing.T) {
	db := openMem(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// run twice — should be a no-op
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 migration applied, got %d", n)
	}
}
```

- [ ] **Step 3: Run to verify fail**

```bash
go test ./internal/store/ -v
```

Expected: FAIL — `Migrate` undefined.

- [ ] **Step 4: Implement migration runner**

Create `internal/store/migrations/0001_init.sql`:

```sql
CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

Create `internal/store/db.go`:

```go
package store

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal=WAL&_busy_timeout=5000&_fk=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}
```

Create `internal/store/migrate.go`:

```go
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".sql")
		// expect "NNNN_name"
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("bad migration filename %q", e.Name())
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("bad version in %q: %w", e.Name(), err)
		}
		raw, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{version: v, name: parts[1], sql: string(raw)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	return ms, nil
}

func Migrate(db *sql.DB) error {
	ms, err := loadMigrations()
	if err != nil {
		return err
	}
	// Bootstrap: apply 0001 unconditionally if schema_migrations doesn't exist
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'")
	var tableName string
	if err := row.Scan(&tableName); err == sql.ErrNoRows {
		// no migrations table yet — apply 0001 which CREATEs it, then record
		if len(ms) == 0 || ms[0].version != 1 {
			return fmt.Errorf("migration 0001 missing")
		}
		if _, err := db.Exec(ms[0].sql); err != nil {
			return fmt.Errorf("apply 0001: %w", err)
		}
		if _, err := db.Exec("INSERT INTO schema_migrations(version) VALUES(?)", 1); err != nil {
			return fmt.Errorf("record 0001: %w", err)
		}
		ms = ms[1:]
	} else if err != nil {
		return err
	}
	// Apply each migration not yet applied
	applied := map[int]bool{1: true}
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		_ = rows.Scan(&v)
		applied[v] = true
	}
	_ = rows.Close()
	for _, m := range ms {
		if applied[m.version] {
			continue
		}
		if _, err := db.Exec(m.sql); err != nil {
			return fmt.Errorf("apply %04d_%s: %w", m.version, m.name, err)
		}
		if _, err := db.Exec("INSERT INTO schema_migrations(version) VALUES(?)", m.version); err != nil {
			return fmt.Errorf("record %d: %w", m.version, err)
		}
	}
	return nil
}
```

- [ ] **Step 5: Run to verify pass**

```bash
go test ./internal/store/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat(store): sqlite open + embedded SQL migration runner"
```

---

## Task 4: Users table + store

**Files:**
- Create: `internal/store/migrations/0002_users.sql`, `internal/store/users.go`
- Test: `internal/store/users_test.go`

- [ ] **Step 1: Add bcrypt dep**

```bash
go get golang.org/x/crypto/bcrypt
```

- [ ] **Step 2: Write users.sql migration**

Create `internal/store/migrations/0002_users.sql`:

```sql
CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

- [ ] **Step 3: Write the failing tests**

Create `internal/store/users_test.go`:

```go
package store

import (
	"errors"
	"testing"
)

func setupUsers(t *testing.T) *Store {
	t.Helper()
	db := openMem(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &Store{DB: db}
}

func TestCreateUser_StoresHashedPassword(t *testing.T) {
	s := setupUsers(t)
	u, err := s.CreateUser("alice", "supersecret123")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == 0 || u.Username != "alice" {
		t.Fatalf("user = %+v", u)
	}
	// fetch the raw hash
	var hash string
	if err := s.DB.QueryRow("SELECT password_hash FROM users WHERE id=?", u.ID).Scan(&hash); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if hash == "supersecret123" || hash == "" {
		t.Fatalf("password not hashed: %q", hash)
	}
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	s := setupUsers(t)
	if _, err := s.CreateUser("alice", "supersecret123"); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateUser("alice", "anotherpassword")
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestVerifyPassword_HappyPath(t *testing.T) {
	s := setupUsers(t)
	u, _ := s.CreateUser("alice", "supersecret123")
	got, err := s.VerifyPassword("alice", "supersecret123")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID {
		t.Fatalf("id mismatch")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	s := setupUsers(t)
	_, _ = s.CreateUser("alice", "supersecret123")
	_, err := s.VerifyPassword("alice", "wrong")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestVerifyPassword_UnknownUser(t *testing.T) {
	s := setupUsers(t)
	_, err := s.VerifyPassword("ghost", "x")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}
```

- [ ] **Step 4: Run to verify fail**

```bash
go test ./internal/store/ -run TestCreateUser -v
```

Expected: FAIL — `Store`, `CreateUser` etc. undefined.

- [ ] **Step 5: Implement users + Store**

Create `internal/store/users.go`:

```go
package store

import (
	"database/sql"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDuplicate          = errors.New("duplicate")
	ErrNotFound           = errors.New("not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type Store struct {
	DB *sql.DB
}

type User struct {
	ID       int64
	Username string
}

func (s *Store) CreateUser(username, password string) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	res, err := s.DB.Exec("INSERT INTO users(username, password_hash) VALUES(?, ?)", username, string(hash))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return User{}, ErrDuplicate
		}
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{ID: id, Username: username}, nil
}

func (s *Store) GetUserByID(id int64) (User, error) {
	row := s.DB.QueryRow("SELECT id, username FROM users WHERE id=?", id)
	var u User
	if err := row.Scan(&u.ID, &u.Username); err != nil {
		if err == sql.ErrNoRows {
			return u, ErrNotFound
		}
		return u, err
	}
	return u, nil
}

func (s *Store) VerifyPassword(username, password string) (User, error) {
	row := s.DB.QueryRow("SELECT id, username, password_hash FROM users WHERE username=?", username)
	var u User
	var hash string
	if err := row.Scan(&u.ID, &u.Username, &hash); err != nil {
		if err == sql.ErrNoRows {
			return u, ErrInvalidCredentials
		}
		return u, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}
	return u, nil
}

func (s *Store) UpdatePassword(userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("UPDATE users SET password_hash=? WHERE id=?", string(hash), userID)
	return err
}
```

- [ ] **Step 6: Run to verify pass**

```bash
go test ./internal/store/ -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/store/
git commit -m "feat(store): users table + bcrypt-backed CRUD"
```

---

## Task 5: Sessions table + store

**Files:**
- Create: `internal/store/migrations/0003_sessions.sql`, `internal/store/sessions.go`
- Test: `internal/store/sessions_test.go`

- [ ] **Step 1: Sessions migration**

Create `internal/store/migrations/0003_sessions.sql`:

```sql
CREATE TABLE sessions (
  token         TEXT PRIMARY KEY,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  last_used_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  user_agent    TEXT,
  expires_at    DATETIME NOT NULL
);
CREATE INDEX idx_sessions_user ON sessions(user_id);
```

- [ ] **Step 2: Write failing tests**

Create `internal/store/sessions_test.go`:

```go
package store

import (
	"errors"
	"testing"
	"time"
)

func setupSessions(t *testing.T) (*Store, User) {
	t.Helper()
	s := setupUsers(t)
	u, err := s.CreateUser("alice", "supersecret123")
	if err != nil {
		t.Fatal(err)
	}
	return s, u
}

func TestCreateSession_ReturnsTokenAndPersists(t *testing.T) {
	s, u := setupSessions(t)
	sess, err := s.CreateSession(u.ID, "Mozilla/5.0", 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Token) != 64 {
		t.Fatalf("token length = %d, want 64", len(sess.Token))
	}
	if sess.UserID != u.ID {
		t.Fatalf("user_id mismatch")
	}
	got, err := s.GetSession(sess.Token)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UserID != u.ID {
		t.Fatalf("get user_id mismatch")
	}
}

func TestGetSession_Expired(t *testing.T) {
	s, u := setupSessions(t)
	sess, _ := s.CreateSession(u.ID, "ua", -1*time.Hour) // already expired
	_, err := s.GetSession(sess.Token)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("want ErrSessionExpired, got %v", err)
	}
}

func TestDeleteSession(t *testing.T) {
	s, u := setupSessions(t)
	sess, _ := s.CreateSession(u.ID, "ua", time.Hour)
	if err := s.DeleteSession(sess.Token); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetSession(sess.Token)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestListSessionsByUser(t *testing.T) {
	s, u := setupSessions(t)
	_, _ = s.CreateSession(u.ID, "laptop", time.Hour)
	_, _ = s.CreateSession(u.ID, "phone", time.Hour)
	list, err := s.ListSessionsByUser(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
}

func TestRefreshSession_UpdatesLastUsed(t *testing.T) {
	s, u := setupSessions(t)
	sess, _ := s.CreateSession(u.ID, "ua", time.Hour)
	time.Sleep(1100 * time.Millisecond)
	if err := s.RefreshSession(sess.Token, time.Hour); err != nil {
		t.Fatal(err)
	}
	var lastUsed time.Time
	_ = s.DB.QueryRow("SELECT last_used_at FROM sessions WHERE token=?", sess.Token).Scan(&lastUsed)
	if time.Since(lastUsed) > 500*time.Millisecond {
		t.Fatalf("last_used_at not refreshed: %v", lastUsed)
	}
}

func TestDeleteUserSessionsExcept(t *testing.T) {
	s, u := setupSessions(t)
	keep, _ := s.CreateSession(u.ID, "keep", time.Hour)
	_, _ = s.CreateSession(u.ID, "drop1", time.Hour)
	_, _ = s.CreateSession(u.ID, "drop2", time.Hour)
	if err := s.DeleteUserSessionsExcept(u.ID, keep.Token); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListSessionsByUser(u.ID)
	if len(list) != 1 || list[0].Token != keep.Token {
		t.Fatalf("kept = %+v", list)
	}
}
```

- [ ] **Step 3: Run to verify fail**

```bash
go test ./internal/store/ -run Session -v
```

Expected: FAIL.

- [ ] **Step 4: Implement sessions**

Create `internal/store/sessions.go`:

```go
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

var ErrSessionExpired = errors.New("session expired")

type Session struct {
	Token      string
	UserID     int64
	CreatedAt  time.Time
	LastUsedAt time.Time
	UserAgent  string
	ExpiresAt  time.Time
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Store) CreateSession(userID int64, ua string, ttl time.Duration) (Session, error) {
	tok, err := newToken()
	if err != nil {
		return Session{}, err
	}
	exp := time.Now().Add(ttl)
	if _, err := s.DB.Exec(
		"INSERT INTO sessions(token, user_id, user_agent, expires_at) VALUES(?,?,?,?)",
		tok, userID, ua, exp,
	); err != nil {
		return Session{}, err
	}
	return Session{Token: tok, UserID: userID, UserAgent: ua, ExpiresAt: exp}, nil
}

func (s *Store) GetSession(token string) (Session, error) {
	row := s.DB.QueryRow(
		"SELECT token, user_id, created_at, last_used_at, user_agent, expires_at FROM sessions WHERE token=?",
		token,
	)
	var sess Session
	if err := row.Scan(&sess.Token, &sess.UserID, &sess.CreatedAt, &sess.LastUsedAt, &sess.UserAgent, &sess.ExpiresAt); err != nil {
		if err == sql.ErrNoRows {
			return sess, ErrNotFound
		}
		return sess, err
	}
	if time.Now().After(sess.ExpiresAt) {
		return sess, ErrSessionExpired
	}
	return sess, nil
}

func (s *Store) RefreshSession(token string, ttl time.Duration) error {
	_, err := s.DB.Exec(
		"UPDATE sessions SET last_used_at=CURRENT_TIMESTAMP, expires_at=? WHERE token=?",
		time.Now().Add(ttl), token,
	)
	return err
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE token=?", token)
	return err
}

func (s *Store) DeleteUserSessionsExcept(userID int64, keepToken string) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE user_id=? AND token<>?", userID, keepToken)
	return err
}

func (s *Store) ListSessionsByUser(userID int64) ([]Session, error) {
	rows, err := s.DB.Query(
		"SELECT token, user_id, created_at, last_used_at, user_agent, expires_at FROM sessions WHERE user_id=? ORDER BY last_used_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.Token, &sess.UserID, &sess.CreatedAt, &sess.LastUsedAt, &sess.UserAgent, &sess.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Run to verify pass**

```bash
go test ./internal/store/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat(store): sessions table + token-based session CRUD"
```

---

## Task 6: Crypto package (AES-GCM)

**Files:**
- Create: `internal/crypto/aesgcm.go`
- Test: `internal/crypto/aesgcm_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/crypto/aesgcm_test.go`:

```go
package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func randKey() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	c, err := New(randKey())
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("hello prod-db password!")
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, dec) {
		t.Fatalf("got %q, want %q", dec, plain)
	}
}

func TestEncrypt_DifferentCiphertextForSamePlaintext(t *testing.T) {
	c, _ := New(randKey())
	a, _ := c.Encrypt([]byte("same"))
	b, _ := c.Encrypt([]byte("same"))
	if bytes.Equal(a, b) {
		t.Fatal("nonce reuse — ciphertext should differ")
	}
}

func TestDecrypt_WrongKeyFails(t *testing.T) {
	c1, _ := New(randKey())
	enc, _ := c1.Encrypt([]byte("secret"))
	c2, _ := New(randKey())
	if _, err := c2.Decrypt(enc); err == nil {
		t.Fatal("expected decrypt to fail with wrong key")
	}
}

func TestNew_RejectsBadKeyLength(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Fatal("16-byte key should be rejected")
	}
}
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/crypto/ -v
```

Expected: FAIL — `New` undefined.

- [ ] **Step 3: Implement AES-GCM cipher**

Create `internal/crypto/aesgcm.go`:

```go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

var ErrKeyLength = errors.New("master key must be 32 bytes (AES-256)")

type Cipher struct {
	aead cipher.AEAD
}

func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, ErrKeyLength
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns nonce || ciphertext || tag.
func (c *Cipher) Encrypt(plain []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := c.aead.Seal(nil, nonce, plain, nil)
	out := make([]byte, 0, len(nonce)+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

func (c *Cipher) Decrypt(blob []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(blob) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := blob[:ns], blob[ns:]
	return c.aead.Open(nil, nonce, ct, nil)
}
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/crypto/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/crypto/
git commit -m "feat(crypto): AES-GCM encrypt/decrypt with nonce-prefixed blob"
```

---

## Task 7: Master key bootstrap

**Files:**
- Create: `internal/crypto/masterkey.go`
- Test: `internal/crypto/masterkey_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/crypto/masterkey_test.go`:

```go
package crypto

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGenerateKey_FromHex(t *testing.T) {
	dir := t.TempDir()
	hexKey := hex.EncodeToString(make([]byte, 32))
	key, source, err := LoadOrGenerateKey(hexKey, filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("key len = %d", len(key))
	}
	if source != "env" {
		t.Fatalf("source = %q, want env", source)
	}
}

func TestLoadOrGenerateKey_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.key")
	// Pre-write a key file
	hexKey := hex.EncodeToString(make([]byte, 32))
	if err := os.WriteFile(path, []byte(hexKey), 0600); err != nil {
		t.Fatal(err)
	}
	_, source, err := LoadOrGenerateKey("", path)
	if err != nil {
		t.Fatal(err)
	}
	if source != "file" {
		t.Fatalf("source = %q, want file", source)
	}
}

func TestLoadOrGenerateKey_GeneratesAndPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.key")
	key1, source, err := LoadOrGenerateKey("", path)
	if err != nil {
		t.Fatal(err)
	}
	if source != "generated" {
		t.Fatalf("source = %q, want generated", source)
	}
	// second call should read the freshly-persisted key
	key2, source, err := LoadOrGenerateKey("", path)
	if err != nil {
		t.Fatal(err)
	}
	if source != "file" {
		t.Fatalf("second source = %q, want file", source)
	}
	if string(key1) != string(key2) {
		t.Fatal("persisted key differs from generated")
	}
}

func TestLoadOrGenerateKey_BadHexFails(t *testing.T) {
	dir := t.TempDir()
	_, _, err := LoadOrGenerateKey("not-hex-at-all", filepath.Join(dir, "master.key"))
	if err == nil {
		t.Fatal("expected error for bad hex")
	}
}
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/crypto/ -run LoadOrGenerate -v
```

Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/crypto/masterkey.go`:

```go
package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// LoadOrGenerateKey returns a 32-byte AES key. Sources in priority order:
//  1. envHex non-empty → decode hex and return
//  2. file at path exists → read hex and return
//  3. generate fresh 32 bytes, write to path (0600), return
//
// Source string is one of: "env", "file", "generated".
func LoadOrGenerateKey(envHex, path string) ([]byte, string, error) {
	if envHex != "" {
		k, err := hex.DecodeString(envHex)
		if err != nil {
			return nil, "", fmt.Errorf("decode env hex: %w", err)
		}
		if len(k) != 32 {
			return nil, "", ErrKeyLength
		}
		return k, "env", nil
	}
	if data, err := os.ReadFile(path); err == nil {
		k, err := hex.DecodeString(string(data))
		if err != nil {
			return nil, "", fmt.Errorf("decode file hex: %w", err)
		}
		if len(k) != 32 {
			return nil, "", ErrKeyLength
		}
		return k, "file", nil
	}
	// generate
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(k)), 0600); err != nil {
		return nil, "", err
	}
	return k, "generated", nil
}
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/crypto/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/crypto/
git commit -m "feat(crypto): master key bootstrap (env > file > generate)"
```

---

## Task 8: Auth middleware (Bearer token → user context)

**Files:**
- Create: `internal/auth/middleware.go`
- Test: `internal/auth/middleware_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/auth/middleware_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/auth/ -v
```

Expected: FAIL — `Middleware`, `UserFromContext` undefined.

- [ ] **Step 3: Implement**

Create `internal/auth/middleware.go`:

```go
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
			// fire-and-forget sliding refresh; ignore error
			_ = s.RefreshSession(tok, SessionTTL)
			ctx := context.WithValue(r.Context(), userKey, UserCtx{ID: u.ID, Username: u.Username})
			ctx = context.WithValue(ctx, sessionKey, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/auth/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/
git commit -m "feat(auth): Bearer-token middleware with sliding refresh"
```

---

## Task 9: Register handler

**Files:**
- Create: `internal/api/auth.go` (initial), `internal/api/auth_test.go`
- Modify: `internal/api/router.go` (wire register route)

- [ ] **Step 1: Write failing test**

Create `internal/api/auth_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/api/ -run TestRegister -v
```

Expected: FAIL.

- [ ] **Step 3: Implement register + extend Deps + wire route**

Create `internal/api/auth.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/store"
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
```

Replace `internal/api/router.go` with:

```go
package api

import (
	"net/http"

	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)

type Deps struct {
	Version      string
	Store        *store.Store
	Registration string // "open" | "closed"
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", handleHealth(d.Version))
	r.Post("/api/auth/register", handleRegister(d))
	return r
}
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/api/ -v
```

Expected: PASS (including the existing TestHealth — the Deps now has more fields but TestHealth still works because it only relied on Version).

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat(api): POST /api/auth/register with validation + dup check"
```

---

## Task 10: Login handler

**Files:**
- Modify: `internal/api/auth.go` (add `handleLogin`), `internal/api/router.go` (wire route)
- Modify: `internal/api/auth_test.go` (add login tests)

- [ ] **Step 1: Add failing login tests**

Append to `internal/api/auth_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/api/ -run TestLogin -v
```

Expected: FAIL.

- [ ] **Step 3: Implement handleLogin and wire route**

Append to `internal/api/auth.go`:

```go
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
```

Modify `internal/api/router.go` to add the route:

```go
	r.Post("/api/auth/register", handleRegister(d))
	r.Post("/api/auth/login", handleLogin(d))
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/api/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat(api): POST /api/auth/login"
```

---

## Task 11: Logout + /me handlers

**Files:**
- Modify: `internal/api/auth.go`, `internal/api/router.go`, `internal/api/auth_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/api/auth_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/api/ -run "TestMe|TestLogout" -v
```

Expected: FAIL.

- [ ] **Step 3: Implement handlers + wire authenticated subrouter**

Append to `internal/api/auth.go`:

```go
func handleMe(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"user": map[string]any{"id": u.ID, "username": u.Username},
		})
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
```

Modify `internal/api/router.go` to add an authenticated subrouter:

```go
package api

import (
	"net/http"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)

type Deps struct {
	Version      string
	Store        *store.Store
	Registration string
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", handleHealth(d.Version))
	r.Post("/api/auth/register", handleRegister(d))
	r.Post("/api/auth/login", handleLogin(d))

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(d.Store))
		r.Get("/api/auth/me", handleMe(d))
		r.Post("/api/auth/logout", handleLogout(d))
	})
	return r
}
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/api/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat(api): /api/auth/me + logout behind auth middleware"
```

---

## Task 12: Password change handler

**Files:**
- Modify: `internal/api/auth.go`, `internal/api/router.go`, `internal/api/auth_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/api/auth_test.go`:

```go
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
	// open a second session
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
	// tok1 still works
	if rc := get(t, r, "/api/auth/me", tok1).Code; rc != http.StatusOK {
		t.Fatalf("tok1 me = %d (should still work)", rc)
	}
	// tok2 is revoked
	if rc := get(t, r, "/api/auth/me", tok2).Code; rc != http.StatusUnauthorized {
		t.Fatalf("tok2 me = %d (should be revoked)", rc)
	}
	// new password works for next login
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
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/api/ -run TestChangePassword -v
```

Expected: FAIL.

- [ ] **Step 3: Implement password change**

Append to `internal/api/auth.go`:

```go
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
```

Add route in `router.go` inside the auth-middleware group:

```go
		r.Put("/api/auth/password", handlePasswordChange(d))
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/api/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat(api): PUT /api/auth/password (revokes other sessions)"
```

---

## Task 13: List sessions + revoke session handlers

**Files:**
- Modify: `internal/api/auth.go`, `internal/api/router.go`, `internal/api/auth_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/api/auth_test.go`:

```go
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
	// open a second session
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
	// must not leak full token
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

	// alice lists & finds tok2 (by prefix), revokes it
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
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/api/ -run "TestListSessions|TestRevokeSession" -v
```

Expected: FAIL.

- [ ] **Step 3: Implement handlers**

The list/revoke handlers use `strings` and `chi.URLParam`. Update the imports at the top of `internal/api/auth.go` so the import block reads:

```go
import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)
```

Then append the new handlers:

```go
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
				"id":           tokenPrefix(s.Token), // never expose full token
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
		// id is the 8-char prefix; resolve to a full token belonging to this user
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
```

Wire routes in `router.go` inside the auth group:

```go
		r.Get("/api/auth/sessions", handleListSessions(d))
		r.Delete("/api/auth/sessions/{id}", handleRevokeSession(d))
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/api/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat(api): list + revoke sessions (token prefix only)"
```

---

## Task 14: Rate limit middleware for /api/auth/login + /api/auth/register

**Files:**
- Create: `internal/api/ratelimit.go`, `internal/api/ratelimit_test.go`
- Modify: `internal/api/router.go` (apply middleware)

- [ ] **Step 1: Add rate dep**

```bash
go get golang.org/x/time/rate
```

- [ ] **Step 2: Write failing test**

Create `internal/api/ratelimit_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimit_BlocksAfterBurst(t *testing.T) {
	mw := NewRateLimiter(3, 1) // 3-burst, 1/sec refill
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	hit := func() int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}
	if hit() != 200 || hit() != 200 || hit() != 200 {
		t.Fatal("first 3 should succeed")
	}
	if hit() != http.StatusTooManyRequests {
		t.Fatal("4th should be 429")
	}
}

func TestRateLimit_PerIP(t *testing.T) {
	mw := NewRateLimiter(1, 1)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	hit := func(addr string) int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = addr + ":1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}
	if hit("10.0.0.1") != 200 || hit("10.0.0.2") != 200 {
		t.Fatal("first hit per IP should pass")
	}
	if hit("10.0.0.1") != 429 {
		t.Fatal("10.0.0.1 second should be 429")
	}
	if hit("10.0.0.2") != 429 {
		t.Fatal("10.0.0.2 second should be 429")
	}
}
```

- [ ] **Step 3: Run to verify fail**

```bash
go test ./internal/api/ -run TestRateLimit -v
```

Expected: FAIL — `NewRateLimiter` undefined.

- [ ] **Step 4: Implement rate limiter**

Create `internal/api/ratelimit.go`:

```go
package api

import (
	"net"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// NewRateLimiter returns middleware that allows `burst` requests up front and
// refills at `perSec` tokens per second, tracked per source IP.
func NewRateLimiter(burst int, perSec int) func(http.Handler) http.Handler {
	var (
		mu  sync.Mutex
		ips = map[string]*rate.Limiter{}
	)
	get := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		l, ok := ips[ip]
		if !ok {
			l = rate.NewLimiter(rate.Limit(perSec), burst)
			ips[ip] = l
		}
		return l
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			if !get(host).Allow() {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 5: Wire onto register + login**

In `internal/api/router.go`, wrap the two auth routes:

```go
	loginLimiter := NewRateLimiter(5, 1)    // 5-burst, 1/sec
	registerLimiter := NewRateLimiter(3, 1) // 3-burst, 1/sec
	r.With(registerLimiter).Post("/api/auth/register", handleRegister(d))
	r.With(loginLimiter).Post("/api/auth/login", handleLogin(d))
```

- [ ] **Step 6: Run all api tests**

```bash
go test ./internal/api/ -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/
git commit -m "feat(api): per-IP rate limit on register + login"
```

---

## Task 15: Wire main.go end-to-end (config → store → migrate → router → ListenAndServe)

**Files:**
- Modify: `cmd/mysqlweb/main.go`

- [ ] **Step 1: Rewrite main.go**

Replace `cmd/mysqlweb/main.go` with:

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/conray/mysqlweb/internal/api"
	"github.com/conray/mysqlweb/internal/config"
	"github.com/conray/mysqlweb/internal/crypto"
	"github.com/conray/mysqlweb/internal/store"
)

var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Master key bootstrap (used by later subsystems; load now so a missing/bad
	// key fails fast at boot rather than the first connection write).
	keyPath := filepath.Join(filepath.Dir(cfg.DBPath), "master.key")
	key, source, err := crypto.LoadOrGenerateKey(cfg.MasterKeyHex, keyPath)
	if err != nil {
		log.Fatalf("master key: %v", err)
	}
	if source == "generated" {
		log.Printf("⚠ generated new master key at %s — set MYSQLWEB_MASTER_KEY in env to persist", keyPath)
	}
	if _, err := crypto.New(key); err != nil {
		log.Fatalf("crypto init: %v", err)
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("store open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	s := &store.Store{DB: db}

	r := api.NewRouter(api.Deps{
		Version:      version,
		Store:        s,
		Registration: cfg.Registration,
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("mysqlweb listening on %s (version=%s, key=%s)", addr, version, source)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Smoke-test the binary**

```bash
mkdir -p /tmp/mysqlweb-data
MYSQLWEB_DB_PATH=/tmp/mysqlweb-data/test.db \
MYSQLWEB_PORT=53306 \
  go run ./cmd/mysqlweb &
SVR=$!
sleep 0.5
curl -s http://localhost:53306/api/health
echo
curl -s -X POST http://localhost:53306/api/auth/register \
  -H 'content-type: application/json' \
  -d '{"username":"alice","password":"supersecret123"}'
echo
kill $SVR
rm -rf /tmp/mysqlweb-data
```

Expected output (token will differ):
```
{"ok":true,"uptime_s":0,"version":"dev"}
{"token":"<hex64>","user":{"id":1,"username":"alice"}}
```

- [ ] **Step 3: Run the full test suite once**

```bash
go test ./...
```

Expected: all packages PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/
git commit -m "feat(cmd): wire config → crypto → store → migrate → router"
```

---

## Task 16: Frontend skeleton (Vite + React + TS)

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`, `web/index.html`, `web/src/main.tsx`, `web/src/App.tsx`

- [ ] **Step 1: Create package.json**

Create `web/package.json`:

```json
{
  "name": "mysqlweb-web",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc --noEmit && vite build",
    "preview": "vite preview",
    "test": "vitest run",
    "lint": "eslint src --max-warnings 0"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "zustand": "^4.5.4"
  },
  "devDependencies": {
    "@types/react": "^18.3.3",
    "@types/react-dom": "^18.3.0",
    "@vitejs/plugin-react": "^4.3.1",
    "typescript": "^5.5.4",
    "vite": "^5.4.0",
    "vitest": "^2.0.5",
    "@testing-library/react": "^16.0.0",
    "@testing-library/jest-dom": "^6.4.8",
    "jsdom": "^25.0.0",
    "eslint": "^9.9.0",
    "@typescript-eslint/parser": "^8.2.0",
    "@typescript-eslint/eslint-plugin": "^8.2.0"
  }
}
```

- [ ] **Step 2: Create vite.config.ts**

Create `web/vite.config.ts`:

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:53306',
      '/ws': { target: 'ws://localhost:53306', ws: true },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
  },
})
```

- [ ] **Step 3: Create tsconfig.json**

Create `web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "esModuleInterop": true,
    "isolatedModules": true,
    "skipLibCheck": true,
    "types": ["vitest/globals", "@testing-library/jest-dom"]
  },
  "include": ["src"]
}
```

- [ ] **Step 4: Create index.html and shell sources**

Create `web/index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>mysqlweb</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

Create `web/src/main.tsx`:

```tsx
import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
```

Create `web/src/App.tsx`:

```tsx
import { useEffect, useState } from 'react'

export default function App() {
  const [health, setHealth] = useState<string>('loading...')
  useEffect(() => {
    fetch('/api/health')
      .then((r) => r.json())
      .then((j) => setHealth(`mysqlweb ${j.version} — uptime ${j.uptime_s}s`))
      .catch(() => setHealth('backend unreachable'))
  }, [])
  return (
    <main style={{ fontFamily: 'system-ui', padding: 24 }}>
      <h1>mysqlweb</h1>
      <p>{health}</p>
    </main>
  )
}
```

Create `web/src/test-setup.ts`:

```ts
import '@testing-library/jest-dom'
```

- [ ] **Step 5: Install + dev sanity check**

```bash
cd web
npm install
npm run build        # should succeed
cd ..
```

Expected: `web/dist/` is created.

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/vite.config.ts web/tsconfig.json web/index.html web/src/main.tsx web/src/App.tsx web/src/test-setup.ts web/package-lock.json
git commit -m "feat(web): Vite + React + TS skeleton with /api/health probe"
```

---

## Task 17: Frontend API client + auth store

**Files:**
- Create: `web/src/lib/api.ts`, `web/src/store/auth.ts`
- Test: `web/src/store/auth.test.ts`

- [ ] **Step 1: Implement API client**

Create `web/src/lib/api.ts`:

```ts
const TOKEN_KEY = 'mysqlweb.token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}
export function setToken(t: string | null) {
  if (t === null) localStorage.removeItem(TOKEN_KEY)
  else localStorage.setItem(TOKEN_KEY, t)
}

export class ApiError extends Error {
  constructor(public status: number, public payload: unknown, message: string) {
    super(message)
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Content-Type', 'application/json')
  const t = getToken()
  if (t) headers.set('Authorization', `Bearer ${t}`)
  const res = await fetch(path, { ...init, headers })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const json = text ? JSON.parse(text) : undefined
  if (!res.ok) {
    const msg = (json && (json.error || json.message)) || `HTTP ${res.status}`
    throw new ApiError(res.status, json, msg)
  }
  return json as T
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body: unknown) => request<T>(path, { method: 'POST', body: JSON.stringify(body) }),
  put: <T>(path: string, body: unknown) => request<T>(path, { method: 'PUT', body: JSON.stringify(body) }),
  del: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
```

- [ ] **Step 2: Write failing auth-store test**

Create `web/src/store/auth.test.ts`:

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useAuth } from './auth'
import { setToken } from '../lib/api'

describe('useAuth', () => {
  beforeEach(() => {
    localStorage.clear()
    useAuth.setState({ user: null, ready: false })
  })

  it('starts logged out', () => {
    expect(useAuth.getState().user).toBeNull()
  })

  it('login sets token and user', () => {
    useAuth.getState().login('tok-xyz', { id: 1, username: 'alice' })
    expect(localStorage.getItem('mysqlweb.token')).toBe('tok-xyz')
    expect(useAuth.getState().user?.username).toBe('alice')
  })

  it('logout clears token + user', () => {
    setToken('tok-xyz')
    useAuth.setState({ user: { id: 1, username: 'alice' } })
    useAuth.getState().logout()
    expect(localStorage.getItem('mysqlweb.token')).toBeNull()
    expect(useAuth.getState().user).toBeNull()
  })
})
```

- [ ] **Step 3: Run to verify fail**

```bash
cd web && npm test -- auth.test.ts
```

Expected: FAIL — `./auth` cannot resolve.

- [ ] **Step 4: Implement auth store**

Create `web/src/store/auth.ts`:

```ts
import { create } from 'zustand'
import { api, getToken, setToken } from '../lib/api'

export interface User {
  id: number
  username: string
}

interface AuthState {
  user: User | null
  ready: boolean // becomes true after the initial /me probe completes
  login: (token: string, user: User) => void
  logout: () => Promise<void>
  bootstrap: () => Promise<void>
}

export const useAuth = create<AuthState>((set) => ({
  user: null,
  ready: false,
  login: (token, user) => {
    setToken(token)
    set({ user })
  },
  logout: async () => {
    try {
      await api.post('/api/auth/logout', null)
    } catch {
      // ignore — server may have already revoked
    }
    setToken(null)
    set({ user: null })
  },
  bootstrap: async () => {
    if (!getToken()) {
      set({ ready: true })
      return
    }
    try {
      const me = await api.get<{ user: User }>('/api/auth/me')
      set({ user: me.user, ready: true })
    } catch {
      setToken(null)
      set({ user: null, ready: true })
    }
  },
}))
```

- [ ] **Step 5: Run to verify pass**

```bash
cd web && npm test -- auth.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/ web/src/store/
git commit -m "feat(web): API client + Zustand auth store with bootstrap"
```

---

## Task 18: Login + Register pages

**Files:**
- Create: `web/src/routes/Login.tsx`, `web/src/routes/Register.tsx`

- [ ] **Step 1: Implement Login**

Create `web/src/routes/Login.tsx`:

```tsx
import { FormEvent, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { useAuth, User } from '../store/auth'

interface Props {
  onSwitchToRegister: () => void
}

export default function Login({ onSwitchToRegister }: Props) {
  const login = useAuth((s) => s.login)
  const [username, setU] = useState('')
  const [password, setP] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      const res = await api.post<{ token: string; user: User }>('/api/auth/login', { username, password })
      login(res.token, res.user)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'login failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main style={{ maxWidth: 360, margin: '6rem auto', fontFamily: 'system-ui' }}>
      <h1>mysqlweb · login</h1>
      <form onSubmit={submit} style={{ display: 'grid', gap: 12 }}>
        <input
          placeholder="username"
          value={username}
          onChange={(e) => setU(e.target.value)}
          required
          autoFocus
        />
        <input
          placeholder="password"
          type="password"
          value={password}
          onChange={(e) => setP(e.target.value)}
          required
        />
        {error && <div style={{ color: 'crimson', fontSize: 14 }}>{error}</div>}
        <button disabled={busy} type="submit">
          {busy ? 'logging in...' : 'log in'}
        </button>
      </form>
      <p style={{ marginTop: 24, fontSize: 14 }}>
        No account?{' '}
        <a
          href="#"
          onClick={(e) => {
            e.preventDefault()
            onSwitchToRegister()
          }}
        >
          register
        </a>
      </p>
    </main>
  )
}
```

- [ ] **Step 2: Implement Register**

Create `web/src/routes/Register.tsx`:

```tsx
import { FormEvent, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { useAuth, User } from '../store/auth'

interface Props {
  onSwitchToLogin: () => void
}

export default function Register({ onSwitchToLogin }: Props) {
  const login = useAuth((s) => s.login)
  const [username, setU] = useState('')
  const [password, setP] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      const res = await api.post<{ token: string; user: User }>('/api/auth/register', { username, password })
      login(res.token, res.user)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'register failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main style={{ maxWidth: 360, margin: '6rem auto', fontFamily: 'system-ui' }}>
      <h1>mysqlweb · register</h1>
      <form onSubmit={submit} style={{ display: 'grid', gap: 12 }}>
        <input placeholder="username (3-32 chars)" value={username} onChange={(e) => setU(e.target.value)} required autoFocus />
        <input
          placeholder="password (≥8 chars, letters+digits)"
          type="password"
          value={password}
          onChange={(e) => setP(e.target.value)}
          required
        />
        {error && <div style={{ color: 'crimson', fontSize: 14 }}>{error}</div>}
        <button disabled={busy} type="submit">
          {busy ? 'creating...' : 'create account'}
        </button>
      </form>
      <p style={{ marginTop: 24, fontSize: 14 }}>
        Already have an account?{' '}
        <a
          href="#"
          onClick={(e) => {
            e.preventDefault()
            onSwitchToLogin()
          }}
        >
          log in
        </a>
      </p>
    </main>
  )
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/routes/
git commit -m "feat(web): Login + Register pages with API integration"
```

---

## Task 19: Settings page (password change + session management)

**Files:**
- Create: `web/src/routes/Settings.tsx`

- [ ] **Step 1: Implement Settings**

Create `web/src/routes/Settings.tsx`:

```tsx
import { FormEvent, useEffect, useState } from 'react'
import { api, ApiError } from '../lib/api'

interface SessionRow {
  id: string
  user_agent: string
  created_at: string
  last_used_at: string
  expires_at: string
  current: boolean
}

interface Props {
  onClose: () => void
}

export default function Settings({ onClose }: Props) {
  const [oldPw, setOld] = useState('')
  const [newPw, setNew] = useState('')
  const [pwMsg, setPwMsg] = useState<string | null>(null)
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null)

  async function loadSessions() {
    try {
      const r = await api.get<{ sessions: SessionRow[] }>('/api/auth/sessions')
      setSessions(r.sessions)
    } catch (err) {
      setLoadErr(err instanceof ApiError ? err.message : 'failed to load sessions')
    }
  }

  useEffect(() => {
    void loadSessions()
  }, [])

  async function changePassword(e: FormEvent) {
    e.preventDefault()
    setPwMsg(null)
    try {
      await api.put('/api/auth/password', { old: oldPw, new: newPw })
      setPwMsg('password changed (other sessions were revoked)')
      setOld('')
      setNew('')
      await loadSessions()
    } catch (err) {
      setPwMsg(err instanceof ApiError ? err.message : 'change failed')
    }
  }

  async function revoke(id: string) {
    try {
      await api.del(`/api/auth/sessions/${id}`)
      await loadSessions()
    } catch (err) {
      alert(err instanceof ApiError ? err.message : 'revoke failed')
    }
  }

  return (
    <main style={{ fontFamily: 'system-ui', padding: 24, maxWidth: 720, margin: '0 auto' }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h1 style={{ margin: 0 }}>settings</h1>
        <button onClick={onClose}>back</button>
      </header>

      <section style={{ marginBottom: 32 }}>
        <h2>change password</h2>
        <form onSubmit={changePassword} style={{ display: 'grid', gap: 8, maxWidth: 360 }}>
          <input type="password" placeholder="current password" value={oldPw} onChange={(e) => setOld(e.target.value)} required />
          <input type="password" placeholder="new password" value={newPw} onChange={(e) => setNew(e.target.value)} required />
          <button type="submit">change</button>
          {pwMsg && <div style={{ fontSize: 14 }}>{pwMsg}</div>}
        </form>
      </section>

      <section>
        <h2>active sessions</h2>
        {loadErr && <div style={{ color: 'crimson' }}>{loadErr}</div>}
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th style={th}>id</th>
              <th style={th}>device</th>
              <th style={th}>last used</th>
              <th style={th}>expires</th>
              <th style={th}></th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((s) => (
              <tr key={s.id}>
                <td style={td}>
                  {s.id}
                  {s.current && <span style={{ marginLeft: 6, fontSize: 11, color: 'green' }}>(this)</span>}
                </td>
                <td style={td}>{s.user_agent}</td>
                <td style={td}>{new Date(s.last_used_at).toLocaleString()}</td>
                <td style={td}>{new Date(s.expires_at).toLocaleString()}</td>
                <td style={td}>
                  {!s.current && <button onClick={() => revoke(s.id)}>revoke</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </main>
  )
}

const th: React.CSSProperties = { textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid #ddd', fontSize: 13 }
const td: React.CSSProperties = { padding: '6px 8px', borderBottom: '1px solid #f3f3f3', fontSize: 13 }
```

- [ ] **Step 2: Commit**

```bash
git add web/src/routes/Settings.tsx
git commit -m "feat(web): Settings page (password change + session list/revoke)"
```

---

## Task 20: Workspace shell + auth guard + App router

**Files:**
- Create: `web/src/routes/Workspace.tsx`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Workspace placeholder**

Create `web/src/routes/Workspace.tsx`:

```tsx
import { useAuth } from '../store/auth'

interface Props {
  onOpenSettings: () => void
}

export default function Workspace({ onOpenSettings }: Props) {
  const user = useAuth((s) => s.user!)
  const logout = useAuth((s) => s.logout)
  return (
    <main style={{ fontFamily: 'system-ui', padding: 24 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h1 style={{ margin: 0 }}>mysqlweb</h1>
        <nav style={{ display: 'flex', gap: 12 }}>
          <span>logged in as <b>{user.username}</b></span>
          <button onClick={onOpenSettings}>settings</button>
          <button onClick={() => logout()}>log out</button>
        </nav>
      </header>
      <section
        style={{
          border: '1px dashed #999',
          padding: 32,
          borderRadius: 8,
          color: '#666',
          textAlign: 'center',
        }}
      >
        connections sidebar + workspace coming in Plan 2
      </section>
    </main>
  )
}
```

- [ ] **Step 2: Rewrite App.tsx with simple routing + bootstrap**

Replace `web/src/App.tsx` with:

```tsx
import { useEffect, useState } from 'react'
import Login from './routes/Login'
import Register from './routes/Register'
import Workspace from './routes/Workspace'
import Settings from './routes/Settings'
import { useAuth } from './store/auth'

type View = 'auth-login' | 'auth-register' | 'workspace' | 'settings'

export default function App() {
  const { user, ready, bootstrap } = useAuth()
  const [view, setView] = useState<View>('auth-login')

  useEffect(() => {
    void bootstrap()
  }, [bootstrap])

  if (!ready) {
    return <main style={{ fontFamily: 'system-ui', padding: 24 }}>loading…</main>
  }

  if (!user) {
    if (view === 'auth-register') return <Register onSwitchToLogin={() => setView('auth-login')} />
    return <Login onSwitchToRegister={() => setView('auth-register')} />
  }

  if (view === 'settings') return <Settings onClose={() => setView('workspace')} />
  return <Workspace onOpenSettings={() => setView('settings')} />
}
```

- [ ] **Step 3: Build to verify no missing imports**

```bash
cd web && npm run build && cd ..
```

Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add web/src/App.tsx web/src/routes/Workspace.tsx
git commit -m "feat(web): Workspace shell + auth-aware App routing"
```

---

## Task 21: Embed frontend dist into Go binary

**Files:**
- Create: `embed.go`
- Modify: `internal/api/router.go` (serve SPA), `cmd/mysqlweb/main.go` (pass FS through)

- [ ] **Step 1: Build the frontend so dist exists**

```bash
cd web && npm run build && cd ..
ls web/dist
```

Expected: contains `index.html` and an `assets/` directory.

- [ ] **Step 2: Add embed.go**

Create `embed.go` (at the repo root):

```go
package mysqlweb

import "embed"

//go:embed web/dist
var WebFS embed.FS
```

- [ ] **Step 3: Extend router to serve SPA**

Modify `internal/api/router.go` — replace the file with:

```go
package api

import (
	"io/fs"
	"net/http"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)

type Deps struct {
	Version      string
	Store        *store.Store
	Registration string
	WebFS        fs.FS // sub-FS rooted at the SPA's dist; nil → no SPA serving (test mode)
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", handleHealth(d.Version))

	loginLimiter := NewRateLimiter(5, 1)
	registerLimiter := NewRateLimiter(3, 1)
	r.With(registerLimiter).Post("/api/auth/register", handleRegister(d))
	r.With(loginLimiter).Post("/api/auth/login", handleLogin(d))

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(d.Store))
		r.Get("/api/auth/me", handleMe(d))
		r.Post("/api/auth/logout", handleLogout(d))
		r.Put("/api/auth/password", handlePasswordChange(d))
		r.Get("/api/auth/sessions", handleListSessions(d))
		r.Delete("/api/auth/sessions/{id}", handleRevokeSession(d))
	})

	if d.WebFS != nil {
		fileServer := http.FileServer(http.FS(d.WebFS))
		r.Handle("/assets/*", fileServer)
		r.Get("/*", spaHandler(d.WebFS))
	}
	return r
}

func spaHandler(webFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only serve the index for non-API GETs. Anything starting with /api/
		// has already matched routes above; if we reach here it's a 404.
		f, err := webFS.Open("index.html")
		if err != nil {
			http.Error(w, "spa not built", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, mustReadSeeker(f))
	}
}

// mustReadSeeker wraps a fs.File as an io.ReadSeeker via in-memory buffering.
func mustReadSeeker(f fs.File) *readSeekerFile {
	return &readSeekerFile{File: f}
}

// readSeekerFile is a tiny adapter so we can use http.ServeContent on an embed.FS file.
type readSeekerFile struct{ File fs.File }

func (rs *readSeekerFile) Read(p []byte) (int, error)              { return rs.File.Read(p) }
func (rs *readSeekerFile) Seek(int64, int) (int64, error)          { return 0, nil } // no-op: index.html is tiny
func (rs *readSeekerFile) Close() error                            { return rs.File.Close() }
```

Add `time` to the imports:

```go
import (
	"io/fs"
	"net/http"
	"time"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)
```

- [ ] **Step 4: Pass WebFS through main.go**

In `cmd/mysqlweb/main.go`, change the imports and `api.Deps`:

```go
import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"

	mysqlweb "github.com/conray/mysqlweb"
	"github.com/conray/mysqlweb/internal/api"
	"github.com/conray/mysqlweb/internal/config"
	"github.com/conray/mysqlweb/internal/crypto"
	"github.com/conray/mysqlweb/internal/store"
)
```

Inside `main()`, after building `s`:

```go
	sub, err := fs.Sub(mysqlweb.WebFS, "web/dist")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	r := api.NewRouter(api.Deps{
		Version:      version,
		Store:        s,
		Registration: cfg.Registration,
		WebFS:        sub,
	})
```

- [ ] **Step 5: Build & smoke-test full stack**

```bash
cd web && npm run build && cd ..
go build -o /tmp/mysqlweb ./cmd/mysqlweb
MYSQLWEB_DB_PATH=/tmp/mysqlweb-data/test.db /tmp/mysqlweb &
SVR=$!
sleep 0.4
curl -s http://localhost:53306/ | head -c 80
echo
curl -s http://localhost:53306/api/health
echo
kill $SVR
rm -rf /tmp/mysqlweb-data
```

Expected: first curl prints the HTML (`<!doctype html><html...`), second curl prints the health JSON.

- [ ] **Step 6: Commit**

```bash
git add embed.go internal/api/router.go cmd/mysqlweb/main.go
git commit -m "feat(api): embed React dist + serve SPA index fallback"
```

---

## Task 22: Dockerfile (multi-stage build)

**Files:**
- Create: `Dockerfile`

- [ ] **Step 1: Write Dockerfile**

Create `Dockerfile`:

```dockerfile
# ---- 1. frontend build ----
FROM node:20-alpine AS frontend
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- 2. go build ----
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/mysqlweb ./cmd/mysqlweb

# ---- 3. final ----
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /out/mysqlweb /usr/local/bin/mysqlweb
WORKDIR /data
VOLUME ["/data"]
EXPOSE 53306
ENV MYSQLWEB_PORT=53306 \
    MYSQLWEB_DB_PATH=/data/mysqlweb.db \
    TZ=Asia/Taipei
ENTRYPOINT ["mysqlweb"]
```

- [ ] **Step 2: Build the image locally**

```bash
docker build -t mysqlweb:dev .
```

Expected: build succeeds. Note final image size (`docker images mysqlweb:dev`) — target ~30MB.

- [ ] **Step 3: Run the image**

```bash
mkdir -p /tmp/mysqlweb-data
docker run --rm -d --name mysqlweb-smoke \
  -p 53306:53306 \
  -v /tmp/mysqlweb-data:/data \
  mysqlweb:dev
sleep 1
curl -s http://localhost:53306/api/health
echo
docker logs mysqlweb-smoke | head -5
docker stop mysqlweb-smoke
rm -rf /tmp/mysqlweb-data
```

Expected: health JSON printed; logs show "mysqlweb listening on :53306" and a master-key-generated warning.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile
git commit -m "build: multi-stage Dockerfile (~30MB alpine final)"
```

---

## Task 23: docker-compose.yml + .env.example

**Files:**
- Create: `docker-compose.yml`, `.env.example`

- [ ] **Step 1: Compose file**

Create `docker-compose.yml`:

```yaml
services:
  mysqlweb:
    image: mysqlweb:dev
    build: .
    ports:
      - "53306:53306"
    volumes:
      - ./data:/data
    env_file: .env
    restart: unless-stopped
```

- [ ] **Step 2: Sample env file**

Create `.env.example`:

```bash
# 64-char hex (32 bytes) — generate with: openssl rand -hex 32
# If omitted, mysqlweb will generate one at ./data/master.key on first launch.
MYSQLWEB_MASTER_KEY=

# open | closed
MYSQLWEB_REGISTRATION=open

# Optional overrides
# MYSQLWEB_PORT=53306
# MYSQLWEB_HISTORY_MAX=1000
# MYSQLWEB_QUERY_TIMEOUT_S=5
# MYSQLWEB_QUERY_HTTP_MAX_MB=10

# LLM (used by Plan 5 — leave empty for now)
# MYSQLWEB_LLM_DEFAULT=anthropic
# ANTHROPIC_API_KEY=
# OPENAI_API_KEY=
# MCP_MYSQL_URL=
```

- [ ] **Step 3: Smoke-test the compose stack**

```bash
cp .env.example .env
docker compose up -d --build
sleep 2
curl -s http://localhost:53306/api/health
docker compose down
rm .env
rm -rf data
```

Expected: health JSON; compose brings the stack up.

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml .env.example
git commit -m "build: docker-compose.yml + sample .env"
```

---

## Task 24: CI workflow (GitHub Actions)

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Workflow file**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [master, main]
  pull_request:

jobs:
  go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Vet
        run: go vet ./...
      - name: Test
        run: go test ./... -count=1

  web:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
          cache-dependency-path: web/package-lock.json
      - run: npm ci
      - run: npm run build
      - run: npm test

  docker:
    runs-on: ubuntu-latest
    needs: [go, web]
    steps:
      - uses: actions/checkout@v4
      - name: Build image
        run: docker build -t mysqlweb:ci .
```

- [ ] **Step 2: Commit**

```bash
git add .github/
git commit -m "ci: GitHub Actions for go test + web build + docker build"
```

---

## Task 25: README + final smoke walkthrough

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write README**

Create `README.md`:

```markdown
# mysqlweb

Self-hosted MySQL administration tool for small teams. Browse, query, and edit your databases from a browser; an integrated AI chat panel (Plan 5) lets you talk to your DB via MCP.

This is Plan 1 — foundation + authentication. Connections, browsing, queries, import/export, and chat arrive in subsequent plans.

## Quick start (Docker Compose)

```bash
cp .env.example .env
# Generate a master key and paste it into .env:
echo "MYSQLWEB_MASTER_KEY=$(openssl rand -hex 32)" >> .env

docker compose up -d --build
open http://localhost:53306    # registration is open by default
```

## Quick start (Go)

```bash
cd web && npm ci && npm run build && cd ..
go build -o ./bin/mysqlweb ./cmd/mysqlweb
MYSQLWEB_DB_PATH=./data/mysqlweb.db ./bin/mysqlweb
```

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `MYSQLWEB_PORT` | `53306` | HTTP listen port |
| `MYSQLWEB_DB_PATH` | `/data/mysqlweb.db` | sqlite location (mount this for persistence) |
| `MYSQLWEB_MASTER_KEY` | (auto-generated) | 64-char hex (32 bytes), used to encrypt stored DB connection passwords in later plans. Generate with `openssl rand -hex 32`. |
| `MYSQLWEB_REGISTRATION` | `open` | `open` / `closed` |
| `MYSQLWEB_HISTORY_MAX` | `1000` | per-user query history cap (Plan 3) |
| `MYSQLWEB_QUERY_TIMEOUT_S` | `5` | short-query timeout (Plan 3) |
| `MYSQLWEB_QUERY_HTTP_MAX_MB` | `10` | short-query response cap (Plan 3) |

## What's in this plan

- POST `/api/auth/register`, `/api/auth/login` (rate-limited per IP)
- POST `/api/auth/logout`, GET `/api/auth/me`
- PUT `/api/auth/password` (revokes other sessions)
- GET `/api/auth/sessions`, DELETE `/api/auth/sessions/:id`
- GET `/api/health`
- React SPA: login / register / workspace placeholder / settings
- AES-GCM crypto package and master-key bootstrap (used in Plan 2 onward)

## Manual checklist for first deploy

1. `docker compose up -d --build`
2. Open http://localhost:53306
3. Register → land in the Workspace placeholder
4. Open Settings → change password → confirm "other sessions were revoked"
5. From a private window: log in with the new password → confirm 2 sessions in Settings → revoke the other one
6. Log out → confirm the login page

## Tests

- Go: `go test ./...`
- Web: `cd web && npm test`

## Project layout

See [`docs/superpowers/specs/2026-06-03-mysqlweb-design.md`](docs/superpowers/specs/2026-06-03-mysqlweb-design.md) for the full design and [`docs/superpowers/plans/`](docs/superpowers/plans/) for plans.
```

- [ ] **Step 2: Run the full test suite + manual checklist**

```bash
go test ./...
cd web && npm test && cd ..
docker compose up -d --build
sleep 3
echo "=== health ==="
curl -s http://localhost:53306/api/health
echo
echo "=== register alice ==="
curl -s -X POST http://localhost:53306/api/auth/register \
  -H 'content-type: application/json' \
  -d '{"username":"alice","password":"supersecret123"}'
echo
docker compose down
rm -rf data
```

Expected:
- All tests pass
- health JSON appears
- register returns a token

Then follow the README manual checklist in a browser.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: README with quickstart + Plan 1 feature list"
```

---

## Done — Plan 1 milestone

After Task 25 the repository should be:

- Go service with `/api/health` + 7 auth endpoints, all unit-tested
- React SPA with Login / Register / Workspace placeholder / Settings, all consuming the API through a single client
- Master-key bootstrap (used by Plan 2's connection password encryption)
- Multi-stage Docker image (~30MB) launchable via `docker compose up`
- CI on every push

The next plan (Plan 2 — Connections + DB browse) will be drafted after this one is executed and the codebase shape is real, not predicted.
