# PostgreSQL Dialect Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a PostgreSQL implementation of `db.Dialect` so users can create `engine: "postgres"` connections that flow through the same schema discovery, browse, DML, kill, stream, export, and import paths as MySQL.

**Architecture:** Mirror the MySQL dialect's package layout under `internal/db/pg/`. Use `jackc/pgx/v5` (pinned via `database/sql` driver registration `pgx`) so the existing pool / `*sql.DB` machinery applies unchanged. PG-specific quirks land in dialect methods: placeholders are `$N`, identifiers quote with double-quotes, "SHOW CREATE TABLE" is synthesized from columns, primary key auto-increment uses `RETURNING`, KILL is `pg_cancel_backend`, "database" surface = PG schemas (so the connection sidebar behaves like MySQL — pick the schema as if it were a database).

**Tech Stack:** Go 1.25, `database/sql`, `github.com/jackc/pgx/v5`, `github.com/jackc/pgx/v5/stdlib`, `github.com/jackc/pgx/v5/pgconn` (for SSH-tunneled dialing via `Config.DialFunc`), existing `golang.org/x/crypto/ssh`.

**Scope guard:** This plan adds **only direct-connection PG support** from the web app. The connector binary is untouched — `via_agent` PG connections are explicitly rejected (HTTP 400) and the frontend hides the agent option when engine = postgres. PG-via-connector is a separate plan.

**Schema/database concept:** PG has cluster → database → schema → table. To keep the UI consistent with MySQL (which has database → table), this dialect treats **PG schemas as "databases"** in the `Dialect.ListDatabases` / `ListTables` / `DescribeTable` / etc API. The PG database the user connects to is set via `connection.default_db`. The connection sidebar then lists the PG schemas in that database. This matches what a DataGrip / DBeaver "MySQL-style" view does.

**Branch:** `feat/db-pg-dialect` cut from `feat/db-dialect-abstraction` (or `main` once that merges).

---

## File Structure

### New package `internal/db/pg/`
- `internal/db/pg/dialect.go` — `PG` struct embeds `db.UnimplementedDialect`, overrides Engine/DriverName, registers via init.
- `internal/db/pg/dsn.go` — `(PG).BuildDSN` builds a pgx DSN string. Supports SSL modes via `db.DSNInput.TLS` mapping (`disabled` → `disable`, `preferred` → `prefer`, `required` → `require`, `skip-verify` → `verify-ca` or `require` with custom verifier — implementation note).
- `internal/db/pg/ident.go` — `(PG).QuoteIdent` (double-quote) + `(PG).Placeholder(n)` returning `$N`.
- `internal/db/pg/sqlclass.go` — `(PG).ClassifySQL`. Borrow MySQL's regex shape but swap out MySQL-only modifiers (`LOW_PRIORITY`, `DELAYED`, `QUICK`, `IGNORE`) and add PG-specifics (`WITH RECURSIVE`, `RETURNING`, schema-qualified table refs via `"schema"."table"`).
- `internal/db/pg/schema.go` — `(PG).DescribeTable`, `ListIndexes`, `ListForeignKeys`. PG has no `SHOW CREATE TABLE`; synthesize a `CREATE TABLE` string from `information_schema.columns` for parity.
- `internal/db/pg/browse.go` — `(PG).ListDatabases` (= schemas), `ListTables`, `ListSchemaColumns`, `FetchTableRows`. The `FetchTableRows` rewrite uses `$1, $2, ...` placeholders and PG-friendly LIMIT/OFFSET (same syntax as MySQL).
- `internal/db/pg/dml.go` — `(PG).PrimaryKey` (via `information_schema.table_constraints` + `key_column_usage`), `(PG).UpdateCell`, `(PG).InsertRow` (uses `RETURNING <pk>` to fetch the inserted id), `(PG).DeleteRow`.
- `internal/db/pg/kill.go` — `(PG).KillQuery` issues `SELECT pg_cancel_backend($1)`; `(PG).ConnectionIDQuery()` returns `SELECT pg_backend_pid()`.
- `internal/db/pg/ssh.go` — `(PG).RegisterSSHDialer` returns a dialer name. Unlike MySQL (which uses a global `RegisterDialContext` registry), pgx exposes per-`Config.DialFunc`. The implementation injects the SSH dialer into a `pgxstdlib.RegisterConnConfig(cfg)` — see pgx docs — and returns a key that BuildDSN places in the DSN so the runtime resolves it. Closer disposes the SSH client.
- `internal/db/pg/stream.go`, `query.go`, `export.go`, `import.go` — PG-flavored implementations of the MySQL-only helpers (these are NOT part of the `Dialect` interface but live alongside it for namespace parity). Initially implement the minimum needed for current `internal/api/*` callers; defer rarely-used paths if they require significant rework, but FLAG them in DONE_WITH_CONCERNS.

### Modified files
- `internal/db/types.go` — add `EnginePostgres Engine = "postgres"` constant. Add `ParseEngine` case. Keep `EngineMSSQL` reserved as a comment.
- `internal/api/connections.go` — extend the validate whitelist: `{"mysql": true, "postgres": true}`.
- `internal/api/db.go` — `dialectForConn` already routes by `conn.Engine` (post-abstraction); no change needed there, only the registry has to have PG registered. Add a blank `_ "github.com/conray/dataseai/internal/db/pg"` import in `cmd/dataseai/main.go` (mirrors the MySQL import).
- `cmd/dataseai/main.go` — add the blank import.
- `web/src/store/connections.ts` — extend `ConnectionEngine = "mysql" | "postgres"`.
- `web/src/components/ConnectionDialog.tsx` — convert the read-only badge into a `<select>` dropdown with two options (`MySQL`, `PostgreSQL`); send the chosen engine; default port adjusts (MySQL 3306, Postgres 5432).
- `web/src/components/ConnectionsManager.tsx` — `engineLabel()` switch already shape-extensible; add the `postgres` case.
- `web/src/i18n/messages.ts` — add `engine_postgres` label in en / zh-TW.

### Tests / fixtures
- `internal/db/pg/*_test.go` — mirror the MySQL test structure. For schema/dml integration tests that need a real PG, either:
  (a) use `github.com/testcontainers/testcontainers-go` to spin up a temporary Postgres in tests, OR
  (b) gate the deep tests behind `t.Skip(...)` unless `PG_DSN` env var is set, and run unit tests against sqlite where pgx-compatible queries also happen to work.
  Pick (b) for this plan — testcontainers adds CI dependencies that aren't in the repo. The DSN/ident/placeholder/classifier tests are sufficient for the dialect contract; deeper tests can be added in a follow-up.
- `internal/api/connections_test.go` — accept `engine: "postgres"` in the create test alongside the existing reject case.
- `web/src/store/connections.test.ts` — fixture additions (no behavior changes).
- `web/src/components/ConnectionDialog.test.tsx` — verify the selector renders both options.

---

## Pre-flight Tasks

### Task 0: Branch + dependency

**Files:**
- `go.mod`, `go.sum`

- [ ] **Step 1: Confirm dialect abstraction landed**

