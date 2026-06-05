# AI Write Permissions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users opt in per (connection, database, table, operation) for AI-driven INSERT/UPDATE/DELETE/TRUNCATE/ALTER/RENAME via a `propose_write` tool that previews every write before user clicks Execute.

**Architecture:** New SQLite tables (`ai_write_policy`, `ai_write_audit`) and a master switch column on `users`. New Go packages `internal/mysql/sqlclass` and `internal/policy`. Chat orchestrator gains a `ProposalGateway` so it can block a `propose_write` tool call until the WS hands a user decision back. `run_sql`/`mysql_query` hardened to read-only via classifier. New HTTP endpoints under `/api/auth/ai-writes`, `/api/auth/ai-policy`, `/api/auth/ai-audit`. New Settings section and chat proposal card on the frontend.

**Tech Stack:** Go 1.22 (chi, mattn/go-sqlite3, coder/websocket), React 18 + TS + Vite + Vitest + Zustand, i18n module already in place.

**Spec:** `docs/superpowers/specs/2026-06-05-ai-write-permissions-design.md`

---

## Notes for Implementers

- Project root: `/home/conray/project/mysqlweb`. Binary is `bin/dataseai`. Default DB path is `./data/dataseai.db` (NOT `mysqlweb.db`).
- Frontend embedded via `//go:embed web/dist` in `embed.go`; run `cd web && npm run build` then `go build -o bin/dataseai ./cmd/dataseai` to ship a UI change.
- Restart command: `cd /home/conray/project/mysqlweb && kill $(cat .mysqlweb.pid) 2>/dev/null; sleep 1; setsid env MYSQLWEB_DB_PATH=./data/dataseai.db MYSQLWEB_PORT=53306 ./bin/dataseai > logs/mysqlweb.log 2>&1 < /dev/null & disown; sleep 1; pgrep -nf './bin/dataseai' > .mysqlweb.pid`
- Existing test DB driver in Go tests is `_ "github.com/mattn/go-sqlite3"` with in-memory DSN `:memory:`.

---

## Task 1: Migration 0009 — master switch + policy + audit tables

**Files:**
- Create: `internal/store/migrations/0009_ai_writes.sql`
- Test: `internal/store/migrate_test.go` (extend existing)

- [ ] **Step 1: Inspect existing migrate test pattern**

Run: `head -40 /home/conray/project/mysqlweb/internal/store/migrate_test.go`

- [ ] **Step 2: Write the migration**

Create `internal/store/migrations/0009_ai_writes.sql`:

```sql
-- AI chat write permissions: per-user master switch + per-table policy + audit log.
ALTER TABLE users ADD COLUMN ai_writes_enabled INTEGER NOT NULL DEFAULT 0;

CREATE TABLE ai_write_policy (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL,
  connection_id   INTEGER NOT NULL,
  database_name   TEXT    NOT NULL,
  table_name      TEXT    NOT NULL,
  allow_insert    INTEGER NOT NULL DEFAULT 0,
  allow_update    INTEGER NOT NULL DEFAULT 0,
  allow_delete    INTEGER NOT NULL DEFAULT 0,
  allow_ddl       INTEGER NOT NULL DEFAULT 0,
  updated_at      DATETIME NOT NULL,
  UNIQUE(user_id, connection_id, database_name, table_name),
  FOREIGN KEY (user_id)       REFERENCES users(id)       ON DELETE CASCADE,
  FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE
);

CREATE INDEX idx_ai_write_policy_user_conn ON ai_write_policy(user_id, connection_id);

CREATE TABLE ai_write_audit (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL,
  connection_id   INTEGER NOT NULL,
  database_name   TEXT    NOT NULL,
  table_name      TEXT    NOT NULL,
  operation       TEXT    NOT NULL,
  sql_text        TEXT    NOT NULL,
  status          TEXT    NOT NULL,
  rows_affected   INTEGER,
  error_message   TEXT,
  explain_summary TEXT,
  created_at      DATETIME NOT NULL,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_ai_write_audit_user ON ai_write_audit(user_id, created_at DESC);
```

- [ ] **Step 3: Write a migration test that verifies all three objects exist after Migrate**

Append to `internal/store/migrate_test.go`:

```go
func TestMigrate0009AIWrites(t *testing.T) {
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil { t.Fatal(err) }
    defer db.Close()
    if err := Migrate(db); err != nil { t.Fatal(err) }

    // users.ai_writes_enabled exists
    var cnt int
    if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('users') WHERE name='ai_writes_enabled'`).Scan(&cnt); err != nil {
        t.Fatal(err)
    }
    if cnt != 1 { t.Fatalf("ai_writes_enabled column missing, got %d", cnt) }

    // policy + audit tables present
    for _, tbl := range []string{"ai_write_policy", "ai_write_audit"} {
        var name string
        err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
        if err != nil { t.Fatalf("table %s missing: %v", tbl, err) }
    }
}
```

- [ ] **Step 4: Run the test, expect PASS**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/store/ -run TestMigrate0009 -v`
Expected: `--- PASS: TestMigrate0009AIWrites`

- [ ] **Step 5: Commit**

```bash
git add internal/store/migrations/0009_ai_writes.sql internal/store/migrate_test.go
git -c commit.gpgsign=false commit -m "feat(store): add migration for ai write policy + audit tables"
```

---

## Task 2: Store layer — master switch on users

**Files:**
- Modify: `internal/store/users.go`
- Test: `internal/store/users_test.go`

- [ ] **Step 1: Write test for GetAIWritesEnabled / SetAIWritesEnabled**

Append to `internal/store/users_test.go`:

```go
func TestAIWritesEnabledRoundTrip(t *testing.T) {
    s := newTestStore(t)  // reuse existing helper that runs Migrate
    u, err := s.CreateUser("alice", "longpassword1")
    if err != nil { t.Fatal(err) }

    enabled, err := s.GetAIWritesEnabled(u.ID)
    if err != nil { t.Fatal(err) }
    if enabled { t.Fatal("expected default false") }

    if err := s.SetAIWritesEnabled(u.ID, true); err != nil { t.Fatal(err) }
    enabled, _ = s.GetAIWritesEnabled(u.ID)
    if !enabled { t.Fatal("expected true after set") }

    if err := s.SetAIWritesEnabled(u.ID, false); err != nil { t.Fatal(err) }
    enabled, _ = s.GetAIWritesEnabled(u.ID)
    if enabled { t.Fatal("expected false after clear") }
}
```

- [ ] **Step 2: Run test, expect FAIL (methods undefined)**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/store/ -run TestAIWritesEnabledRoundTrip -v`
Expected: compile error or undefined.

- [ ] **Step 3: Implement methods in `internal/store/users.go`**

Append:

```go
// GetAIWritesEnabled returns the user's master AI-writes opt-in flag.
func (s *Store) GetAIWritesEnabled(userID int64) (bool, error) {
    var v int
    err := s.DB.QueryRow("SELECT COALESCE(ai_writes_enabled, 0) FROM users WHERE id=?", userID).Scan(&v)
    if err != nil { return false, err }
    return v == 1, nil
}

// SetAIWritesEnabled toggles the master AI-writes opt-in flag.
func (s *Store) SetAIWritesEnabled(userID int64, enabled bool) error {
    v := 0
    if enabled { v = 1 }
    _, err := s.DB.Exec("UPDATE users SET ai_writes_enabled=? WHERE id=?", v, userID)
    return err
}
```

- [ ] **Step 4: Run test, expect PASS**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/store/ -run TestAIWritesEnabledRoundTrip -v`

- [ ] **Step 5: Commit**

```bash
git add internal/store/users.go internal/store/users_test.go
git -c commit.gpgsign=false commit -m "feat(store): add GetAIWritesEnabled / SetAIWritesEnabled"
```

---

## Task 3: Store layer — policy CRUD

**Files:**
- Create: `internal/store/ai_policy.go`
- Test: `internal/store/ai_policy_test.go`

- [ ] **Step 1: Write tests for the full CRUD surface**

Create `internal/store/ai_policy_test.go`:

```go
package store

import (
    "testing"
)

func TestAIPolicySetGet(t *testing.T) {
    s := newTestStore(t)
    u, _ := s.CreateUser("alice", "longpassword1")

    // Missing row → zero Policy + (false, nil) sentinel
    p, found, err := s.GetAIPolicy(u.ID, 1, "db1", "t1")
    if err != nil { t.Fatal(err) }
    if found { t.Fatal("expected !found for missing row") }
    if (p != AIPolicy{}) { t.Fatalf("expected zero value, got %+v", p) }

    if err := s.UpsertAIPolicy(u.ID, 1, "db1", "t1", AIPolicy{Insert: true, Delete: true}); err != nil {
        t.Fatal(err)
    }
    p, found, _ = s.GetAIPolicy(u.ID, 1, "db1", "t1")
    if !found { t.Fatal("expected found after upsert") }
    if !p.Insert || p.Update || !p.Delete || p.DDL {
        t.Fatalf("unexpected policy: %+v", p)
    }

    // Update existing
    if err := s.UpsertAIPolicy(u.ID, 1, "db1", "t1", AIPolicy{Insert: true, Update: true, Delete: true, DDL: true}); err != nil {
        t.Fatal(err)
    }
    p, _, _ = s.GetAIPolicy(u.ID, 1, "db1", "t1")
    if !p.Insert || !p.Update || !p.Delete || !p.DDL { t.Fatalf("upsert didn't update: %+v", p) }
}

func TestAIPolicyBatchAndList(t *testing.T) {
    s := newTestStore(t)
    u, _ := s.CreateUser("alice", "longpassword1")

    err := s.BatchUpsertAIPolicy(u.ID, 1, "db1", []string{"t1", "t2", "t3"}, AIPolicy{Insert: true})
    if err != nil { t.Fatal(err) }

    rows, err := s.ListAIPolicy(u.ID, 1, "db1")
    if err != nil { t.Fatal(err) }
    if len(rows) != 3 { t.Fatalf("expected 3, got %d", len(rows)) }
    for _, r := range rows {
        if !r.Policy.Insert { t.Fatalf("%s missing Insert", r.Table) }
    }
}

func TestAIPolicyDelete(t *testing.T) {
    s := newTestStore(t)
    u, _ := s.CreateUser("alice", "longpassword1")
    _ = s.UpsertAIPolicy(u.ID, 1, "db1", "t1", AIPolicy{Insert: true})

    if err := s.DeleteAIPolicy(u.ID, 1, "db1", "t1"); err != nil { t.Fatal(err) }
    _, found, _ := s.GetAIPolicy(u.ID, 1, "db1", "t1")
    if found { t.Fatal("expected not found after delete") }
}

func TestAIPolicyCrossUserIsolation(t *testing.T) {
    s := newTestStore(t)
    a, _ := s.CreateUser("alice", "longpassword1")
    b, _ := s.CreateUser("bob",   "longpassword2")
    _ = s.UpsertAIPolicy(a.ID, 1, "db1", "t1", AIPolicy{Insert: true})

    _, found, _ := s.GetAIPolicy(b.ID, 1, "db1", "t1")
    if found { t.Fatal("alice's policy leaked to bob") }
}
```

- [ ] **Step 2: Run tests, expect FAIL**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/store/ -run TestAIPolicy -v`
Expected: compile error.

- [ ] **Step 3: Create the package file**

Create `internal/store/ai_policy.go`:

```go
package store

import (
    "database/sql"
    "errors"
    "time"
)

type AIPolicy struct {
    Insert bool `json:"insert"`
    Update bool `json:"update"`
    Delete bool `json:"delete"`
    DDL    bool `json:"ddl"`
}

type AITablePolicy struct {
    Table  string   `json:"table"`
    Policy AIPolicy `json:"policy"`
}

func boolI(b bool) int {
    if b { return 1 }
    return 0
}

// GetAIPolicy returns (policy, found, err). When not found, policy is zero.
func (s *Store) GetAIPolicy(userID, connID int64, db, table string) (AIPolicy, bool, error) {
    var p AIPolicy
    var ai, au, ad, addl int
    row := s.DB.QueryRow(`
        SELECT allow_insert, allow_update, allow_delete, allow_ddl
          FROM ai_write_policy
         WHERE user_id=? AND connection_id=? AND database_name=? AND table_name=?`,
        userID, connID, db, table)
    if err := row.Scan(&ai, &au, &ad, &addl); err != nil {
        if errors.Is(err, sql.ErrNoRows) { return p, false, nil }
        return p, false, err
    }
    p.Insert = ai == 1
    p.Update = au == 1
    p.Delete = ad == 1
    p.DDL    = addl == 1
    return p, true, nil
}

// UpsertAIPolicy inserts or updates a single (user, conn, db, table) policy.
func (s *Store) UpsertAIPolicy(userID, connID int64, db, table string, p AIPolicy) error {
    _, err := s.DB.Exec(`
        INSERT INTO ai_write_policy (user_id, connection_id, database_name, table_name,
            allow_insert, allow_update, allow_delete, allow_ddl, updated_at)
        VALUES (?,?,?,?,?,?,?,?,?)
        ON CONFLICT(user_id, connection_id, database_name, table_name) DO UPDATE SET
            allow_insert = excluded.allow_insert,
            allow_update = excluded.allow_update,
            allow_delete = excluded.allow_delete,
            allow_ddl    = excluded.allow_ddl,
            updated_at   = excluded.updated_at`,
        userID, connID, db, table,
        boolI(p.Insert), boolI(p.Update), boolI(p.Delete), boolI(p.DDL),
        time.Now().UTC().Format(time.RFC3339Nano),
    )
    return err
}

// BatchUpsertAIPolicy applies the same policy to a set of tables in a transaction.
func (s *Store) BatchUpsertAIPolicy(userID, connID int64, db string, tables []string, p AIPolicy) error {
    tx, err := s.DB.Begin()
    if err != nil { return err }
    defer tx.Rollback()
    stmt, err := tx.Prepare(`
        INSERT INTO ai_write_policy (user_id, connection_id, database_name, table_name,
            allow_insert, allow_update, allow_delete, allow_ddl, updated_at)
        VALUES (?,?,?,?,?,?,?,?,?)
        ON CONFLICT(user_id, connection_id, database_name, table_name) DO UPDATE SET
            allow_insert = excluded.allow_insert,
            allow_update = excluded.allow_update,
            allow_delete = excluded.allow_delete,
            allow_ddl    = excluded.allow_ddl,
            updated_at   = excluded.updated_at`)
    if err != nil { return err }
    defer stmt.Close()
    now := time.Now().UTC().Format(time.RFC3339Nano)
    for _, t := range tables {
        if _, err := stmt.Exec(userID, connID, db, t,
            boolI(p.Insert), boolI(p.Update), boolI(p.Delete), boolI(p.DDL), now); err != nil {
            return err
        }
    }
    return tx.Commit()
}

