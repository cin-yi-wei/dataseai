# Plan 3 — SQL Editor + Query History + Schema Views Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A user can write ad-hoc SQL in a CodeMirror editor, run it (short-path 5 s timeout, 10 MB / 10 000 row cap), see results in a grid, browse history, and view a table's schema (CREATE TABLE, indexes, foreign keys).

**Architecture:** Schema views are read-only SQL against `information_schema` exposed through `resolveConn` from Plan 2. The query endpoint classifies a statement (SELECT-ish → `Query` / DML-DDL → `Exec`), bounds it with `context.WithTimeout`, caps rows server-side, and logs every run to a per-user `query_history` table (also pruned to `MYSQLWEB_HISTORY_MAX`). Frontend uses CodeMirror 6 via `@uiw/react-codemirror` and the `lang-sql` package, embedded in the right-group "SQL Editor" bottom tab. Three new left-group tabs (Structure / Indexes / FKs) become enabled.

**Tech Stack:**
- Go: existing `database/sql` + `internal/mysql.Pool` from Plan 2
- Frontend: `@uiw/react-codemirror`, `@codemirror/lang-sql`, existing TanStack Table, Zustand
- All existing primitives reused: `resolveConn`, `connSession`, `QuoteIdent`, `BuildDSN`

**Spec reference:** `docs/superpowers/specs/2026-06-03-dataseai-design.md` — Sections 5 (`query_history` table), 6.3 (structure/indexes/fks), 6.5 (POST /api/query short path, response/cap rules), 6.6 (history endpoints), 8.1 (short-query path), 8.5 (identifier escaping).

**Plan 2 carryover (still open):**
- I2 — middleware 5-second cache (perf; skip)
- Browse handler 500 leaks raw driver error string (cosmetic; skip)
- Spec §6.3 query-param drift (`per_page`/`sort_col` vs spec `pageSize`/`sort=col:dir`) — Plan 3 keeps the implemented names for consistency with the React DataGrid

**Out of scope (Plan 4):** long-query WebSocket / streaming, query cancellation via KILL QUERY, DML cell edit / row insert/delete, CSV/SQL import-export, multi-tab top bar.

---

## File Structure (created or modified by this plan)

```
dataseai/
├── internal/
│   ├── store/
│   │   ├── migrations/0005_query_history.sql      # new
│   │   ├── history.go                             # new
│   │   └── history_test.go                        # new
│   ├── mysql/
│   │   ├── schema.go                              # new — Structure/Indexes/FKs introspection
│   │   ├── ident.go                               # untouched (reused)
│   │   └── query.go                               # new — query classifier + execute helper
│   └── api/
│       ├── db.go                                  # extended (structure/indexes/fks handlers)
│       ├── db_test.go                             # extended
│       ├── query.go                               # new — POST /api/query handler
│       ├── query_test.go                          # new
│       ├── history.go                             # new — GET/DELETE history handlers
│       ├── history_test.go                        # new
│       └── router.go                              # extended (8 new routes)
├── web/
│   ├── package.json                               # add @uiw/react-codemirror + @codemirror/lang-sql
│   └── src/
│       ├── store/
│       │   └── editor.ts                          # new — global editor draft + last-result Zustand
│       ├── components/
│       │   ├── SqlEditor.tsx                      # new — CodeMirror + Run button
│       │   ├── ResultPanel.tsx                    # new — result grid + status line
│       │   ├── StructureView.tsx                  # new
│       │   ├── IndexesView.tsx                    # new
│       │   ├── ForeignKeysView.tsx                # new
│       │   ├── QueryHistory.tsx                   # new — modal
│       │   └── BottomTabs.tsx                     # extended — enable left-group + right-group tabs
│       └── routes/Workspace.tsx                   # rewritten — route bottom tabs to new views
```

**Conventions reused from Plans 1 & 2:**
- Module path `github.com/conray/dataseai`
- `_test.go` alongside source; in-memory sqlite for store tests; sqlite-as-MySQL stub for handler routing tests
- Per-task TDD discipline; one commit per task
- Subagents must NOT modify the orchestrator task list

---

## Task 1: query_history table + store

**Files:**
- Create: `internal/store/migrations/0005_query_history.sql`, `internal/store/history.go`, `internal/store/history_test.go`

- [ ] **Step 1: Migration**

Create `internal/store/migrations/0005_query_history.sql`:

```sql
CREATE TABLE query_history (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  connection_id INTEGER NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  database_name TEXT,
  sql_text      TEXT NOT NULL,
  duration_ms   INTEGER,
  rows_affected INTEGER,
  error_message TEXT,
  source        TEXT NOT NULL DEFAULT 'user',
  executed_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_history_user_time ON query_history(user_id, executed_at DESC);
```

- [ ] **Step 2: Failing tests at `internal/store/history_test.go`**

```go
package store

import (
	"testing"
)

func setupHistory(t *testing.T) (*Store, User, int64) {
	t.Helper()
	s, u, c := setupConnections(t)
	conn, err := s.CreateConnection(c, u.ID, ConnectionInput{Name: "x", Host: "h", Port: 3306, Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	return s, u, conn.ID
}

func TestAddHistory_Persists(t *testing.T) {
	s, u, connID := setupHistory(t)
	err := s.AddHistory(HistoryInput{
		UserID: u.ID, ConnectionID: connID, DatabaseName: "demo",
		SQLText: "SELECT 1", DurationMs: 5, RowsAffected: 0, Source: "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListHistory(u.ID, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d entries", len(list))
	}
	if list[0].SQLText != "SELECT 1" || list[0].DurationMs != 5 {
		t.Fatalf("entry = %+v", list[0])
	}
}

func TestListHistory_ScopedToUser(t *testing.T) {
	s, alice, aliceConn := setupHistory(t)
	bob, _ := s.CreateUser("bob", "anothersecret456")
	bobConn, _ := s.CreateConnection(newCipher(t), bob.ID, ConnectionInput{Name: "b", Host: "h", Port: 3306, Username: "u", Password: "p"})
	_ = s.AddHistory(HistoryInput{UserID: alice.ID, ConnectionID: aliceConn, SQLText: "a-query"})
	_ = s.AddHistory(HistoryInput{UserID: bob.ID, ConnectionID: bobConn.ID, SQLText: "b-query"})
	list, _ := s.ListHistory(alice.ID, 50, 0)
	if len(list) != 1 || list[0].SQLText != "a-query" {
		t.Fatalf("alice sees %+v", list)
	}
}

func TestDeleteHistoryEntry(t *testing.T) {
	s, u, connID := setupHistory(t)
	_ = s.AddHistory(HistoryInput{UserID: u.ID, ConnectionID: connID, SQLText: "x"})
	list, _ := s.ListHistory(u.ID, 50, 0)
	if err := s.DeleteHistoryEntry(u.ID, list[0].ID); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListHistory(u.ID, 50, 0)
	if len(list) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list))
	}
}

func TestDeleteHistoryEntry_CrossUserBlocked(t *testing.T) {
	s, alice, aliceConn := setupHistory(t)
	bob, _ := s.CreateUser("bob", "anothersecret456")
	_ = s.AddHistory(HistoryInput{UserID: alice.ID, ConnectionID: aliceConn, SQLText: "a"})
	list, _ := s.ListHistory(alice.ID, 50, 0)
	// bob tries to delete alice's entry
	if err := s.DeleteHistoryEntry(bob.ID, list[0].ID); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestClearHistory(t *testing.T) {
	s, u, connID := setupHistory(t)
	for i := 0; i < 5; i++ {
		_ = s.AddHistory(HistoryInput{UserID: u.ID, ConnectionID: connID, SQLText: "q"})
	}
	if err := s.ClearHistory(u.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListHistory(u.ID, 50, 0)
	if len(list) != 0 {
		t.Fatalf("expected 0 after clear, got %d", len(list))
	}
}

func TestAddHistory_PrunesOverCap(t *testing.T) {
	s, u, connID := setupHistory(t)
	for i := 0; i < 12; i++ {
		_ = s.AddHistoryWithCap(HistoryInput{UserID: u.ID, ConnectionID: connID, SQLText: "q"}, 10)
	}
	list, _ := s.ListHistory(u.ID, 50, 0)
	if len(list) != 10 {
		t.Fatalf("expected 10 entries after prune cap=10, got %d", len(list))
	}
}
```