```bash
cd /home/conray/project/mysqlweb
git log --oneline | grep -q "drop legacy internal/mysql package" || echo "WARN: dialect plan not merged yet — branching from feat/db-dialect-abstraction directly"
```

- [ ] **Step 2: Cut feature branch**

```bash
git fetch origin
git checkout feat/db-dialect-abstraction
git checkout -b feat/db-pg-dialect
```

- [ ] **Step 3: Add pgx dependency**

```bash
go get github.com/jackc/pgx/v5@latest
go get github.com/jackc/pgx/v5/stdlib@latest
```

(`go get` will update go.mod and go.sum.)

- [ ] **Step 4: Baseline test**

```bash
go test ./...
```

Record baseline. Expected: 244 passing across 14 packages.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum docs/superpowers/plans/2026-06-09-postgres-dialect.md
git commit -m "chore(deps): add jackc/pgx/v5 for postgres dialect"
```

---

## Phase 1: PG package scaffold + DSN + ident

### Task 1: Add EnginePostgres constant

**Files:**
- Modify: `internal/db/types.go`
- Modify: `internal/db/types_test.go`

- [ ] **Step 1: Add failing test**

In `internal/db/types_test.go`, add:
```go
func TestParseEnginePostgres(t *testing.T) {
	e, err := ParseEngine("postgres")
	if err != nil {
		t.Fatal(err)
	}
	if e != EnginePostgres {
		t.Fatalf("got %v, want EnginePostgres", e)
	}
}
```

- [ ] **Step 2: Run (compile fail)**

```bash
go test ./internal/db/ -run TestParseEnginePostgres
```
Expected: build failure — `EnginePostgres` undefined.

- [ ] **Step 3: Add constant**

In `internal/db/types.go`:
```go
const (
	EngineMySQL    Engine = "mysql"
	EnginePostgres Engine = "postgres"
	// EngineMSSQL    Engine = "mssql"    // reserved for future plan
)
```

And update `ParseEngine`:
```go
case EngineMySQL:
	return EngineMySQL, nil
case EnginePostgres:
	return EnginePostgres, nil
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/...
```
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add internal/db/types.go internal/db/types_test.go
git commit -m "feat(db): add EnginePostgres engine constant"
```

### Task 2: Scaffold internal/db/pg/ with dialect, DSN, ident

**Files:**
- Create: `internal/db/pg/dialect.go`
- Create: `internal/db/pg/dsn.go`
- Create: `internal/db/pg/ident.go`
- Create: `internal/db/pg/dsn_test.go`
- Create: `internal/db/pg/ident_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/db/pg/ident_test.go`:
```go
package pg

import "testing"

func TestQuoteIdent(t *testing.T) {
	d := PG{}
	cases := []struct{ in, want string }{
		{"users", `"users"`},
		{"my table", `"my table"`},
		{`"weird"`, `""""weird"""`},  // each " doubled
		{"", `""`},
	}
	for _, c := range cases {
		if got := d.QuoteIdent(c.in); got != c.want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	d := PG{}
	if got := d.Placeholder(1); got != "$1" {
		t.Fatalf("Placeholder(1) = %q, want $1", got)
	}
	if got := d.Placeholder(42); got != "$42" {
		t.Fatalf("Placeholder(42) = %q, want $42", got)
	}
}
```

Create `internal/db/pg/dsn_test.go`:
```go
package pg

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestBuildDSNBasics(t *testing.T) {
	d := PG{}
	got := d.BuildDSN(db.DSNInput{
		Host: "h", Port: 5432, Username: "u", Password: "p", DefaultDB: "appdb",
	})
	// We assert key substrings rather than the exact string (pgx DSN order is implementation-defined).
	for _, want := range []string{"host=h", "port=5432", "user=u", "password=p", "dbname=appdb"} {
		if !strings.Contains(got, want) {
			t.Fatalf("DSN missing %q: %s", want, got)
		}
	}
}

func TestBuildDSNTLSRequired(t *testing.T) {
	d := PG{}
	got := d.BuildDSN(db.DSNInput{
		Host: "h", Port: 5432, Username: "u", Password: "p", TLS: "required",
	})
	if !strings.Contains(got, "sslmode=require") {
		t.Fatalf("sslmode flag missing in %q", got)
	}
}

func TestBuildDSNTLSPreferred(t *testing.T) {
	d := PG{}
	got := d.BuildDSN(db.DSNInput{Host: "h", Port: 5432, Username: "u", TLS: "preferred"})
	if !strings.Contains(got, "sslmode=prefer") {
		t.Fatalf("sslmode missing: %q", got)
	}
}

func TestBuildDSNTLSDisabled(t *testing.T) {
	d := PG{}
	got := d.BuildDSN(db.DSNInput{Host: "h", Port: 5432, Username: "u", TLS: "disabled"})
	if !strings.Contains(got, "sslmode=disable") {
		t.Fatalf("sslmode missing: %q", got)
	}
}

func TestBuildDSNAwkwardPassword(t *testing.T) {
	// pgx accepts URL-encoded passwords; key=value DSN form requires the
	// password to be quoted with backslash-escapes if it contains spaces or '.
	d := PG{}
	got := d.BuildDSN(db.DSNInput{
		Host: "h", Port: 5432, Username: "u", Password: "p:@/", DefaultDB: "x",
	})
	// The DSN must survive a round-trip parse — assert no obvious shell-quote breakage.
	if !strings.Contains(got, "password=") {
		t.Fatalf("password absent: %s", got)
	}
}
```

- [ ] **Step 2: Run (compile fail)**

```bash
go test ./internal/db/pg/...
```
Expected: package not found.

- [ ] **Step 3: Implement**

Create `internal/db/pg/dialect.go`:
```go
// Package pg is the PostgreSQL implementation of db.Dialect.
package pg

import (
	"github.com/conray/dataseai/internal/db"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PG is the PostgreSQL dialect. Methods are split across dsn.go / ident.go /
// schema.go / browse.go / dml.go / kill.go / sqlclass.go. Package init
// registers the singleton.
type PG struct {
	db.UnimplementedDialect
}

func (PG) Engine() db.Engine  { return db.EnginePostgres }
func (PG) DriverName() string { return "pgx" }

var singleton = PG{}

func init() {
	db.Register(db.EnginePostgres, singleton)
}
```

Create `internal/db/pg/ident.go`:
```go
package pg

import "strings"

// QuoteIdent wraps a PG identifier in double quotes. Embedded double quotes
// are escaped by doubling them — same rule as MySQL's backtick scheme.
func (PG) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Placeholder returns the PG-style positional placeholder for index n.
// PG indexes are 1-based; n=1 yields "$1".
func (PG) Placeholder(n int) string {
	return "$" + strconv.Itoa(n)
}
```

Add the `strconv` import. If you prefer to avoid `strconv`, use `fmt.Sprintf("$%d", n)` — pick one style and stick with it. (Recommendation: `strconv` — zero allocation.)