// ListAIPolicy returns every configured row for (user, conn, db).
func (s *Store) ListAIPolicy(userID, connID int64, db string) ([]AITablePolicy, error) {
    rows, err := s.DB.Query(`
        SELECT table_name, allow_insert, allow_update, allow_delete, allow_ddl
          FROM ai_write_policy
         WHERE user_id=? AND connection_id=? AND database_name=?
         ORDER BY table_name`,
        userID, connID, db)
    if err != nil { return nil, err }
    defer rows.Close()
    var out []AITablePolicy
    for rows.Next() {
        var tp AITablePolicy
        var ai, au, ad, addl int
        if err := rows.Scan(&tp.Table, &ai, &au, &ad, &addl); err != nil {
            return nil, err
        }
        tp.Policy = AIPolicy{Insert: ai == 1, Update: au == 1, Delete: ad == 1, DDL: addl == 1}
        out = append(out, tp)
    }
    return out, rows.Err()
}

// DeleteAIPolicy removes a single (user, conn, db, table) row.
func (s *Store) DeleteAIPolicy(userID, connID int64, db, table string) error {
    _, err := s.DB.Exec(`DELETE FROM ai_write_policy
        WHERE user_id=? AND connection_id=? AND database_name=? AND table_name=?`,
        userID, connID, db, table)
    return err
}
```

- [ ] **Step 4: Run tests, expect PASS**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/store/ -run TestAIPolicy -v`
Expected: 4 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/ai_policy.go internal/store/ai_policy_test.go
git -c commit.gpgsign=false commit -m "feat(store): CRUD + batch helpers for ai_write_policy"
```

---

## Task 4: Store layer — audit log

**Files:**
- Create: `internal/store/ai_audit.go`
- Test: `internal/store/ai_audit_test.go`

- [ ] **Step 1: Write tests**

Create `internal/store/ai_audit_test.go`:

```go
package store

import (
    "testing"
)

func TestAIAuditWriteAndTransition(t *testing.T) {
    s := newTestStore(t)
    u, _ := s.CreateUser("alice", "longpassword1")

    id, err := s.WriteAIAudit(AIAuditRow{
        UserID: u.ID, ConnectionID: 1, Database: "db1", Table: "t1",
        Operation: "INSERT", SQL: "INSERT INTO t1 VALUES (1)",
        Status: "proposed", ExplainSummary: `{"rows":1}`,
    })
    if err != nil { t.Fatal(err) }
    if id == 0 { t.Fatal("expected id") }

    n := int64(3)
    if err := s.UpdateAIAuditStatus(id, "executed", &n, ""); err != nil { t.Fatal(err) }

    rows, err := s.RecentAIAudit(u.ID, 10)
    if err != nil { t.Fatal(err) }
    if len(rows) != 1 { t.Fatalf("expected 1 row, got %d", len(rows)) }
    if rows[0].Status != "executed" { t.Fatalf("status=%s", rows[0].Status) }
    if rows[0].RowsAffected == nil || *rows[0].RowsAffected != 3 {
        t.Fatalf("rows_affected mismatch: %v", rows[0].RowsAffected)
    }
}

func TestAIAuditUserIsolation(t *testing.T) {
    s := newTestStore(t)
    a, _ := s.CreateUser("alice", "longpassword1")
    b, _ := s.CreateUser("bob",   "longpassword2")
    _, _ = s.WriteAIAudit(AIAuditRow{UserID: a.ID, ConnectionID: 1, Database: "d", Table: "t", Operation: "INSERT", SQL: "x", Status: "proposed"})
    rows, _ := s.RecentAIAudit(b.ID, 10)
    if len(rows) != 0 { t.Fatalf("bob sees alice's row(s)") }
}
```

- [ ] **Step 2: Run tests, expect FAIL**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/store/ -run TestAIAudit -v`

- [ ] **Step 3: Implement**

Create `internal/store/ai_audit.go`:

```go
package store

import (
    "database/sql"
    "time"
)

type AIAuditRow struct {
    ID             int64
    UserID         int64
    ConnectionID   int64
    Database       string
    Table          string
    Operation      string
    SQL            string
    Status         string  // proposed|executed|denied|cancelled|failed
    RowsAffected   *int64
    ErrorMessage   string
    ExplainSummary string
    CreatedAt      time.Time
}

func (s *Store) WriteAIAudit(r AIAuditRow) (int64, error) {
    now := time.Now().UTC()
    res, err := s.DB.Exec(`
        INSERT INTO ai_write_audit
            (user_id, connection_id, database_name, table_name, operation,
             sql_text, status, rows_affected, error_message, explain_summary, created_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
        r.UserID, r.ConnectionID, r.Database, r.Table, r.Operation,
        r.SQL, r.Status, r.RowsAffected, r.ErrorMessage, r.ExplainSummary, now.Format(time.RFC3339Nano),
    )
    if err != nil { return 0, err }
    return res.LastInsertId()
}

func (s *Store) UpdateAIAuditStatus(id int64, status string, rowsAffected *int64, errMsg string) error {
    _, err := s.DB.Exec(
        `UPDATE ai_write_audit SET status=?, rows_affected=?, error_message=? WHERE id=?`,
        status, rowsAffected, errMsg, id,
    )
    return err
}

func (s *Store) RecentAIAudit(userID int64, limit int) ([]AIAuditRow, error) {
    if limit <= 0 || limit > 500 { limit = 50 }
    rows, err := s.DB.Query(`
        SELECT id, user_id, connection_id, database_name, table_name, operation,
               sql_text, status, rows_affected, COALESCE(error_message,''),
               COALESCE(explain_summary,''), created_at
          FROM ai_write_audit
         WHERE user_id=?
         ORDER BY created_at DESC LIMIT ?`,
        userID, limit)
    if err != nil { return nil, err }
    defer rows.Close()
    var out []AIAuditRow
    for rows.Next() {
        var r AIAuditRow
        var rowsAff sql.NullInt64
        var created string
        if err := rows.Scan(&r.ID, &r.UserID, &r.ConnectionID, &r.Database, &r.Table,
            &r.Operation, &r.SQL, &r.Status, &rowsAff, &r.ErrorMessage,
            &r.ExplainSummary, &created); err != nil {
            return nil, err
        }
        if rowsAff.Valid { v := rowsAff.Int64; r.RowsAffected = &v }
        r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
        out = append(out, r)
    }
    return out, rows.Err()
}
```

- [ ] **Step 4: Run tests, expect PASS**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/store/ -run TestAIAudit -v`

- [ ] **Step 5: Commit**

```bash
git add internal/store/ai_audit.go internal/store/ai_audit_test.go
git -c commit.gpgsign=false commit -m "feat(store): ai_write_audit insert/update/recent helpers"
```

---

## Task 5: SQL classifier package

**Files:**
- Create: `internal/mysql/sqlclass.go`
- Test: `internal/mysql/sqlclass_test.go`

- [ ] **Step 1: Write a thorough test table**

Create `internal/mysql/sqlclass_test.go`:

```go
package mysql

import "testing"

func TestClassifyBasic(t *testing.T) {
    cases := []struct {
        name      string
        sql       string
        wantOp    Op
        wantDB    string
        wantTable string
        wantMulti bool
        wantErr   bool
    }{
        {"select", "SELECT * FROM t", OpSelect, "", "t", false, false},
        {"select-no-from", "SELECT 1", OpSelect, "", "", false, false},
        {"insert-basic", "INSERT INTO t (a) VALUES (1)", OpInsert, "", "t", false, false},
        {"insert-low-priority", "INSERT LOW_PRIORITY INTO t VALUES(1)", OpInsert, "", "t", false, false},
        {"insert-ignore", "INSERT IGNORE INTO `db`.`t` VALUES(1)", OpInsert, "db", "t", false, false},
        {"insert-select", "INSERT INTO a SELECT * FROM b", OpInsert, "", "a", false, false},
        {"update", "UPDATE t SET a=1", OpUpdate, "", "t", false, false},
        {"update-ignore", "UPDATE IGNORE db.t SET a=1", OpUpdate, "db", "t", false, false},
        {"update-join", "UPDATE a JOIN b ON a.x=b.x SET a.y=b.y", OpUpdate, "", "a", false, false},
        {"update-comma", "UPDATE a, b SET a.y=b.y", OpUpdate, "", "a", false, false},
        {"delete", "DELETE FROM t WHERE id=1", OpDelete, "", "t", false, false},
        {"delete-quick", "DELETE LOW_PRIORITY QUICK IGNORE FROM `db`.`t`", OpDelete, "db", "t", false, false},
        {"truncate-bare", "TRUNCATE t", OpTruncate, "", "t", false, false},
        {"truncate-table", "TRUNCATE TABLE db.t", OpTruncate, "db", "t", false, false},
        {"alter", "ALTER TABLE t ADD COLUMN x INT", OpDDL, "", "t", false, false},
        {"rename-table", "RENAME TABLE a TO b", OpDDL, "", "a", false, false},
        {"create-forbidden", "CREATE TABLE t (id INT)", OpForbidden, "", "", false, false},
        {"drop-forbidden", "DROP TABLE t", OpForbidden, "", "", false, false},
        {"show", "SHOW TABLES", OpReadMeta, "", "", false, false},
        {"describe", "DESCRIBE t", OpReadMeta, "", "", false, false},
        {"desc", "DESC t", OpReadMeta, "", "", false, false},
        {"explain", "EXPLAIN SELECT * FROM t", OpReadMeta, "", "", false, false},
        {"multi", "SELECT 1; SELECT 2", OpSelect, "", "", true, false},
        {"multi-trailing-semicolon", "SELECT 1;", OpSelect, "", "", false, false},
        {"line-comment", "-- hi\nSELECT * FROM t", OpSelect, "", "t", false, false},
        {"block-comment", "/* x */ SELECT * FROM t", OpSelect, "", "t", false, false},
        {"backtick-with-escape", "SELECT * FROM `weird``name`", OpSelect, "", "weird`name", false, false},
        {"unknown", "DO SOMETHING", OpUnknown, "", "", false, false},
        {"empty", "   ", OpUnknown, "", "", false, true},
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            got, err := Classify(c.sql)
            if c.wantErr {
                if err == nil { t.Fatal("expected error") }
                return
            }
            if err != nil { t.Fatalf("unexpected error: %v", err) }
            if got.Op != c.wantOp { t.Errorf("op=%v want %v", got.Op, c.wantOp) }
            if got.DB != c.wantDB { t.Errorf("db=%q want %q", got.DB, c.wantDB) }
            if got.Table != c.wantTable { t.Errorf("table=%q want %q", got.Table, c.wantTable) }
            if got.Multi != c.wantMulti { t.Errorf("multi=%v want %v", got.Multi, c.wantMulti) }
        })
    }
}
```

- [ ] **Step 2: Run tests, expect FAIL**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/mysql/ -run TestClassify -v`

- [ ] **Step 3: Implement the classifier**

Create `internal/mysql/sqlclass.go`:

```go
package mysql

import (
    "errors"
    "regexp"
    "strings"
)

type Op string

const (
    OpSelect    Op = "SELECT"
    OpInsert    Op = "INSERT"
    OpUpdate    Op = "UPDATE"
    OpDelete    Op = "DELETE"
    OpTruncate  Op = "TRUNCATE"
    OpDDL       Op = "DDL"        // ALTER / RENAME on existing table
    OpForbidden Op = "FORBIDDEN"  // CREATE / DROP / GRANT / etc — never allowed
    OpReadMeta  Op = "READMETA"   // SHOW / DESCRIBE / DESC / EXPLAIN
    OpUnknown   Op = "UNKNOWN"
)

type Classified struct {
    Op    Op
    DB    string
    Table string
    Multi bool
}

// stripComments removes /* ... */ block comments and `-- ...` line comments.
// Keeps content inside string literals and backtick identifiers intact.
func stripComments(s string) string {
    var b strings.Builder
    i := 0
    for i < len(s) {
        c := s[i]
        switch {
        case c == '-' && i+1 < len(s) && s[i+1] == '-':
            // line comment until newline
            j := i + 2
            for j < len(s) && s[j] != '\n' { j++ }
            i = j
        case c == '/' && i+1 < len(s) && s[i+1] == '*':
            j := i + 2
            for j+1 < len(s) && !(s[j] == '*' && s[j+1] == '/') { j++ }
            i = j + 2
            if i > len(s) { i = len(s) }
        case c == '\'' || c == '"':
            // skip string literal preserving content
            b.WriteByte(c)
            quote := c
            i++
            for i < len(s) {
                if s[i] == '\\' && i+1 < len(s) {
                    b.WriteByte(s[i]); b.WriteByte(s[i+1]); i += 2; continue
                }
                b.WriteByte(s[i])
                if s[i] == quote { i++; break }
                i++
            }
        case c == '`':
            b.WriteByte(c)
            i++
            for i < len(s) {
                if s[i] == '`' && i+1 < len(s) && s[i+1] == '`' {
                    b.WriteByte('`'); b.WriteByte('`'); i += 2; continue
                }
                b.WriteByte(s[i])
                if s[i] == '`' { i++; break }
                i++
            }
        default:
            b.WriteByte(c)
            i++
        }
    }
    return b.String()
}