- [ ] **Step 3: Verify fail**

```bash
go test ./internal/store/ -run History -v
```

Expected: FAIL — `History*` undefined.

- [ ] **Step 4: Implement at `internal/store/history.go`**

```go
package store

import (
	"database/sql"
	"time"
)

type HistoryInput struct {
	UserID       int64
	ConnectionID int64
	DatabaseName string
	SQLText      string
	DurationMs   int64
	RowsAffected int64
	ErrorMessage string
	Source       string // "user" | "ai"
}

type History struct {
	ID           int64
	UserID       int64
	ConnectionID int64
	DatabaseName string
	SQLText      string
	DurationMs   int64
	RowsAffected int64
	ErrorMessage string
	Source       string
	ExecutedAt   time.Time
}

// AddHistory persists a query history entry. No pruning. Use AddHistoryWithCap
// for retention enforcement.
func (s *Store) AddHistory(in HistoryInput) error {
	if in.Source == "" {
		in.Source = "user"
	}
	_, err := s.DB.Exec(
		`INSERT INTO query_history(user_id, connection_id, database_name, sql_text, duration_ms, rows_affected, error_message, source)
		 VALUES (?,?,?,?,?,?,?,?)`,
		in.UserID, in.ConnectionID, in.DatabaseName, in.SQLText,
		in.DurationMs, in.RowsAffected, in.ErrorMessage, in.Source,
	)
	return err
}

// AddHistoryWithCap inserts an entry then deletes anything beyond `max` per-user
// rows, keeping the newest. Cap <= 0 disables pruning.
func (s *Store) AddHistoryWithCap(in HistoryInput, max int) error {
	if err := s.AddHistory(in); err != nil {
		return err
	}
	if max <= 0 {
		return nil
	}
	_, err := s.DB.Exec(
		`DELETE FROM query_history
		 WHERE user_id = ?
		   AND id NOT IN (
		     SELECT id FROM query_history WHERE user_id = ?
		     ORDER BY executed_at DESC, id DESC
		     LIMIT ?
		   )`,
		in.UserID, in.UserID, max,
	)
	return err
}

func (s *Store) ListHistory(userID int64, limit, offset int) ([]History, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.DB.Query(
		`SELECT id, user_id, connection_id, database_name, sql_text,
		        duration_ms, rows_affected, error_message, source, executed_at
		 FROM query_history WHERE user_id = ?
		 ORDER BY executed_at DESC, id DESC LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []History
	for rows.Next() {
		var h History
		var dbName, errMsg sql.NullString
		if err := rows.Scan(&h.ID, &h.UserID, &h.ConnectionID, &dbName,
			&h.SQLText, &h.DurationMs, &h.RowsAffected, &errMsg,
			&h.Source, &h.ExecutedAt); err != nil {
			return nil, err
		}
		h.DatabaseName = dbName.String
		h.ErrorMessage = errMsg.String
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) DeleteHistoryEntry(userID, id int64) error {
	res, err := s.DB.Exec("DELETE FROM query_history WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearHistory(userID int64) error {
	_, err := s.DB.Exec("DELETE FROM query_history WHERE user_id = ?", userID)
	return err
}
```

- [ ] **Step 5: Verify pass**

```bash
go test ./internal/store/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat(store): query_history table + per-user CRUD with retention cap"
```

---

## Task 2: Schema introspection helpers

**Files:**
- Create: `internal/mysql/schema.go`

(No test for this file — same rationale as `browse.go`: it issues `information_schema` queries that sqlite can't service, manual smoke covers the happy path.)

- [ ] **Step 1: Implement**

Create `internal/mysql/schema.go`:

```go
package mysql

import (
	"context"
	"database/sql"
)

type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Default  string `json:"default"`
	Extra    string `json:"extra"`
	Comment  string `json:"comment"`
	Key      string `json:"key"` // "PRI", "UNI", "MUL", or ""
}

type Structure struct {
	Columns   []Column `json:"columns"`
	CreateSQL string   `json:"create_sql"`
}

type Index struct {
	Name     string   `json:"name"`
	Columns  []string `json:"columns"`
	Unique   bool     `json:"unique"`
	IndexType string  `json:"index_type"`
}

type ForeignKey struct {
	Name      string `json:"name"`
	Column    string `json:"column"`
	RefTable  string `json:"ref_table"`
	RefColumn string `json:"ref_column"`
	OnDelete  string `json:"on_delete"`
	OnUpdate  string `json:"on_update"`
}

func DescribeTable(ctx context.Context, db *sql.DB, schema, table string) (Structure, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT column_name, column_type, is_nullable, IFNULL(column_default,''),
		        IFNULL(extra,''), IFNULL(column_comment,''), IFNULL(column_key,'')
		 FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = ?
		 ORDER BY ordinal_position`,
		schema, table,
	)
	if err != nil {
		return Structure{}, err
	}
	var out Structure
	for rows.Next() {
		var c Column
		var nullable string
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &c.Default, &c.Extra, &c.Comment, &c.Key); err != nil {
			rows.Close()
			return Structure{}, err
		}
		c.Nullable = nullable == "YES"
		out.Columns = append(out.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Structure{}, err
	}

	// SHOW CREATE TABLE quotes its identifiers — schema/table must be safe.
	qualified := QuoteIdent(schema) + "." + QuoteIdent(table)
	var tbl, createSQL string
	if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE "+qualified).Scan(&tbl, &createSQL); err != nil {
		return out, err
	}
	out.CreateSQL = createSQL
	return out, nil
}

func ListIndexes(ctx context.Context, db *sql.DB, schema, table string) ([]Index, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT index_name, column_name, non_unique, IFNULL(index_type,'BTREE')
		 FROM information_schema.statistics
		 WHERE table_schema = ? AND table_name = ?
		 ORDER BY index_name, seq_in_index`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Group rows by index_name into ordered column lists.
	type acc struct {
		Name      string
		Columns   []string
		NonUnique int
		IndexType string
	}
	var ordered []*acc
	byName := map[string]*acc{}
	for rows.Next() {
		var iname, cname, idxType string
		var nonUnique int
		if err := rows.Scan(&iname, &cname, &nonUnique, &idxType); err != nil {
			return nil, err
		}
		entry, ok := byName[iname]
		if !ok {
			entry = &acc{Name: iname, NonUnique: nonUnique, IndexType: idxType}
			byName[iname] = entry
			ordered = append(ordered, entry)
		}
		entry.Columns = append(entry.Columns, cname)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Index, 0, len(ordered))
	for _, e := range ordered {
		out = append(out, Index{
			Name: e.Name, Columns: e.Columns,
			Unique: e.NonUnique == 0, IndexType: e.IndexType,
		})
	}
	return out, nil
}

