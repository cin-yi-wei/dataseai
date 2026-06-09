# DB Dialect Abstraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor `internal/mysql/` into a generic `internal/db/` dialect layer with MySQL as the first implementation, so PostgreSQL and MSSQL dialects can be added in follow-up plans without further touching consumers.

**Architecture:** Introduce a `Dialect` interface in `internal/db/` that exposes every MySQL-specific behavior currently scattered across the codebase: identifier quoting, DSN construction, placeholder style, schema-discovery SQL, classification, pagination, primary-key lookup, SSH tunnel registration, and driver name. Move existing MySQL code into `internal/db/mysql/` as the first dialect implementation. Add an `engine` column on the `connections` table (locked to `mysql` for this plan) so future dialects slot in. Consumers receive a `db.Dialect` rather than calling the old `internal/mysql` package directly.

**Tech Stack:** Go 1.25, `database/sql`, `github.com/go-sql-driver/mysql`, `github.com/mattn/go-sqlite3` (metadata store), `golang.org/x/crypto/ssh`.

**Scope guard:** This plan does NOT add PG or MSSQL implementations, does NOT touch the connector binary, and does NOT change frontend connection wizards beyond surfacing the existing single-engine value as read-only. Behavior on prod must be identical pre/post merge.

**Branch:** `feat/db-dialect-abstraction` cut from `main`.

---

## File Structure

### New package `internal/db/`
- `internal/db/types.go` — engine-agnostic types: `Engine`, `DSNInput`, `SSHConfig`, `Column`, `Structure`, `Index`, `ForeignKey`, `TableInfo`, `RowsPage`, `RowsOpts`, `Filter`, `Classified`, `Op`.
- `internal/db/dialect.go` — `Dialect` interface and `MustGet(engine Engine) Dialect` registry.
- `internal/db/registry.go` — `Register(engine Engine, d Dialect)` + lookup; supports tests injecting fakes.
- `internal/db/pool.go` — generic `Pool` keyed by `(UserID, ConnID)`, takes a `Dialect` per call so future engines can pool side-by-side.
- `internal/db/executor.go` — generic `Executor` interface and `DirectExecutor` (engine-agnostic; runs whatever SQL the caller passes).
- `internal/db/kill.go` — generic `Registry` for active queries (engine-agnostic; kill SQL is delegated to dialect).
- `internal/db/db_test.go` — interface contract tests using a fake dialect.

### New package `internal/db/mysql/`
- `internal/db/mysql/dialect.go` — `Dialect` implementation; `init()` calls `db.Register(db.EngineMySQL, &MySQL{})`.
- `internal/db/mysql/dsn.go` — moved from `internal/mysql/dsn.go`, exported as `(MySQL).BuildDSN`.
- `internal/db/mysql/ident.go` — moved from `internal/mysql/ident.go`, exported as `(MySQL).QuoteIdent`.
- `internal/db/mysql/schema.go` — moved from `internal/mysql/schema.go`; `(MySQL).DescribeTable / ListIndexes / ListForeignKeys`.
- `internal/db/mysql/browse.go` — moved from `internal/mysql/browse.go`; `(MySQL).ListDatabases / ListTables / ListSchemaColumns / FetchTableRows`.
- `internal/db/mysql/dml.go` — moved from `internal/mysql/dml.go`; `(MySQL).PrimaryKey / UpdateCell / InsertRow / DeleteRow`.
- `internal/db/mysql/kill.go` — moved from `internal/mysql/kill.go`; `(MySQL).KillQuerySQL` + `ConnectionIDExpr`.
- `internal/db/mysql/sqlclass.go` — moved from `internal/mysql/sqlclass.go`; `(MySQL).ClassifySQL`.
- `internal/db/mysql/query.go` — moved from `internal/mysql/query.go`; `(MySQL).RunQuery` (returns streaming reader).
- `internal/db/mysql/stream.go` — moved from `internal/mysql/stream.go`.
- `internal/db/mysql/export.go` — moved from `internal/mysql/export.go`.
- `internal/db/mysql/import.go` — moved from `internal/mysql/import.go`.
- `internal/db/mysql/ssh.go` — moved from `internal/mysql/ssh.go`; SSH dialer registration kept driver-specific.
- All existing `*_test.go` files move alongside their target file.

### Modified files (consumer migration)
- `internal/api/db.go` — `Deps.Pool` becomes `*db.Pool`, `Deps.Dialect db.Dialect` added; helpers route through `Dialect`.
- `internal/api/dml.go`, `internal/api/queries_test.go`, `internal/api/db_test.go`, `internal/api/agent_read.go`, `internal/api/connections.go`, `internal/api/connections_test.go`, `internal/api/ai_policy.go`, `internal/api/router.go`, `internal/api/import.go`, `internal/api/export.go`, `internal/api/query.go`, `internal/api/ws.go`, `internal/api/chat.go` — swap `mysql.X` for `db.X` / dialect calls.
- `internal/chat/orchestrator.go`, `internal/chat/propose.go`, `internal/chat/execute.go`, `internal/chat/execute_test.go` — `Executor` becomes `db.Executor`; classification through dialect.
- `internal/agent/executor.go`, `internal/agent/executor_test.go` — `mysql.SSHConfig` → `db.SSHConfig`; `mysql.DSNInput` → `db.DSNInput`.
- `internal/store/migrations/0017_engine_column.sql` — NEW migration adding `engine` column.
- `internal/store/connections.go` — add `Engine` field on `Connection` / `ConnectionInput`, default `"mysql"`.
- `internal/store/connections_test.go` — assert default engine.
- `cmd/dataseai/main.go` (or wherever `Deps` is wired) — pass `db.MustGet(conn.Engine)` to API handlers; call `_ "github.com/conray/dataseai/internal/db/mysql"` for side-effect registration.
- `web/src/types/connection.ts` (or equivalent) — add `engine` field (read-only, value `"mysql"`).
- `web/src/components/ConnectionForm.*` — show engine badge "MySQL"; no editing.

### Deleted at the end
- The entire `internal/mysql/` directory (after all consumers migrated).

---

## Pre-flight Tasks

### Task 0: Create feature branch and verify baseline

**Files:**
- None modified; branch setup only

- [ ] **Step 1: Confirm clean working tree**

Run from `/home/conray/project/mysqlweb`:
```bash
git status
```
Expected: `nothing to commit, working tree clean`. If not clean, stash or commit before proceeding.

- [ ] **Step 2: Pull latest main**

```bash
git checkout main
git pull --ff-only origin main
```
Expected: fast-forward or already up to date.

- [ ] **Step 3: Cut feature branch**

```bash
git checkout -b feat/db-dialect-abstraction
```

- [ ] **Step 4: Capture baseline test result**

```bash
go test ./... 2>&1 | tee /tmp/dialect-baseline.log
```
Expected: all packages PASS. Record `<N>` passing tests as the floor — every later task must keep `go test ./...` green and not lower this count.

- [ ] **Step 5: Commit branch baseline placeholder**

```bash
mkdir -p docs/superpowers/plans
git add docs/superpowers/plans/2026-06-09-db-dialect-abstraction.md
git commit -m "docs(plan): add db dialect abstraction plan"
```

---

## Phase 1: Define generic `internal/db/` package

### Task 1: Create `internal/db/types.go`

**Files:**
- Create: `internal/db/types.go`
- Test: `internal/db/types_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/db/types_test.go`:
```go
package db

import "testing"

func TestEngineString(t *testing.T) {
	if EngineMySQL.String() != "mysql" {
		t.Fatalf("EngineMySQL string = %q, want %q", EngineMySQL.String(), "mysql")
	}
}

func TestParseEngineKnown(t *testing.T) {
	e, err := ParseEngine("mysql")
	if err != nil {
		t.Fatalf("ParseEngine(mysql): %v", err)
	}
	if e != EngineMySQL {
		t.Fatalf("got %v, want EngineMySQL", e)
	}
}

func TestParseEngineUnknown(t *testing.T) {
	if _, err := ParseEngine("oracle"); err == nil {
		t.Fatal("expected error for unknown engine")
	}
}

func TestSSHConfigIsZero(t *testing.T) {
	if !(SSHConfig{}).IsZero() {
		t.Fatal("empty SSHConfig should be zero")
	}
	cfg := SSHConfig{Host: "h", User: "u"}
	if cfg.IsZero() {
		t.Fatal("populated SSHConfig should not be zero")
	}
}
```

- [ ] **Step 2: Run test (should fail to compile)**

```bash
go test ./internal/db/...
```
Expected: build failure — package `internal/db` does not exist yet.

- [ ] **Step 3: Implement minimal `types.go`**