// splitTopLevel breaks a comment-stripped SQL string at top-level semicolons
// (i.e. semicolons not inside quotes or backticks). Returns trimmed non-empty parts.
func splitTopLevel(s string) []string {
    var parts []string
    var cur strings.Builder
    i := 0
    for i < len(s) {
        c := s[i]
        switch c {
        case '\'', '"':
            cur.WriteByte(c)
            quote := c
            i++
            for i < len(s) {
                if s[i] == '\\' && i+1 < len(s) {
                    cur.WriteByte(s[i]); cur.WriteByte(s[i+1]); i += 2; continue
                }
                cur.WriteByte(s[i])
                if s[i] == quote { i++; break }
                i++
            }
        case '`':
            cur.WriteByte(c)
            i++
            for i < len(s) {
                if s[i] == '`' && i+1 < len(s) && s[i+1] == '`' {
                    cur.WriteByte('`'); cur.WriteByte('`'); i += 2; continue
                }
                cur.WriteByte(s[i])
                if s[i] == '`' { i++; break }
                i++
            }
        case ';':
            t := strings.TrimSpace(cur.String())
            if t != "" { parts = append(parts, t) }
            cur.Reset()
            i++
        default:
            cur.WriteByte(c)
            i++
        }
    }
    t := strings.TrimSpace(cur.String())
    if t != "" { parts = append(parts, t) }
    return parts
}

// identifier matches a bare or backticked table reference, optionally schema-qualified.
//   `name`         → name (with `` -> `)
//   name           → name
//   `db`.`table`   → db, table
//   db.table       → db, table
var identRE = regexp.MustCompile("`(?:[^`]|``)*`|[A-Za-z_][A-Za-z0-9_$]*")

// firstTableRef pulls the first table reference from the head of s (after a
// keyword like FROM / INTO / TABLE / UPDATE) and returns (db, table, remainder).
func firstTableRef(s string) (string, string) {
    s = strings.TrimSpace(s)
    m := identRE.FindStringIndex(s)
    if m == nil { return "", "" }
    first := unquote(s[m[0]:m[1]])
    rest := strings.TrimLeft(s[m[1]:], " \t\r\n")
    if strings.HasPrefix(rest, ".") {
        rest = strings.TrimLeft(rest[1:], " \t\r\n")
        m2 := identRE.FindStringIndex(rest)
        if m2 != nil {
            return first, unquote(rest[m2[0]:m2[1]])
        }
    }
    return "", first
}

func unquote(s string) string {
    if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
        inner := s[1:len(s)-1]
        return strings.ReplaceAll(inner, "``", "`")
    }
    return s
}