func ListForeignKeys(ctx context.Context, db *sql.DB, schema, table string) ([]ForeignKey, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT k.constraint_name, k.column_name,
		        k.referenced_table_name, k.referenced_column_name,
		        IFNULL(c.delete_rule,''), IFNULL(c.update_rule,'')
		 FROM information_schema.key_column_usage k
		 LEFT JOIN information_schema.referential_constraints c
		   ON c.constraint_schema = k.constraint_schema
		  AND c.constraint_name = k.constraint_name
		 WHERE k.table_schema = ? AND k.table_name = ?
		   AND k.referenced_table_name IS NOT NULL
		 ORDER BY k.constraint_name, k.ordinal_position`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ForeignKey
	for rows.Next() {
		var fk ForeignKey
		if err := rows.Scan(&fk.Name, &fk.Column, &fk.RefTable, &fk.RefColumn, &fk.OnDelete, &fk.OnUpdate); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}
```

- [ ] **Step 2: Sanity build**

```bash
go build ./internal/mysql/
```

Expected: no errors. (No tests since these queries require real MySQL.)

- [ ] **Step 3: Commit**

```bash
git add internal/mysql/schema.go
git commit -m "feat(mysql): schema introspection (DescribeTable / ListIndexes / ListForeignKeys)"
```

---

## Task 3: GET /api/db/.../structure

**Files:**
- Modify: `internal/api/db.go`, `internal/api/db_test.go`, `internal/api/router.go`

- [ ] **Step 1: Append failing tests to `internal/api/db_test.go`**

```go
func TestStructure_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := get(t, r, "/api/db/1/databases/x/tables/users/structure", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestStructure_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := get(t, r, "/api/db/999/databases/x/tables/users/structure", tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}
```

- [ ] **Step 2: Verify fail**

```bash
go test ./internal/api/ -run TestStructure -v
```

- [ ] **Step 3: Append handler to `internal/api/db.go`**

```go
func handleStructure(d Deps) http.HandlerFunc {
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
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := mysql.DescribeTable(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "describe failed")
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
```

- [ ] **Step 4: Wire route**

In `internal/api/router.go` inside the auth group, add:

```go
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/structure", handleStructure(d))
```

- [ ] **Step 5: Verify pass**

```bash
go test ./internal/api/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat(api): GET /api/db/:connId/databases/:db/tables/:t/structure"
```

---

## Task 4: GET /api/db/.../indexes

**Files:**
- Modify: `internal/api/db.go`, `internal/api/db_test.go`, `internal/api/router.go`

- [ ] **Step 1: Append failing tests**

```go
func TestIndexes_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := get(t, r, "/api/db/1/databases/x/tables/users/indexes", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestIndexes_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := get(t, r, "/api/db/999/databases/x/tables/users/indexes", tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run, fail**

- [ ] **Step 3: Implement handler in `db.go`**

```go
func handleIndexes(d Deps) http.HandlerFunc {
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
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := mysql.ListIndexes(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "indexes failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"indexes": out})
	}
}
```

- [ ] **Step 4: Wire route**

```go
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/indexes", handleIndexes(d))
```

- [ ] **Step 5: Verify pass**

```bash
go test ./internal/api/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat(api): GET /api/db/:connId/databases/:db/tables/:t/indexes"
```

---

## Task 5: GET /api/db/.../fks

**Files:**
- Modify: `internal/api/db.go`, `internal/api/db_test.go`, `internal/api/router.go`

- [ ] **Step 1: Append failing tests**

```go
func TestFKs_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := get(t, r, "/api/db/1/databases/x/tables/users/fks", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestFKs_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := get(t, r, "/api/db/999/databases/x/tables/users/fks", tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run, fail**

- [ ] **Step 3: Implement handler**

```go
func handleFKs(d Deps) http.HandlerFunc {
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
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := mysql.ListForeignKeys(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "fks failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"fks": out})
	}
}
```

- [ ] **Step 4: Wire route**

```go
		r.Get("/api/db/{connId}/databases/{db}/tables/{table}/fks", handleFKs(d))
```

- [ ] **Step 5: Verify pass**

```bash
go test ./internal/api/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat(api): GET /api/db/:connId/databases/:db/tables/:t/fks"
```

---

## Task 6: Query classifier + execute helper

**Files:**
- Create: `internal/mysql/query.go`, `internal/mysql/query_test.go`

- [ ] **Step 1: Failing tests at `internal/mysql/query_test.go`**

```go
package mysql

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		sql  string
		want StatementKind
	}{
		{"SELECT 1", StmtSelect},
		{"  select * from users", StmtSelect},
		{"-- comment\nSELECT 1", StmtSelect},
		{"/* multi-line */\nSELECT 1", StmtSelect},
		{"SHOW DATABASES", StmtSelect},
		{"EXPLAIN SELECT 1", StmtSelect},
		{"DESCRIBE users", StmtSelect},
		{"DESC users", StmtSelect},
		{"WITH t AS (SELECT 1) SELECT * FROM t", StmtSelect},
		{"INSERT INTO users VALUES (1)", StmtExec},
		{"UPDATE users SET x=1", StmtExec},
		{"DELETE FROM users", StmtExec},
		{"CREATE TABLE x (id INT)", StmtExec},
		{"ALTER TABLE x ADD y INT", StmtExec},
		{"DROP TABLE x", StmtExec},
		{"REPLACE INTO users VALUES (1)", StmtExec},
		{"TRUNCATE users", StmtExec},
		{"CALL myproc()", StmtExec},
		{"BEGIN", StmtExec},
		{"COMMIT", StmtExec},
		{"", StmtSelect}, // empty defaults to select-ish; handler should reject
	}
	for _, c := range cases {
		got := Classify(c.sql)
		if got != c.want {
			t.Errorf("Classify(%q) = %v, want %v", c.sql, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Verify fail**

```bash
go test ./internal/mysql/ -run TestClassify -v
```

- [ ] **Step 3: Implement at `internal/mysql/query.go`**

```go
package mysql

import (
	"context"
	"database/sql"
	"strings"
)

type StatementKind int

const (
	StmtSelect StatementKind = iota
	StmtExec
)

// Classify returns whether `sql` should be run via *sql.DB.Query (read) or
// *sql.DB.Exec (write/DDL). It strips leading whitespace and SQL comments
// (line `--` and block `/* */`) before inspecting the first keyword.
func Classify(sql string) StatementKind {
	s := stripLeadingComments(sql)
	if s == "" {
		return StmtSelect
	}
	// First word (alphanumeric)
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			end++
			continue
		}
		break
	}
	word := strings.ToUpper(s[:end])
	switch word {
	case "SELECT", "SHOW", "EXPLAIN", "DESCRIBE", "DESC", "HELP", "WITH", "VALUES":
		return StmtSelect
	}
	return StmtExec
}

func stripLeadingComments(s string) string {
	for {
		s = strings.TrimLeft(s, " \t\r\n")
		if strings.HasPrefix(s, "--") {
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = s[i+1:]
				continue
			}
			return ""
		}
		if strings.HasPrefix(s, "/*") {
			if j := strings.Index(s, "*/"); j >= 0 {
				s = s[j+2:]
				continue
			}
			return ""
		}
		return s
	}
}

// ExecResult is the unified result of running an ad-hoc statement.
type ExecResult struct {
	Kind         StatementKind `json:"-"`
	Columns      []string      `json:"columns,omitempty"`
	Rows         [][]any       `json:"rows,omitempty"`
	RowsAffected int64         `json:"rows_affected"`
	DurationMs   int64         `json:"duration_ms"`
	Truncated    bool          `json:"truncated"`
}