Create `internal/db/pg/dsn.go`:
```go
package pg

import (
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// BuildDSN constructs a pgx key=value DSN. pgx accepts both URL and key=value
// formats; we use key=value because it round-trips arbitrary passwords without
// percent-encoding subtleties.
func (PG) BuildDSN(in db.DSNInput) string {
	var b strings.Builder
	add := func(key, val string) {
		if val == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		// Quote values containing spaces, quotes, or backslashes.
		needsQuote := strings.ContainsAny(val, " '\\")
		if needsQuote {
			fmt.Fprintf(&b, `%s='%s'`, key, strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(val))
		} else {
			fmt.Fprintf(&b, "%s=%s", key, val)
		}
	}
	add("host", in.Host)
	if in.Port != 0 {
		add("port", fmt.Sprintf("%d", in.Port))
	}
	add("user", in.Username)
	add("password", in.Password)
	add("dbname", in.DefaultDB)
	switch in.TLS {
	case "disabled":
		add("sslmode", "disable")
	case "preferred":
		add("sslmode", "prefer")
	case "required":
		add("sslmode", "require")
	case "skip-verify":
		add("sslmode", "require") // pgx has "require" (no cert verify); "verify-ca"/"verify-full" tighten
	default:
		add("sslmode", "prefer")
	}
	// in.Network (SSH dialer name) gets wired in the SSH integration via pgx
	// Config registration; the DSN itself doesn't expose it.
	add("connect_timeout", "30")
	return b.String()
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/pg/...
```
Expected: PASS for QuoteIdent (4 cases), Placeholder, and 5 BuildDSN cases.

- [ ] **Step 5: Commit**

```bash
git add internal/db/pg/dialect.go internal/db/pg/dsn.go internal/db/pg/ident.go internal/db/pg/ident_test.go internal/db/pg/dsn_test.go
git commit -m "feat(db/pg): scaffold dialect with DSN and double-quote identifier"
```

---

## Phase 2: ClassifySQL for PG

### Task 3: Port + adapt SQL classifier

**Files:**
- Create: `internal/db/pg/sqlclass.go`
- Create: `internal/db/pg/sqlclass_test.go`

- [ ] **Step 1: Failing test**

Create `internal/db/pg/sqlclass_test.go`. Start from `internal/db/mysql/sqlclass_test.go` and:
- Swap `(MySQL).ClassifySQL(s)` for `(PG).ClassifySQL(s)`.
- Adjust the comment-stripping cases — PG has `--` comments and `/* ... */` (same as MySQL — no change there).
- Replace MySQL-only modifiers in test cases (`LOW_PRIORITY`, `DELAYED`, `QUICK`, `IGNORE`) with PG-style modifiers (`ONLY` in DELETE, `CONCURRENTLY` in CREATE INDEX, `WITH RECURSIVE` in SELECT). Add tests:
```go
{"select_with_recursive", `WITH RECURSIVE t AS (SELECT 1) SELECT * FROM t`, db.OpSelect, "", "t", false},
{"delete_only", `DELETE FROM ONLY mytable WHERE id=1`, db.OpDelete, "", "mytable", false},
{"insert_returning", `INSERT INTO users (name) VALUES ('x') RETURNING id`, db.OpInsert, "", "users", false},
{"schema_qualified", `SELECT * FROM "myschema"."mytable"`, db.OpSelect, "myschema", "mytable", false},
```

- [ ] **Step 2: Run (compile fail)**

```bash
go test ./internal/db/pg/ -run TestClassifyBasic
```
Expected: failure — `ClassifySQL` missing.

- [ ] **Step 3: Implement**