// verbMatchers run in order; first match wins. Each returns table info.
var (
    reInsert   = regexp.MustCompile(`(?is)^INSERT\s+(?:LOW_PRIORITY\s+|DELAYED\s+|HIGH_PRIORITY\s+|IGNORE\s+)*INTO\s+(.+)$`)
    reUpdate   = regexp.MustCompile(`(?is)^UPDATE\s+(?:LOW_PRIORITY\s+|IGNORE\s+)*(.+)$`)
    reDelete   = regexp.MustCompile(`(?is)^DELETE\s+(?:LOW_PRIORITY\s+|QUICK\s+|IGNORE\s+)*FROM\s+(.+)$`)
    reTruncate = regexp.MustCompile(`(?is)^TRUNCATE\s+(?:TABLE\s+)?(.+)$`)
    reAlter    = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+(.+)$`)
    reRename   = regexp.MustCompile(`(?is)^RENAME\s+TABLE\s+(.+)$`)
    reSelect   = regexp.MustCompile(`(?is)^SELECT\b(.*)$`)
    reFromTbl  = regexp.MustCompile(`(?is)\bFROM\s+(.+)$`)
)

// firstVerb returns the leading word of stmt uppercased.
func firstVerb(stmt string) string {
    i := 0
    for i < len(stmt) && (stmt[i] == ' ' || stmt[i] == '\t' || stmt[i] == '\n' || stmt[i] == '\r') { i++ }
    j := i
    for j < len(stmt) && ((stmt[j] >= 'A' && stmt[j] <= 'Z') || (stmt[j] >= 'a' && stmt[j] <= 'z')) { j++ }
    return strings.ToUpper(stmt[i:j])
}

func Classify(sql string) (Classified, error) {
    cleaned := strings.TrimSpace(stripComments(sql))
    if cleaned == "" { return Classified{Op: OpUnknown}, errors.New("empty sql") }
    parts := splitTopLevel(cleaned)
    if len(parts) == 0 { return Classified{Op: OpUnknown}, errors.New("empty sql") }
    multi := len(parts) > 1
    head := parts[0]
    verb := firstVerb(head)

    var c Classified
    c.Multi = multi
    switch verb {
    case "SELECT":
        c.Op = OpSelect
        if m := reFromTbl.FindStringSubmatch(head); m != nil {
            c.DB, c.Table = firstTableRef(m[1])
        }
    case "INSERT":
        if m := reInsert.FindStringSubmatch(head); m != nil {
            c.Op = OpInsert
            c.DB, c.Table = firstTableRef(m[1])
        } else {
            c.Op = OpUnknown
        }
    case "UPDATE":
        if m := reUpdate.FindStringSubmatch(head); m != nil {
            c.Op = OpUpdate
            c.DB, c.Table = firstTableRef(m[1])
        } else { c.Op = OpUnknown }
    case "DELETE":
        if m := reDelete.FindStringSubmatch(head); m != nil {
            c.Op = OpDelete
            c.DB, c.Table = firstTableRef(m[1])
        } else { c.Op = OpUnknown }
    case "TRUNCATE":
        if m := reTruncate.FindStringSubmatch(head); m != nil {
            c.Op = OpTruncate
            c.DB, c.Table = firstTableRef(m[1])
        } else { c.Op = OpUnknown }
    case "ALTER":
        if m := reAlter.FindStringSubmatch(head); m != nil {
            c.Op = OpDDL
            c.DB, c.Table = firstTableRef(m[1])
        } else { c.Op = OpUnknown }
    case "RENAME":
        if m := reRename.FindStringSubmatch(head); m != nil {
            c.Op = OpDDL
            c.DB, c.Table = firstTableRef(m[1])
        } else { c.Op = OpUnknown }
    case "CREATE", "DROP", "GRANT", "REVOKE", "REPLACE", "FLUSH", "RESET":
        c.Op = OpForbidden
    case "SHOW", "DESCRIBE", "DESC", "EXPLAIN":
        c.Op = OpReadMeta
    default:
        c.Op = OpUnknown
    }
    return c, nil
}
```

- [ ] **Step 4: Run tests, expect PASS**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/mysql/ -run TestClassify -v`
Expected: all subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mysql/sqlclass.go internal/mysql/sqlclass_test.go
git -c commit.gpgsign=false commit -m "feat(mysql): sqlclass.Classify for INSERT/UPDATE/DELETE/TRUNCATE/DDL/SELECT"
```

---

## Task 6: Policy decision package

**Files:**
- Create: `internal/policy/policy.go`
- Test: `internal/policy/policy_test.go`

- [ ] **Step 1: Write tests**

Create `internal/policy/policy_test.go`:

```go
package policy

import (
    "testing"

    "github.com/conray/dataseai/internal/mysql"
    "github.com/conray/dataseai/internal/store"
)

func newStore(t *testing.T) *store.Store {
    s, err := store.Open(":memory:")
    if err != nil { t.Fatal(err) }
    return s
}

func TestCheckMasterDisabled(t *testing.T) {
    s := newStore(t)
    u, _ := s.CreateUser("alice", "longpassword1")
    _ = s.UpsertAIPolicy(u.ID, 1, "db", "t", store.AIPolicy{Insert: true})

    d := Check(s, u.ID, 1, "db", "t", mysql.OpInsert)
    if d.Allowed { t.Fatal("master off should deny") }
    if d.Reason != "master_disabled" { t.Fatalf("reason=%q", d.Reason) }
}

func TestCheckMissingPolicy(t *testing.T) {
    s := newStore(t)
    u, _ := s.CreateUser("alice", "longpassword1")
    _ = s.SetAIWritesEnabled(u.ID, true)
    d := Check(s, u.ID, 1, "db", "missing", mysql.OpInsert)
    if d.Allowed { t.Fatal("missing row should deny") }
    if d.Reason != "policy_denied" { t.Fatalf("reason=%q", d.Reason) }
}

func TestCheckPerOp(t *testing.T) {
    s := newStore(t)
    u, _ := s.CreateUser("alice", "longpassword1")
    _ = s.SetAIWritesEnabled(u.ID, true)
    _ = s.UpsertAIPolicy(u.ID, 1, "db", "t", store.AIPolicy{Insert: true, Delete: true, DDL: true})

    cases := []struct {
        op    mysql.Op
        allow bool
    }{
        {mysql.OpInsert,   true},
        {mysql.OpUpdate,   false},  // not in policy
        {mysql.OpDelete,   true},
        {mysql.OpTruncate, true},   // TRUNCATE rides on Delete
        {mysql.OpDDL,      true},
        {mysql.OpSelect,   false},  // shouldn't go through propose anyway
        {mysql.OpForbidden,false},
        {mysql.OpUnknown,  false},
    }
    for _, c := range cases {
        d := Check(s, u.ID, 1, "db", "t", c.op)
        if d.Allowed != c.allow { t.Errorf("%v: allowed=%v want %v", c.op, d.Allowed, c.allow) }
    }
}
```

- [ ] **Step 2: Verify `store.Open` exists**

Run: `grep -n 'func Open' /home/conray/project/mysqlweb/internal/store/*.go`
If `Open` isn't already there, find the existing constructor (likely `New` or `NewStore`) and replace `store.Open` with it in the test.

- [ ] **Step 3: Run tests, expect FAIL**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/policy/ -v`
Expected: compile error.

- [ ] **Step 4: Implement**

Create `internal/policy/policy.go`:

```go
package policy

import (
    "github.com/conray/dataseai/internal/mysql"
    "github.com/conray/dataseai/internal/store"
)

type Decision struct {
    Allowed bool
    Reason  string  // master_disabled | policy_denied | ""
}

func Check(s *store.Store, userID, connID int64, db, table string, op mysql.Op) Decision {
    enabled, err := s.GetAIWritesEnabled(userID)
    if err != nil || !enabled {
        return Decision{false, "master_disabled"}
    }
    p, found, err := s.GetAIPolicy(userID, connID, db, table)
    if err != nil || !found {
        return Decision{false, "policy_denied"}
    }
    switch op {
    case mysql.OpInsert:
        if p.Insert { return Decision{true, ""} }
    case mysql.OpUpdate:
        if p.Update { return Decision{true, ""} }
    case mysql.OpDelete, mysql.OpTruncate:
        if p.Delete { return Decision{true, ""} }
    case mysql.OpDDL:
        if p.DDL { return Decision{true, ""} }
    }
    return Decision{false, "policy_denied"}
}
```

- [ ] **Step 5: Run tests, expect PASS**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/policy/ -v`

- [ ] **Step 6: Commit**

```bash
git add internal/policy/policy.go internal/policy/policy_test.go
git -c commit.gpgsign=false commit -m "feat(policy): Check applies master + per-(user,conn,db,table,op) gating"
```

---

## Task 7: HTTP — master switch & policy & audit endpoints

**Files:**
- Create: `internal/api/ai_policy.go`
- Create: `internal/api/ai_policy_test.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Inspect an existing auth-protected handler test for the helper shape**

Run: `grep -l 'auth.Middleware\|httptest.NewRecorder' /home/conray/project/mysqlweb/internal/api/*_test.go | head -3`
Read one (e.g. `api_keys_test.go` if it exists, otherwise `connections_test.go`) to understand the auth-injection helper.

- [ ] **Step 2: Write handler tests**

Create `internal/api/ai_policy_test.go`:

```go
package api

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "testing"
)

// reuse the testServer / authedReq helpers used by other api_test files.

func TestAIWritesMasterToggle(t *testing.T) {
    srv, tok := newAuthedServer(t)

    res := authedReq(t, srv, tok, "GET", "/api/auth/ai-writes", nil)
    if res.StatusCode != 200 { t.Fatalf("status=%d", res.StatusCode) }
    var got struct{ Enabled bool `json:"enabled"` }
    _ = json.NewDecoder(res.Body).Decode(&got)
    if got.Enabled { t.Fatal("expected default false") }

    body, _ := json.Marshal(map[string]any{"enabled": true})
    res = authedReq(t, srv, tok, "PUT", "/api/auth/ai-writes", bytes.NewReader(body))
    if res.StatusCode != 200 { t.Fatalf("PUT status=%d", res.StatusCode) }

    res = authedReq(t, srv, tok, "GET", "/api/auth/ai-writes", nil)
    _ = json.NewDecoder(res.Body).Decode(&got)
    if !got.Enabled { t.Fatal("expected true after PUT") }
}

func TestAIPolicyUpsertAndList(t *testing.T) {
    srv, tok := newAuthedServer(t)

    body := mustJSON(map[string]any{
        "conn": 1, "db": "db1", "table": "t1",
        "policy": map[string]bool{"insert": true, "update": false, "delete": true, "ddl": false},
    })
    res := authedReq(t, srv, tok, "PUT", "/api/auth/ai-policy", bytes.NewReader(body))
    if res.StatusCode != 200 {
        t.Fatalf("status=%d body=%s", res.StatusCode, readAll(res.Body))
    }

    res = authedReq(t, srv, tok, "GET", "/api/auth/ai-policy?conn=1&db=db1", nil)
    if res.StatusCode != 200 { t.Fatalf("GET status=%d", res.StatusCode) }
    // body should contain "configured":[{"table":"t1","policy":{...}}]
    raw, _ := io.ReadAll(res.Body)
    if !bytes.Contains(raw, []byte(`"t1"`)) { t.Fatalf("missing t1 in %s", raw) }
}

func TestAIPolicyBatch(t *testing.T) {
    srv, tok := newAuthedServer(t)
    body := mustJSON(map[string]any{
        "conn": 1, "db": "db1", "tables": []string{"a", "b", "c"},
        "policy": map[string]bool{"insert": true},
    })
    res := authedReq(t, srv, tok, "PUT", "/api/auth/ai-policy/batch", bytes.NewReader(body))
    if res.StatusCode != 200 { t.Fatalf("status=%d", res.StatusCode) }
    var got struct{ Updated int `json:"updated"` }
    _ = json.NewDecoder(res.Body).Decode(&got)
    if got.Updated != 3 { t.Fatalf("updated=%d", got.Updated) }
}

func TestAIPolicyDelete(t *testing.T) {
    srv, tok := newAuthedServer(t)
    _ = upsertPolicy(srv, tok, 1, "db1", "t1", true, false, false, false)
    res := authedReq(t, srv, tok, "DELETE", "/api/auth/ai-policy?conn=1&db=db1&table=t1", nil)
    if res.StatusCode != 200 { t.Fatalf("status=%d", res.StatusCode) }
}

// helper used in this file
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func readAll(r io.ReadCloser) string { b, _ := io.ReadAll(r); return string(b) }

func upsertPolicy(srv *httptest.Server, tok string, conn int, db, table string, ins, upd, del, ddl bool) int {
    body := mustJSON(map[string]any{"conn": conn, "db": db, "table": table,
        "policy": map[string]bool{"insert": ins, "update": upd, "delete": del, "ddl": ddl}})
    res := authedReqRaw(srv, tok, "PUT", "/api/auth/ai-policy", bytes.NewReader(body))
    defer res.Body.Close()
    return res.StatusCode
}

func authedReqRaw(srv *httptest.Server, tok, method, path string, body io.Reader) *http.Response {
    req, _ := http.NewRequest(method, srv.URL+path, body)
    if tok != "" { req.Header.Set("Authorization", "Bearer "+tok) }
    if body != nil { req.Header.Set("Content-Type", "application/json") }
    res, err := srv.Client().Do(req)
    if err != nil { panic(err) }
    return res
}
```

If `newAuthedServer` / `authedReq` don't exist in `internal/api`, look for the existing convention (search for `httptest.NewServer` in `internal/api/*_test.go`) and adapt — keep the helpers in a shared `helpers_test.go` if you need to introduce them.

- [ ] **Step 3: Implement handlers**

Create `internal/api/ai_policy.go`:

```go
package api

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/conray/dataseai/internal/auth"
    "github.com/conray/dataseai/internal/store"
)

type aiWritesResp struct {
    Enabled bool `json:"enabled"`
}

func handleGetAIWrites(d Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        u, _ := auth.UserFromContext(r.Context())
        en, err := d.Store.GetAIWritesEnabled(u.ID)
        if err != nil {
            writeError(w, http.StatusInternalServerError, err.Error()); return
        }
        writeJSON(w, http.StatusOK, aiWritesResp{Enabled: en})
    }
}

func handlePutAIWrites(d Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        u, _ := auth.UserFromContext(r.Context())
        var body struct{ Enabled bool `json:"enabled"` }
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
            writeError(w, http.StatusBadRequest, "bad body"); return
        }
        if err := d.Store.SetAIWritesEnabled(u.ID, body.Enabled); err != nil {
            writeError(w, http.StatusInternalServerError, err.Error()); return
        }
        writeJSON(w, http.StatusOK, aiWritesResp{Enabled: body.Enabled})
    }
}

type aiPolicyBody struct {
    Conn   int64          `json:"conn"`
    DB     string         `json:"db"`
    Table  string         `json:"table"`
    Policy store.AIPolicy `json:"policy"`
}

func handlePutAIPolicy(d Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        u, _ := auth.UserFromContext(r.Context())
        var body aiPolicyBody
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Conn == 0 || body.DB == "" || body.Table == "" {
            writeError(w, http.StatusBadRequest, "bad body"); return
        }
        // Ownership: user must own this connection.
        if _, err := d.Store.GetConnection(u.ID, body.Conn); err != nil {
            writeError(w, http.StatusNotFound, "connection not found"); return
        }
        if err := d.Store.UpsertAIPolicy(u.ID, body.Conn, body.DB, body.Table, body.Policy); err != nil {
            writeError(w, http.StatusInternalServerError, err.Error()); return
        }
        writeJSON(w, http.StatusOK, map[string]any{"table": body.Table, "policy": body.Policy})
    }
}

type aiPolicyBatchBody struct {
    Conn   int64          `json:"conn"`
    DB     string         `json:"db"`
    Tables []string       `json:"tables"`
    Policy store.AIPolicy `json:"policy"`
}

func handleBatchAIPolicy(d Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        u, _ := auth.UserFromContext(r.Context())
        var body aiPolicyBatchBody
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Conn == 0 || body.DB == "" || len(body.Tables) == 0 {
            writeError(w, http.StatusBadRequest, "bad body"); return
        }
        if _, err := d.Store.GetConnection(u.ID, body.Conn); err != nil {
            writeError(w, http.StatusNotFound, "connection not found"); return
        }
        if err := d.Store.BatchUpsertAIPolicy(u.ID, body.Conn, body.DB, body.Tables, body.Policy); err != nil {
            writeError(w, http.StatusInternalServerError, err.Error()); return
        }
        writeJSON(w, http.StatusOK, map[string]any{"updated": len(body.Tables)})
    }
}

func handleDeleteAIPolicy(d Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        u, _ := auth.UserFromContext(r.Context())
        q := r.URL.Query()
        connStr, db, table := q.Get("conn"), q.Get("db"), q.Get("table")
        connID, err := strconv.ParseInt(connStr, 10, 64)
        if err != nil || db == "" || table == "" {
            writeError(w, http.StatusBadRequest, "bad query"); return
        }
        if err := d.Store.DeleteAIPolicy(u.ID, connID, db, table); err != nil {
            writeError(w, http.StatusInternalServerError, err.Error()); return
        }
        writeJSON(w, http.StatusOK, map[string]any{"ok": true})
    }
}

// handleListAIPolicy returns configured + unconfigured tables. unconfigured
// is sourced from MySQL information_schema.tables for the given db.
func handleListAIPolicy(d Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        u, _ := auth.UserFromContext(r.Context())
        connID, err := strconv.ParseInt(r.URL.Query().Get("conn"), 10, 64)
        db := r.URL.Query().Get("db")
        if err != nil || db == "" {
            writeError(w, http.StatusBadRequest, "bad query"); return
        }
        if _, err := d.Store.GetConnection(u.ID, connID); err != nil {
            writeError(w, http.StatusNotFound, "connection not found"); return
        }
        configured, err := d.Store.ListAIPolicy(u.ID, connID, db)
        if err != nil {
            writeError(w, http.StatusInternalServerError, err.Error()); return
        }
        // Open the MySQL pool entry for this user+conn and list tables.
        all, err := listAllTables(r.Context(), d, u.ID, connID, db)
        if err != nil {
            writeError(w, http.StatusBadGateway, "list tables: "+err.Error()); return
        }
        configuredSet := map[string]struct{}{}
        for _, c := range configured { configuredSet[c.Table] = struct{}{} }
        var unconfigured []string
        for _, name := range all {
            if _, ok := configuredSet[name]; !ok {
                unconfigured = append(unconfigured, name)
            }
        }
        writeJSON(w, http.StatusOK, map[string]any{
            "configured":   configured,
            "unconfigured": unconfigured,
        })
    }
}

func handleListAIAudit(d Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        u, _ := auth.UserFromContext(r.Context())
        limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
        if limit <= 0 { limit = 50 }
        rows, err := d.Store.RecentAIAudit(u.ID, limit)
        if err != nil {
            writeError(w, http.StatusInternalServerError, err.Error()); return
        }
        writeJSON(w, http.StatusOK, rows)
    }
}
```

- [ ] **Step 4: Add the `listAllTables` helper at the bottom of the file**

Append to `internal/api/ai_policy.go`:

```go
import_helper_placeholder // (the import block above already has what we need;
                          // this comment is a marker — delete after writing the body below.)
```

Replace with the actual helper (and delete the placeholder comment):

```go
func listAllTables(ctx context.Context, d Deps, userID, connID int64, db string) ([]string, error) {
    conn, err := d.Store.GetConnection(userID, connID)
    if err != nil { return nil, err }
    pw, err := d.Store.GetConnectionPassword(d.Cipher, userID, connID)
    if err != nil { return nil, err }
    dsn := mysql.DSNInput{Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw, DefaultDB: conn.DefaultDB, TLS: conn.TLS}
    var ssh mysql.SSHConfig
    if conn.SSHEnabled {
        sshPw, _ := d.Store.GetSSHPassword(d.Cipher, userID, connID)
        ssh = mysql.SSHConfig{Host: conn.SSHHost, Port: conn.SSHPort, User: conn.SSHUser, Password: sshPw}
    }
    pool, err := d.Pool.Get(mysql.PoolKey{UserID: userID, ConnID: connID}, dsn, ssh)
    if err != nil { return nil, err }
    return mysql.ListTables(ctx, pool, db)
}
```

And add the missing imports at the top of `internal/api/ai_policy.go`:

```go
import (
    "context"
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/conray/dataseai/internal/auth"
    "github.com/conray/dataseai/internal/mysql"
    "github.com/conray/dataseai/internal/store"
)
```

- [ ] **Step 5: Wire routes in `internal/api/router.go`**

In the authed group (after `r.Put("/api/auth/api-keys", handlePutAPIKey(d))`), append:

```go
r.Get("/api/auth/ai-writes",          handleGetAIWrites(d))
r.Put("/api/auth/ai-writes",          handlePutAIWrites(d))
r.Get("/api/auth/ai-policy",          handleListAIPolicy(d))
r.Put("/api/auth/ai-policy",          handlePutAIPolicy(d))
r.Put("/api/auth/ai-policy/batch",    handleBatchAIPolicy(d))
r.Delete("/api/auth/ai-policy",       handleDeleteAIPolicy(d))
r.Get("/api/auth/ai-audit",           handleListAIAudit(d))
```

- [ ] **Step 6: Run tests, expect PASS**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/api/ -run 'TestAIWrites|TestAIPolicy' -v`

If `ListAIPolicy` test fails on `listAllTables` (because the MySQL pool can't reach a real server in tests), gate that test behind a separate `TestListAIPolicyConfigured` which only exercises the configured-side path with a fake table-list-fetcher; introduce `Deps.ListTablesFunc` indirection if needed.

- [ ] **Step 7: Commit**

```bash
git add internal/api/ai_policy.go internal/api/ai_policy_test.go internal/api/router.go
git -c commit.gpgsign=false commit -m "feat(api): ai-writes / ai-policy / ai-audit endpoints"
```

---

## Task 8: Harden run_sql / mysql_query to read-only

**Files:**
- Modify: `internal/chat/execute.go`
- Modify: `internal/chat/orchestrator_mcp.go`
- Modify: `internal/chat/execute_test.go`

- [ ] **Step 1: Write a test that confirms run_sql refuses DML**

Append to `internal/chat/execute_test.go`:

```go
func TestRunSQLBlocksDML(t *testing.T) {
    ctx := context.Background()
    db := setupTestSQLite(t)   // existing helper that returns *sql.DB w/ a table
    out, err := Execute(ctx, db, "run_sql", map[string]any{"sql": "DELETE FROM t WHERE id=1"})
    if err != nil { t.Fatalf("unexpected err: %v", err) }
    if !strings.Contains(out, "run_sql_readonly") {
        t.Fatalf("expected reject, got %s", out)
    }
}

func TestRunSQLAllowsExplain(t *testing.T) {
    ctx := context.Background()
    db := setupTestSQLite(t)
    out, err := Execute(ctx, db, "run_sql", map[string]any{"sql": "EXPLAIN SELECT * FROM t"})
    if err != nil { t.Fatalf("err: %v", err) }
    if strings.Contains(out, "run_sql_readonly") {
        t.Fatalf("EXPLAIN should pass, got %s", out)
    }
}
```

(Use whatever existing helper sets up sqlite + table `t`; if none, copy the pattern from `dml_test.go`.)

- [ ] **Step 2: Run, expect FAIL**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/chat/ -run TestRunSQL -v`

- [ ] **Step 3: Add the guard to `Execute` in `execute.go`**

Replace the body of the `case "run_sql":` block:

```go
case "run_sql":
    sqlStr, _ := input["sql"].(string)
    if sqlStr == "" {
        return "", fmt.Errorf("sql required")
    }
    cls, _ := mysql.Classify(sqlStr)
    if cls.Op != mysql.OpSelect && cls.Op != mysql.OpReadMeta {
        return marshal(map[string]any{
            "error": "run_sql_readonly",
            "hint":  "use propose_write for any INSERT/UPDATE/DELETE/TRUNCATE/ALTER/RENAME; only SELECT/SHOW/DESCRIBE/EXPLAIN are allowed via run_sql",
        })
    }
    out, err := mysql.Run(ctx, db, sqlStr, mysql.RunOpts{MaxRows: 1000})
    if err != nil {
        return "", err
    }
    return marshal(map[string]any{
        "columns":       out.Columns,
        "rows":          out.Rows,
        "rows_affected": out.RowsAffected,
        "truncated":     out.Truncated,
    })
```

- [ ] **Step 4: Mirror the guard in `orchestrator_mcp.go`'s `executeMCP` for `mysql_query`**

Replace the body of `case "mysql_query":` in `executeMCP`:

```go
case "mysql_query":
    sqlText, _ := input["sql"].(string)
    if sqlText == "" {
        return "", fmt.Errorf("sql required")
    }
    cls, _ := mysql.Classify(sqlText)
    if cls.Op != mysql.OpSelect && cls.Op != mysql.OpReadMeta {
        return marshalErr(map[string]any{
            "error": "run_sql_readonly",
            "hint":  "use propose_write for writes; mysql_query is read-only",
        })
    }
    return d.MCP.CallTool(ctx, "mysql_query", map[string]any{
        "dsn_name": d.DSNName,
        "sql":      sqlText,
    })
```

Add a tiny `marshalErr` helper in the same file (or reuse `marshal` from `execute.go` — depends on package boundary; since this is the same package `chat`, the existing `marshal` is fine but it returns `(string, error)`. Replace `marshalErr(...)` with `marshal(...)` — both branches must return a JSON string + nil error.).

- [ ] **Step 5: Update the imports of `orchestrator_mcp.go` to include `mysql`**

Top of file:

```go
import (
    "context"
    "fmt"

    "github.com/conray/dataseai/internal/llm"
    "github.com/conray/dataseai/internal/mysql"
)
```

- [ ] **Step 6: Run, expect PASS**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/chat/ -run TestRunSQL -v`
Run: `cd /home/conray/project/mysqlweb && go test ./internal/chat/...`

- [ ] **Step 7: Commit**

```bash
git add internal/chat/execute.go internal/chat/orchestrator_mcp.go internal/chat/execute_test.go
git -c commit.gpgsign=false commit -m "fix(chat): harden run_sql / mysql_query to read-only via sqlclass.Classify"
```

---

## Task 9: ProposalGateway interface + chat.Deps additions

**Files:**
- Create: `internal/chat/proposal.go`
- Modify: `internal/chat/orchestrator.go`
- Modify: `internal/chat/orchestrator_mcp.go`

- [ ] **Step 1: Define the gateway type**

Create `internal/chat/proposal.go`:

```go
package chat

import "context"

// Proposal carries everything the orchestrator wants the WS layer to render.
type Proposal struct {
    ID             string
    Database       string
    Table          string
    Operation      string // INSERT|UPDATE|DELETE|TRUNCATE|DDL
    SQL            string
    ExplainSummary string // JSON; empty if EXPLAIN didn't run
}

// Decision is what the WS layer returns after the user clicks Execute or Cancel.
type Decision struct {
    Accept       bool
    RowsAffected *int64 // populated by the gateway if it executed; usually nil and execute happens server-side
    Error        string // populated on failure
}

// ProposalGateway is the bridge between the chat orchestrator and the
// WebSocket layer. The orchestrator calls Propose and BLOCKS until the
// WS layer returns a Decision (user click or session close).
//
// The gateway is also responsible for emitting the matching write_executed /
// write_failed / write_cancelled WS events so the UI sees status transitions.
type ProposalGateway interface {
    Propose(ctx context.Context, p Proposal) (Decision, error)
}
```

- [ ] **Step 2: Extend chat.Deps to carry the gateway + store + context IDs**

Modify `internal/chat/orchestrator.go`:

```go
type Deps struct {
    LLM           llm.LLMClient
    DB            *sql.DB
    MaxIterations int
    System        string

    // New for propose_write:
    Store     *store.Store
    Gateway   ProposalGateway
    UserID    int64
    ConnID    int64
    DefaultDB string
}
```

Add the import for `store`:

```go
import (
    // ...existing
    "github.com/conray/dataseai/internal/store"
)
```

- [ ] **Step 3: Mirror the same fields on `MCPDeps` in `orchestrator_mcp.go`**

```go
type MCPDeps struct {
    LLM           llm.LLMClient
    MCP           MCPClient
    DSNName       string
    MaxIterations int
    System        string

    DB        *sql.DB
    Store     *store.Store
    Gateway   ProposalGateway
    UserID    int64
    ConnID    int64
    DefaultDB string
}
```

Add imports for `database/sql` and `store` if not already.

- [ ] **Step 4: Compile check**

Run: `cd /home/conray/project/mysqlweb && go build ./...`
Expected: builds (callers in `internal/api/chat.go` will still pass — the new Deps fields are zero-valued until Task 11 wires them).

- [ ] **Step 5: Commit**

```bash
git add internal/chat/proposal.go internal/chat/orchestrator.go internal/chat/orchestrator_mcp.go
git -c commit.gpgsign=false commit -m "feat(chat): ProposalGateway interface + Deps fields for propose_write"
```

---

## Task 10: Add propose_write tool + handler

**Files:**
- Modify: `internal/chat/tools.go`
- Modify: `internal/chat/execute.go`
- Modify: `internal/chat/orchestrator.go`
- Modify: `internal/chat/orchestrator_mcp.go`
- Create: `internal/chat/propose.go`
- Test: `internal/chat/propose_test.go`

- [ ] **Step 1: Write the propose handler tests**

Create `internal/chat/propose_test.go`:

```go
package chat

import (
    "context"
    "encoding/json"
    "strings"
    "testing"

    "github.com/conray/dataseai/internal/store"
)

// fakeGateway records Propose calls and returns a canned Decision.
type fakeGateway struct {
    proposals []Proposal
    decision  Decision
}

func (g *fakeGateway) Propose(ctx context.Context, p Proposal) (Decision, error) {
    g.proposals = append(g.proposals, p)
    return g.decision, nil
}

func TestProposeWriteRejectsMultiStatement(t *testing.T) {
    s := newTestChatStore(t)
    user, _ := s.CreateUser("alice", "longpassword1")
    _ = s.SetAIWritesEnabled(user.ID, true)
    _ = s.UpsertAIPolicy(user.ID, 1, "db", "t", store.AIPolicy{Insert: true})

    g := &fakeGateway{decision: Decision{Accept: true}}
    out, _ := handleProposeWrite(context.Background(), proposeCtx{
        Store: s, Gateway: g, DB: nil, UserID: user.ID, ConnID: 1, DefaultDB: "db",
    }, map[string]any{"database": "db", "table": "t", "operation": "INSERT",
        "sql": "INSERT INTO t VALUES (1); DELETE FROM t"})
    if !strings.Contains(out, "multi_statement") { t.Fatalf("got %s", out) }
}

func TestProposeWriteClassifyMismatch(t *testing.T) {
    s := newTestChatStore(t)
    user, _ := s.CreateUser("alice", "longpassword1")
    _ = s.SetAIWritesEnabled(user.ID, true)
    _ = s.UpsertAIPolicy(user.ID, 1, "db", "t", store.AIPolicy{Insert: true})

    g := &fakeGateway{decision: Decision{Accept: true}}
    out, _ := handleProposeWrite(context.Background(), proposeCtx{
        Store: s, Gateway: g, DB: nil, UserID: user.ID, ConnID: 1, DefaultDB: "db",
    }, map[string]any{"database": "db", "table": "t", "operation": "INSERT",
        "sql": "DELETE FROM t WHERE id=1"})
    if !strings.Contains(out, "invalid_proposal") { t.Fatalf("got %s", out) }
}

func TestProposeWritePolicyDenied(t *testing.T) {
    s := newTestChatStore(t)
    user, _ := s.CreateUser("alice", "longpassword1")
    _ = s.SetAIWritesEnabled(user.ID, true)
    // no policy row → denied
    g := &fakeGateway{decision: Decision{Accept: true}}
    out, _ := handleProposeWrite(context.Background(), proposeCtx{
        Store: s, Gateway: g, DB: nil, UserID: user.ID, ConnID: 1, DefaultDB: "db",
    }, map[string]any{"database": "db", "table": "t", "operation": "INSERT",
        "sql": "INSERT INTO t VALUES (1)"})
    if !strings.Contains(out, "policy_denied") { t.Fatalf("got %s", out) }
    rows, _ := s.RecentAIAudit(user.ID, 10)
    if len(rows) != 1 || rows[0].Status != "denied" {
        t.Fatalf("expected one 'denied' audit, got %v", rows)
    }
}

func TestProposeWriteAcceptAndExecute(t *testing.T) {
    s := newTestChatStore(t)
    user, _ := s.CreateUser("alice", "longpassword1")
    _ = s.SetAIWritesEnabled(user.ID, true)
    _ = s.UpsertAIPolicy(user.ID, 1, "db", "t", store.AIPolicy{Insert: true})

    db := setupTestSQLiteWithT(t)  // create table t(id int)
    g := &fakeGateway{decision: Decision{Accept: true}}

    out, _ := handleProposeWrite(context.Background(), proposeCtx{
        Store: s, Gateway: g, DB: db, UserID: user.ID, ConnID: 1, DefaultDB: "db",
    }, map[string]any{"database": "db", "table": "t", "operation": "INSERT",
        "sql": "INSERT INTO t VALUES (42)"})

    var got map[string]any
    _ = json.Unmarshal([]byte(out), &got)
    if got["status"] != "executed" { t.Fatalf("got %v", got) }
    rows, _ := s.RecentAIAudit(user.ID, 10)
    if len(rows) != 1 || rows[0].Status != "executed" {
        t.Fatalf("expected 'executed' audit, got %+v", rows)
    }
}

func TestProposeWriteUserCancels(t *testing.T) {
    s := newTestChatStore(t)
    user, _ := s.CreateUser("alice", "longpassword1")
    _ = s.SetAIWritesEnabled(user.ID, true)
    _ = s.UpsertAIPolicy(user.ID, 1, "db", "t", store.AIPolicy{Insert: true})

    db := setupTestSQLiteWithT(t)
    g := &fakeGateway{decision: Decision{Accept: false}}

    out, _ := handleProposeWrite(context.Background(), proposeCtx{
        Store: s, Gateway: g, DB: db, UserID: user.ID, ConnID: 1, DefaultDB: "db",
    }, map[string]any{"database": "db", "table": "t", "operation": "INSERT",
        "sql": "INSERT INTO t VALUES (42)"})
    if !strings.Contains(out, "cancelled") { t.Fatalf("got %s", out) }
}
```

Add small helpers at the top of `propose_test.go`:

```go
import "database/sql"
import _ "github.com/mattn/go-sqlite3"

func newTestChatStore(t *testing.T) *store.Store {
    s, err := store.Open(":memory:")
    if err != nil { t.Fatal(err) }
    // Seed one connection so policy lookups don't need a real conn row;
    // the store layer doesn't FK-check at policy upsert in SQLite by default
    // unless PRAGMA foreign_keys=ON. Either turn FKs off in tests or seed:
    return s
}

func setupTestSQLiteWithT(t *testing.T) *sql.DB {
    db, _ := sql.Open("sqlite3", ":memory:")
    if _, err := db.Exec("CREATE TABLE t (id INTEGER)"); err != nil { t.Fatal(err) }
    return db
}
```

If `store.Open` doesn't exist, use whatever the actual constructor name is.

- [ ] **Step 2: Implement `handleProposeWrite`**

Create `internal/chat/propose.go`:

```go
package chat

import (
    "context"
    "crypto/rand"
    "database/sql"
    "encoding/hex"
    "encoding/json"
    "fmt"

    "github.com/conray/dataseai/internal/mysql"
    "github.com/conray/dataseai/internal/policy"
    "github.com/conray/dataseai/internal/store"
)

type proposeCtx struct {
    Store     *store.Store
    Gateway   ProposalGateway
    DB        *sql.DB
    UserID    int64
    ConnID    int64
    DefaultDB string
}

func proposalID() string {
    var b [12]byte
    _, _ = rand.Read(b[:])
    return hex.EncodeToString(b[:])
}

func jsonObj(m map[string]any) string {
    b, _ := json.Marshal(m)
    return string(b)
}

// handleProposeWrite is invoked from the orchestrator when the LLM calls the
// propose_write tool. It returns a JSON string (the tool_result body).
func handleProposeWrite(ctx context.Context, pc proposeCtx, input map[string]any) (string, error) {
    decl := struct {
        Database, Table, Operation, SQL string
    }{}
    decl.Database, _  = input["database"].(string)
    decl.Table, _     = input["table"].(string)
    decl.Operation, _ = input["operation"].(string)
    decl.SQL, _       = input["sql"].(string)
    if decl.Database == "" || decl.Table == "" || decl.Operation == "" || decl.SQL == "" {
        return jsonObj(map[string]any{"error": "invalid_proposal", "reason": "missing field"}), nil
    }

    cls, err := mysql.Classify(decl.SQL)
    if err != nil {
        return jsonObj(map[string]any{"error": "invalid_proposal", "reason": err.Error()}), nil
    }
    if cls.Multi {
        return jsonObj(map[string]any{"error": "multi_statement", "reason": "one statement at a time"}), nil
    }
    declOp := opFromDecl(decl.Operation)
    if !classifiedMatches(cls, declOp) {
        return jsonObj(map[string]any{
            "error":  "invalid_proposal",
            "reason": fmt.Sprintf("classified as %s, declared %s", cls.Op, declOp),
        }), nil
    }
    // Resolve DB: classifier prefers the SQL's qualifier, otherwise the declared db.
    db := cls.DB
    if db == "" { db = decl.Database }
    if db == "" { db = pc.DefaultDB }
    if cls.Table == "" || db == "" {
        return jsonObj(map[string]any{"error": "invalid_proposal", "reason": "could not resolve db/table"}), nil
    }
    if !ciEq(db, decl.Database) || !ciEq(cls.Table, decl.Table) {
        return jsonObj(map[string]any{
            "error":  "invalid_proposal",
            "reason": fmt.Sprintf("sql targets %s.%s, declared %s.%s", db, cls.Table, decl.Database, decl.Table),
        }), nil
    }

    // First policy check (early reject saves a round-trip and audit row).
    dec := policy.Check(pc.Store, pc.UserID, pc.ConnID, decl.Database, decl.Table, declOp)
    if !dec.Allowed {
        _, _ = pc.Store.WriteAIAudit(store.AIAuditRow{
            UserID: pc.UserID, ConnectionID: pc.ConnID,
            Database: decl.Database, Table: decl.Table, Operation: string(declOp),
            SQL: decl.SQL, Status: "denied", ErrorMessage: dec.Reason,
        })
        return jsonObj(map[string]any{
            "error":     "policy_denied",
            "reason":    dec.Reason,
            "database":  decl.Database,
            "table":     decl.Table,
            "operation": string(declOp),
            "hint":      "ask the user to enable this in Settings → AI 寫入權限",
        }), nil
    }

    explainJSON := ""
    if declOp == mysql.OpUpdate || declOp == mysql.OpDelete {
        explainJSON = runExplain(ctx, pc.DB, decl.SQL)
    }

    audID, _ := pc.Store.WriteAIAudit(store.AIAuditRow{
        UserID: pc.UserID, ConnectionID: pc.ConnID,
        Database: decl.Database, Table: decl.Table, Operation: string(declOp),
        SQL: decl.SQL, Status: "proposed", ExplainSummary: explainJSON,
    })

    pid := proposalID()
    d, err := pc.Gateway.Propose(ctx, Proposal{
        ID: pid, Database: decl.Database, Table: decl.Table,
        Operation: string(declOp), SQL: decl.SQL, ExplainSummary: explainJSON,
    })
    if err != nil {
        _ = pc.Store.UpdateAIAuditStatus(audID, "cancelled", nil, err.Error())
        return jsonObj(map[string]any{"status": "cancelled", "error": err.Error()}), nil
    }
    if !d.Accept {
        _ = pc.Store.UpdateAIAuditStatus(audID, "cancelled", nil, "")
        return jsonObj(map[string]any{"status": "cancelled"}), nil
    }

    // Re-check at execute time (catches policy revoke between propose and execute).
    dec2 := policy.Check(pc.Store, pc.UserID, pc.ConnID, decl.Database, decl.Table, declOp)
    if !dec2.Allowed {
        _ = pc.Store.UpdateAIAuditStatus(audID, "denied", nil, dec2.Reason)
        return jsonObj(map[string]any{"error": "policy_denied", "reason": "revoked before execute"}), nil
    }

    res, err := pc.DB.ExecContext(ctx, decl.SQL)
    if err != nil {
        _ = pc.Store.UpdateAIAuditStatus(audID, "failed", nil, err.Error())
        return jsonObj(map[string]any{"status": "failed", "error": err.Error()}), nil
    }
    n, _ := res.RowsAffected()
    _ = pc.Store.UpdateAIAuditStatus(audID, "executed", &n, "")
    return jsonObj(map[string]any{"status": "executed", "rows_affected": n}), nil
}

func opFromDecl(s string) mysql.Op {
    switch s {
    case "INSERT":   return mysql.OpInsert
    case "UPDATE":   return mysql.OpUpdate
    case "DELETE":   return mysql.OpDelete
    case "TRUNCATE": return mysql.OpTruncate
    case "DDL":      return mysql.OpDDL
    }
    return mysql.OpUnknown
}

func classifiedMatches(cls mysql.Classified, declOp mysql.Op) bool {
    switch declOp {
    case mysql.OpInsert, mysql.OpUpdate, mysql.OpDelete, mysql.OpTruncate, mysql.OpDDL:
        return cls.Op == declOp
    }
    return false
}

func ciEq(a, b string) bool {
    if len(a) != len(b) { return false }
    for i := 0; i < len(a); i++ {
        ca, cb := a[i], b[i]
        if ca >= 'A' && ca <= 'Z' { ca += 32 }
        if cb >= 'A' && cb <= 'Z' { cb += 32 }
        if ca != cb { return false }
    }
    return true
}

func runExplain(ctx context.Context, db *sql.DB, sql string) string {
    if db == nil { return "" }
    rows, err := db.QueryContext(ctx, "EXPLAIN "+sql)
    if err != nil {
        return jsonObj(map[string]any{"error": err.Error()})
    }
    defer rows.Close()
    cols, _ := rows.Columns()
    var data []map[string]any
    for rows.Next() {
        vals := make([]any, len(cols))
        ptrs := make([]any, len(cols))
        for i := range vals { ptrs[i] = &vals[i] }
        if err := rows.Scan(ptrs...); err != nil {
            return jsonObj(map[string]any{"error": err.Error()})
        }
        row := map[string]any{}
        for i, c := range cols { row[c] = vals[i] }
        data = append(data, row)
    }
    return jsonObj(map[string]any{"rows": data})
}
```

- [ ] **Step 3: Add the tool to the tool lists**

In `internal/chat/tools.go`, the function `Tools()` becomes a method on a small `ToolOpts` or just takes a bool. Replace its signature:

```go
type ToolOpts struct{ IncludeProposeWrite bool }

func Tools(opts ToolOpts) []llm.Tool {
    base := []llm.Tool{ /* the existing 5 tools, unchanged */ }
    if !opts.IncludeProposeWrite { return base }
    return append(base, llm.Tool{
        Name:        "propose_write",
        Description: "Propose a single INSERT/UPDATE/DELETE/TRUNCATE/DDL statement for user approval. The user must click Execute before anything runs. You MUST declare the target database, table, and operation; the backend verifies these against the SQL and rejects mismatches.",
        InputSchema: map[string]any{
            "type":     "object",
            "required": []string{"database", "table", "operation", "sql"},
            "properties": map[string]any{
                "database":  map[string]any{"type": "string"},
                "table":     map[string]any{"type": "string"},
                "operation": map[string]any{"type": "string", "enum": []string{"INSERT","UPDATE","DELETE","TRUNCATE","DDL"}},
                "sql":       map[string]any{"type": "string", "description": "A single statement. No trailing semicolon required."},
            },
        },
    })
}
```

Update the orchestrator to call `Tools(ToolOpts{IncludeProposeWrite: ...})` — wired at session start (Task 11).

Same change for `MCPTools()` in `orchestrator_mcp.go`.

- [ ] **Step 4: Route the `propose_write` tool call**

In `internal/chat/execute.go`, add a switch case before `default`:

```go
case "propose_write":
    return handleProposeWrite(ctx, proposeCtx{
        Store: pc.Store, Gateway: pc.Gateway, DB: pc.DB,
        UserID: pc.UserID, ConnID: pc.ConnID, DefaultDB: pc.DefaultDB,
    }, input)