// RunOpts caps the result size and optionally pins a default database.
type RunOpts struct {
	MaxRows  int    // hard row cap, 0 = 10000
	Database string // if set, "USE <db>" is run on the same underlying connection
}

// Run executes one statement, classifies it, returns rows or rows_affected.
// Uses ctx for timeout. Acquires a dedicated *sql.Conn from the pool so that
// USE + query share session state (default database). Caller owns the pool.
func Run(ctx context.Context, db *sql.DB, statement string, opts RunOpts) (ExecResult, error) {
	if opts.MaxRows <= 0 {
		opts.MaxRows = 10000
	}
	kind := Classify(statement)

	conn, err := db.Conn(ctx)
	if err != nil {
		return ExecResult{Kind: kind}, err
	}
	defer conn.Close()

	if opts.Database != "" {
		if _, err := conn.ExecContext(ctx, "USE "+QuoteIdent(opts.Database)); err != nil {
			return ExecResult{Kind: kind}, err
		}
	}

	if kind == StmtExec {
		res, err := conn.ExecContext(ctx, statement)
		if err != nil {
			return ExecResult{Kind: kind}, err
		}
		n, _ := res.RowsAffected()
		return ExecResult{Kind: kind, RowsAffected: n}, nil
	}
	rows, err := conn.QueryContext(ctx, statement)
	if err != nil {
		return ExecResult{Kind: kind}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return ExecResult{Kind: kind}, err
	}
	out := ExecResult{Kind: kind, Columns: cols}
	for rows.Next() {
		if len(out.Rows) >= opts.MaxRows {
			out.Truncated = true
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return out, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		out.Rows = append(out.Rows, vals)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Verify pass**

```bash
go test ./internal/mysql/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/mysql/
git commit -m "feat(mysql): Classify(sql) + Run(ctx, db, sql) (Query vs Exec, 10000-row cap)"
```

---

## Task 7: POST /api/query handler

**Files:**
- Create: `internal/api/query.go`, `internal/api/query_test.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Failing tests at `internal/api/query_test.go`**

```go
package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestQuery_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := post(t, r, "/api/query", map[string]any{"conn_id": 1, "sql": "SELECT 1"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestQuery_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/query", map[string]any{"conn_id": 999, "sql": "SELECT 1"}, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestQuery_EmptySQL(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{"name": "c", "host": "h", "port": 3306, "username": "u", "password": "p"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	rec = post(t, r, "/api/query", map[string]any{"conn_id": created.Connection.ID, "sql": ""}, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestQuery_HistoryIsWritten(t *testing.T) {
	r, s := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{"name": "c", "host": "h", "port": 3306, "username": "u", "password": "p"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)

	// SELECT 1 works against sqlite — Run will succeed.
	rec = post(t, r, "/api/query", map[string]any{
		"conn_id":       created.Connection.ID,
		"database_name": "",
		"sql":           "SELECT 1",
	}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	// History row should exist
	var n int
	if err := s.DB.QueryRow("SELECT count(*) FROM query_history WHERE user_id=?", userIDOfAlice(s)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("history rows = %d, want 1", n)
	}
}

func TestQuery_HistoryRecordsFailures(t *testing.T) {
	r, s := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{"name": "c", "host": "h", "port": 3306, "username": "u", "password": "p"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	// SELECT from non-existent table — sqlite returns "no such table"
	rec = post(t, r, "/api/query", map[string]any{
		"conn_id": created.Connection.ID, "sql": "SELECT * FROM no_such_table",
	}, tok)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d", rec.Code)
	}
	var errMsg string
	if err := s.DB.QueryRow("SELECT error_message FROM query_history WHERE user_id=? ORDER BY id DESC LIMIT 1", userIDOfAlice(s)).Scan(&errMsg); err != nil {
		t.Fatal(err)
	}
	if errMsg == "" {
		t.Fatal("error_message empty for failed query")
	}
}
```

- [ ] **Step 2: Verify fail**

```bash
go test ./internal/api/ -run TestQuery -v
```

- [ ] **Step 3: Implement at `internal/api/query.go`**

```go
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
)

type queryReq struct {
	ConnID       int64  `json:"conn_id"`
	DatabaseName string `json:"database_name"`
	SQL          string `json:"sql"`
}

func resolveConnByID(d Deps, w http.ResponseWriter, r *http.Request, connID int64) (*connSession, bool) {
	u, _ := auth.UserFromContext(r.Context())
	conn, err := d.Store.GetConnection(u.ID, connID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "connection not found")
		} else {
			writeError(w, http.StatusInternalServerError, "lookup failed")
		}
		return nil, false
	}
	pw, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, connID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "decrypt failed")
		return nil, false
	}
	dsn := mysql.BuildDSN(mysql.DSNInput{
		Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
		DefaultDB: conn.DefaultDB, TLS: conn.TLS,
	})
	key := mysql.PoolKey{UserID: u.ID, ConnID: connID}
	db, err := d.Pool.Get(key, dsn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pool open failed")
		return nil, false
	}
	return &connSession{Conn: conn, DB: db, Pool: d.Pool, Key: key}, true
}

func handleQuery(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var req queryReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.SQL == "" {
			writeError(w, http.StatusBadRequest, "sql required")
			return
		}
		cs, ok := resolveConnByID(d, w, r, req.ConnID)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(d.QueryTimeoutS)*time.Second)
		defer cancel()
		start := time.Now()
		out, err := mysql.Run(ctx, cs.DB, req.SQL, mysql.RunOpts{
			Database: req.DatabaseName,
		})
		dur := time.Since(start).Milliseconds()

		// Always record in history, success or failure
		entry := store.HistoryInput{
			UserID: u.ID, ConnectionID: req.ConnID,
			DatabaseName: req.DatabaseName, SQLText: req.SQL,
			DurationMs: dur, Source: "user",
		}
		if err != nil {
			entry.ErrorMessage = err.Error()
		} else {
			entry.RowsAffected = out.RowsAffected
		}
		_ = d.Store.AddHistoryWithCap(entry, d.HistoryMax)

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"columns":       out.Columns,
			"rows":          out.Rows,
			"rows_affected": out.RowsAffected,
			"duration_ms":   dur,
			"truncated":     out.Truncated,
		})
	}
}
```

- [ ] **Step 4: Extend `Deps` with `QueryTimeoutS` and `HistoryMax`**

Edit `internal/api/router.go`:

```go
type Deps struct {
	Version       string
	Store         *store.Store
	Cipher        *crypto.Cipher
	Pool          *mysql.Pool
	Registration  string
	QueryTimeoutS int
	HistoryMax    int
	WebFS         fs.FS
}
```

Default values (if zero):

```go
func NewRouter(d Deps) http.Handler {
	if d.QueryTimeoutS == 0 {
		d.QueryTimeoutS = 5
	}
	if d.HistoryMax == 0 {
		d.HistoryMax = 1000
	}
	r := chi.NewRouter()
	// ... rest unchanged
```

Wire the route inside the auth group:

```go
		r.Post("/api/query", handleQuery(d))
```

- [ ] **Step 5: Pass config values from `cmd/dataseai/main.go`**

After building `pool := mysqlpkg.NewPool(...)`, change the `api.NewRouter` call:

```go
	r := api.NewRouter(api.Deps{
		Version:       version,
		Store:         s,
		Cipher:        cipher,
		Pool:          pool,
		Registration:  cfg.Registration,
		QueryTimeoutS: cfg.QueryTimeoutS,
		HistoryMax:    cfg.HistoryMax,
		WebFS:         sub,
	})
```

- [ ] **Step 6: Verify pass**

```bash
go test ./...
```

- [ ] **Step 7: Commit**

```bash
git add internal/api/ cmd/dataseai/main.go
git commit -m "feat(api): POST /api/query (timeout, 10k-row cap, writes history)"
```

---

## Task 8: History endpoints

**Files:**
- Create: `internal/api/history.go`, `internal/api/history_test.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Failing tests**

Create `internal/api/history_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func seedHistory(t *testing.T, r http.Handler, tok string) int64 {
	t.Helper()
	rec := post(t, r, "/api/connections", map[string]any{"name": "c", "host": "h", "port": 3306, "username": "u", "password": "p"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	rec = post(t, r, "/api/query", map[string]any{"conn_id": created.Connection.ID, "sql": "SELECT 1"}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed query failed: %d %s", rec.Code, rec.Body.String())
	}
	return created.Connection.ID
}

func TestListHistory(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	_ = seedHistory(t, r, tok)
	rec := get(t, r, "/api/history", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var body struct {
		History []map[string]any `json:"history"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.History) != 1 {
		t.Fatalf("len = %d", len(body.History))
	}
}