Create `internal/db/types.go`:
```go
// Package db defines the engine-agnostic surface every supported relational
// database must implement. The dialect interface lives in dialect.go; this
// file collects the value types those methods produce or consume.
package db

import (
	"fmt"
)

// Engine names a database engine supported by the platform.
type Engine string

const (
	EngineMySQL Engine = "mysql"
	// EnginePostgres Engine = "postgres" // reserved for future plan
	// EngineMSSQL    Engine = "mssql"    // reserved for future plan
)

func (e Engine) String() string { return string(e) }

// ParseEngine normalizes a stored string into a known Engine. Returns an
// error for empty or unrecognized values so callers can decide how to
// surface the failure (HTTP 400, migration default, etc).
func ParseEngine(s string) (Engine, error) {
	switch Engine(s) {
	case EngineMySQL:
		return EngineMySQL, nil
	default:
		return "", fmt.Errorf("unknown engine %q", s)
	}
}

// DSNInput holds the bits a dialect needs to build its driver-specific DSN.
// Not every dialect uses every field (e.g. TLS string maps differently per
// driver), but the shape is shared so consumers don't branch on engine.
type DSNInput struct {
	Host      string
	Port      int
	Username  string
	Password  string
	DefaultDB string
	TLS       string // "disabled" | "preferred" | "required" | "skip-verify"
	Network   string // SSH dialer name override; empty = engine default ("tcp")
}

// SSHConfig describes how to reach an SSH bastion.
type SSHConfig struct {
	Host          string
	Port          int
	User          string
	Password      string
	PrivateKey    string
	KeyPassphrase string
}

func (c SSHConfig) IsZero() bool { return c.Host == "" || c.User == "" }

// Column describes one column returned by Dialect.DescribeTable.
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Default  string `json:"default"`
	Extra    string `json:"extra"`
	Comment  string `json:"comment"`
	Key      string `json:"key"`
}

// Structure groups columns and a CREATE-like representation. Dialects that
// have no SHOW CREATE TABLE equivalent should synthesize a best-effort
// CREATE TABLE string from the columns for parity.
type Structure struct {
	Columns   []Column `json:"columns"`
	CreateSQL string   `json:"create_sql"`
}

type Index struct {
	Name      string   `json:"name"`
	Columns   []string `json:"columns"`
	Unique    bool     `json:"unique"`
	IndexType string   `json:"index_type"`
}

type ForeignKey struct {
	Name      string `json:"name"`
	Column    string `json:"column"`
	RefTable  string `json:"ref_table"`
	RefColumn string `json:"ref_column"`
	OnDelete  string `json:"on_delete"`
	OnUpdate  string `json:"on_update"`
}

type TableInfo struct {
	Name    string `json:"name"`
	RowsEst int64  `json:"rows_est"`
	SizeMB  int64  `json:"size_mb"`
}

type Filter struct {
	Column   string `json:"column"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type RowsOpts struct {
	Schema  string
	Table   string
	Page    int
	PerPage int
	SortCol string
	SortDir string
	Filters []Filter
}

type RowsPage struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Total   int64    `json:"total"`
	Page    int      `json:"page"`
	PerPage int      `json:"per_page"`
}

// Op classifies a parsed SQL statement.
type Op string

const (
	OpSelect    Op = "SELECT"
	OpInsert    Op = "INSERT"
	OpUpdate    Op = "UPDATE"
	OpDelete    Op = "DELETE"
	OpTruncate  Op = "TRUNCATE"
	OpDDL       Op = "DDL"
	OpForbidden Op = "FORBIDDEN"
	OpReadMeta  Op = "READMETA"
	OpUnknown   Op = "UNKNOWN"
)

type Classified struct {
	Op    Op
	DB    string
	Table string
	Multi bool
}

// MaxRowsPerPage is the absolute upper bound on a paginated table-rows
// response. Dialects must clamp `PerPage` to at most this value.
const MaxRowsPerPage = 10000
```

- [ ] **Step 4: Run tests to verify pass**

```bash
go test ./internal/db/...
```
Expected: PASS — three subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/db/types.go internal/db/types_test.go
git commit -m "feat(db): add engine-agnostic types and Engine enum"
```

### Task 2: Define `Dialect` interface in `internal/db/dialect.go`

**Files:**
- Create: `internal/db/dialect.go`
- Create: `internal/db/registry.go`
- Test: `internal/db/registry_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/db/registry_test.go`:
```go
package db

import (
	"errors"
	"testing"
)

type stubDialect struct{}

func (stubDialect) Engine() Engine            { return Engine("stub") }
func (stubDialect) DriverName() string        { return "stub" }
func (stubDialect) BuildDSN(DSNInput) string  { return "" }
func (stubDialect) QuoteIdent(s string) string { return s }
func (stubDialect) Placeholder(int) string    { return "?" }
func (stubDialect) ClassifySQL(string) (Classified, error) {
	return Classified{}, errors.New("not implemented")
}

func TestRegisterAndGet(t *testing.T) {
	const e Engine = "stubengine"
	Register(e, stubDialect{})
	got, ok := Lookup(e)
	if !ok {
		t.Fatal("Lookup failed for registered engine")
	}
	if got.DriverName() != "stub" {
		t.Fatalf("driver name = %q, want %q", got.DriverName(), "stub")
	}
}

func TestMustGetPanicsForUnknown(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unknown engine")
		}
	}()
	MustGet(Engine("absent"))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/db/...
```
Expected: build failure — `Dialect`, `Register`, `Lookup`, `MustGet` undefined.

- [ ] **Step 3: Define the interface**

Create `internal/db/dialect.go`:
```go
package db

import (
	"context"
	"database/sql"
)

// Dialect collects every engine-specific behavior the platform depends on.
// Adding a new database means writing a new Dialect implementation and
// registering it in this package's init chain — no consumer touches the
// engine name directly.
//
// Methods grouped by concern:
//   - Identity:      Engine, DriverName
//   - Connection:    BuildDSN, RegisterSSHDialer
//   - Quoting:       QuoteIdent, Placeholder
//   - Parsing:       ClassifySQL
//   - Discovery:     ListDatabases, ListTables, ListSchemaColumns,
//                    DescribeTable, ListIndexes, ListForeignKeys
//   - Browsing:      FetchTableRows
//   - DML:           PrimaryKey, UpdateCell, InsertRow, DeleteRow
//   - Admin:         KillQuery, ConnectionIDQuery
type Dialect interface {
	// Engine returns the canonical engine name (e.g. "mysql").
	Engine() Engine
	// DriverName is the string passed to sql.Open (e.g. "mysql", "pgx", "sqlserver").
	DriverName() string
	// BuildDSN turns generic input into a driver-specific DSN string.
	BuildDSN(DSNInput) string
	// RegisterSSHDialer wires an SSH-tunneled dialer into the underlying driver
	// and returns a name suitable for passing back into DSNInput.Network so
	// the next BuildDSN routes through the tunnel. Returns a closer the pool
	// invokes when the entry is evicted.
	RegisterSSHDialer(SSHConfig) (name string, closer func(), err error)

	// QuoteIdent wraps a user-controlled identifier in the dialect's
	// quoting style. Required to be SQL-injection-safe.
	QuoteIdent(string) string
	// Placeholder returns the placeholder string for parameter index n
	// (1-based for engines that care, ignored otherwise).
	Placeholder(n int) string

	// ClassifySQL parses a statement well enough to enforce policy:
	// op kind, target table, multi-statement flag.
	ClassifySQL(string) (Classified, error)

	ListDatabases(ctx context.Context, db *sql.DB, includeSystem bool) ([]string, error)
	ListTables(ctx context.Context, db *sql.DB, schema string) ([]TableInfo, error)
	ListSchemaColumns(ctx context.Context, db *sql.DB, schema string) (map[string][]string, error)
	DescribeTable(ctx context.Context, db *sql.DB, schema, table string) (Structure, error)
	ListIndexes(ctx context.Context, db *sql.DB, schema, table string) ([]Index, error)
	ListForeignKeys(ctx context.Context, db *sql.DB, schema, table string) ([]ForeignKey, error)

	FetchTableRows(ctx context.Context, db *sql.DB, opts RowsOpts) (RowsPage, error)

	PrimaryKey(ctx context.Context, db *sql.DB, schema, table string) ([]string, error)
	UpdateCell(ctx context.Context, db *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error)
	InsertRow(ctx context.Context, db *sql.DB, schema, table string, cols []string, vals []any) (int64, error)
	DeleteRow(ctx context.Context, db *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error)

	// KillQuery cancels a server-side running query identified by a
	// dialect-specific handle. MySQL uses CONNECTION_ID(); PG uses
	// pg_cancel_backend(pid); MSSQL uses KILL <spid>.
	KillQuery(ctx context.Context, db *sql.DB, connID int64) error
	// ConnectionIDQuery returns the SQL that yields the current
	// session's server-side connection id as a single int column.
	ConnectionIDQuery() string
}
```