Create `internal/db/pg/sqlclass.go` by porting `internal/db/mysql/sqlclass.go` with these changes:
- Method receiver: `(PG)` instead of `(MySQL)`.
- Identifier regex must accept double-quoted identifiers (`"name"`) and unquoted PG identifiers (which can include letters, digits, `_`, and `$` after the first character). Replace the backtick-aware `identRE` with: ``regexp.MustCompile(`"(?:[^"]|"")*"|[A-Za-z_][A-Za-z0-9_$]*`)``.
- `unquote` should detect leading double-quote instead of backtick.
- INSERT/UPDATE/DELETE regex variants: drop the MySQL-only modifiers (`LOW_PRIORITY`, `DELAYED`, `QUICK`, `IGNORE`). Add `ONLY` to the DELETE regex: `^DELETE\s+(?:FROM\s+)?(?:ONLY\s+)?(.+)$`.
- For SELECT, accept a leading `WITH RECURSIVE` clause: strip a `WITH\s+(?:RECURSIVE\s+)?...\)\s*SELECT` if present, then re-extract the table.
- Verb list for OpForbidden / OpReadMeta needs PG terms:
  - OpReadMeta: `EXPLAIN`, `\d` (psql backslash commands — won't reach the API, ignore).
  - OpForbidden: `CREATE`, `DROP`, `GRANT`, `REVOKE`, `CLUSTER`, `VACUUM`, `ANALYZE`, `REINDEX`, `LISTEN`, `NOTIFY`, `UNLISTEN`, `SECURITY LABEL`.
- The split-by-`;` logic stays the same; quoted strings use `'` (single quote) and `"` (double quote). Drop the backtick handling.

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/pg/...
```
Expected: all classifier tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/db/pg/sqlclass.go internal/db/pg/sqlclass_test.go
git commit -m "feat(db/pg): SQL classifier with PG-specific verbs and quoting"
```

---

## Phase 3: SSH dialer

### Task 4: Wire SSH tunnel into pgx Config

**Files:**
- Create: `internal/db/pg/ssh.go`
- Create: `internal/db/pg/ssh_test.go`

- [ ] **Step 1: Failing test**

Create `internal/db/pg/ssh_test.go`:
```go
package pg

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestRegisterSSHDialerRejectsZero(t *testing.T) {
	d := PG{}
	if _, _, err := d.RegisterSSHDialer(db.SSHConfig{}); err == nil {
		t.Fatal("expected error for zero SSHConfig")
	}
}
```

- [ ] **Step 2: Run (compile fail)**

```bash
go test ./internal/db/pg/ -run TestRegisterSSHDialerRejectsZero
```

- [ ] **Step 3: Implement**

pgx integrates custom dialers via `pgx.ConnConfig.DialFunc` and the connection-string mechanism for stdlib is `stdlib.RegisterConnConfig(cfg)` which returns an opaque string usable as a DSN. We exploit this:

Create `internal/db/pg/ssh.go`:
```go
package pg

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/ssh"

	"github.com/conray/dataseai/internal/db"
)

// RegisterSSHDialer opens an SSH bastion, parses the per-tunnel pgx
// ConnConfig, injects an SSH-backed DialFunc, and registers the result with
// stdlib. The returned "name" is actually a pgx DSN — BuildDSN-aware callers
// pass it through DSNInput.Network so the pool feeds it to sql.Open directly.
//
// Limitation: because BuildDSN constructs the DSN from scratch, the tunnel
// integration ALSO needs the host/port/user/password/dbname to be set when
// the DialFunc is registered. The PG pool path calls RegisterSSHDialer first,
// then constructs the DSN with Network=<registered name>. To work around the
// fact that pgx's registered key replaces the DSN entirely, the pool's
// Get() must use the registered string verbatim when Network is non-empty
// rather than calling BuildDSN. Document this clearly.
//
// Implementation: registered names are strings of the form
// "registeredConnConfig:<random>" — stdlib accepts that as a DSN when looking
// up the pre-bound config.
func (PG) RegisterSSHDialer(cfg db.SSHConfig) (string, func(), error) {
	if cfg.IsZero() {
		return "", nil, fmt.Errorf("ssh: host/user required")
	}
	client, err := dialSSH(cfg)
	if err != nil {
		return "", nil, err
	}
	// Build a placeholder pgx ConnConfig. Host/port/user/dbname will be
	// overridden via SetXxx() before stdlib.RegisterConnConfig consumes it.
	// We register a minimal config; the caller patches it before opening.
	connCfg, err := pgx.ParseConfig("postgres://placeholder@127.0.0.1:5432/postgres?sslmode=disable")
	if err != nil {
		_ = client.Close()
		return "", nil, fmt.Errorf("pgx parse placeholder: %w", err)
	}
	connCfg.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		return sshDial(ctx, client, address)
	}

	name := stdlib.RegisterConnConfig(connCfg)
	pgSSHRegistry.put(name, &pgSSHEntry{client: client, connCfg: connCfg})
	return name, func() { closePGSSH(name) }, nil
}

func sshDial(ctx context.Context, client *ssh.Client, address string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := client.Dial("tcp", address)
		ch <- result{conn: c, err: err}
	}()
	select {
	case r := <-ch:
		return r.conn, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func dialSSH(cfg db.SSHConfig) (*ssh.Client, error) {
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	var authMethod ssh.AuthMethod
	if cfg.PrivateKey != "" {
		var signer ssh.Signer
		var err error
		if cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(cfg.PrivateKey), []byte(cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(cfg.PrivateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("ssh key parse: %w", err)
		}
		authMethod = ssh.PublicKeys(signer)
	} else {
		authMethod = ssh.Password(cfg.Password)
	}
	clientCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return client, nil
}

type pgSSHEntry struct {
	client  *ssh.Client
	connCfg *pgx.ConnConfig
}

type pgSSHRegistryT struct {
	mu sync.Mutex
	m  map[string]*pgSSHEntry
}

func (r *pgSSHRegistryT) put(name string, e *pgSSHEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[name] = e
}

func (r *pgSSHRegistryT) take(name string) *pgSSHEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.m[name]
	delete(r.m, name)
	return e
}

var pgSSHRegistry = &pgSSHRegistryT{m: map[string]*pgSSHEntry{}}
var pgSSHCounter uint64

func closePGSSH(name string) {
	e := pgSSHRegistry.take(name)
	if e == nil {
		return
	}
	stdlib.UnregisterConnConfig(name)
	_ = e.client.Close()
}

// Unused helper — silences unused warning for atomic counter pattern if we
// ever switch to deterministic name generation. Keep it on hand for tests.
func nextPGSSHCounter() uint64 {
	return atomic.AddUint64(&pgSSHCounter, 1)
}
```

**CRITICAL caveat:** pgx's `RegisterConnConfig` returns a full DSN; the pool currently does `cacheKey := engine + "\x00" + BuildDSN(in) + sshFingerprint`. When SSH is used, the pool sets `in.Network = name` and re-calls `BuildDSN`. For PG, this means we must change `BuildDSN` to detect when `in.Network` is non-empty and **return the network value verbatim** (it's already a fully-formed pgx DSN reference). Update `dsn.go`:

```go
func (PG) BuildDSN(in db.DSNInput) string {
	if in.Network != "" {
		// in.Network is a pre-registered pgx DSN reference from RegisterSSHDialer.
		// Returning it verbatim short-circuits the normal DSN composition; the
		// connection details (host/port/user/dbname) are already on the
		// registered config and pgx will use them directly.
		return in.Network
	}
	// existing key=value composition...
}
```

The trade-off: SSH-tunneled PG connections lose visibility into host/port/user/dbname at the pool's cacheKey level. Mitigation: the pool already adds `sshFingerprint(ssh)` to the cacheKey, so distinct SSH targets stay distinct.

Additional adjustment: the pgx config registered in `RegisterSSHDialer` needs to be UPDATED with the real host/port/user/dbname/sslmode BEFORE being used. We expose a helper `PatchSSHConfig(name string, in db.DSNInput)` that the pool calls right before `cfg.Open`. Adding this to `Dialect` is invasive; simpler: have the pool detect PG-with-SSH and call a non-interface method on `PG{}`. To avoid coupling the pool to specific dialect logic, instead register the config with the *real* connection details extracted from `in`. Refactor: change `RegisterSSHDialer` to take `db.DSNInput` plus `db.SSHConfig`.

That ergonomic conflict means `db.Dialect.RegisterSSHDialer` must accept BOTH the SSH config AND the DSNInput. Updating the interface signature is a breaking change for MySQL too. Update both:

```go
RegisterSSHDialer(SSHConfig, DSNInput) (name string, closer func(), err error)
```

MySQL ignores the DSNInput argument; PG uses it. Update the pool to pass both. Re-run the MySQL tests.

(If you want to keep the existing two-arg signature in `db.Dialect`, define a separate `SSHDialerWithDSN` optional interface for engines that need the DSNInput; the pool does a type assertion. Pick whichever is cleaner; the explicit DSNInput in the interface is simpler if you're OK with the MySQL no-op overload.)

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/db/...
go test ./...
```

- [ ] **Step 5: Commit**

Three commits in this task, since it spans multiple files:
```bash
git add internal/db/pg/ssh.go internal/db/pg/ssh_test.go
git commit -m "feat(db/pg): RegisterSSHDialer via pgx stdlib config registration"

git add internal/db/pg/dsn.go
git commit -m "feat(db/pg): BuildDSN passes through pre-registered SSH DSN reference"

# If interface change made:
git add internal/db/dialect.go internal/db/pool.go internal/db/mysql/ssh.go
git commit -m "refactor(db): RegisterSSHDialer accepts DSNInput for engine-specific routing"
```

---

## Phase 4: Schema discovery

### Task 5: ListDatabases / ListTables / ListSchemaColumns

**Files:**
- Create: `internal/db/pg/browse.go`

- [ ] **Step 1: Skeleton tests**

Create `internal/db/pg/browse_test.go` with a compile-time interface satisfaction check (similar to mysql's `schema_test.go`) — no live PG dependency.

- [ ] **Step 2: Implement**

Create `internal/db/pg/browse.go`:

```go
package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// ListDatabases returns PG schemas (not real databases). This mirrors the
// MySQL UI: the connection picks one PG database via DSN; "schemas" appear
// in the sidebar as the second level. System schemas (pg_catalog, pg_toast,
// information_schema) are filtered unless includeSystem is true.
func (PG) ListDatabases(ctx context.Context, sqlDB *sql.DB, includeSystem bool) ([]string, error) {
	var query string
	if includeSystem {
		query = `SELECT schema_name FROM information_schema.schemata ORDER BY schema_name`
	} else {
		query = `SELECT schema_name FROM information_schema.schemata
		         WHERE schema_name NOT IN ('pg_catalog','pg_toast','information_schema')
		         ORDER BY schema_name`
	}
	rows, err := sqlDB.QueryContext(ctx, query)
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

func (PG) ListTables(ctx context.Context, sqlDB *sql.DB, schema string) ([]db.TableInfo, error) {
	// row count via pg_class.reltuples (estimate); size via pg_total_relation_size.
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT c.relname,
		       COALESCE(c.reltuples::bigint, 0),
		       COALESCE(pg_total_relation_size(c.oid) / 1048576, 0)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relkind IN ('r','p')
		ORDER BY c.relname
	`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.TableInfo
	for rows.Next() {
		var t db.TableInfo
		if err := rows.Scan(&t.Name, &t.RowsEst, &t.SizeMB); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (PG) ListSchemaColumns(ctx context.Context, sqlDB *sql.DB, schema string) (map[string][]string, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = $1
		ORDER BY table_name, ordinal_position
	`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]string)
	for rows.Next() {
		var t, c string
		if err := rows.Scan(&t, &c); err != nil {
			return nil, err
		}
		out[t] = append(out[t], c)
	}
	return out, rows.Err()
}