func TestDeleteHistoryEntry(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	_ = seedHistory(t, r, tok)
	rec := get(t, r, "/api/history", tok)
	var body struct {
		History []struct{ ID int64 } `json:"history"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	id := body.History[0].ID
	rec = delete_(t, r, "/api/history/"+itoa(id), tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete code = %d", rec.Code)
	}
	rec = get(t, r, "/api/history", tok)
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.History) != 0 {
		t.Fatalf("len after delete = %d", len(body.History))
	}
}

func TestClearHistory(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	connID := seedHistory(t, r, tok)
	_ = post(t, r, "/api/query", map[string]any{"conn_id": connID, "sql": "SELECT 1"}, tok)
	_ = post(t, r, "/api/query", map[string]any{"conn_id": connID, "sql": "SELECT 1"}, tok)
	rec := delete_(t, r, "/api/history", tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("clear code = %d", rec.Code)
	}
	rec = get(t, r, "/api/history", tok)
	var body struct {
		History []map[string]any `json:"history"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.History) != 0 {
		t.Fatalf("len after clear = %d", len(body.History))
	}
}
```

- [ ] **Step 2: Verify fail**

```bash
go test ./internal/api/ -run "TestListHistory|TestDeleteHistoryEntry|TestClearHistory" -v
```

- [ ] **Step 3: Implement at `internal/api/history.go`**

```go
package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

func handleListHistory(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		list, err := d.Store.ListHistory(u.ID, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(list))
		for _, h := range list {
			out = append(out, map[string]any{
				"id":            h.ID,
				"connection_id": h.ConnectionID,
				"database_name": h.DatabaseName,
				"sql_text":      h.SQLText,
				"duration_ms":   h.DurationMs,
				"rows_affected": h.RowsAffected,
				"error_message": h.ErrorMessage,
				"source":        h.Source,
				"executed_at":   h.ExecutedAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"history": out})
	}
}