```

This requires `Execute` to take a `proposeCtx`-like value. Refactor: rename `Execute(ctx, db, name, input)` to `Execute(ctx, ec ExecCtx, name, input)` where `ExecCtx` carries `DB, Store, Gateway, UserID, ConnID, DefaultDB`. Update callers in `orchestrator.go` accordingly.

Equivalent change in `orchestrator_mcp.go`'s `executeMCP`.

- [ ] **Step 5: Run tests**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/chat/ -run TestProposeWrite -v`
Expected: 5 tests PASS.

Run: `cd /home/conray/project/mysqlweb && go test ./internal/chat/...` to ensure earlier tests still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/chat/propose.go internal/chat/propose_test.go internal/chat/tools.go internal/chat/execute.go internal/chat/orchestrator.go internal/chat/orchestrator_mcp.go
git -c commit.gpgsign=false commit -m "feat(chat): propose_write tool + gateway-driven approval flow"
```

---

## Task 11: Wire the WS handler as the ProposalGateway

**Files:**
- Modify: `internal/api/chat.go`

- [ ] **Step 1: Sketch the gateway implementation in `internal/api/chat.go`**

Add inside the `handleWSChat` function, after the connection + LLM are resolved (around line 122 in the current file):

```go
// Per-WS state for in-flight write proposals.
type pendingState struct {
    mu      sync.Mutex
    chans   map[string]chan chat.Decision
}
pending := &pendingState{chans: map[string]chan chat.Decision{}}