Create `internal/db/registry.go`:
```go
package db

import (
	"fmt"
	"sync"
)

var (
	regMu      sync.RWMutex
	registered = map[Engine]Dialect{}
)

// Register installs a dialect. Engines call this from their package init.
func Register(e Engine, d Dialect) {
	regMu.Lock()
	defer regMu.Unlock()
	registered[e] = d
}

// Lookup returns the dialect for an engine, ok=false if missing.
func Lookup(e Engine) (Dialect, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	d, ok := registered[e]
	return d, ok
}

// MustGet returns the dialect for an engine and panics if absent. Use only
// in startup paths where a missing dialect is a programmer error.
func MustGet(e Engine) Dialect {
	d, ok := Lookup(e)
	if !ok {
		panic(fmt.Sprintf("db: no dialect registered for engine %q", e))
	}
	return d
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/db/...
```
Expected: PASS — registry tests green; types tests unaffected.

- [ ] **Step 5: Commit**

```bash
git add internal/db/dialect.go internal/db/registry.go internal/db/registry_test.go
git commit -m "feat(db): add Dialect interface and engine registry"
```

### Task 3: Generic `Pool` skeleton (engine-aware)

**Files:**
- Create: `internal/db/pool.go`
- Test: `internal/db/pool_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/db/pool_test.go`:
```go
package db

import (
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDialect struct {
	stubDialect
	dsnCounter int32
}

func (f *fakeDialect) BuildDSN(in DSNInput) string {
	atomic.AddInt32(&f.dsnCounter, 1)
	return in.Host
}
func (f *fakeDialect) Engine() Engine     { return Engine("fake") }
func (f *fakeDialect) DriverName() string { return "sqlite3" } // unused; pool uses cfg.Open

func TestPoolReusesEntryWhenKeyAndDSNMatch(t *testing.T) {
	opens := int32(0)
	p := NewPool(PoolConfig{
		Open: func(driver, dsn string) (*sql.DB, error) {
			atomic.AddInt32(&opens, 1)
			return sql.Open("sqlite3", ":memory:")
		},
	})
	d := &fakeDialect{}
	key := PoolKey{UserID: 1, ConnID: 1}
	a, err := p.Get(key, d, DSNInput{Host: "h"}, SSHConfig{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Get(key, d, DSNInput{Host: "h"}, SSHConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("expected pool to reuse entry")
	}
	if atomic.LoadInt32(&opens) != 1 {
		t.Fatalf("opens = %d, want 1", opens)
	}
}

func TestPoolReopensWhenDSNChanges(t *testing.T) {
	opens := int32(0)
	p := NewPool(PoolConfig{
		Open: func(driver, dsn string) (*sql.DB, error) {
			atomic.AddInt32(&opens, 1)
			return sql.Open("sqlite3", ":memory:")
		},
	})
	d := &fakeDialect{}
	key := PoolKey{UserID: 1, ConnID: 1}
	if _, err := p.Get(key, d, DSNInput{Host: "h1"}, SSHConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Get(key, d, DSNInput{Host: "h2"}, SSHConfig{}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&opens) != 2 {
		t.Fatalf("opens = %d, want 2", opens)
	}
}

func TestPoolEvict(t *testing.T) {
	closed := false
	p := NewPool(PoolConfig{Open: func(driver, dsn string) (*sql.DB, error) {
		db, _ := sql.Open("sqlite3", ":memory:")
		return db, nil
	}})
	d := &fakeDialect{}
	key := PoolKey{UserID: 1, ConnID: 2}
	if _, err := p.Get(key, d, DSNInput{Host: "h"}, SSHConfig{}); err != nil {
		t.Fatal(err)
	}
	p.Evict(key)
	_ = closed
	if _, ok := p.entryFor(key); ok {
		t.Fatal("evict did not remove entry")
	}
}

func TestPoolSweepIdle(t *testing.T) {
	p := NewPool(PoolConfig{IdleTimeout: 10 * time.Millisecond, Open: func(driver, dsn string) (*sql.DB, error) {
		db, _ := sql.Open("sqlite3", ":memory:")
		return db, nil
	}})
	d := &fakeDialect{}
	key := PoolKey{UserID: 1, ConnID: 3}
	if _, err := p.Get(key, d, DSNInput{Host: "h"}, SSHConfig{}); err != nil {
		t.Fatal(err)
	}
	p.Sweep(time.Now().Add(time.Second))
	if _, ok := p.entryFor(key); ok {
		t.Fatal("sweep did not evict idle entry")
	}
}

func TestPoolOpenError(t *testing.T) {
	want := errors.New("boom")
	p := NewPool(PoolConfig{Open: func(driver, dsn string) (*sql.DB, error) { return nil, want }})
	d := &fakeDialect{}
	_, err := p.Get(PoolKey{UserID: 1, ConnID: 4}, d, DSNInput{Host: "h"}, SSHConfig{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want wrap of boom", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/db/...
```
Expected: build failure — `Pool`, `NewPool`, etc undefined.

- [ ] **Step 3: Implement `Pool`**

Create `internal/db/pool.go`:
```go
package db

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// PoolKey identifies a pooled connection. UserID isolates users so an admin
// switching identities never reuses another user's DB handle.
type PoolKey struct {
	UserID int64
	ConnID int64
}

// PoolConfig customizes Pool behavior. Open lets tests inject an in-memory
// driver; production callers leave it nil and the pool uses sql.Open with
// the dialect's DriverName.
type PoolConfig struct {
	IdleTimeout time.Duration
	Open        func(driver, dsn string) (*sql.DB, error)
}

type pooled struct {
	db         *sql.DB
	cacheKey   string
	sshCloser  func()
	lastUsed   time.Time
}

// Pool caches *sql.DB handles keyed by PoolKey. It is engine-aware: each
// Get call carries a Dialect, so the pool can call dialect.BuildDSN and
// dialect.RegisterSSHDialer without consumers re-implementing engine logic.
type Pool struct {
	cfg PoolConfig
	mu  sync.Mutex
	m   map[PoolKey]*pooled
}

func NewPool(cfg PoolConfig) *Pool {
	if cfg.Open == nil {
		cfg.Open = func(driver, dsn string) (*sql.DB, error) {
			return sql.Open(driver, dsn)
		}
	}
	return &Pool{cfg: cfg, m: map[PoolKey]*pooled{}}
}

// Get returns or opens a *sql.DB for the key. SSH config, when non-zero,
// registers a per-tunnel dialer name with the dialect's driver and the DSN
// is regenerated to route through it.
func (p *Pool) Get(key PoolKey, d Dialect, in DSNInput, ssh SSHConfig) (*sql.DB, error) {
	cacheKey := d.Engine().String() + "|" + d.BuildDSN(in) + sshFingerprint(ssh)
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.m[key]; ok {
		if entry.cacheKey == cacheKey {
			entry.lastUsed = time.Now()
			return entry.db, nil
		}
		_ = entry.db.Close()
		if entry.sshCloser != nil {
			entry.sshCloser()
		}
		delete(p.m, key)
	}

	var sshCloser func()
	if !ssh.IsZero() {
		name, closer, err := d.RegisterSSHDialer(ssh)
		if err != nil {
			return nil, fmt.Errorf("ssh tunnel: %w", err)
		}
		sshCloser = closer
		in.Network = name
	}
	dsn := d.BuildDSN(in)

	db, err := p.cfg.Open(d.DriverName(), dsn)
	if err != nil {
		if sshCloser != nil {
			sshCloser()
		}
		return nil, fmt.Errorf("open: %w", err)
	}
	p.m[key] = &pooled{db: db, cacheKey: cacheKey, sshCloser: sshCloser, lastUsed: time.Now()}
	return db, nil
}

func (p *Pool) Evict(key PoolKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.m[key]; ok {
		_ = entry.db.Close()
		if entry.sshCloser != nil {
			entry.sshCloser()
		}
		delete(p.m, key)
	}
}

func (p *Pool) EvictUser(userID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, entry := range p.m {
		if k.UserID == userID {
			_ = entry.db.Close()
			if entry.sshCloser != nil {
				entry.sshCloser()
			}
			delete(p.m, k)
		}
	}
}

func (p *Pool) Sweep(now time.Time) {
	if p.cfg.IdleTimeout == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, entry := range p.m {
		if now.Sub(entry.lastUsed) >= p.cfg.IdleTimeout {
			_ = entry.db.Close()
			if entry.sshCloser != nil {
				entry.sshCloser()
			}
			delete(p.m, k)
		}
	}
}

// entryFor is test-only.
func (p *Pool) entryFor(key PoolKey) (*pooled, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.m[key]
	return e, ok
}

func sshFingerprint(s SSHConfig) string {
	if s.IsZero() {
		return ""
	}
	return fmt.Sprintf("|ssh=%s@%s:%d", s.User, s.Host, s.Port)
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/...
```
Expected: PASS for `TestPoolReusesEntryWhenKeyAndDSNMatch`, `TestPoolReopensWhenDSNChanges`, `TestPoolEvict`, `TestPoolSweepIdle`, `TestPoolOpenError`.