// FetchTableRows uses $1, $2 ... placeholders, requires ORDER BY when no
// explicit sort is requested (PG accepts LIMIT without ORDER BY, but the
// result order is non-deterministic; we accept that for parity with MySQL).
func (PG) FetchTableRows(ctx context.Context, sqlDB *sql.DB, o db.RowsOpts) (db.RowsPage, error) {
	if o.Page < 1 {
		o.Page = 1
	}
	if o.PerPage < 1 {
		o.PerPage = 50
	}
	if o.PerPage > db.MaxRowsPerPage {
		o.PerPage = db.MaxRowsPerPage
	}
	offset := (o.Page - 1) * o.PerPage

	d := PG{}
	schema := d.QuoteIdent(o.Schema)
	table := d.QuoteIdent(o.Table)
	qualified := schema + "." + table

	whereClause, whereArgs := buildPGWhereClause(o.Filters)

	var total int64
	countArgs := append([]any{}, whereArgs...)
	if err := sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+qualified+whereClause, countArgs...).Scan(&total); err != nil {
		return db.RowsPage{}, err
	}

	orderBy := ""
	if o.SortCol != "" {
		dir := "ASC"
		if strings.ToLower(o.SortDir) == "desc" {
			dir = "DESC"
		}
		orderBy = " ORDER BY " + d.QuoteIdent(o.SortCol) + " " + dir
	}

	// Use $N placeholders. whereArgs already burned $1..$k, so LIMIT/OFFSET
	// take $k+1 and $k+2.
	k := len(whereArgs)
	queryArgs := append([]any{}, whereArgs...)
	queryArgs = append(queryArgs, o.PerPage, offset)
	rows, err := sqlDB.QueryContext(ctx, fmt.Sprintf(
		"SELECT * FROM %s%s%s LIMIT $%d OFFSET $%d",
		qualified, whereClause, orderBy, k+1, k+2,
	), queryArgs...)
	if err != nil {
		return db.RowsPage{}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return db.RowsPage{}, err
	}
	page := db.RowsPage{Columns: cols, Total: total, Page: o.Page, PerPage: o.PerPage}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return db.RowsPage{}, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		page.Rows = append(page.Rows, vals)
	}
	return page, rows.Err()
}