gw := wsGateway{
    write:   func(m chatMsg) error { return wsjson.Write(ctx, c, m) },
    pending: pending,
}
```

Define `wsGateway` near the top of the file or in a new `internal/api/chat_gateway.go`:

```go
type wsGateway struct {
    write   func(chatMsg) error
    pending *pendingState
}

func (g wsGateway) Propose(ctx context.Context, p chat.Proposal) (chat.Decision, error) {
    ch := make(chan chat.Decision, 1)
    g.pending.mu.Lock()
    g.pending.chans[p.ID] = ch
    g.pending.mu.Unlock()
    defer func() {
        g.pending.mu.Lock()
        delete(g.pending.chans, p.ID)
        g.pending.mu.Unlock()
    }()
    if err := g.write(chatMsg{
        Type: "write_proposed", ToolUseID: p.ID, // reuse field for proposal_id transport
        ToolName: p.Operation, Output: p.SQL, Message: p.ExplainSummary,
        Text: p.Database + "." + p.Table,
    }); err != nil {
        return chat.Decision{}, err
    }
    select {
    case d := <-ch:
        return d, nil
    case <-ctx.Done():
        return chat.Decision{}, ctx.Err()
    }
}
```

(Reusing existing `chatMsg` fields is the pragmatic minimum — the frontend will key off `type`. If you want cleaner JSON, add explicit `ProposalID/DB/Table/Op/SQL/Explain` fields to `chatMsg`.)

- [ ] **Step 2: Add a sibling goroutine that reads further client envelopes for `execute_write`**

After the `wsjson.Read(ctx, c, &req)` line that took the first `exec` envelope, start a reader goroutine BEFORE running the orchestrator:

```go
go func() {
    for {
        var m chatExecReq
        if err := wsjson.Read(ctx, c, &m); err != nil { return }
        if m.Type == "execute_write" {
            pending.mu.Lock()
            ch, ok := pending.chans[m.ProposalID]
            pending.mu.Unlock()
            if ok {
                ch <- chat.Decision{Accept: m.Accept}
            }
        } else if m.Type == "cancel" {
            cancel()
            return
        }
    }
}()
```

Add new fields to `chatExecReq`:

```go
type chatExecReq struct {
    Type        string `json:"type"`
    ConnID      int64  `json:"conn_id"`
    DB          string `json:"db"`
    Provider    string `json:"provider"`
    Messages    []llm.Message `json:"messages"`
    ProposalID  string `json:"proposal_id,omitempty"`
    Accept      bool   `json:"accept,omitempty"`
}
```

- [ ] **Step 3: Pass the gateway + store + user/conn IDs into the orchestrator deps**

Update the orchestrator call sites in `handleWSChat`:

```go
// look up master switch up front to decide tool list
masterOn, _ := d.Store.GetAIWritesEnabled(u.ID)

if d.MCP != nil {
    // ...existing dsnName/AddConnection
    events, runErr = chat.RunMCP(ctx, chat.MCPDeps{
        LLM: llmClient, MCP: d.MCP, DSNName: dsnName,
        DB: db, Store: d.Store, Gateway: gw,
        UserID: u.ID, ConnID: req.ConnID, DefaultDB: req.DB,
        IncludeProposeWrite: masterOn,
    }, chat.Input{Messages: req.Messages})
} else {
    events, runErr = chat.Run(ctx, chat.Deps{
        LLM: llmClient, DB: db, Store: d.Store, Gateway: gw,
        UserID: u.ID, ConnID: req.ConnID, DefaultDB: req.DB,
        IncludeProposeWrite: masterOn,
    }, chat.Input{Messages: req.Messages})
}
```

Add `IncludeProposeWrite bool` to both `Deps` and `MCPDeps`; wire it into `Tools(ToolOpts{IncludeProposeWrite: d.IncludeProposeWrite})` inside the orchestrators.

- [ ] **Step 4: Compile**

Run: `cd /home/conray/project/mysqlweb && go build ./...`

- [ ] **Step 5: Quick smoke test for the new envelope shape**

Run: `cd /home/conray/project/mysqlweb && go test ./internal/api/... ./internal/chat/...`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/chat.go internal/chat/orchestrator.go internal/chat/orchestrator_mcp.go internal/chat/tools.go
git -c commit.gpgsign=false commit -m "feat(api): WS chat ProposalGateway routes execute_write to orchestrator"
```

---

## Task 12: Frontend — i18n keys + CSS warning variable + TS types

**Files:**
- Modify: `web/src/i18n/messages.ts`
- Modify: `web/src/index.css` (or wherever `--bg-primary` lives — `grep -rn 'bg-primary' web/src/`)

- [ ] **Step 1: Find where existing CSS vars are defined**

Run: `grep -rln '\-\-bg-primary' /home/conray/project/mysqlweb/web/src`

- [ ] **Step 2: Add `--bg-warning` in both dark and light themes**

In the file from Step 1, alongside `--bg-primary`/`--bg-secondary` for the dark theme:

```css
--bg-warning: #3b1f1a;     /* deep warm bg for dark mode */
--fg-warning: #ffd9b3;     /* warning text on --bg-warning */
--border-warning: #6e3b2a;
```

And for the light theme block:

```css
--bg-warning: #fff6e0;
--fg-warning: #6a3b00;
--border-warning: #f0c97a;
```

- [ ] **Step 3: Append i18n keys (EN block first)**

In `web/src/i18n/messages.ts`, EN section (around `'edit.insert_failed'` neighborhood — add a new section):

```ts
// AI write permissions (Settings)
'settings.ai_writes.title':           'AI Write Permissions',
'settings.ai_writes.master_label':    'Allow AI to propose writes',
'settings.ai_writes.master_hint_off': 'When off, the AI can read your data but cannot propose any INSERT / UPDATE / DELETE / DDL.',
'settings.ai_writes.connection':      'Connection',
'settings.ai_writes.database':        'Database',
'settings.ai_writes.configured':      'Configured ({count})',
'settings.ai_writes.unconfigured':    'Unconfigured ({count})',
'settings.ai_writes.col.ins':         'INS',
'settings.ai_writes.col.upd':         'UPD',
'settings.ai_writes.col.del':         'DEL',
'settings.ai_writes.col.ddl':         'DDL',
'settings.ai_writes.col.select_all':  'all',
'settings.ai_writes.batch_apply':     'Apply to {n} selected',
'settings.ai_writes.audit_title':     'Recent activity (50)',
'settings.ai_writes.status.executed': 'executed',
'settings.ai_writes.status.denied':   'denied',
'settings.ai_writes.status.cancelled':'cancelled',
'settings.ai_writes.status.failed':   'failed',
'settings.ai_writes.status.proposed': 'proposed',

// Chat write proposal card
'chat.proposal.title':           'AI wants to run a {op} on {db}.{table}',
'chat.proposal.execute':         'Execute',
'chat.proposal.cancel':          'Cancel',
'chat.proposal.executed':        '✓ Executed ({rows} rows affected)',
'chat.proposal.failed':          '✗ Failed: {error}',
'chat.proposal.cancelled':       '— Cancelled',
'chat.proposal.explain':         'EXPLAIN',
'chat.proposal.explain_failed':  '⚠ EXPLAIN failed: {error}',
```

Then the zh-TW block (same shape, translated). Insert into the matching zh-TW area:

```ts
'settings.ai_writes.title':           'AI 寫入權限',
'settings.ai_writes.master_label':    '允許 AI 提議寫入',
'settings.ai_writes.master_hint_off': '關閉時，AI 仍可讀資料，但無法提議任何 INSERT / UPDATE / DELETE / DDL。',
'settings.ai_writes.connection':      '連線',
'settings.ai_writes.database':        '資料庫',
'settings.ai_writes.configured':      '已設定 ({count})',
'settings.ai_writes.unconfigured':    '未設定 ({count})',
'settings.ai_writes.col.ins':         '新增',
'settings.ai_writes.col.upd':         '修改',
'settings.ai_writes.col.del':         '刪除',
'settings.ai_writes.col.ddl':         '結構',
'settings.ai_writes.col.select_all':  '全選',
'settings.ai_writes.batch_apply':     '套用到選取的 {n} 個 table',
'settings.ai_writes.audit_title':     '最近 50 筆稽核紀錄',
'settings.ai_writes.status.executed': '已執行',
'settings.ai_writes.status.denied':   '拒絕',
'settings.ai_writes.status.cancelled':'取消',
'settings.ai_writes.status.failed':   '失敗',
'settings.ai_writes.status.proposed': '提議中',

'chat.proposal.title':           'AI 想對 {db}.{table} 執行 {op}',
'chat.proposal.execute':         '執行',
'chat.proposal.cancel':          '取消',
'chat.proposal.executed':        '✓ 已執行（影響 {rows} 列）',
'chat.proposal.failed':          '✗ 失敗：{error}',
'chat.proposal.cancelled':       '— 已取消',
'chat.proposal.explain':         'EXPLAIN',
'chat.proposal.explain_failed':  '⚠ EXPLAIN 失敗：{error}',
```