If `sqlite3` not pulled into `internal/db` test scope, add a blank import in the test file: `_ "github.com/mattn/go-sqlite3"`.

- [ ] **Step 5: Commit**

```bash
git add internal/db/pool.go internal/db/pool_test.go
git commit -m "feat(db): add engine-aware Pool keyed by dialect+DSN"
```

### Task 4: Generic kill `Registry`

**Files:**
- Create: `internal/db/kill.go`
- Test: `internal/db/kill_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/db/kill_test.go`:
```go
package db

import (
	"errors"
	"testing"
)

func TestKillRegistryRoundTrip(t *testing.T) {
	r := NewKillRegistry()
	r.Register("q1", 123, "SELECT 1", 7, 9)
	q, ok := r.Lookup("q1")
	if !ok {
		t.Fatal("expected to find q1")
	}
	if q.ConnectionID != 123 || q.UserID != 7 || q.ConnID != 9 {
		t.Fatalf("unexpected: %+v", q)
	}
	r.Unregister("q1")
	if _, ok := r.Lookup("q1"); ok {
		t.Fatal("expected q1 cleared after Unregister")
	}
}

func TestKillRegistryAuthorize(t *testing.T) {
	r := NewKillRegistry()
	r.Register("q1", 123, "SELECT 1", 7, 9)
	_, err := r.Authorize("q1", 8) // wrong user
	if !errors.Is(err, ErrKillNoMatch) {
		t.Fatalf("expected ErrKillNoMatch, got %v", err)
	}
	q, err := r.Authorize("q1", 7)
	if err != nil {
		t.Fatal(err)
	}
	if q.ConnectionID != 123 {
		t.Fatalf("connection id = %d", q.ConnectionID)
	}
}
```

- [ ] **Step 2: Run test (expect compile fail)**

```bash
go test ./internal/db/...
```
Expected: build failure — `KillRegistry` undefined.

- [ ] **Step 3: Implement**

Create `internal/db/kill.go`:
```go
package db

import (
	"errors"
	"sync"
	"time"
)

// ActiveQuery captures the bookkeeping needed to kill a running query
// without re-querying the DB. Engine-agnostic: each dialect interprets
// ConnectionID per its own KILL semantics.
type ActiveQuery struct {
	QueryID      string    `json:"query_id"`
	ConnectionID int64     `json:"-"`
	SQLExcerpt   string    `json:"sql_excerpt"`
	UserID       int64     `json:"-"`
	ConnID       int64     `json:"conn_id"`
	StartedAt    time.Time `json:"started_at"`
}

// KillRegistry is a process-wide map of in-flight queries. The dialect's
// KillQuery method consumes the ConnectionID to issue the engine-specific
// cancellation statement.
type KillRegistry struct {
	mu sync.Mutex
	m  map[string]ActiveQuery
}

// ErrKillNoMatch is returned when a kill targets a non-existent or
// foreign-owned query.
var ErrKillNoMatch = errors.New("no matching active query")

func NewKillRegistry() *KillRegistry {
	return &KillRegistry{m: map[string]ActiveQuery{}}
}

func (r *KillRegistry) Register(queryID string, connectionID int64, sqlText string, userID, connID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[queryID] = ActiveQuery{
		QueryID:      queryID,
		ConnectionID: connectionID,
		SQLExcerpt:   excerpt(sqlText),
		UserID:       userID,
		ConnID:       connID,
		StartedAt:    time.Now(),
	}
}

func (r *KillRegistry) Unregister(queryID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, queryID)
}

func (r *KillRegistry) Lookup(queryID string) (ActiveQuery, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.m[queryID]
	return q, ok
}

// Authorize verifies the query belongs to the requesting user and returns
// it for the caller to feed to dialect.KillQuery.
func (r *KillRegistry) Authorize(queryID string, userID int64) (ActiveQuery, error) {
	q, ok := r.Lookup(queryID)
	if !ok || q.UserID != userID {
		return ActiveQuery{}, ErrKillNoMatch
	}
	return q, nil
}

func excerpt(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/...
```
Expected: PASS — three new tests added; prior tests still green.

- [ ] **Step 5: Commit**

```bash
git add internal/db/kill.go internal/db/kill_test.go
git commit -m "feat(db): add engine-agnostic kill registry"
```

### Task 5: Generic Executor adapter

**Files:**
- Create: `internal/db/executor.go`
- Test: `internal/db/executor_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/db/executor_test.go`:
```go
package db

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestDirectExecutorRunsSelect(t *testing.T) {
	dbh, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dbh.Close()
	if _, err := dbh.Exec("CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := dbh.Exec("INSERT INTO t VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	exec := DirectExecutor{DB: dbh}
	res, err := exec.Run(context.Background(), "SELECT id FROM t")
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsAffected != 0 {
		t.Fatalf("RowsAffected = %d, want 0 for select", res.RowsAffected)
	}
}
```

- [ ] **Step 2: Run test (compile fail)**

```bash
go test ./internal/db/...
```
Expected: build failure — `DirectExecutor`, `ExecResult` undefined.

- [ ] **Step 3: Implement**

Create `internal/db/executor.go`:
```go
package db

import (
	"context"
	"database/sql"
)

// ExecResult is the minimal shape a query response carries beyond raw rows.
// Engine-specific rich results live in streaming helpers per dialect.
type ExecResult struct {
	RowsAffected int64
}

// Executor runs an ad-hoc SQL statement. The interface stays engine-agnostic
// so the chat orchestrator and agent layer can be wired to either a direct
// pool or a connector-backed transport without caring about the dialect.
type Executor interface {
	Run(ctx context.Context, statement string) (ExecResult, error)
}

// DirectExecutor wires Executor straight to a *sql.DB. Used by API
// handlers that talk directly to the target DB.
type DirectExecutor struct {
	DB *sql.DB
}

func (e DirectExecutor) Run(ctx context.Context, statement string) (ExecResult, error) {
	res, err := e.DB.ExecContext(ctx, statement)
	if err != nil {
		return ExecResult{}, err
	}
	n, _ := res.RowsAffected()
	return ExecResult{RowsAffected: n}, nil
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/executor.go internal/db/executor_test.go
git commit -m "feat(db): add generic Executor and DirectExecutor"
```

---

## Phase 2: Move MySQL implementation into `internal/db/mysql/`

The strategy is to **add the new mysql dialect package alongside** the existing `internal/mysql/` package, then migrate consumers, then delete the old package. This avoids a giant single commit and keeps `go test ./...` green at every step.

### Task 6: Scaffold `internal/db/mysql/` and implement DSN + ident

**Files:**
- Create: `internal/db/mysql/dialect.go`
- Create: `internal/db/mysql/dsn.go`
- Create: `internal/db/mysql/ident.go`
- Create: `internal/db/mysql/ident_test.go`
- Create: `internal/db/mysql/dsn_test.go`

- [ ] **Step 1: Write failing tests (copy from existing files but call new dialect)**