func handleDeleteHistoryEntry(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		if err := d.Store.DeleteHistoryEntry(u.ID, id); err != nil {
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

func handleClearHistory(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		if err := d.Store.ClearHistory(u.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "clear failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 4: Wire routes inside the auth group**

```go
		r.Get("/api/history", handleListHistory(d))
		r.Delete("/api/history/{id}", handleDeleteHistoryEntry(d))
		r.Delete("/api/history", handleClearHistory(d))
```

- [ ] **Step 5: Verify pass**

```bash
go test ./internal/api/ -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat(api): GET/DELETE /api/history endpoints (list / one / clear all)"
```

---

## Task 9: Frontend — add CodeMirror dependencies

**Files:**
- Modify: `web/package.json` (and `web/package-lock.json`)

- [ ] **Step 1: Install**

```bash
cd web && npm install @uiw/react-codemirror@^4.23.0 @codemirror/lang-sql@^6.7.0 && cd ..
```

- [ ] **Step 2: Verify build still works**

```bash
cd web && npm run build && cd ..
```

Expected: build success, dist bundle grows by ~150 KB raw / ~45 KB gzip.

- [ ] **Step 3: Commit**

```bash
git add web/package.json web/package-lock.json
git commit -m "deps(web): add @uiw/react-codemirror + @codemirror/lang-sql"
```

---

## Task 10: SqlEditor + editor store

**Files:**
- Create: `web/src/store/editor.ts`, `web/src/components/SqlEditor.tsx`

- [ ] **Step 1: Create editor store**

`web/src/store/editor.ts`:

```ts
import { create } from 'zustand'

export interface QueryResult {
  columns: string[]
  rows: any[][]
  rows_affected: number
  duration_ms: number
  truncated: boolean
}

interface State {
  draft: string
  setDraft: (s: string) => void
  result: QueryResult | null
  setResult: (r: QueryResult | null) => void
  error: string | null
  setError: (e: string | null) => void
  busy: boolean
  setBusy: (b: boolean) => void
}

export const useEditor = create<State>((set) => ({
  draft: 'SELECT 1;\n',
  setDraft: (s) => set({ draft: s }),
  result: null,
  setResult: (r) => set({ result: r }),
  error: null,
  setError: (e) => set({ error: e }),
  busy: false,
  setBusy: (b) => set({ busy: b }),
}))
```

- [ ] **Step 2: Implement SqlEditor**

`web/src/components/SqlEditor.tsx`:

```tsx
import { useCallback } from 'react'
import type { CSSProperties } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import { sql } from '@codemirror/lang-sql'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'
import { useEditor, QueryResult } from '../store/editor'

interface Props {
  onShowHistory: () => void
  database?: string
}

export default function SqlEditor({ onShowHistory, database }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const draft = useEditor((s) => s.draft)
  const setDraft = useEditor((s) => s.setDraft)
  const setResult = useEditor((s) => s.setResult)
  const setError = useEditor((s) => s.setError)
  const busy = useEditor((s) => s.busy)
  const setBusy = useEditor((s) => s.setBusy)

  const run = useCallback(async () => {
    if (connId == null || !draft.trim()) return
    setBusy(true)
    setError(null)
    try {
      const res = await api.post<QueryResult>('/api/query', {
        conn_id: connId,
        database_name: database ?? '',
        sql: draft,
      })
      setResult(res)
    } catch (err) {
      setResult(null)
      setError(err instanceof ApiError ? err.message : 'query failed')
    } finally {
      setBusy(false)
    }
  }, [connId, draft, database, setBusy, setError, setResult])

  return (
    <div style={wrap}>
      <div style={bar}>
        <button onClick={() => void run()} disabled={busy || connId == null}>
          {busy ? '⏳ running…' : '▶ run (Ctrl+↵)'}
        </button>
        <button onClick={onShowHistory}>📜 history</button>
        <span style={{ flex: 1 }} />
        {database && <span style={{ fontSize: 12, color: '#666' }}>db: {database}</span>}
      </div>
      <div style={{ flex: 1, minHeight: 0 }}>
        <CodeMirror
          value={draft}
          height="100%"
          extensions={[sql()]}
          onChange={setDraft}
          basicSetup={{ lineNumbers: true, foldGutter: true }}
          onKeyDown={(e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
              e.preventDefault()
              void run()
            }
          }}
        />
      </div>
    </div>
  )
}

const wrap: CSSProperties = {
  display: 'flex', flexDirection: 'column', height: '100%',
  fontFamily: 'system-ui',
}
const bar: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8, padding: 6,
  borderBottom: '1px solid #ddd', background: '#fafafa',
}
```

- [ ] **Step 3: Typecheck**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 4: Commit**

```bash
git add web/src/store/editor.ts web/src/components/SqlEditor.tsx
git commit -m "feat(web): SqlEditor (CodeMirror 6) + editor Zustand store"
```

---

## Task 11: ResultPanel

**Files:**
- Create: `web/src/components/ResultPanel.tsx`

- [ ] **Step 1: Implement**

```tsx
import type { CSSProperties } from 'react'
import { useEditor } from '../store/editor'

export default function ResultPanel() {
  const result = useEditor((s) => s.result)
  const error = useEditor((s) => s.error)

  if (error) {
    return (
      <div style={panel}>
        <div style={{ color: 'crimson', padding: 8, fontFamily: 'monospace', fontSize: 13 }}>{error}</div>
      </div>
    )
  }
  if (!result) {
    return (
      <div style={{ ...panel, color: '#999' }}>
        run a query to see results here
      </div>
    )
  }
  return (
    <div style={panel}>
      <div style={status}>
        {result.columns?.length ? (
          <>columns: {result.columns.length} · rows: {result.rows?.length ?? 0}</>
        ) : (
          <>rows_affected: {result.rows_affected}</>
        )}
        {' · '}
        {result.duration_ms} ms
        {result.truncated && <span style={{ color: '#cc7700' }}> · ⚠ truncated at 10 000 rows</span>}
      </div>
      <div style={{ flex: 1, overflow: 'auto' }}>
        {result.columns?.length > 0 && (
          <table style={{ borderCollapse: 'collapse', fontSize: 13, width: '100%' }}>
            <thead style={{ background: '#f4f4f4', position: 'sticky', top: 0 }}>
              <tr>
                {result.columns.map((c) => (
                  <th key={c} style={th}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {result.rows?.map((row, i) => (
                <tr key={i}>
                  {row.map((v, j) => (
                    <td key={j} style={td}>
                      {v === null || v === undefined ? <span style={{ color: '#999' }}>NULL</span> : String(v)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

const panel: CSSProperties = {
  display: 'flex', flexDirection: 'column', height: '100%',
  borderTop: '1px solid #ddd', fontFamily: 'system-ui',
}
const status: CSSProperties = {
  padding: '4px 8px', fontSize: 12, color: '#555',
  background: '#fafafa', borderBottom: '1px solid #ddd',
}
const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #ddd', whiteSpace: 'nowrap' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid #f3f3f3', whiteSpace: 'nowrap' }
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/ResultPanel.tsx
git commit -m "feat(web): ResultPanel (status line + rows table)"
```

---

## Task 12: StructureView

**Files:**
- Create: `web/src/components/StructureView.tsx`

- [ ] **Step 1: Implement**

```tsx
import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'

interface Column {
  name: string
  type: string
  nullable: boolean
  default: string
  extra: string
  comment: string
  key: string
}

interface Structure {
  columns: Column[]
  create_sql: string
}

interface Props {
  db: string
  table: string
}

export default function StructureView({ db, table }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [data, setData] = useState<Structure | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (connId == null) return
    setError(null)
    setData(null)
    api.get<Structure>(`/api/db/${connId}/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}/structure`)
      .then(setData)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'load failed'))
  }, [connId, db, table])

  if (error) return <div style={err}>{error}</div>
  if (!data) return <div style={muted}>loading…</div>
  return (
    <div style={{ height: '100%', overflow: 'auto', padding: 12, fontFamily: 'system-ui' }}>
      <h3 style={{ margin: '0 0 8px' }}>columns</h3>
      <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: 13 }}>
        <thead>
          <tr>
            <th style={th}>name</th>
            <th style={th}>type</th>
            <th style={th}>null</th>
            <th style={th}>default</th>
            <th style={th}>key</th>
            <th style={th}>extra</th>
            <th style={th}>comment</th>
          </tr>
        </thead>
        <tbody>
          {data.columns.map((c) => (
            <tr key={c.name}>
              <td style={td}><b>{c.name}</b></td>
              <td style={td}><code>{c.type}</code></td>
              <td style={td}>{c.nullable ? 'YES' : 'NO'}</td>
              <td style={td}>{c.default || <span style={{ color: '#bbb' }}>—</span>}</td>
              <td style={td}>{c.key}</td>
              <td style={td}>{c.extra}</td>
              <td style={td}>{c.comment}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <h3 style={{ marginTop: 16 }}>create sql</h3>
      <pre style={pre}>{data.create_sql}</pre>
    </div>
  )
}

const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #ddd' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid #f3f3f3', fontSize: 13 }
const muted: CSSProperties = { padding: 12, color: '#999', fontFamily: 'system-ui' }
const err: CSSProperties = { padding: 12, color: 'crimson', fontFamily: 'monospace', fontSize: 13 }
const pre: CSSProperties = { background: '#f6f8fa', padding: 12, borderRadius: 6, fontSize: 12, overflow: 'auto', whiteSpace: 'pre' }
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/StructureView.tsx
git commit -m "feat(web): StructureView (columns table + CREATE TABLE)"
```

---

## Task 13: IndexesView

**Files:**
- Create: `web/src/components/IndexesView.tsx`

- [ ] **Step 1: Implement**

```tsx
import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'

interface Index {
  name: string
  columns: string[]
  unique: boolean
  index_type: string
}

interface Props {
  db: string
  table: string
}

export default function IndexesView({ db, table }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [list, setList] = useState<Index[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (connId == null) return
    setError(null)
    setList(null)
    api.get<{ indexes: Index[] }>(`/api/db/${connId}/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}/indexes`)
      .then((r) => setList(r.indexes ?? []))
      .catch((e) => setError(e instanceof ApiError ? e.message : 'load failed'))
  }, [connId, db, table])

  if (error) return <div style={err}>{error}</div>
  if (!list) return <div style={muted}>loading…</div>
  if (list.length === 0) return <div style={muted}>(no indexes)</div>
  return (
    <div style={{ height: '100%', overflow: 'auto', padding: 12, fontFamily: 'system-ui' }}>
      <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: 13 }}>
        <thead>
          <tr>
            <th style={th}>name</th>
            <th style={th}>columns</th>
            <th style={th}>unique</th>
            <th style={th}>type</th>
          </tr>
        </thead>
        <tbody>
          {list.map((i) => (
            <tr key={i.name}>
              <td style={td}><b>{i.name}</b></td>
              <td style={td}>{i.columns.join(', ')}</td>
              <td style={td}>{i.unique ? 'YES' : 'no'}</td>
              <td style={td}>{i.index_type}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #ddd' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid #f3f3f3' }
const muted: CSSProperties = { padding: 12, color: '#999', fontFamily: 'system-ui' }
const err: CSSProperties = { padding: 12, color: 'crimson', fontFamily: 'monospace', fontSize: 13 }
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/IndexesView.tsx
git commit -m "feat(web): IndexesView"
```

---

## Task 14: ForeignKeysView

**Files:**
- Create: `web/src/components/ForeignKeysView.tsx`

- [ ] **Step 1: Implement**

```tsx
import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'

interface FK {
  name: string
  column: string
  ref_table: string
  ref_column: string
  on_delete: string
  on_update: string
}

interface Props {
  db: string
  table: string
}

export default function ForeignKeysView({ db, table }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [list, setList] = useState<FK[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (connId == null) return
    setError(null)
    setList(null)
    api.get<{ fks: FK[] }>(`/api/db/${connId}/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}/fks`)
      .then((r) => setList(r.fks ?? []))
      .catch((e) => setError(e instanceof ApiError ? e.message : 'load failed'))
  }, [connId, db, table])

  if (error) return <div style={err}>{error}</div>
  if (!list) return <div style={muted}>loading…</div>
  if (list.length === 0) return <div style={muted}>(no foreign keys)</div>
  return (
    <div style={{ height: '100%', overflow: 'auto', padding: 12, fontFamily: 'system-ui' }}>
      <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: 13 }}>
        <thead>
          <tr>
            <th style={th}>name</th>
            <th style={th}>column</th>
            <th style={th}>references</th>
            <th style={th}>on delete</th>
            <th style={th}>on update</th>
          </tr>
        </thead>
        <tbody>
          {list.map((f) => (
            <tr key={f.name}>
              <td style={td}><b>{f.name}</b></td>
              <td style={td}>{f.column}</td>
              <td style={td}>{f.ref_table}.{f.ref_column}</td>
              <td style={td}>{f.on_delete}</td>
              <td style={td}>{f.on_update}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #ddd' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid #f3f3f3' }
const muted: CSSProperties = { padding: 12, color: '#999', fontFamily: 'system-ui' }
const err: CSSProperties = { padding: 12, color: 'crimson', fontFamily: 'monospace', fontSize: 13 }
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/ForeignKeysView.tsx
git commit -m "feat(web): ForeignKeysView"
```

---

## Task 15: QueryHistory modal

**Files:**
- Create: `web/src/components/QueryHistory.tsx`

- [ ] **Step 1: Implement**

```tsx
import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useEditor } from '../store/editor'

interface Entry {
  id: number
  connection_id: number
  database_name: string
  sql_text: string
  duration_ms: number
  rows_affected: number
  error_message: string
  source: string
  executed_at: string
}

interface Props {
  onClose: () => void
}

export default function QueryHistory({ onClose }: Props) {
  const setDraft = useEditor((s) => s.setDraft)
  const [list, setList] = useState<Entry[]>([])
  const [error, setError] = useState<string | null>(null)

  async function load() {
    try {
      const r = await api.get<{ history: Entry[] }>('/api/history')
      setList(r.history ?? [])
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'load failed')
    }
  }

  useEffect(() => { void load() }, [])

  async function removeEntry(id: number) {
    try {
      await api.del(`/api/history/${id}`)
      await load()
    } catch (e) {
      alert(e instanceof ApiError ? e.message : 'delete failed')
    }
  }

  async function clearAll() {
    if (!confirm('Clear ALL history?')) return
    try {
      await api.del('/api/history')
      await load()
    } catch (e) {
      alert(e instanceof ApiError ? e.message : 'clear failed')
    }
  }

  function loadIntoEditor(sql: string) {
    setDraft(sql)
    onClose()
  }

  return (
    <div style={backdrop}>
      <div style={modal}>
        <header style={header}>
          <h2 style={{ margin: 0 }}>query history</h2>
          <div style={{ display: 'flex', gap: 8 }}>
            <button onClick={clearAll}>clear all</button>
            <button onClick={onClose}>close</button>
          </div>
        </header>
        {error && <div style={{ color: 'crimson', padding: 8 }}>{error}</div>}
        <div style={{ overflow: 'auto', flex: 1 }}>
          {list.length === 0 ? (
            <div style={{ padding: 24, color: '#999', textAlign: 'center' }}>(no history yet)</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
              <thead style={{ background: '#f4f4f4', position: 'sticky', top: 0 }}>
                <tr>
                  <th style={th}>when</th>
                  <th style={th}>sql</th>
                  <th style={th}>ms</th>
                  <th style={th}>rows</th>
                  <th style={th}>error</th>
                  <th style={th}></th>
                </tr>
              </thead>
              <tbody>
                {list.map((e) => (
                  <tr key={e.id}>
                    <td style={td}>{new Date(e.executed_at).toLocaleString()}</td>
                    <td style={tdSql} onClick={() => loadIntoEditor(e.sql_text)}>
                      <code>{e.sql_text.length > 80 ? e.sql_text.slice(0, 80) + '…' : e.sql_text}</code>
                    </td>
                    <td style={td}>{e.duration_ms}</td>
                    <td style={td}>{e.rows_affected}</td>
                    <td style={td}>{e.error_message ? <span style={{ color: 'crimson' }}>{e.error_message}</span> : ''}</td>
                    <td style={td}>
                      <button onClick={() => loadIntoEditor(e.sql_text)}>load</button>{' '}
                      <button onClick={() => removeEntry(e.id)}>delete</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

const backdrop: CSSProperties = {
  position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)',
  display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
}
const modal: CSSProperties = {
  background: 'white', borderRadius: 8, minWidth: 700, maxWidth: '90vw',
  minHeight: 400, maxHeight: '80vh', display: 'flex', flexDirection: 'column',
  fontFamily: 'system-ui',
}
const header: CSSProperties = {
  display: 'flex', justifyContent: 'space-between', alignItems: 'center',
  padding: '12px 16px', borderBottom: '1px solid #ddd',
}
const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #ddd' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid #f3f3f3' }
const tdSql: CSSProperties = { ...td, cursor: 'pointer', maxWidth: 400, overflow: 'hidden' }
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/QueryHistory.tsx
git commit -m "feat(web): QueryHistory modal (load into editor / delete one / clear all)"
```

---

## Task 16: BottomTabs — enable all left+right group tabs

**Files:**
- Modify: `web/src/components/BottomTabs.tsx`

- [ ] **Step 1: Rewrite `BottomTabs.tsx`**

Replace the entire file:

```tsx
import type { CSSProperties } from 'react'

export type BottomTab =
  | 'data'
  | 'structure'
  | 'indexes'
  | 'fks'
  | 'sql'
  | 'chat'

interface Props {
  value: BottomTab
  onChange: (t: BottomTab) => void
  hasTable: boolean // disable table-scoped tabs when no table selected
}

const LEFT: { key: BottomTab; label: string }[] = [
  { key: 'data', label: '📊 Data' },
  { key: 'structure', label: '🏗 Structure' },
  { key: 'indexes', label: '🔑 Indexes' },
  { key: 'fks', label: '🔗 FK' },
]

const RIGHT: { key: BottomTab; label: string; enabled: boolean }[] = [
  { key: 'sql', label: '⌨ SQL Editor', enabled: true },
  { key: 'chat', label: '🤖 AI Chat (Plan 5)', enabled: false },
]

export default function BottomTabs({ value, onChange, hasTable }: Props) {
  return (
    <div style={bar}>
      <span style={label}>TABLE</span>
      {LEFT.map((t) => {
        const enabled = hasTable
        const active = t.key === value
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
      <span style={{ flex: 1 }} />
      {RIGHT.map((t) => {
        const active = t.key === value
        return (
          <button
            key={t.key}
            onClick={() => t.enabled && onChange(t.key)}
            disabled={!t.enabled}
            style={{ ...tab, ...(active ? tabActive : null), opacity: t.enabled ? 1 : 0.4 }}
          >
            {t.label}
          </button>
        )
      })}
      <span style={label}>DB-WIDE</span>
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

(Expect a `TS6133: 'hasTable' is declared but its value is never read` type error in `Workspace.tsx` if it isn't already passing `hasTable`. That's fixed in Task 17 — temporarily expect failure here OR pre-empt by also updating Workspace.)

Actually: until Workspace passes `hasTable`, TS will fail (`required prop`). To avoid a broken commit, make `hasTable` optional with default `true`:

Update `BottomTabs.tsx`:

```tsx
interface Props {
  value: BottomTab
  onChange: (t: BottomTab) => void
  hasTable?: boolean
}
```

And inside:

```tsx
export default function BottomTabs({ value, onChange, hasTable = false }: Props) {
```

Now Task 17 will pass the real boolean.

Re-run `npx tsc --noEmit` — should pass.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/BottomTabs.tsx
git commit -m "feat(web): BottomTabs — enable all left tabs + right group (SQL Editor)"
```

---

## Task 17: Workspace integration — route bottom tabs to views

**Files:**
- Modify: `web/src/routes/Workspace.tsx`

- [ ] **Step 1: Rewrite Workspace**

Replace `web/src/routes/Workspace.tsx`:

```tsx
import { useState } from 'react'
import type { CSSProperties } from 'react'
import TopBar from '../components/TopBar'
import Sidebar from '../components/Sidebar'
import DataGrid from '../components/DataGrid'
import BottomTabs, { BottomTab } from '../components/BottomTabs'
import ConnectionsManager from '../components/ConnectionsManager'
import StructureView from '../components/StructureView'
import IndexesView from '../components/IndexesView'
import ForeignKeysView from '../components/ForeignKeysView'
import SqlEditor from '../components/SqlEditor'
import ResultPanel from '../components/ResultPanel'
import QueryHistory from '../components/QueryHistory'
import { useActiveConn } from '../store/activeConn'

interface Props {
  onOpenSettings: () => void
}

export default function Workspace({ onOpenSettings }: Props) {
  const [view, setView] = useState<'workspace' | 'connections'>('workspace')
  const [selected, setSelected] = useState<{ db: string; table: string } | null>(null)
  const [bottom, setBottom] = useState<BottomTab>('data')
  const [historyOpen, setHistoryOpen] = useState(false)
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
            {connId == null && <div style={center}>pick a connection in the top bar</div>}

            {connId != null && bottom === 'sql' && (
              <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                <div style={{ flex: 1, minHeight: 0 }}>
                  <SqlEditor onShowHistory={() => setHistoryOpen(true)} database={selected?.db} />
                </div>
                <div style={{ flex: 1, minHeight: 0 }}>
                  <ResultPanel />
                </div>
              </div>
            )}

            {connId != null && selected == null && bottom !== 'sql' && (
              <div style={center}>pick a table in the sidebar</div>
            )}
            {connId != null && selected != null && bottom === 'data' && (
              <DataGrid key={`${connId}-${selected.db}-${selected.table}`} db={selected.db} table={selected.table} />
            )}
            {connId != null && selected != null && bottom === 'structure' && (
              <StructureView key={`${connId}-${selected.db}-${selected.table}-s`} db={selected.db} table={selected.table} />
            )}
            {connId != null && selected != null && bottom === 'indexes' && (
              <IndexesView key={`${connId}-${selected.db}-${selected.table}-i`} db={selected.db} table={selected.table} />
            )}
            {connId != null && selected != null && bottom === 'fks' && (
              <ForeignKeysView key={`${connId}-${selected.db}-${selected.table}-f`} db={selected.db} table={selected.table} />
            )}
          </div>
          <BottomTabs value={bottom} onChange={setBottom} hasTable={selected != null} />
        </main>
      </div>
      {historyOpen && <QueryHistory onClose={() => setHistoryOpen(false)} />}
    </div>
  )
}

const center: CSSProperties = {
  display: 'flex', alignItems: 'center', justifyContent: 'center',
  height: '100%', color: '#999', fontFamily: 'system-ui',
}
```

- [ ] **Step 2: Build to verify everything compiles**

```bash
cd web && npm run build && cd ..
```

Expected: vite build succeeds.

- [ ] **Step 3: Commit**

```bash
git add web/src/routes/Workspace.tsx
git commit -m "feat(web): Workspace routes bottom tabs to Structure/Indexes/FK/SQL views + history modal"
```

---

## Task 18: README addendum

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Append a Plan 3 section after the existing "What's in this plan (Plan 2)" block**

Edit `/home/conray/project/dataseai/README.md`. After the Plan 2 bullet list and before "## Manual checklist for first deploy", insert:

```markdown
## What's in this plan (Plan 3)

- GET `/api/db/:connId/databases/:db/tables/:t/structure` — columns + CREATE TABLE
- GET `/api/db/:connId/databases/:db/tables/:t/indexes`
- GET `/api/db/:connId/databases/:db/tables/:t/fks`
- POST `/api/query` — ad-hoc SQL, 5 s timeout, 10 000-row cap, writes to history
- GET `/api/history` (?limit=&offset=), DELETE `/api/history/:id`, DELETE `/api/history` (clear all)
- Frontend: CodeMirror 6 SQL editor (Ctrl+↵ to run), ResultPanel, QueryHistory modal,
  StructureView / IndexesView / ForeignKeysView in the bottom-left tabs
```

Also update the top-of-file line that currently says "Plans 1 + 2 are landed" to "Plans 1 + 2 + 3 are landed":

```markdown
Plans 1 + 2 + 3 are landed (foundation, auth, connection management, DB browse, SQL editor + history + schema views). Import/export and chat arrive in plans 4-5.
```

Append a new manual-smoke section right after the existing "### Manual MySQL smoke (Plan 2)" block:

```markdown
### Manual SQL smoke (Plan 3)

Continuing from the Plan 2 smoke (`smoke-mysql` container is still up):

1. Connection is selected → click `📊 Data` tab on `users`. Confirm 3 rows.
2. Click `🏗 Structure` → should show 3 columns and the CREATE TABLE statement.
3. Click `🔑 Indexes` → at minimum the `PRIMARY` index on `id` should appear.
4. Click `🔗 FK` → (no foreign keys on this fixture) → "(no foreign keys)".
5. Click `⌨ SQL Editor` (right group) → type `SELECT * FROM users WHERE id > 1`, press `Ctrl+↵`.
6. ResultPanel below should show 2 rows.
7. Click `📜 history` button in the editor toolbar → at least one entry should be there.
8. Click `load` next to the entry → SQL reloads into the editor.
9. Click `delete` next to an entry → row removed.
10. Click `clear all` → confirm → list goes empty.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README addendum for Plan 3 endpoints + manual SQL/schema smoke"
```

---

## Plan 3 Done — milestone

After Task 18 the repository should let an authenticated user:
- View a table's column list + CREATE TABLE statement (Structure tab)
- View its indexes (Indexes tab) and foreign keys (FK tab)
- Open a CodeMirror SQL editor (right-group), type a query, press Ctrl+Enter, and see results
- Open a history modal showing every past query (success or failure), load any back into the editor, delete one, or clear all

Total: 18 commits expected.

**Not in scope (Plan 4):** long-query WebSocket / cancel, DML cell-edit, row insert/delete, CSV/SQL import/export, multi-tab in the top bar.

**Plan 4 prep notes:**
- `internal/api/query.go::resolveConnByID` (Task 7) is the shared seam Plan 4's `/ws/query` should reuse — already factored.
- `internal/mysql.Run` (Task 6) is the synchronous version of what Plan 4 will stream. Re-use `Classify` to decide whether streaming makes sense (DML can never stream rows; SELECT can).
- Frontend `editor.ts` store holds `draft` + `result`. Plan 4 will add `running queryId`, `progress`, `cancel()`.
- BottomTabs already wires the right group; Plan 5 just needs to flip the `chat` entry's `enabled: false` to `true` and provide a ChatPanel component.

**Plan 2 backlog items still open:** see Plan 2 plan doc bottom — pool's stale-DSN fix (now landed), rate-limit semantics (now landed), VerifyPassword timing leak (now landed), password change rate limit (now landed). Cosmetic items (browse 500 leaks driver text, query-param spec drift) deferred.