- [ ] **Step 4: Build**

Run: `cd /home/conray/project/mysqlweb/web && npm run build`
Expected: builds clean. Any missing-key TS errors at this point would surface here.

- [ ] **Step 5: Commit**

```bash
git add web/src/i18n/messages.ts web/src/index.css
git -c commit.gpgsign=false commit -m "feat(web): i18n keys + --bg-warning for AI write permissions UI"
```

(If the CSS file path is different, swap `web/src/index.css` for the actual path.)

---

## Task 13: Frontend — AIWritePolicyTable component

**Files:**
- Create: `web/src/components/AIWritePolicyTable.tsx`
- Create: `web/src/components/AIWritePolicyTable.test.tsx`

- [ ] **Step 1: Write tests first**

Create `web/src/components/AIWritePolicyTable.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import AIWritePolicyTable from './AIWritePolicyTable'

const baseProps = {
  connId: 1,
  db: 'db1',
  configured: [
    { table: 't1', policy: { insert: true,  update: false, delete: false, ddl: false } },
    { table: 't2', policy: { insert: false, update: true,  delete: true,  ddl: false } },
  ],
  unconfigured: ['t3', 't4'],
  onUpsert:   vi.fn(),
  onBatch:    vi.fn(),
}

describe('AIWritePolicyTable', () => {
  it('renders configured rows with checkboxes', () => {
    render(<AIWritePolicyTable {...baseProps} />)
    expect(screen.getByText('t1')).toBeInTheDocument()
    expect(screen.getByText('t2')).toBeInTheDocument()
  })

  it('select-all toggles all four checkboxes in a row', () => {
    const props = { ...baseProps, onUpsert: vi.fn() }
    render(<AIWritePolicyTable {...props} />)
    const row = screen.getByText('t1').closest('tr')!
    const all = row.querySelector('[data-testid="select-all"]')! as HTMLInputElement
    fireEvent.click(all)
    expect(props.onUpsert).toHaveBeenCalledWith(1, 'db1', 't1',
      { insert: true, update: true, delete: true, ddl: true })
  })

  it('batch-applies to selected unconfigured tables', () => {
    const props = { ...baseProps, onBatch: vi.fn() }
    render(<AIWritePolicyTable {...props} />)
    fireEvent.click(screen.getByLabelText('select t3'))
    fireEvent.click(screen.getByTestId('batch-ins'))
    fireEvent.click(screen.getByText(/Apply to 1 selected/))
    expect(props.onBatch).toHaveBeenCalledWith(1, 'db1', ['t3'],
      { insert: true, update: false, delete: false, ddl: false })
  })
})
```

- [ ] **Step 2: Run tests, expect FAIL**

Run: `cd /home/conray/project/mysqlweb/web && npm test -- --run AIWritePolicyTable`

- [ ] **Step 3: Implement**

Create `web/src/components/AIWritePolicyTable.tsx`:

```tsx
import { useState } from 'react'
import type { CSSProperties } from 'react'
import { useT } from '../i18n'

export interface AIPolicy {
  insert: boolean
  update: boolean
  delete: boolean
  ddl: boolean
}

export interface TablePolicy {
  table: string
  policy: AIPolicy
}

interface Props {
  connId: number
  db: string
  configured: TablePolicy[]
  unconfigured: string[]
  onUpsert: (connId: number, db: string, table: string, policy: AIPolicy) => void
  onBatch: (connId: number, db: string, tables: string[], policy: AIPolicy) => void
}

const FLAGS = ['insert', 'update', 'delete', 'ddl'] as const
type Flag = (typeof FLAGS)[number]

export default function AIWritePolicyTable({ connId, db, configured, unconfigured, onUpsert, onBatch }: Props) {
  const t = useT()
  return (
    <div>
      <h4>{t('settings.ai_writes.configured', { count: configured.length })}</h4>
      <table style={tbl}>
        <thead>
          <tr>
            <th style={th}>{t('settings.ai_writes.database')}.table</th>
            <th style={th}>{t('settings.ai_writes.col.ins')}</th>
            <th style={th}>{t('settings.ai_writes.col.upd')}</th>
            <th style={th}>{t('settings.ai_writes.col.del')}</th>
            <th style={th}>{t('settings.ai_writes.col.ddl')}</th>
            <th style={th}>{t('settings.ai_writes.col.select_all')}</th>
          </tr>
        </thead>
        <tbody>
          {configured.map((row) => (
            <ConfiguredRow
              key={row.table}
              row={row}
              onChange={(p) => onUpsert(connId, db, row.table, p)}
            />
          ))}
        </tbody>
      </table>

      <h4>{t('settings.ai_writes.unconfigured', { count: unconfigured.length })}</h4>
      <UnconfiguredBatch
        connId={connId}
        db={db}
        tables={unconfigured}
        onBatch={onBatch}
      />
    </div>
  )
}

function ConfiguredRow({ row, onChange }: { row: TablePolicy; onChange: (p: AIPolicy) => void }) {
  const set = (flag: Flag, value: boolean) => {
    onChange({ ...row.policy, [flag]: value })
  }
  const allOn = FLAGS.every((f) => row.policy[f])
  const toggleAll = () => {
    const v = !allOn
    onChange({ insert: v, update: v, delete: v, ddl: v })
  }
  return (
    <tr>
      <td style={td}>{row.table}</td>
      {FLAGS.map((f) => (
        <td key={f} style={td}>
          <input
            type="checkbox"
            checked={row.policy[f]}
            onChange={(e) => set(f, e.target.checked)}
            aria-label={`${row.table} ${f}`}
          />
        </td>
      ))}
      <td style={td}>
        <input
          type="checkbox"
          checked={allOn}
          onChange={toggleAll}
          data-testid="select-all"
          aria-label={`${row.table} select-all`}
        />
      </td>
    </tr>
  )
}

function UnconfiguredBatch({ connId, db, tables, onBatch }: { connId: number; db: string; tables: string[]; onBatch: Props['onBatch'] }) {
  const t = useT()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [draft, setDraft] = useState<AIPolicy>({ insert: false, update: false, delete: false, ddl: false })

  const flip = (table: string) => {
    setSelected((s) => {
      const next = new Set(s)
      if (next.has(table)) next.delete(table); else next.add(table)
      return next
    })
  }

  return (
    <div>
      <div style={batchBar}>
        {FLAGS.map((f) => (
          <label key={f} style={{ marginRight: 8 }}>
            <input
              type="checkbox"
              checked={draft[f]}
              data-testid={`batch-${f === 'insert' ? 'ins' : f === 'update' ? 'upd' : f === 'delete' ? 'del' : 'ddl'}`}
              onChange={(e) => setDraft((d) => ({ ...d, [f]: e.target.checked }))}
            />
            {t(`settings.ai_writes.col.${f === 'insert' ? 'ins' : f === 'update' ? 'upd' : f === 'delete' ? 'del' : 'ddl'}`)}
          </label>
        ))}
        <button
          disabled={selected.size === 0}
          onClick={() => onBatch(connId, db, Array.from(selected), draft)}
        >
          {t('settings.ai_writes.batch_apply', { n: selected.size })}
        </button>
      </div>
      <table style={tbl}>
        <tbody>
          {tables.map((tname) => (
            <tr key={tname}>
              <td style={td}>
                <input
                  type="checkbox"
                  checked={selected.has(tname)}
                  onChange={() => flip(tname)}
                  aria-label={`select ${tname}`}
                />
              </td>
              <td style={td}>{tname}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const tbl: CSSProperties = { width: '100%', borderCollapse: 'collapse', fontSize: 13 }
const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border-color)' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid var(--table-border, var(--border-color))' }
const batchBar: CSSProperties = { padding: '4px 0', display: 'flex', alignItems: 'center', gap: 8 }
```

- [ ] **Step 4: Run tests, expect PASS**

Run: `cd /home/conray/project/mysqlweb/web && npm test -- --run AIWritePolicyTable`
Expected: 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/AIWritePolicyTable.tsx web/src/components/AIWritePolicyTable.test.tsx
git -c commit.gpgsign=false commit -m "feat(web): AIWritePolicyTable with per-row select-all + unconfigured batch"
```

---

## Task 14: Frontend — AIWriteAuditList + Settings integration

**Files:**
- Create: `web/src/components/AIWriteAuditList.tsx`
- Modify: `web/src/routes/Settings.tsx`

- [ ] **Step 1: AuditList component**

Create `web/src/components/AIWriteAuditList.tsx`:

```tsx
import type { CSSProperties } from 'react'
import { useT } from '../i18n'

export interface AuditRow {
  id: number
  database_name: string
  table_name: string
  operation: string
  sql_text: string
  status: 'proposed' | 'executed' | 'denied' | 'cancelled' | 'failed'
  rows_affected: number | null
  error_message: string
  created_at: string
}

const CHIP_COLOR: Record<AuditRow['status'], string> = {
  executed:  '#3a8',
  denied:    '#c44',
  cancelled: '#888',
  failed:    '#e90',
  proposed:  '#48a',
}

export default function AIWriteAuditList({ rows }: { rows: AuditRow[] }) {
  const t = useT()
  if (!rows.length) return <p style={muted}>—</p>
  return (
    <ul style={list}>
      {rows.map((r) => (
        <li key={r.id} style={item}>
          <span style={{ ...chip, background: CHIP_COLOR[r.status] }}>
            {t(`settings.ai_writes.status.${r.status}`)}
          </span>
          <span style={timeCell}>{r.created_at.slice(0, 19).replace('T', ' ')}</span>
          <span>{r.database_name}.{r.table_name}</span>
          <span style={op}>{r.operation}</span>
          {r.rows_affected != null && <span>rows={r.rows_affected}</span>}
          {r.error_message && <span style={err}>{r.error_message}</span>}
          <pre style={sql} title={r.sql_text}>{r.sql_text.slice(0, 120)}{r.sql_text.length > 120 ? '…' : ''}</pre>
        </li>
      ))}
    </ul>
  )
}