// buildPGWhereClause is the PG twin of MySQL's buildWhereClause but uses
// $1, $2... placeholders. The Filter contract is identical.
func buildPGWhereClause(filters []db.Filter) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}
	d := PG{}
	var conds []string
	var args []any
	param := 1
	next := func() string {
		s := fmt.Sprintf("$%d", param)
		param++
		return s
	}
	for _, f := range filters {
		if f.Column == "" {
			continue
		}
		col := d.QuoteIdent(f.Column)
		switch f.Operator {
		case "=", "<>", "<", ">", "<=", ">=":
			conds = append(conds, col+" "+f.Operator+" "+next())
			args = append(args, f.Value)
		case "LIKE":
			conds = append(conds, col+" LIKE "+next())
			args = append(args, f.Value)
		case "Contains":
			conds = append(conds, col+" LIKE "+next())
			args = append(args, "%"+f.Value+"%")
		case "Not contains":
			conds = append(conds, col+" NOT LIKE "+next())
			args = append(args, "%"+f.Value+"%")
		case "Has prefix":
			conds = append(conds, col+" LIKE "+next())
			args = append(args, f.Value+"%")
		case "Has suffix":
			conds = append(conds, col+" LIKE "+next())
			args = append(args, "%"+f.Value)
		case "IS NULL":
			conds = append(conds, col+" IS NULL")
		case "IS NOT NULL":
			conds = append(conds, col+" IS NOT NULL")
		case "IN", "NOT IN":
			parts := strings.Split(f.Value, ",")
			placeholders := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				placeholders = append(placeholders, next())
				args = append(args, p)
			}
			if len(placeholders) == 0 {
				continue
			}
			op := "IN"
			if f.Operator == "NOT IN" {
				op = "NOT IN"
			}
			conds = append(conds, col+" "+op+" ("+strings.Join(placeholders, ", ")+")")
		case "BETWEEN", "NOT BETWEEN":
			parts := strings.Split(f.Value, ",")
			if len(parts) != 2 {
				continue
			}
			op := "BETWEEN"
			if f.Operator == "NOT BETWEEN" {
				op = "NOT BETWEEN"
			}
			conds = append(conds, col+" "+op+" "+next()+" AND "+next())
			args = append(args, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}
```

- [ ] **Step 3: Build + test**

```bash
go build ./...
go test ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/pg/browse.go internal/db/pg/browse_test.go
git commit -m "feat(db/pg): ListDatabases/Tables/SchemaColumns/FetchTableRows"
```

### Task 6: DescribeTable, ListIndexes, ListForeignKeys

**Files:**
- Create: `internal/db/pg/schema.go`

PG has no `SHOW CREATE TABLE`. Synthesize one from `information_schema.columns` + `pg_indexes` + foreign-key constraints.

- [ ] **Step 1: Skeleton test**

```go
package pg

import (
	"context"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestPGImplementsSchemaMethods(t *testing.T) {
	var d db.Dialect = PG{}
	_ = d
	_ = func(ctx context.Context) {
		_, _ = d.ListDatabases(ctx, nil, false)
	}
}
```

- [ ] **Step 2: Implement**

Create `internal/db/pg/schema.go`:

```go
package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func (PG) DescribeTable(ctx context.Context, sqlDB *sql.DB, schema, table string) (db.Structure, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT column_name, udt_name,
		       is_nullable, COALESCE(column_default,''),
		       COALESCE('', ''),  -- "extra" — PG has no MySQL-equivalent
		       COALESCE(col_description((quote_ident($1)||'.'||quote_ident($2))::regclass, ordinal_position), '')
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position
	`, schema, table)
	if err != nil {
		return db.Structure{}, err
	}
	var cols []db.Column
	for rows.Next() {
		var c db.Column
		var nullable string
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &c.Default, &c.Extra, &c.Comment); err != nil {
			rows.Close()
			return db.Structure{}, err
		}
		c.Nullable = nullable == "YES"
		cols = append(cols, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return db.Structure{}, err
	}

	// Identify primary key columns; mark c.Key = "PRI".
	pkRows, err := sqlDB.QueryContext(ctx, `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = (quote_ident($1) || '.' || quote_ident($2))::regclass
		  AND i.indisprimary
	`, schema, table)
	if err != nil {
		return db.Structure{}, err
	}
	pkSet := map[string]bool{}
	for pkRows.Next() {
		var name string
		if err := pkRows.Scan(&name); err != nil {
			pkRows.Close()
			return db.Structure{}, err
		}
		pkSet[name] = true
	}
	pkRows.Close()
	for i := range cols {
		if pkSet[cols[i].Name] {
			cols[i].Key = "PRI"
		}
	}

	// Synthesize a CREATE TABLE.
	d := PG{}
	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE TABLE %s.%s (\n", d.QuoteIdent(schema), d.QuoteIdent(table))
	for i, c := range cols {
		fmt.Fprintf(&sb, "  %s %s", d.QuoteIdent(c.Name), c.Type)
		if !c.Nullable {
			sb.WriteString(" NOT NULL")
		}
		if c.Default != "" {
			fmt.Fprintf(&sb, " DEFAULT %s", c.Default)
		}
		if i < len(cols)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}
	if len(pkSet) > 0 {
		quoted := make([]string, 0, len(pkSet))
		for k := range pkSet {
			quoted = append(quoted, d.QuoteIdent(k))
		}
		fmt.Fprintf(&sb, ",  PRIMARY KEY (%s)\n", strings.Join(quoted, ", "))
	}
	sb.WriteString(");\n")

	return db.Structure{Columns: cols, CreateSQL: sb.String()}, nil
}

func (PG) ListIndexes(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.Index, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT i.relname AS index_name,
		       a.attname AS column_name,
		       idx.indisunique,
		       am.amname
		FROM pg_index idx
		JOIN pg_class i  ON i.oid = idx.indexrelid
		JOIN pg_class t  ON t.oid = idx.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_am am ON am.oid = i.relam
		JOIN pg_attribute a ON a.attrelid = idx.indrelid AND a.attnum = ANY(idx.indkey)
		WHERE n.nspname = $1 AND t.relname = $2
		ORDER BY i.relname, array_position(idx.indkey, a.attnum)
	`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type acc struct {
		Name      string
		Columns   []string
		Unique    bool
		IndexType string
	}
	var ordered []*acc
	byName := map[string]*acc{}
	for rows.Next() {
		var iname, cname, idxType string
		var unique bool
		if err := rows.Scan(&iname, &cname, &unique, &idxType); err != nil {
			return nil, err
		}
		e, ok := byName[iname]
		if !ok {
			e = &acc{Name: iname, Unique: unique, IndexType: strings.ToUpper(idxType)}
			byName[iname] = e
			ordered = append(ordered, e)
		}
		e.Columns = append(e.Columns, cname)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]db.Index, 0, len(ordered))
	for _, e := range ordered {
		out = append(out, db.Index{Name: e.Name, Columns: e.Columns, Unique: e.Unique, IndexType: e.IndexType})
	}
	return out, nil
}

func (PG) ListForeignKeys(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.ForeignKey, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT tc.constraint_name,
		       kcu.column_name,
		       ccu.table_name  AS ref_table,
		       ccu.column_name AS ref_column,
		       rc.delete_rule, rc.update_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON kcu.constraint_name = tc.constraint_name
		 AND kcu.table_schema    = tc.table_schema
		JOIN information_schema.referential_constraints rc
		  ON rc.constraint_name = tc.constraint_name
		 AND rc.constraint_schema = tc.constraint_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON ccu.constraint_name = tc.constraint_name
		 AND ccu.constraint_schema = tc.constraint_schema
		WHERE tc.table_schema = $1
		  AND tc.table_name   = $2
		  AND tc.constraint_type = 'FOREIGN KEY'
		ORDER BY tc.constraint_name, kcu.ordinal_position
	`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.ForeignKey
	for rows.Next() {
		var fk db.ForeignKey
		if err := rows.Scan(&fk.Name, &fk.Column, &fk.RefTable, &fk.RefColumn, &fk.OnDelete, &fk.OnUpdate); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: Build + test**

```bash
go build ./...
go test ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/pg/schema.go internal/db/pg/schema_test.go
git commit -m "feat(db/pg): DescribeTable/ListIndexes/ListForeignKeys"
```

---

## Phase 5: DML with RETURNING

### Task 7: PrimaryKey, UpdateCell, InsertRow, DeleteRow

**Files:**
- Create: `internal/db/pg/dml.go`
- Create: `internal/db/pg/dml_test.go`

The PG-specific bits:
- Placeholders are `$1, $2, ...` not `?`.
- `InsertRow` returns the inserted row's PK via `RETURNING <pkcol>`. If there is no auto-increment PK, return 0 (current MySQL behavior is `LastInsertId()` returning 0 in the same case).
- `PrimaryKey` queries `pg_index` (the same query used in DescribeTable above; share the helper).

- [ ] **Step 1: Failing test**

```go
package pg

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/conray/dataseai/internal/db"
)

func TestPGUpdateCellRejectsNoPK(t *testing.T) {
	// We can't easily simulate PG locally; verify the no-PK error path
	// with a fixture that yields an empty pkCols slice.
	d := PG{}
	dbh, _ := sql.Open("sqlite3", ":memory:")
	defer dbh.Close()
	_, err := d.UpdateCell(context.Background(), dbh, "public", "t", nil, nil, "x", "v")
	if !errors.Is(err, db.ErrNoPrimaryKey) {
		t.Fatalf("want ErrNoPrimaryKey, got %v", err)
	}
}

func TestPGDeleteRowRejectsNoPK(t *testing.T) {
	d := PG{}
	dbh, _ := sql.Open("sqlite3", ":memory:")
	defer dbh.Close()
	_, err := d.DeleteRow(context.Background(), dbh, "public", "t", nil, nil)
	if !errors.Is(err, db.ErrNoPrimaryKey) {
		t.Fatalf("want ErrNoPrimaryKey, got %v", err)
	}
}
```

- [ ] **Step 2: Implement**

Create `internal/db/pg/dml.go`:

```go
package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func (PG) PrimaryKey(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = (quote_ident($1) || '.' || quote_ident($2))::regclass
		  AND i.indisprimary
		ORDER BY array_position(i.indkey, a.attnum)
	`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func pgQualified(schema, table string) string {
	d := PG{}
	if schema == "" {
		return d.QuoteIdent(table)
	}
	return d.QuoteIdent(schema) + "." + d.QuoteIdent(table)
}

func pgWhereByPK(pkCols []string, pkVals []any, start int) (string, []any) {
	d := PG{}
	parts := make([]string, len(pkCols))
	args := make([]any, len(pkCols))
	for i, col := range pkCols {
		parts[i] = d.QuoteIdent(col) + " = $" + fmt.Sprintf("%d", start+i)
		args[i] = pkVals[i]
	}
	return strings.Join(parts, " AND "), args
}

func (PG) UpdateCell(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	d := PG{}
	// newVal is $1, then PK placeholders start at $2.
	where, args := pgWhereByPK(pkCols, pkVals, 2)
	sqlStr := "UPDATE " + pgQualified(schema, table) + " SET " + d.QuoteIdent(col) + " = $1 WHERE " + where
	res, err := sqlDB.ExecContext(ctx, sqlStr, append([]any{coerceValue(newVal)}, args...)...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (PG) InsertRow(ctx context.Context, sqlDB *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
	if len(cols) == 0 || len(cols) != len(vals) {
		return 0, errors.New("cols/vals empty or mismatched")
	}
	d := PG{}
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = d.QuoteIdent(col)
		placeholders[i] = "$" + fmt.Sprintf("%d", i+1)
	}
	// Find a PK we can RETURN.
	pkCols, _ := PG{}.PrimaryKey(ctx, sqlDB, schema, table)
	sqlStr := "INSERT INTO " + pgQualified(schema, table) +
		" (" + strings.Join(quotedCols, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	if len(pkCols) == 1 {
		sqlStr += " RETURNING " + d.QuoteIdent(pkCols[0])
		var id int64
		err := sqlDB.QueryRowContext(ctx, sqlStr, coerceValues(vals)...).Scan(&id)
		if err != nil {
			return 0, err
		}
		return id, nil
	}
	// Composite or no PK — execute without returning, last-insert-id is 0.
	if _, err := sqlDB.ExecContext(ctx, sqlStr, coerceValues(vals)...); err != nil {
		return 0, err
	}
	return 0, nil
}

func (PG) DeleteRow(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where, args := pgWhereByPK(pkCols, pkVals, 1)
	res, err := sqlDB.ExecContext(ctx, "DELETE FROM "+pgQualified(schema, table)+" WHERE "+where, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// coerceValue / coerceValues — share with the MySQL implementation's
// ISO-datetime coercion. The original is package-private to internal/db/mysql,
// so copy the logic here. Future cleanup: hoist into internal/db/internal/.
func coerceValue(v any) any {
	// (copy from internal/db/mysql/dml.go)
	// ...
	return v
}

func coerceValues(vs []any) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = coerceValue(v)
	}
	return out
}
```

Actually copy the full `coerceValue`/`coerceValues` body from `internal/db/mysql/dml.go`. They produce strings PG also accepts for `timestamp`/`timestamptz` columns. Future refactor: hoist to `internal/db/internal/datetime` or similar — not in this plan.

- [ ] **Step 3: Tests pass**

```bash
go test ./internal/db/pg/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/pg/dml.go internal/db/pg/dml_test.go
git commit -m "feat(db/pg): DML with $N placeholders and INSERT...RETURNING"
```

---

## Phase 6: KILL and ConnectionID

### Task 8: pg_cancel_backend + pg_backend_pid

**Files:**
- Create: `internal/db/pg/kill.go`
- Create: `internal/db/pg/kill_test.go`

- [ ] **Step 1: Failing test**

```go
package pg

import "testing"

func TestPGConnectionIDQuery(t *testing.T) {
	if got := (PG{}).ConnectionIDQuery(); got != "SELECT pg_backend_pid()" {
		t.Fatalf("ConnectionIDQuery = %q", got)
	}
}
```

- [ ] **Step 2: Implement**

```go
package pg

import (
	"context"
	"database/sql"
)

// KillQuery cancels the server-side query running on the given PG backend pid.
// PG uses pg_cancel_backend(pid) to abort a running statement while keeping
// the session alive — analogous to MySQL's KILL QUERY.
func (PG) KillQuery(ctx context.Context, sqlDB *sql.DB, connID int64) error {
	_, err := sqlDB.ExecContext(ctx, "SELECT pg_cancel_backend($1)", connID)
	return err
}

func (PG) ConnectionIDQuery() string { return "SELECT pg_backend_pid()" }
```

- [ ] **Step 3: Test**

```bash
go test ./internal/db/pg/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/pg/kill.go internal/db/pg/kill_test.go
git commit -m "feat(db/pg): KillQuery via pg_cancel_backend + ConnectionIDQuery"
```

---

## Phase 7: Stream/query/export/import (non-Dialect helpers)

### Task 9: PG-flavored helpers

**Files:**
- Create: `internal/db/pg/query.go`
- Create: `internal/db/pg/stream.go`
- Create: `internal/db/pg/export.go`
- Create: `internal/db/pg/import.go`
- Create: `internal/db/pg/executor.go`
- Create: `internal/db/pg/*_test.go`

These are NOT part of `db.Dialect`. They live in the pg package so callers that need engine-aware streaming can switch on engine.

- [ ] **Step 1: Read the MySQL equivalents**

Read all of:
- `internal/db/mysql/query.go`
- `internal/db/mysql/stream.go`
- `internal/db/mysql/export.go`
- `internal/db/mysql/import.go`
- `internal/db/mysql/executor.go`

These contain `Run`, `RunOpts`, `ExecResult`, `StreamQuery`, `StreamOpts`, `StreamSink`, `ExportCSV`, `ExportSQL`, `ImportCSV`, `Executor` interface + `DirectExecutor`.

- [ ] **Step 2: Port to PG**

Most of these files do not contain MySQL-specific SQL — they just stream rows through `database/sql`. The differences:
- `query.go`: `Classify(sql)` calls a MySQL-flavored classifier returning `StmtSelect`/`StmtExec`. The PG version needs the same shape but should detect PG-specific verbs (`WITH RECURSIVE`, `EXPLAIN`, `COPY`). Port the function and adapt the verb table.
- `export.go`: `ExportCSV` and `ExportSQL`. The SQL form needs PG syntax: `INSERT INTO ... VALUES (...);` is portable; the dump style stays the same. For `ExportSQL`, omit the `LOCK TABLES` / MySQL-specific preludes.
- `import.go`: `ImportCSV` is engine-agnostic except for the INSERT statement; double-check placeholder style ($N).
- `executor.go`: `Executor` interface unchanged; `DirectExecutor.Run` delegates to `Run`. Port verbatim.

Each ported file:
1. Change `package mysql` → `package pg`.
2. Inside, replace `MySQL{}.QuoteIdent` with `PG{}.QuoteIdent`.
3. Replace `?` placeholders with PG's positional form.
4. Tests: port with the same swaps.

- [ ] **Step 3: Tests pass**

```bash
go test ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/pg/
git commit -m "feat(db/pg): query/stream/export/import helpers"
```

---

## Phase 8: API + main wiring

### Task 10: Register PG dialect, validate engine, dispatch

**Files:**
- Modify: `cmd/dataseai/main.go`
- Modify: `internal/api/connections.go`
- Modify: `internal/api/connections_test.go`

- [ ] **Step 1: Add blank import**

In `cmd/dataseai/main.go`, alongside the existing mysql dialect blank import:
```go
_ "github.com/conray/dataseai/internal/db/pg"
```

- [ ] **Step 2: Whitelist engine**

In `internal/api/connections.go`:
```go
var allowedEngines = map[string]bool{
    "mysql":    true,
    "postgres": true,
}
```

- [ ] **Step 3: Test**

In `internal/api/connections_test.go`, add:
```go
func TestCreateConnectionAcceptsPostgres(t *testing.T) {
    // construct a connection req with engine: "postgres"; expect 200
    // ...
}
```

If existing rejection test rejects `"postgres"`, update it to a new unsupported engine name (e.g. `"oracle"`).

- [ ] **Step 4: Run all**

```bash
go test ./...
```

- [ ] **Step 5: Commit**

```bash
git add cmd/dataseai/main.go internal/api/connections.go internal/api/connections_test.go
git commit -m "feat(api): register and accept postgres engine"
```

---

## Phase 9: Frontend selector

### Task 11: Connection form engine dropdown

**Files:**
- Modify: `web/src/store/connections.ts`
- Modify: `web/src/components/ConnectionDialog.tsx`
- Modify: `web/src/components/ConnectionsManager.tsx`
- Modify: `web/src/i18n/messages.ts`
- Modify: `web/src/components/ConnectionDialog.test.tsx`
- Modify: `web/src/store/connections.test.ts`

- [ ] **Step 1: Extend type**

In `connections.ts`:
```ts
export type ConnectionEngine = "mysql" | "postgres";
```

- [ ] **Step 2: Form dropdown**

In `ConnectionDialog.tsx`, replace the read-only badge with a `<select>`:
```tsx
<div className="form-row">
  <label htmlFor="engine">{t('connection_dialog.engine')}</label>
  <select id="engine" value={engine} onChange={e => setEngine(e.target.value as ConnectionEngine)}>
    <option value="mysql">{t('engine_mysql')}</option>
    <option value="postgres">{t('engine_postgres')}</option>
  </select>
</div>
```

When engine changes, also set the default port (3306 for mysql, 5432 for postgres) unless the user has already typed one.

- [ ] **Step 3: Card label**

In `ConnectionsManager.tsx` extend `engineLabel`:
```ts
function engineLabel(engine: ConnectionEngine): string {
  switch (engine) {
    case 'mysql': return 'MySQL';
    case 'postgres': return 'PostgreSQL';
  }
}
```

- [ ] **Step 4: i18n**

Add `engine_postgres: 'PostgreSQL'` (en) and `engine_postgres: 'PostgreSQL'` (zh-TW — proper noun, same string) to `messages.ts`.

- [ ] **Step 5: Tests**

Update `ConnectionDialog.test.tsx` to assert the selector renders two options.

- [ ] **Step 6: Build**

```bash
cd web && npm run build && npm test -- --run
```

- [ ] **Step 7: Commit**

```bash
git add web/
git commit -m "feat(web): engine selector with MySQL and PostgreSQL options"
```

---

## Phase 10: Docs + PR

### Task 12: Test env update + PR

**Files:**
- Create: `docs/test-env/postgres-dialect.md`
- Modify: `docs/test-env/dialect-abstraction.md` (cross-link)

- [ ] **Step 1: Write docs**

`docs/test-env/postgres-dialect.md`:
```markdown
# Test Environment — feat/db-pg-dialect

Branch under test: `feat/db-pg-dialect`

## What changed
- `internal/db/pg/` adds the PostgreSQL implementation of `db.Dialect`.
- `engine` whitelist now accepts `"postgres"`.
- Connection form has a Engine dropdown (MySQL / PostgreSQL).
- Connector binary is unaffected — `via_agent` PG connections are rejected.

## Verification checklist
- [ ] `go test ./...` green.
- [ ] `cd web && npm test -- --run` green.
- [ ] Connect to a staging PG instance (no SSH), list schemas as the "databases" sidebar, browse a table.
- [ ] SSH-tunneled PG connect from the test VM.
- [ ] Chat orchestrator: propose + execute a SELECT against PG.
- [ ] DML: insert via the row editor; insert returns the new id; update and delete work.
- [ ] Kill query: start a slow `pg_sleep(60)`, hit kill, verify cancellation.

## Known gaps (deferred)
- Composite-PK INSERT path returns 0 (parity with MySQL no-PK behavior).
- `ExportSQL` does not preserve PG-specific column comments / extensions.
- PG-via-connector: separate plan.

## Rollback
- Revert merge. Schema is unchanged (no migration in this plan).
```

- [ ] **Step 2: Commit**

```bash
git add docs/test-env/postgres-dialect.md
git commit -m "docs(test-env): document postgres dialect test branch"
```

- [ ] **Step 3: Push + open PR**

```bash
git push -u origin feat/db-pg-dialect
gh pr create --draft --base feat/db-dialect-abstraction --title "feat(db/pg): PostgreSQL dialect implementation" --body "$(cat <<'EOF'
## Summary
- Add `internal/db/pg/` implementing `db.Dialect` for PostgreSQL via `jackc/pgx/v5/stdlib`.
- Engine whitelist accepts `postgres`; frontend gets an Engine dropdown.
- Schema-discovery treats PG schemas as the "databases" level so the UI stays consistent with MySQL.
- No connector changes — via-agent PG is rejected.

## Test plan
- [ ] `go test ./...` green locally.
- [ ] Manual: connect to staging PG (with + without SSH), browse schemas/tables/rows, INSERT/UPDATE/DELETE, KILL.
- [ ] `npm test` + `npm run build` green.

## Out of scope
- PG via connector binary.
- MSSQL (next plan).
- Postgres-only features (range types, JSONB editor, etc).
EOF
)"
```

---

## Self-Review Checklist

- [ ] Every PG method on `db.Dialect` is implemented (20 methods total).
- [ ] PG's `BuildDSN` passes through `in.Network` verbatim when non-empty (SSH path).
- [ ] `Pool` continues to work for both MySQL and PG without modification, except for the `RegisterSSHDialer` signature change (if adopted — verify the call site in `pool.go`).
- [ ] Frontend dropdown defaults port based on engine.
- [ ] `pg_cancel_backend` returns void; check the dialect's KillQuery doesn't choke on the empty result.
- [ ] No magic numbers (3306, 5432 ports — keep them in the engine defaults map on the frontend or in dialect-level helpers).
- [ ] Migration: none required for this plan (the engine column already exists).
- [ ] Connector binary: no changes; via-agent PG explicitly returns HTTP 400 in the API.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-09-postgres-dialect.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — Dispatch a fresh subagent per task; review the diff between tasks; fast iteration.

**2. Inline Execution** — Execute tasks in this session using `superpowers:executing-plans`; batch execution with checkpoints for review.

Which approach?