Create `internal/db/mysql/ident_test.go` — duplicate body of `internal/mysql/ident_test.go` but call `(&MySQL{}).QuoteIdent`:
```go
package mysql

import "testing"

func TestQuoteIdent(t *testing.T) {
	d := &MySQL{}
	cases := []struct{ in, want string }{
		{"users", "`users`"},
		{"weird`name", "`weird``name`"},
	}
	for _, c := range cases {
		if got := d.QuoteIdent(c.in); got != c.want {
			t.Fatalf("QuoteIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

Create `internal/db/mysql/dsn_test.go` — duplicate of `internal/mysql/dsn_test.go` calling `(&MySQL{}).BuildDSN` with `db.DSNInput{...}`:
```go
package mysql

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestBuildDSNBasics(t *testing.T) {
	d := &MySQL{}
	got := d.BuildDSN(db.DSNInput{
		Host: "127.0.0.1", Port: 3306, Username: "u", Password: "p", DefaultDB: "x",
	})
	if !strings.Contains(got, "u:p@tcp(127.0.0.1:3306)/x") {
		t.Fatalf("DSN = %q", got)
	}
	if !strings.Contains(got, "parseTime=true") {
		t.Fatalf("expected parseTime=true in %q", got)
	}
}

func TestBuildDSNTLSRequired(t *testing.T) {
	d := &MySQL{}
	got := d.BuildDSN(db.DSNInput{
		Host: "h", Port: 3306, Username: "u", Password: "p", TLS: "required",
	})
	if !strings.Contains(got, "tls=true") {
		t.Fatalf("tls flag missing in %q", got)
	}
}
```

- [ ] **Step 2: Run tests (expect compile fail)**

```bash
go test ./internal/db/mysql/...
```
Expected: build failure — `MySQL` type missing.

- [ ] **Step 3: Implement the dialect skeleton**

Create `internal/db/mysql/dialect.go`:
```go
// Package mysql is the MySQL implementation of db.Dialect.
package mysql

import (
	"github.com/conray/dataseai/internal/db"
)

// MySQL is the MySQL dialect. Its methods live across this package, grouped
// by concern (dsn.go, ident.go, schema.go, browse.go, dml.go, kill.go,
// sqlclass.go). The package init registers the singleton into db's
// registry so consumers obtain it via db.MustGet(db.EngineMySQL).
type MySQL struct{}

func (MySQL) Engine() db.Engine  { return db.EngineMySQL }
func (MySQL) DriverName() string { return "mysql" }

var singleton = MySQL{}

func init() {
	db.Register(db.EngineMySQL, singleton)
}
```

Create `internal/db/mysql/ident.go`:
```go
package mysql

import "strings"

func (MySQL) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// Placeholder returns "?" — MySQL uses positional anonymous placeholders.
// The index argument is ignored.
func (MySQL) Placeholder(int) string { return "?" }
```

Create `internal/db/mysql/dsn.go` (lifted verbatim from `internal/mysql/dsn.go` but receiver-bound and using `db.DSNInput`):
```go
package mysql

import (
	"fmt"
	"time"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/conray/dataseai/internal/db"
)

// BuildDSN constructs a go-sql-driver/mysql DSN via the driver's own Config
// struct. Building by hand and url.QueryEscape'ing the password is wrong —
// the driver does NOT URL-decode the password section when parsing a DSN,
// so any escaping we do gets sent literally to MySQL. FormatDSN() handles
// the right escape rules per field.
func (MySQL) BuildDSN(in db.DSNInput) string {
	cfg := gomysql.NewConfig()
	cfg.User = in.Username
	cfg.Passwd = in.Password
	if in.Network != "" {
		cfg.Net = in.Network
	} else {
		cfg.Net = "tcp"
	}
	cfg.Addr = fmt.Sprintf("%s:%d", in.Host, in.Port)
	cfg.DBName = in.DefaultDB
	cfg.ParseTime = true
	cfg.Collation = "utf8mb4_general_ci"
	cfg.Timeout = 30 * time.Second
	switch in.TLS {
	case "required":
		cfg.TLSConfig = "true"
	case "preferred":
		cfg.TLSConfig = "preferred"
	case "skip-verify":
		cfg.TLSConfig = "skip-verify"
	default:
		cfg.TLSConfig = "false"
	}
	return cfg.FormatDSN()
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/db/...
```
Expected: PASS — `internal/db` and `internal/db/mysql` both green.

- [ ] **Step 5: Commit**

```bash
git add internal/db/mysql/dialect.go internal/db/mysql/dsn.go internal/db/mysql/ident.go internal/db/mysql/ident_test.go internal/db/mysql/dsn_test.go
git commit -m "feat(db/mysql): scaffold dialect with DSN and identifier quoting"
```

### Task 7: Move SQL classifier into MySQL dialect

**Files:**
- Create: `internal/db/mysql/sqlclass.go`
- Create: `internal/db/mysql/sqlclass_test.go`

- [ ] **Step 1: Write failing test**

Copy `internal/mysql/sqlclass_test.go` verbatim into `internal/db/mysql/sqlclass_test.go`, but change every call site that does `ClassifySQL(...)` to `(&MySQL{}).ClassifySQL(...)`. Imports become `"github.com/conray/dataseai/internal/db"` so types like `db.OpSelect` resolve.

For each test that asserts a `Classified.Op == OpX` from the old `mysql` package, change to `db.OpX`.

- [ ] **Step 2: Run tests (expect compile fail)**

```bash
go test ./internal/db/mysql/...
```
Expected: build failure — `ClassifySQL` method missing.

- [ ] **Step 3: Move implementation**

Create `internal/db/mysql/sqlclass.go` by copying `internal/mysql/sqlclass.go` verbatim, then:
- Drop the local `type Op` and `type Classified` declarations (use `db.Op` and `db.Classified`).
- Drop the local `const OpSelect Op = "SELECT"` block (use `db.OpSelect`, etc).
- Change the exported `ClassifySQL(sql string) (Classified, error)` to a method:
  ```go
  func (MySQL) ClassifySQL(sql string) (db.Classified, error) {
      // body unchanged except every `Op` -> `db.Op`, every `Classified{...}` -> `db.Classified{...}`
  }
  ```
- Keep helpers (`stripComments`, `splitTopLevel`, `firstTableRef`, `unquote`, `firstVerb`, regex vars) as package-private functions in the file — they are MySQL-syntax-aware.

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/mysql/...
```
Expected: PASS — every classifier test green under the new package.

- [ ] **Step 5: Commit**

```bash
git add internal/db/mysql/sqlclass.go internal/db/mysql/sqlclass_test.go
git commit -m "feat(db/mysql): move SQL classifier into dialect"
```

### Task 8: Move SSH dialer into MySQL dialect with engine-agnostic signature

**Files:**
- Create: `internal/db/mysql/ssh.go`

- [ ] **Step 1: Write failing test**

Add to `internal/db/mysql/ssh_test.go`:
```go
package mysql

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestRegisterSSHDialerRejectsZero(t *testing.T) {
	d := MySQL{}
	if _, _, err := d.RegisterSSHDialer(db.SSHConfig{}); err == nil {
		t.Fatal("expected error for zero SSHConfig")
	}
}
```

- [ ] **Step 2: Run (expect compile fail)**

```bash
go test ./internal/db/mysql/...
```
Expected: build failure — `RegisterSSHDialer` missing.

- [ ] **Step 3: Implement**

Create `internal/db/mysql/ssh.go` by copying `internal/mysql/ssh.go`, then:
- Replace `SSHConfig` with `db.SSHConfig`.
- Replace internal `nextSSHDialerName`, `sshTunnel`, etc with package-private impls (rename to lowercase `mysqlSSHTunnel` if needed to avoid collision with the global registry — but a private type already does that).
- Expose `(MySQL).RegisterSSHDialer(cfg db.SSHConfig) (name string, closer func(), err error)`:

```go
func (MySQL) RegisterSSHDialer(cfg db.SSHConfig) (string, func(), error) {
	if cfg.IsZero() {
		return "", nil, fmt.Errorf("ssh: host/user required")
	}
	tun, err := openMySQLTunnel(cfg)
	if err != nil {
		return "", nil, err
	}
	return tun.name, func() { closeMySQLTunnel(tun.name) }, nil
}
```

Rename the old free functions in this file (`openSSHTunnel` → `openMySQLTunnel`, `closeSSHTunnel` → `closeMySQLTunnel`) so they do not collide with the still-living symbols in `internal/mysql/ssh.go` until that file is deleted.

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/mysql/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/mysql/ssh.go internal/db/mysql/ssh_test.go
git commit -m "feat(db/mysql): expose SSH dialer registration via Dialect"
```

### Task 9: Move schema discovery (`DescribeTable`, `ListIndexes`, `ListForeignKeys`)

**Files:**
- Create: `internal/db/mysql/schema.go`
- Create: `internal/db/mysql/schema_test.go` (new — current `internal/mysql/` has no schema_test)

- [ ] **Step 1: Write integration-style failing test (sqlite-backed unit; document MySQL-only methods)**

`DescribeTable` runs MySQL-specific `SHOW CREATE TABLE` so a sqlite stub will diverge. Instead, write a contract test that just asserts the method exists and signatures compile:

Create `internal/db/mysql/schema_test.go`:
```go
package mysql

import (
	"reflect"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestMySQLImplementsSchemaMethods(t *testing.T) {
	var d db.Dialect = MySQL{}
	if _, err := d.ListDatabases(nil, nil, false); err == nil {
		// nil ctx + db will error; we only assert the method is callable.
	}
	// Compile-time check that types match the interface.
	_ = reflect.TypeOf(d)
}
```

(The deeper MySQL-specific assertions are covered by the existing API integration tests under `internal/api/...` which run against `mysqlweb`'s test container.)

- [ ] **Step 2: Run (compile fail)**

```bash
go test ./internal/db/mysql/...
```
Expected: build failure — `ListDatabases`, `DescribeTable`, etc missing on `MySQL`.

- [ ] **Step 3: Implement**

Create `internal/db/mysql/schema.go` by lifting from `internal/mysql/schema.go`. Transform free functions to methods on `MySQL`, change `Column`, `Structure`, `Index`, `ForeignKey` → `db.Column`, etc, and call `d.QuoteIdent(...)` where the original code did `QuoteIdent(...)`:

```go
package mysql

import (
	"context"
	"database/sql"

	"github.com/conray/dataseai/internal/db"
)

func (m MySQL) DescribeTable(ctx context.Context, dbh *sql.DB, schema, table string) (db.Structure, error) {
	// body identical to internal/mysql/schema.go DescribeTable, but
	// Column → db.Column, Structure → db.Structure, QuoteIdent → m.QuoteIdent
}

func (m MySQL) ListIndexes(ctx context.Context, dbh *sql.DB, schema, table string) ([]db.Index, error) { /* ditto */ }

func (m MySQL) ListForeignKeys(ctx context.Context, dbh *sql.DB, schema, table string) ([]db.ForeignKey, error) { /* ditto */ }
```

Create `internal/db/mysql/browse.go` (move from `internal/mysql/browse.go`):
- `ListDatabases`, `ListTables`, `ListSchemaColumns`, `FetchTableRows` all become methods on `MySQL`.
- Replace local `TableInfo`, `RowsPage`, `RowsOpts`, `Filter`, `MaxRowsPerPage` with the `db.` equivalents.
- `buildWhereClause`, `splitCSV`, `trimSpace`, `joinStrs` stay package-private functions.
- `QuoteIdent(...)` calls become `m.QuoteIdent(...)`.

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/...
```
Expected: PASS (existing tests under `internal/mysql/` still independently green; new methods on `MySQL` compile and the contract test passes).

- [ ] **Step 5: Commit**

```bash
git add internal/db/mysql/schema.go internal/db/mysql/browse.go internal/db/mysql/schema_test.go
git commit -m "feat(db/mysql): move schema and browse helpers onto dialect"
```

### Task 10: Move DML helpers and PrimaryKey

**Files:**
- Create: `internal/db/mysql/dml.go`
- Create: `internal/db/mysql/dml_test.go`

- [ ] **Step 1: Write failing tests**

Copy `internal/mysql/dml_test.go` into `internal/db/mysql/dml_test.go`. Update:
- Import `"github.com/conray/dataseai/internal/db"`.
- Replace function calls (`PrimaryKey(...)`, `UpdateCell(...)`) with `MySQL{}.PrimaryKey(...)`, etc.
- Replace `ErrNoPrimaryKey` with `db.ErrNoPrimaryKey`.

- [ ] **Step 2: Run (compile fail)**

```bash
go test ./internal/db/mysql/...
```
Expected: build failure.

- [ ] **Step 3: Add `ErrNoPrimaryKey` to `internal/db/types.go`**

Insert in `internal/db/types.go`:
```go
import "errors"
// ...
// ErrNoPrimaryKey signals a table has no primary key so edit-via-PK is
// not possible. Returned by Dialect.UpdateCell / DeleteRow.
var ErrNoPrimaryKey = errors.New("table has no primary key, edit disabled")
```

(Tests covering this constant move alongside dml_test in next step.)

- [ ] **Step 4: Implement `dml.go`**

Create `internal/db/mysql/dml.go` by porting `internal/mysql/dml.go`:
- Free functions → methods on `MySQL`.
- Use `m.QuoteIdent`.
- `ErrNoPrimaryKey` → `db.ErrNoPrimaryKey`.
- `coerceValue` and `coerceValues` stay package-private helpers.

- [ ] **Step 5: Tests pass**

```bash
go test ./internal/db/...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/types.go internal/db/mysql/dml.go internal/db/mysql/dml_test.go
git commit -m "feat(db/mysql): move DML helpers and PrimaryKey onto dialect"
```

### Task 11: Move kill query

**Files:**
- Create: `internal/db/mysql/kill.go`
- Create: `internal/db/mysql/kill_test.go`

- [ ] **Step 1: Write failing test (port from existing)**

Copy `internal/mysql/kill_test.go` into `internal/db/mysql/kill_test.go`. Update calls into the new method form: `(&MySQL{}).KillQuery(...)`, and references to the old free `ConnectionIDQuery` into `(&MySQL{}).ConnectionIDQuery()`.

- [ ] **Step 2: Run (compile fail)**

```bash
go test ./internal/db/mysql/...
```
Expected: build failure.

- [ ] **Step 3: Implement**

Create `internal/db/mysql/kill.go`:
```go
package mysql

import (
	"context"
	"database/sql"
	"strconv"
)

func (MySQL) KillQuery(ctx context.Context, dbh *sql.DB, connID int64) error {
	// MySQL identifier rules for KILL: integer literal, not parameter.
	_, err := dbh.ExecContext(ctx, "KILL "+strconv.FormatInt(connID, 10))
	return err
}

func (MySQL) ConnectionIDQuery() string { return "SELECT CONNECTION_ID()" }
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/mysql/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/mysql/kill.go internal/db/mysql/kill_test.go
git commit -m "feat(db/mysql): move KILL query and ConnectionID lookup onto dialect"
```

### Task 12: Move query/stream/export/import helpers as-is

Stream/export/import are not part of the `Dialect` interface (they aren't engine-divergent yet — pure `database/sql` consumers) but still currently live in `internal/mysql/`. Move them to `internal/db/mysql/` so the old package can be deleted.

**Files:**
- Create: `internal/db/mysql/query.go`
- Create: `internal/db/mysql/stream.go`
- Create: `internal/db/mysql/export.go`
- Create: `internal/db/mysql/import.go`
- Move all matching `*_test.go` files into the new package

- [ ] **Step 1: Copy each file verbatim**

For each of `query.go`, `stream.go`, `export.go`, `import.go` (plus their `*_test.go`):
- Copy file body from `internal/mysql/X.go` to `internal/db/mysql/X.go`.
- Change `package mysql` declaration (it already says `mysql`, so the declaration is unchanged — but the IMPORT path used by consumers changes).
- Drop the local declarations of `Op`, `Classified`, etc that have moved to `db`; refer to `db.X`.
- Adjust any references to in-package free helpers that have become methods (`QuoteIdent` → `MySQL{}.QuoteIdent`).

- [ ] **Step 2: Verify build**

```bash
go build ./internal/db/...
```
Expected: success.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/db/...
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/db/mysql/query.go internal/db/mysql/stream.go internal/db/mysql/export.go internal/db/mysql/import.go internal/db/mysql/*_test.go
git commit -m "feat(db/mysql): move stream/query/export/import helpers"
```

---

## Phase 3: Migrate consumers off `internal/mysql/`

Each consumer file is migrated independently so the diff stays reviewable. After every task in this phase, `go test ./...` must remain green.

### Task 13: Add Dialect injection point to `internal/api/db.go`

**Files:**
- Modify: `internal/api/db.go:1-100`
- Modify: `internal/api/db_test.go`

- [ ] **Step 1: Write failing test**

In `internal/api/db_test.go`, add:
```go
func TestDepsRequiresDialect(t *testing.T) {
	d := Deps{ /* fill required fields except Dialect */ }
	if d.Dialect == nil {
		// expected for now — record so we know we need to wire it in main
	}
}
```

(This is a sanity test; the real check is that `go test ./...` keeps passing after the migration.)

- [ ] **Step 2: Run baseline**

```bash
go test ./internal/api/...
```
Record current pass count.

- [ ] **Step 3: Update `Deps` struct**

In `internal/api/db.go`, change:
```go
type Deps struct {
    Pool *mysql.Pool
    Key  mysql.PoolKey
    // ... existing fields
}
```
to:
```go
import (
    "github.com/conray/dataseai/internal/db"
    mysqldialect "github.com/conray/dataseai/internal/db/mysql"
)

type Deps struct {
    Pool    *db.Pool
    Key     db.PoolKey
    Dialect db.Dialect
    // ... existing fields
}
```

Update every helper in this file that uses `mysql.X` to use the corresponding `db.X` or `d.Dialect.X` call. Keep the old import alive only for files not yet migrated.

- [ ] **Step 4: Update one call site (`ListDatabases`) end-to-end**

Replace:
```go
names, err := mysql.ListDatabases(ctx, cs.DB, includeSystem)
```
with:
```go
names, err := deps.Dialect.ListDatabases(ctx, cs.DB, includeSystem)
```
(where `deps` flows from the handler).

- [ ] **Step 5: Wire `Dialect` in main and tests**

In `cmd/dataseai/main.go` (or wherever `Deps` is constructed):
```go
import _ "github.com/conray/dataseai/internal/db/mysql" // register dialect

deps := api.Deps{
    Pool:    db.NewPool(db.PoolConfig{IdleTimeout: 5 * time.Minute}),
    Dialect: db.MustGet(db.EngineMySQL),
    // ...
}
```

In API tests that build a `Deps` literal, add `Dialect: mysqldialect.MySQL{}`.

- [ ] **Step 6: Run tests**

```bash
go test ./...
```
Expected: PASS — baseline pass count preserved.

- [ ] **Step 7: Commit**

```bash
git add internal/api/db.go internal/api/db_test.go cmd/dataseai/main.go
git commit -m "refactor(api): inject db.Dialect into Deps and use it for ListDatabases"
```

### Task 14: Migrate remaining schema/browse calls in `internal/api/`

**Files:**
- Modify: `internal/api/db.go` (full body)
- Modify: `internal/api/queries_test.go`
- Modify: `internal/api/agent_read.go`
- Modify: `internal/api/connections.go`
- Modify: `internal/api/connections_test.go`
- Modify: `internal/api/ai_policy.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: For each `mysql.ListTables`, `mysql.ListSchemaColumns`, `mysql.DescribeTable`, `mysql.ListIndexes`, `mysql.ListForeignKeys`, `mysql.FetchTableRows`**

In each call site:
- Replace with the matching `deps.Dialect.X(...)` call.
- Where the type was `mysql.RowsOpts`/`mysql.TableInfo`/`mysql.Structure`/`mysql.Index`/`mysql.ForeignKey`, replace with `db.X`.

- [ ] **Step 2: Build**

```bash
go build ./...
```
Expected: success.

- [ ] **Step 3: Run tests**

```bash
go test ./...
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/api/db.go internal/api/queries_test.go internal/api/agent_read.go internal/api/connections.go internal/api/connections_test.go internal/api/ai_policy.go internal/api/router.go
git commit -m "refactor(api): route schema/browse calls through db.Dialect"
```

### Task 15: Migrate DML and kill paths in `internal/api/`

**Files:**
- Modify: `internal/api/dml.go`
- Modify: `internal/api/query.go`
- Modify: `internal/api/ws.go`
- Modify: `internal/api/import.go`
- Modify: `internal/api/export.go`

- [ ] **Step 1: Replace DML free-function calls**

`mysql.PrimaryKey(ctx, db, schema, t)` → `deps.Dialect.PrimaryKey(ctx, db, schema, t)`
`mysql.UpdateCell(...)` → `deps.Dialect.UpdateCell(...)`
`mysql.InsertRow(...)` / `mysql.DeleteRow(...)` → corresponding `deps.Dialect.X(...)`.

- [ ] **Step 2: Replace KILL path**

Wherever `mysql.NewRegistry`, `mysql.ActiveQuery` is used, switch to `db.NewKillRegistry`, `db.ActiveQuery`. Where a query is actually killed:
```go
_, _ = poolDB.ExecContext(ctx, "KILL "+...)
```
becomes:
```go
if err := deps.Dialect.KillQuery(ctx, poolDB, connID); err != nil {
    return err
}
```

- [ ] **Step 3: Replace ConnectionID lookup**

`db.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&id)` becomes:
```go
db.QueryRowContext(ctx, deps.Dialect.ConnectionIDQuery()).Scan(&id)
```

- [ ] **Step 4: Build and test**

```bash
go build ./... && go test ./...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/dml.go internal/api/query.go internal/api/ws.go internal/api/import.go internal/api/export.go
git commit -m "refactor(api): route DML and KILL paths through db.Dialect"
```

### Task 16: Migrate `internal/chat/` orchestrator and propose paths

**Files:**
- Modify: `internal/chat/orchestrator.go`
- Modify: `internal/chat/propose.go`
- Modify: `internal/chat/execute.go`
- Modify: `internal/chat/execute_test.go`
- Modify: `internal/chat/propose_test.go`
- Modify: `internal/chat/orchestrator_test.go`

- [ ] **Step 1: Update Executor type**

In `orchestrator.go`:
```go
import (
    "github.com/conray/dataseai/internal/db"
)

type Orchestrator struct {
    Executor db.Executor
    Dialect  db.Dialect
    // ...
}
```

- [ ] **Step 2: Update propose path**

Wherever the chat code calls `mysql.ClassifySQL(stmt)` to enforce policy, switch to `orch.Dialect.ClassifySQL(stmt)`. Where the chat code references `mysql.Op...` constants, use `db.Op...`.

- [ ] **Step 3: Update test fixtures**

Where tests construct an `Orchestrator` with a `mysql.DirectExecutor`, replace with `db.DirectExecutor`. Where tests assert on `Classified`/`Op` types, update imports.

- [ ] **Step 4: Build and test**

```bash
go test ./internal/chat/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/chat/
git commit -m "refactor(chat): switch orchestrator to db.Dialect and db.Executor"
```

### Task 17: Migrate `internal/agent/`

**Files:**
- Modify: `internal/agent/executor.go`
- Modify: `internal/agent/executor_test.go`

- [ ] **Step 1: Update types**

`mysql.SSHConfig` → `db.SSHConfig`.
`mysql.DSNInput` → `db.DSNInput`.
Any `mysql.QuoteIdent` references → use the agent's stored `db.Dialect`.

- [ ] **Step 2: Build and test**

```bash
go test ./internal/agent/...
```
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/agent/
git commit -m "refactor(agent): use engine-agnostic db types"
```

### Task 18: Sweep for remaining `internal/mysql` imports

**Files:** any not yet covered.

- [ ] **Step 1: Find leftovers**

```bash
grep -rln "conray/dataseai/internal/mysql" --include="*.go" | sort -u
```
Expected: empty. Any file printed means a missed migration.

- [ ] **Step 2: For each leftover, repeat the swap pattern**

For every leftover file:
- Add `import "github.com/conray/dataseai/internal/db"` (and the mysql dialect if needed).
- Replace `mysql.X` with `db.X` or `<dialect>.X`.
- Delete the old import.

- [ ] **Step 3: Build and test**

```bash
go build ./... && go test ./...
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: clear final internal/mysql consumers"
```

### Task 19: Delete `internal/mysql/`

**Files:**
- Delete: entire `internal/mysql/` directory

- [ ] **Step 1: Final import scan**

```bash
grep -rln "conray/dataseai/internal/mysql" --include="*.go"
```
Expected: empty.

- [ ] **Step 2: Remove the directory**

```bash
git rm -r internal/mysql/
```

- [ ] **Step 3: Build and test**

```bash
go build ./... && go test ./...
```
Expected: PASS, same baseline count.

- [ ] **Step 4: Commit**

```bash
git commit -m "refactor: drop legacy internal/mysql package"
```

---

## Phase 4: `engine` column on `connections`

### Task 20: Migration `0017_engine_column.sql`

**Files:**
- Create: `internal/store/migrations/0017_engine_column.sql`
- Modify: `internal/store/migrate_test.go` (assert new column)

- [ ] **Step 1: Write failing migration test**

In `internal/store/migrate_test.go`, add:
```go
func TestMigrationAddsEngineColumn(t *testing.T) {
	s := openTestStore(t)
	rows, err := s.DB.Query("PRAGMA table_info(connections)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "engine" {
			found = true
			if !dflt.Valid || dflt.String != "'mysql'" {
				t.Fatalf("engine default = %v, want 'mysql'", dflt)
			}
		}
	}
	if !found {
		t.Fatal("engine column missing")
	}
}
```

- [ ] **Step 2: Run (expect fail)**

```bash
go test ./internal/store/... -run TestMigrationAddsEngineColumn
```
Expected: FAIL — column missing.

- [ ] **Step 3: Write the migration**

Create `internal/store/migrations/0017_engine_column.sql`:
```sql
ALTER TABLE connections ADD COLUMN engine TEXT NOT NULL DEFAULT 'mysql';
UPDATE connections SET engine='mysql' WHERE engine IS NULL OR engine='';
CREATE INDEX idx_connections_engine ON connections(engine);
```

- [ ] **Step 4: Test passes**

```bash
go test ./internal/store/... -run TestMigrationAddsEngineColumn
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/migrations/0017_engine_column.sql internal/store/migrate_test.go
git commit -m "feat(store): add engine column to connections (default 'mysql')"
```

### Task 21: Plumb `Engine` through `store.Connection` / `ConnectionInput`

**Files:**
- Modify: `internal/store/connections.go`
- Modify: `internal/store/connections_test.go`

- [ ] **Step 1: Write failing test**

In `internal/store/connections_test.go`, add:
```go
func TestCreateConnectionDefaultsEngineMySQL(t *testing.T) {
	s, cipher := newTestStore(t)
	in := ConnectionInput{Name: "x", Host: "h", Port: 3306, Username: "u", Password: "p"}
	c, err := s.CreateConnection(cipher, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	if c.Engine != "mysql" {
		t.Fatalf("Engine = %q, want %q", c.Engine, "mysql")
	}
}
```

- [ ] **Step 2: Run (expect fail / compile error)**

```bash
go test ./internal/store/... -run TestCreateConnectionDefaultsEngineMySQL
```
Expected: build failure — `Engine` field missing.

- [ ] **Step 3: Add Engine field**

In `internal/store/connections.go`:
- Add `Engine string` (default "mysql") to both `ConnectionInput` and `Connection`.
- Update `connectionColumns` constant to include `engine` (use `COALESCE(engine, 'mysql')` for safety).
- Update `scanConnection` to scan it.
- Update `CreateConnection` to insert it (default to "mysql" if empty).
- Update `UpdateConnection` (or the per-column setter, depending on which is used) to honor it.

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/store/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/connections.go internal/store/connections_test.go
git commit -m "feat(store): expose Engine field on Connection (default mysql)"
```

### Task 22: Surface `engine` on connection API DTO

**Files:**
- Modify: `internal/api/connections.go` (request/response payloads)
- Modify: `internal/api/connections_test.go`

- [ ] **Step 1: Write failing test**

In `internal/api/connections_test.go`, add an assertion in the existing `TestListConnections` or new `TestGetConnectionReturnsEngine`:
```go
if got.Engine != "mysql" {
    t.Fatalf("payload.Engine = %q", got.Engine)
}
```

- [ ] **Step 2: Run (expect fail)**

```bash
go test ./internal/api/... -run TestGetConnectionReturnsEngine
```
Expected: FAIL.

- [ ] **Step 3: Add `Engine` to JSON payload**

In `internal/api/connections.go`:
- Add `Engine string `json:"engine"`` to the DTO.
- In `CreateConnection` and `UpdateConnection` handlers, accept incoming `engine` (default "mysql"; reject any other value with HTTP 400 for now to preserve scope).
- Wire it through `store.ConnectionInput`.

```go
allowedEngines := map[string]bool{"mysql": true}
if !allowedEngines[req.Engine] && req.Engine != "" {
    http.Error(w, "unsupported engine", http.StatusBadRequest)
    return
}
if req.Engine == "" {
    req.Engine = "mysql"
}
```

- [ ] **Step 4: Use engine when looking up dialect**

In handlers that produce `Deps`, swap the hard-coded `db.MustGet(db.EngineMySQL)` for:
```go
engine, err := db.ParseEngine(conn.Engine)
if err != nil { http.Error(...); return }
deps.Dialect = db.MustGet(engine)
```

- [ ] **Step 5: Tests pass**

```bash
go test ./internal/api/...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/connections.go internal/api/connections_test.go
git commit -m "feat(api): expose connection engine and dispatch dialect by it"
```

### Task 23: Frontend — show engine as read-only

**Files:**
- Modify: `web/src/types/connection.ts` (or wherever the Connection type lives)
- Modify: `web/src/components/ConnectionForm.*` (whichever framework — React/Svelte/Vue per repo convention)
- Modify: `web/src/components/ConnectionList.*` (badge in list)

- [ ] **Step 1: Discover exact file paths**

```bash
grep -rln "host.*port" web/src 2>/dev/null | head
grep -rln "TLS" web/src 2>/dev/null | head
```
Use the discovered file as the connection form. The repo uses React (see `web/dist/assets/index-*.js`); src likely TSX.

- [ ] **Step 2: Add `engine` to the Connection type**

```typescript
export type ConnectionEngine = "mysql";

export interface Connection {
  id: number;
  name: string;
  host: string;
  port: number;
  username: string;
  defaultDb: string;
  tls: "disabled" | "preferred" | "required" | "skip-verify";
  engine: ConnectionEngine;
  // ...rest unchanged
}
```

- [ ] **Step 3: Render engine badge in the form**

In the connection form component, immediately under the Name field, render:
```tsx
<div className="field">
  <label>Engine</label>
  <span className="badge">MySQL</span>
</div>
```
On create, send `engine: "mysql"` in the request body.

- [ ] **Step 4: Build the bundle**

```bash
cd web && npm run build
```
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add web/
git commit -m "feat(web): show engine on connection form (read-only mysql badge)"
```

---

## Phase 5: Test environment branch hookup

### Task 24: Deploy notes + test environment switch

**Files:**
- Create: `docs/test-env/dialect-abstraction.md` (release notes)
- Modify: `deploy/setup-vm.sh` (only if it hardcodes a branch)

- [ ] **Step 1: Document the branch**

Create `docs/test-env/dialect-abstraction.md`:
```markdown
# Test Environment — feat/db-dialect-abstraction

Branch under test: `feat/db-dialect-abstraction`

## What changed
- `internal/mysql/` removed; all DB-engine behavior lives in `internal/db/`
  with a per-engine implementation under `internal/db/<engine>/`.
- MySQL dialect is the sole implementation; `engine` column on connections
  is locked to `mysql`.
- No user-visible behavior change. Connection form shows an "Engine: MySQL"
  badge.

## Verification checklist
- [ ] `go test ./...` green on CI
- [ ] Connect to staging MySQL via SSH tunnel — list databases, list tables,
      describe schema, browse rows, run a SELECT, edit a cell, kill a long
      query.
- [ ] Connect to staging MySQL **without** SSH — repeat above.
- [ ] Chat orchestrator: propose + execute a SELECT against staging.
- [ ] Existing connections (created on `main`) load correctly after the
      0017 migration runs.

## Rollback
- Revert the merge commit; `0017_engine_column.sql` is additive and does
  not need a down migration (the column is ignored when the column simply
  stays present and `engine` field is unused on the prior code path).
```

- [ ] **Step 2: Verify VM provisioning script**

```bash
grep -n "branch\|main" deploy/setup-vm.sh
```
If `setup-vm.sh` hardcodes `main`, leave a TODO comment near it noting the test branch must be passed in via env var rather than editing the script for this plan.

- [ ] **Step 3: Commit**

```bash
git add docs/test-env/dialect-abstraction.md
git commit -m "docs(test-env): document dialect abstraction test branch"
```

### Task 25: Push branch and open draft PR

- [ ] **Step 1: Push**

```bash
git push -u origin feat/db-dialect-abstraction
```

- [ ] **Step 2: Open draft PR**

Use the GitHub CLI:
```bash
gh pr create --draft --base main \
  --title "refactor(db): introduce dialect abstraction (MySQL-only first pass)" \
  --body "$(cat <<'EOF'
## Summary
- Extract every MySQL-specific behavior into `internal/db.Dialect`.
- Move existing MySQL code into `internal/db/mysql/` as the first dialect implementation; delete `internal/mysql/` once consumers migrated.
- Add `engine` column on `connections` (default `mysql`); API + frontend surface it read-only.
- No user-visible behavior change. Sets up follow-up plans for PostgreSQL and MSSQL dialects.

## Test plan
- [ ] `go test ./...` passes locally (preserve baseline pass count from main).
- [ ] Existing prod connections continue to work end-to-end on the test VM.
- [ ] SSH tunnel path verified against staging MySQL.
- [ ] Chat orchestrator continues to classify + propose against staging.
- [ ] Migration `0017_engine_column.sql` applies cleanly to prod-shape sqlite metadata store.

## Out of scope
- PostgreSQL / MSSQL implementations (separate plans).
- Connector binary changes (separate plan).
- Frontend engine selector (locked to MySQL for now).

EOF
)"
```

- [ ] **Step 3: Confirm PR URL**

Capture the URL printed by `gh pr create`. Hand it to the user.

---

## Self-Review Checklist

Before declaring the plan complete:

- [ ] Every spec requirement (Dialect interface, MySQL retrofit, engine column, frontend badge, test environment) maps to at least one task above.
- [ ] No `TODO`, `TBD`, "similar to Task N" placeholders remain.
- [ ] Type names are consistent: `db.Engine`, `db.Dialect`, `db.DSNInput`, `db.SSHConfig`, `db.Column`, `db.Structure`, `db.Index`, `db.ForeignKey`, `db.TableInfo`, `db.RowsOpts`, `db.RowsPage`, `db.Filter`, `db.Classified`, `db.Op`, `db.MaxRowsPerPage`, `db.ErrNoPrimaryKey`, `db.Pool`, `db.PoolKey`, `db.PoolConfig`, `db.KillRegistry`, `db.ActiveQuery`, `db.ErrKillNoMatch`, `db.Executor`, `db.DirectExecutor`, `db.ExecResult`. `MySQL` is the dialect struct; methods on it return `db.X` types.
- [ ] Every task ends with a commit step.
- [ ] Build and test commands appear at every checkpoint.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-09-db-dialect-abstraction.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — Dispatch a fresh subagent per task; review the diff between tasks; fast iteration.

**2. Inline Execution** — Execute tasks in this session using `superpowers:executing-plans`; batch execution with checkpoints for review.

Which approach?