const list: CSSProperties = { listStyle: 'none', padding: 0, margin: 0, fontSize: 12 }
const item: CSSProperties = { display: 'grid', gridTemplateColumns: '80px 130px 1fr 60px auto auto 1fr', gap: 8, padding: '4px 0', borderBottom: '1px solid var(--border-color)' }
const chip: CSSProperties = { color: 'white', padding: '1px 6px', borderRadius: 4, fontSize: 11, textAlign: 'center' }
const timeCell: CSSProperties = { color: 'var(--text-muted, #888)' }
const op: CSSProperties = { fontWeight: 600 }
const err: CSSProperties = { color: '#c44', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }
const sql: CSSProperties = { margin: 0, fontFamily: 'monospace', color: 'var(--text-secondary)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }
const muted: CSSProperties = { color: 'var(--text-muted, #888)' }
```

- [ ] **Step 2: Integrate into `web/src/routes/Settings.tsx`**

Add imports and a new section near the API-keys section:

```tsx
import AIWritePolicyTable, { type AIPolicy, type TablePolicy } from '../components/AIWritePolicyTable'
import AIWriteAuditList, { type AuditRow } from '../components/AIWriteAuditList'
```

State + loaders inside the `Settings` component:

```tsx
const [aiEnabled, setAiEnabled] = useState(false)
const [audit, setAudit] = useState<AuditRow[]>([])
const [connections, setConnections] = useState<{ id: number; name: string }[]>([])
const [selectedConn, setSelectedConn] = useState<number | null>(null)
const [databases, setDatabases] = useState<string[]>([])
const [selectedDb, setSelectedDb] = useState<string | null>(null)
const [policy, setPolicy] = useState<{ configured: TablePolicy[]; unconfigured: string[] }>(
  { configured: [], unconfigured: [] }
)

async function loadMaster() {
  const r = await api.get<{ enabled: boolean }>('/api/auth/ai-writes')
  setAiEnabled(r.enabled)
}
async function toggleMaster(v: boolean) {
  await api.put('/api/auth/ai-writes', { enabled: v })
  setAiEnabled(v)
  if (v) {
    await loadConnections()
    await loadAudit()
  }
}
async function loadConnections() {
  const list = await api.get<{ id: number; name: string }[]>('/api/connections')
  setConnections(list)
}
async function loadDatabases(connId: number) {
  const r = await api.get<{ databases: string[] }>(`/api/db/${connId}/databases`)
  setDatabases(r.databases)
}
async function loadPolicy(connId: number, db: string) {
  const r = await api.get<typeof policy>(`/api/auth/ai-policy?conn=${connId}&db=${encodeURIComponent(db)}`)
  setPolicy({ configured: r.configured ?? [], unconfigured: r.unconfigured ?? [] })
}
async function loadAudit() {
  const rows = await api.get<AuditRow[]>('/api/auth/ai-audit?limit=50')
  setAudit(rows ?? [])
}
async function upsertPolicy(connId: number, db: string, table: string, p: AIPolicy) {
  await api.put('/api/auth/ai-policy', { conn: connId, db, table, policy: p })
  await loadPolicy(connId, db)
}
async function batchPolicy(connId: number, db: string, tables: string[], p: AIPolicy) {
  await api.put('/api/auth/ai-policy/batch', { conn: connId, db, tables, policy: p })
  await loadPolicy(connId, db)
}

useEffect(() => { void loadMaster() }, [])
useEffect(() => { if (selectedConn) void loadDatabases(selectedConn) }, [selectedConn])
useEffect(() => {
  if (selectedConn && selectedDb) void loadPolicy(selectedConn, selectedDb)
}, [selectedConn, selectedDb])
```

JSX block to insert inside the Settings render (above API keys):

```tsx
<section style={section}>
  <h3>{t('settings.ai_writes.title')}</h3>
  <label>
    <input type="checkbox" checked={aiEnabled} onChange={(e) => void toggleMaster(e.target.checked)} />
    {t('settings.ai_writes.master_label')}
  </label>
  {!aiEnabled && <p style={hint}>{t('settings.ai_writes.master_hint_off')}</p>}
  {aiEnabled && (
    <div>
      <div style={pickerRow}>
        <label>
          {t('settings.ai_writes.connection')}:&nbsp;
          <select value={selectedConn ?? ''} onChange={(e) => setSelectedConn(Number(e.target.value) || null)}>
            <option value="">—</option>
            {connections.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        </label>
        <label>
          {t('settings.ai_writes.database')}:&nbsp;
          <select value={selectedDb ?? ''} onChange={(e) => setSelectedDb(e.target.value || null)} disabled={!selectedConn}>
            <option value="">—</option>
            {databases.map((d) => <option key={d} value={d}>{d}</option>)}
          </select>
        </label>
      </div>
      {selectedConn && selectedDb && (
        <AIWritePolicyTable
          connId={selectedConn}
          db={selectedDb}
          configured={policy.configured}
          unconfigured={policy.unconfigured}
          onUpsert={upsertPolicy}
          onBatch={batchPolicy}
        />
      )}
      <h4>{t('settings.ai_writes.audit_title')}</h4>
      <AIWriteAuditList rows={audit} />
    </div>
  )}
</section>
```

Add styles near the bottom of the file:

```tsx
const hint: CSSProperties = { color: 'var(--text-muted, #888)', fontSize: 12 }
const pickerRow: CSSProperties = { display: 'flex', gap: 16, alignItems: 'center', padding: '8px 0' }
```

(`section` style may already exist; if not, copy the pattern from the API-keys section.)

- [ ] **Step 3: Run the existing Settings tests if any**

Run: `cd /home/conray/project/mysqlweb/web && npm test -- --run Settings || true`

- [ ] **Step 4: Build**

Run: `cd /home/conray/project/mysqlweb/web && npm run build`

- [ ] **Step 5: Commit**

```bash
git add web/src/components/AIWriteAuditList.tsx web/src/routes/Settings.tsx
git -c commit.gpgsign=false commit -m "feat(web): Settings AI writes section + audit list integration"
```

---

## Task 15: Frontend — WriteProposalCard + ChatPanel wiring

**Files:**
- Create: `web/src/components/WriteProposalCard.tsx`
- Create: `web/src/components/WriteProposalCard.test.tsx`
- Modify: `web/src/components/ChatPanel.tsx`

- [ ] **Step 1: Card tests**

Create `web/src/components/WriteProposalCard.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import WriteProposalCard from './WriteProposalCard'

const base = {
  proposalId: 'p1',
  db: 'fatgame_development',
  table: 'users',
  op: 'UPDATE',
  sql: 'UPDATE `users` SET is_admin=0 WHERE id=42',
  explainSummary: '',
  status: 'proposed' as const,
}

describe('WriteProposalCard', () => {
  it('renders SQL + Execute/Cancel', () => {
    render(<WriteProposalCard {...base} onDecision={vi.fn()} />)
    expect(screen.getByText(/UPDATE `users`/)).toBeInTheDocument()
    expect(screen.getByText(/Execute/)).toBeInTheDocument()
    expect(screen.getByText(/Cancel/)).toBeInTheDocument()
  })

  it('calls onDecision(true) on Execute click', () => {
    const onDecision = vi.fn()
    render(<WriteProposalCard {...base} onDecision={onDecision} />)
    fireEvent.click(screen.getByText(/Execute/))
    expect(onDecision).toHaveBeenCalledWith('p1', true)
  })

  it('shows executed chip when status flips', () => {
    render(<WriteProposalCard {...base} status="executed" rowsAffected={3} onDecision={vi.fn()} />)
    expect(screen.getByText(/✓ Executed/)).toBeInTheDocument()
  })

  it('shows EXPLAIN error banner when explainSummary has error', () => {
    render(<WriteProposalCard {...base} explainSummary='{"error":"syntax"}' onDecision={vi.fn()} />)
    expect(screen.getByText(/EXPLAIN failed/)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run tests, expect FAIL**

Run: `cd /home/conray/project/mysqlweb/web && npm test -- --run WriteProposalCard`

- [ ] **Step 3: Implement card**

Create `web/src/components/WriteProposalCard.tsx`:

```tsx
import type { CSSProperties } from 'react'
import { useT } from '../i18n'

export type ProposalStatus = 'proposed' | 'executed' | 'failed' | 'cancelled'

interface Props {
  proposalId: string
  db: string
  table: string
  op: string
  sql: string
  explainSummary: string
  status: ProposalStatus
  rowsAffected?: number
  errorMessage?: string
  onDecision: (proposalId: string, accept: boolean) => void
}

export default function WriteProposalCard(p: Props) {
  const t = useT()
  let explainNode: React.ReactNode = null
  if (p.explainSummary) {
    try {
      const obj = JSON.parse(p.explainSummary)
      if (obj.error) {
        explainNode = <p style={warn}>{t('chat.proposal.explain_failed', { error: obj.error })}</p>
      } else if (Array.isArray(obj.rows) && obj.rows.length) {
        explainNode = (
          <pre style={pre}>{t('chat.proposal.explain')}: {JSON.stringify(obj.rows, null, 2)}</pre>
        )
      }
    } catch {
      // ignore parse error
    }
  }

  const isDone = p.status !== 'proposed'
  return (
    <div style={card}>
      <header style={title}>
        {t('chat.proposal.title', { op: p.op, db: p.db, table: p.table })}
      </header>
      <pre style={pre}>{p.sql}</pre>
      {explainNode}
      {!isDone && (
        <div style={actions}>
          <button onClick={() => p.onDecision(p.proposalId, true)}>{t('chat.proposal.execute')}</button>
          <button onClick={() => p.onDecision(p.proposalId, false)}>{t('chat.proposal.cancel')}</button>
        </div>
      )}
      {p.status === 'executed' && <p style={ok}>{t('chat.proposal.executed', { rows: p.rowsAffected ?? 0 })}</p>}
      {p.status === 'failed'   && <p style={bad}>{t('chat.proposal.failed', { error: p.errorMessage ?? '' })}</p>}
      {p.status === 'cancelled'&& <p style={muted}>{t('chat.proposal.cancelled')}</p>}
    </div>
  )
}

const card: CSSProperties = {
  background: 'var(--bg-warning)',
  color: 'var(--fg-warning)',
  border: '1px solid var(--border-warning)',
  borderRadius: 6,
  padding: 10,
  margin: '6px 0',
}
const title: CSSProperties = { fontWeight: 600, marginBottom: 6 }
const pre: CSSProperties = { background: 'rgba(0,0,0,0.25)', padding: 8, borderRadius: 4, fontFamily: 'monospace', fontSize: 12, overflowX: 'auto', margin: '4px 0' }
const actions: CSSProperties = { display: 'flex', gap: 8, marginTop: 6 }
const ok: CSSProperties  = { color: '#3a8',  margin: 0, fontWeight: 600 }
const bad: CSSProperties = { color: '#c44',  margin: 0, fontWeight: 600 }
const muted: CSSProperties = { color: 'var(--text-muted, #888)', margin: 0 }
const warn: CSSProperties = { color: '#e90', fontSize: 12, margin: 0 }
```

- [ ] **Step 4: Wire ChatPanel.tsx to handle new WS messages**

In `web/src/components/ChatPanel.tsx` (study the existing WS message handling first via `grep -n "onmessage\|message.type\|wsjson" web/src/components/ChatPanel.tsx`):

1. Extend the `ChatEvent` union to include:

```ts
| { type: 'write_proposed';  proposalId: string; db: string; table: string; op: string; sql: string; explainSummary: string }
| { type: 'write_executed';  proposalId: string; rowsAffected: number }
| { type: 'write_failed';    proposalId: string; error: string }
| { type: 'write_cancelled'; proposalId: string }
```

2. In the WS `onmessage` handler, route the new types. The current handler likely already keys off `msg.type`. Map server fields onto the union (server uses `ToolUseID` for `proposalId` because we reused the existing field shape):

```ts
case 'write_proposed': {
  const proposalId = msg.tool_use_id as string
  const [db, table] = (msg.text ?? '').split('.')
  addEvent({ type: 'write_proposed', proposalId,
             db, table, op: msg.tool_name ?? '',
             sql: msg.output ?? '',
             explainSummary: msg.message ?? '' })
  break
}
case 'write_executed':
  updateProposal(msg.tool_use_id as string, (e) => ({ ...e, status: 'executed', rowsAffected: msg.rows_affected }))
  break
case 'write_failed':
  updateProposal(msg.tool_use_id as string, (e) => ({ ...e, status: 'failed', errorMessage: msg.message }))
  break
case 'write_cancelled':
  updateProposal(msg.tool_use_id as string, (e) => ({ ...e, status: 'cancelled' }))
  break
```

3. Add a `sendDecision` helper that posts:

```ts
function sendDecision(proposalId: string, accept: boolean) {
  ws.send(JSON.stringify({ type: 'execute_write', proposal_id: proposalId, accept }))
  updateProposal(proposalId, (e) => ({ ...e, decisionSent: accept ? 'execute' : 'cancel' }))
}
```

4. In the JSX render loop, when the event type is `write_proposed`, render `<WriteProposalCard ...event onDecision={sendDecision} />`.

(The exact patch surface depends on the current `ChatPanel.tsx` shape — keep changes minimal and follow existing conventions.)

- [ ] **Step 5: Run frontend tests**

Run: `cd /home/conray/project/mysqlweb/web && npm test -- --run WriteProposalCard`

Run full suite: `cd /home/conray/project/mysqlweb/web && npm test -- --run`

- [ ] **Step 6: Build**

Run: `cd /home/conray/project/mysqlweb/web && npm run build`

- [ ] **Step 7: Commit**

```bash
git add web/src/components/WriteProposalCard.tsx web/src/components/WriteProposalCard.test.tsx web/src/components/ChatPanel.tsx
git -c commit.gpgsign=false commit -m "feat(web): WriteProposalCard + ChatPanel wiring for write proposals"
```

---

## Task 16: Wire it all + smoke test on the live server

**Files:**
- None (operational)

- [ ] **Step 1: Rebuild Go binary with embedded frontend**

Run:
```
cd /home/conray/project/mysqlweb/web && npm run build && \
cd /home/conray/project/mysqlweb && go build -o bin/dataseai ./cmd/dataseai
```

- [ ] **Step 2: Restart server using the CORRECT DB path**

Run:
```
kill $(cat /home/conray/project/mysqlweb/.mysqlweb.pid) 2>/dev/null; sleep 1
cd /home/conray/project/mysqlweb && setsid env MYSQLWEB_DB_PATH=./data/dataseai.db MYSQLWEB_PORT=53306 ./bin/dataseai > logs/mysqlweb.log 2>&1 < /dev/null & disown
sleep 1; pgrep -nf './bin/dataseai' > /home/conray/project/mysqlweb/.mysqlweb.pid
curl -sf http://127.0.0.1:53306/api/health && echo OK
```

- [ ] **Step 3: End-to-end walkthrough on `dataseai.conray.top`**

  - Open Settings → AI 寫入權限 section.
  - Toggle master switch ON.
  - Pick connection `nas` → DB `fatgame_development`.
  - In "未設定", select `categories` → tick `INSERT` → click 套用.
  - Confirm row moves to "已設定" with INS=☑.
  - Open Chat. Ask: "請幫我新增一筆 categories 紀錄 name='qa-test'".
  - Expect a `WriteProposalCard` rendered with the INSERT SQL and `[Execute] [Cancel]`.
  - Click `Execute` → expect chip "✓ 已執行（影響 1 列）" and audit list shows new `executed` row.
  - In Settings, uncheck INS for `categories`.
  - Ask AI again to insert. Expect AI to refuse with the deny hint (no card surfaced).
  - In Settings audit list, confirm a `denied` row appeared.
  - Toggle master OFF. Expect chat AI to refuse explaining its tool surface no longer offers `propose_write`.

- [ ] **Step 4: Final commit (smoke notes)**

Add a brief note to README if you like; otherwise no commit needed for this task — it's verification.

---

## Self-Review

### Spec coverage scan

- §5 Storage: Tasks 1, 2, 3, 4 cover migration + master switch + policy CRUD + audit. ✓
- §6.1 Classifier: Task 5. ✓
- §6.2 Policy.Check: Task 6. ✓
- §6.3 Audit helpers: Task 4. ✓
- §6.4 Tool surface (`run_sql` hardened + `propose_write`): Tasks 8, 10. ✓
- §6.5 Orchestrator gateway + execute flow: Tasks 9, 10, 11. ✓
- §6.6 WS protocol (write_proposed / execute_write / etc): Task 11. ✓
- §6.7 HTTP endpoints: Task 7. ✓
- §7.1 Settings UI: Tasks 13, 14. ✓
- §7.2 WriteProposalCard: Task 15. ✓
- §7.3 i18n + CSS: Task 12. ✓
- §8 Edge cases: covered by Task 5 tests (multi/classify), Task 10 tests (multi/classify mismatch, policy revoke, cancel), Task 6 tests (cross-user / DELETE→TRUNCATE / DDL aliasing). ✓
- §10 Testing strategy: every backend package has a `_test.go` task. Frontend components have `.test.tsx`. Integration via Task 16 smoke. ✓

### Placeholder check

No `TBD` / `TODO` / "add error handling" / "similar to" anywhere. Each step shows the actual code. The CSS file path in Task 12 says "or wherever `--bg-primary` lives — grep" because we don't know for certain; the engineer is given the exact grep to run.

### Type consistency

- `AIPolicy` (Go: `store.AIPolicy`) → frontend `AIPolicy` interface — same four bool fields named `Insert/insert`, etc. ✓
- `ProposalGateway.Propose` returns `chat.Decision` → WS layer pumps into channel → consumed by orchestrator. ✓
- `propose_write` operation strings match across LLM tool schema (Task 10) and `opFromDecl` helper. ✓
- The endpoint URLs in Task 7 match Task 14 frontend calls (`/api/auth/ai-writes`, `/api/auth/ai-policy`, `/api/auth/ai-policy/batch`, `/api/auth/ai-audit`). ✓
- `chatExecReq.ProposalID` (Task 11) matches `proposal_id` JSON field used by frontend `sendDecision` (Task 15). ✓
