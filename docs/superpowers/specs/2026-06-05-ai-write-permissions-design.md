# AI Chat Write Permissions — Design

**Date**: 2026-06-05
**Status**: Approved (brainstorming → ready for implementation plan)
**Owner**: conray
**Tracks**: extends Plan 5 (LLM chat) with destructive operations gated by per-user policy

---

## 1. Goal

Let the AI chat propose `INSERT` / `UPDATE` / `DELETE` / `TRUNCATE` / `ALTER` / `RENAME` operations against the user's MySQL, while ensuring:

1. Writes are off by default and require explicit per-user opt-in.
2. Each user picks per-`(connection, database, table, operation)` exactly what the AI can attempt.
3. Every write goes through a user-visible preview card; nothing executes without a click.
4. Backend enforces policy independently of the LLM's prompt obedience (defence in depth).

## 2. Background

`internal/chat` exposes two orchestrators (`Run`, `RunMCP`) that hand the LLM a `run_sql` / `mysql_query` tool. The system prompt tells the LLM to refuse destructive DML/DDL, but **no enforcement exists in code**: a prompt-injection or a non-compliant model could trigger `DELETE FROM users` and the backend would execute it. The user has confirmed the AI feels read-only today; this work simultaneously closes the latent hole and adds an explicit, user-controlled write path.

Multi-user infrastructure is already in place: `users`, `connections`, `sessions` tables in the SQLite config DB; chat WS sessions are scoped to a `(user_id, conn_id)` pair (see `internal/api/chat.go`). Per-user encrypted LLM API keys provide a precedent for per-user settings UI.

## 3. Decisions Locked In During Brainstorming

| # | Decision | Reasoning |
|---|---|---|
| Q1 | **Always preview** — every write opens a card in chat with `[Execute] [Cancel]`. No "auto-mode". | Destructive ops; user wants belt-and-braces. |
| Q2 | **Per-operation flags**: separate `INSERT` / `UPDATE` / `DELETE` / `DDL` toggles per table. UI provides a per-row "select-all" checkbox. | User wants granularity; UI shortcut covers convenience. |
| Q3 | **Strict per-table allowlist**, no DB-level wildcard. New tables show up in an "未設定" list with multi-select batch-apply. | Default-deny; new tables never get auto-authorized. |
| Q4 | **New `propose_write(database, table, operation, sql)` tool**; `run_sql` hardened to reject anything but `SELECT/SHOW/DESCRIBE/EXPLAIN`. | Explicit AI intent; double-layer enforcement. |
| – | **Master switch** per user (`users.ai_writes_enabled`, default 0). Hides Settings section + drops `propose_write` from tools list when off. | One toggle to fully disable feature. |
| – | **`TRUNCATE` rolls under `allow_delete`**; **DDL flag covers `ALTER` / `RENAME` only**. `CREATE` / `DROP` table out of scope V1. | Matches user mental model; CREATE/DROP can't be table-scoped. |
| – | **One statement per propose**; multi-statement rejected. | Forces clear intent. |
| – | **`EXPLAIN` runs server-side for `UPDATE` / `DELETE`** and embeds in proposal card. | Shows impact before execute. |
| – | **Re-check policy at execute time** (not just at propose). | Orphaned cards after user yanks a permission get rejected. |
| – | **No undo, no time-windows, no rate limits, no wildcards, no cross-user sharing in V1.** | Scope. |

## 4. Architecture

```
┌────────────┐    propose_write(db,table,op,sql)     ┌──────────────────────┐
│   LLM      │ ─────────────────────────────────────►│ sqlclass.Classify    │
│ (any model)│                                       │   ↓                  │
└────────────┘                                       │ verify declared      │
       ▲                                             │   matches classified │
       │ tool_result                                 │   ↓                  │
       │                                             │ policy.Check         │
       │                            ┌─── deny ──────►│   ↓ allow            │
       │                            │                │ EXPLAIN (UPDATE/DEL) │
       │                            │                │   ↓                  │
       │  ┌─────────────────────────┴──┐             │ ws emit "write_      │
       └──┤  audit write w/ result    │◄────────────┤ proposed"            │
          │  resume orchestrator      │             └──────────┬───────────┘
          └────────────┬──────────────┘                        │
                       │                                       ▼
                       │                              ┌──────────────────┐
                       └───── execute ───────────────►│ user sees card   │
                              decision               │ clicks Execute/X │
                                                     └──────────────────┘
```

Single source of truth for policy: `ai_write_policy` SQLite table. Policy is consulted at **propose** (early reject saves a round-trip) and again at **execute** (re-confirms current state).

## 5. Storage

Migration adds one column and two tables under the existing `schema_migrations` framework in `internal/store/migrate.go`.

### 5.1 Master switch

```sql
ALTER TABLE users ADD COLUMN ai_writes_enabled INTEGER NOT NULL DEFAULT 0;
```

### 5.2 Policy

```sql
CREATE TABLE ai_write_policy (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL,
  connection_id   INTEGER NOT NULL,
  database_name   TEXT    NOT NULL,
  table_name      TEXT    NOT NULL,
  allow_insert    INTEGER NOT NULL DEFAULT 0,
  allow_update    INTEGER NOT NULL DEFAULT 0,
  allow_delete    INTEGER NOT NULL DEFAULT 0,  -- TRUNCATE rides here
  allow_ddl       INTEGER NOT NULL DEFAULT 0,  -- ALTER / RENAME only
  updated_at      DATETIME NOT NULL,
  UNIQUE(user_id, connection_id, database_name, table_name),
  FOREIGN KEY (user_id)       REFERENCES users(id)       ON DELETE CASCADE,
  FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE
);
```

### 5.3 Audit

```sql
CREATE TABLE ai_write_audit (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL,
  connection_id   INTEGER NOT NULL,
  database_name   TEXT    NOT NULL,
  table_name      TEXT    NOT NULL,
  operation       TEXT    NOT NULL,   -- INSERT|UPDATE|DELETE|TRUNCATE|DDL
  sql_text        TEXT    NOT NULL,
  status          TEXT    NOT NULL,   -- proposed|executed|denied|cancelled|failed
  rows_affected   INTEGER,
  error_message   TEXT,
  explain_summary TEXT,                -- JSON; UPDATE/DELETE only
  created_at      DATETIME NOT NULL,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_ai_write_audit_user ON ai_write_audit(user_id, created_at DESC);
```

## 6. Backend

### 6.1 SQL classifier — new file `internal/mysql/sqlclass.go`

```go
type Op string

const (
  OpSelect    Op = "SELECT"
  OpInsert    Op = "INSERT"
  OpUpdate    Op = "UPDATE"
  OpDelete    Op = "DELETE"
  OpTruncate  Op = "TRUNCATE"
  OpDDL       Op = "DDL"        // ALTER / RENAME on existing table
  OpForbidden Op = "FORBIDDEN"  // CREATE / DROP / GRANT / etc — never allowed
  OpReadMeta  Op = "READMETA"   // SHOW / DESCRIBE / EXPLAIN — passes run_sql
  OpUnknown   Op = "UNKNOWN"
)

type Classified struct {
  Op    Op
  DB    string  // resolved from `db.table` or empty if only table given
  Table string
  Multi bool    // true if a non-comment ';' separates a second statement
}

func Classify(sql string) (Classified, error)
```

Implementation strategy: strip line comments (`--`) and block comments (`/* */`), look at top-level semicolons (semicolons inside quoted strings or backticks ignored), then match the leading verb of the first statement with case-insensitive regexes. Target table extraction:

| Verb | Pattern (simplified) |
|------|---------------------|
| INSERT | `INSERT [LOW_PRIORITY \| DELAYED \| HIGH_PRIORITY] [IGNORE] INTO <table_ref>` |
| UPDATE | `UPDATE [IGNORE] <table_ref>` — only the first table ref. JOIN targets and comma-cross-join targets (`UPDATE a, b SET ...`) are not separately checked; the policy of the first ref governs. |
| DELETE | `DELETE [LOW_PRIORITY] [QUICK] [IGNORE] FROM <table_ref>` |
| TRUNCATE | `TRUNCATE [TABLE] <table_ref>` |
| ALTER | `ALTER TABLE <table_ref>` → `OpDDL` |
| RENAME | `RENAME TABLE <table_ref>` → `OpDDL` (first ref) |
| CREATE / DROP | → `OpForbidden` |
| SELECT | → `OpSelect` |
| SHOW / DESCRIBE / DESC / EXPLAIN | → `OpReadMeta` |
| anything else | → `OpUnknown` |

`<table_ref>` handles `` `db`.`table` ``, `db.table`, `` `table` ``, `table`. Backticks unescape the doubled-backtick rule.

If parsing fails or yields `OpUnknown`, callers reject. The classifier is intentionally conservative — LLMs produce normalized SQL and an occasional false `OpUnknown` just means the AI must restate.

### 6.2 Policy package — new file `internal/policy/policy.go`

```go
type Policy struct{ Insert, Update, Delete, DDL bool }

type Decision struct {
  Allowed bool
  Reason  string
}

func IsMasterEnabled(s *store.Store, userID int64) (bool, error)
func SetMaster(s *store.Store, userID int64, enabled bool) error

func Get(s *store.Store, userID, connID int64, db, table string) (Policy, error)
func Set(s *store.Store, userID, connID int64, db, table string, p Policy) error
func SetBatch(s *store.Store, userID, connID int64, db string, tables []string, p Policy) error

type TablePolicy struct{ Table string; Policy Policy }
func ListConfigured(s *store.Store, userID, connID int64, db string) ([]TablePolicy, error)

func Check(s *store.Store, userID, connID int64, db, table string, op mysql.Op) Decision
```

`Check` algorithm:

```
if !IsMasterEnabled(user): return {false, "master_disabled"}
row := Get(user, conn, db, table)
if row missing: return {false, "policy_denied"}
switch op {
  case OpInsert:   if !row.Insert: return {false, "policy_denied"}
  case OpUpdate:   if !row.Update: return {false, "policy_denied"}
  case OpDelete, OpTruncate:
                   if !row.Delete: return {false, "policy_denied"}
  case OpDDL:      if !row.DDL:    return {false, "policy_denied"}
  case OpForbidden, OpUnknown:
                   return {false, "policy_denied"}
}
return {true, ""}
```

### 6.3 Audit package — same `internal/policy` file or its own `internal/policy/audit.go`

```go
type AuditStatus string  // "proposed" | "executed" | "denied" | "cancelled" | "failed"

type AuditRow struct {
  ID             int64
  UserID         int64
  ConnectionID   int64
  Database       string
  Table          string
  Operation      mysql.Op
  SQL            string
  Status         AuditStatus
  RowsAffected   *int64
  ErrorMessage   string
  ExplainSummary string
  CreatedAt      time.Time
}

func WriteAudit(s *store.Store, row AuditRow) (int64, error)
func UpdateAuditStatus(s *store.Store, id int64, status AuditStatus, rows *int64, errMsg string) error
func RecentAudit(s *store.Store, userID int64, limit int) ([]AuditRow, error)
```

### 6.4 Tool surface changes — `internal/chat/tools.go` and `orchestrator_mcp.go`

**`run_sql` / `mysql_query`** become gated:

```go
classified, err := mysql.Classify(sql)
if err != nil || (classified.Op != OpSelect && classified.Op != OpReadMeta) {
  return jsonErr("run_sql_readonly",
                  "use propose_write for writes; only SELECT/SHOW/DESCRIBE/EXPLAIN allowed here"), nil
}
// proceed with existing execution
```

**`propose_write`** added to the tool list when `IsMasterEnabled` returns true at WS session start. Note: the tool list is fixed for the life of the WS session, so a user who toggles master off mid-session still has the tool available to the LLM — but `policy.Check` runs on every call and returns `policy_denied` (with `reason="master_disabled"`), so writes still cannot proceed.

```jsonc
{
  "name": "propose_write",
  "description": "Propose a single INSERT/UPDATE/DELETE/TRUNCATE/DDL statement for user approval. The user must click Execute before anything runs. You MUST declare the target database, table, and operation; the backend verifies these against the SQL and rejects mismatches. Use this whenever the user asks you to modify data; never embed write SQL in run_sql.",
  "input_schema": {
    "type": "object",
    "required": ["database", "table", "operation", "sql"],
    "properties": {
      "database":  {"type": "string"},
      "table":     {"type": "string"},
      "operation": {"type": "string", "enum": ["INSERT","UPDATE","DELETE","TRUNCATE","DDL"]},
      "sql":       {"type": "string", "description": "A single statement. No trailing semicolon required."}
    }
  }
}
```

System prompt addendum (only when master is on):

> When you need to modify data, call `propose_write` with the exact target database/table/operation and the single SQL statement. The user must click Execute before anything runs. If `propose_write` returns `{"error":"policy_denied",...}`, explain to the user which table/operation needs to be enabled in Settings → AI 寫入權限 and stop trying to write.

### 6.5 Orchestrator change — `internal/chat/orchestrator.go` and `orchestrator_mcp.go`

Both orchestrators gain a per-session `pending map[string]chan WriteDecision` and a `policy.Service` handle. Pseudocode of the `propose_write` branch inside `Execute` (direct path) / `executeMCP` (MCP path):

```go
case "propose_write":
  decl := parseDecl(input) // database, table, operation, sql
  cls, _ := mysql.Classify(decl.SQL)
  if cls.Multi { return jsonErr("multi_statement", "one statement at a time"), nil }
  if !match(cls, decl) { return jsonErr("invalid_proposal", "classified vs declared mismatch"), nil }
  dec := policy.Check(...)
  if !dec.Allowed {
    auditID, _ := policy.WriteAudit(... status:"denied" ...)
    return jsonErr("policy_denied", hintForDecl(decl)), nil
  }
  expl := maybeExplain(ctx, db, decl) // UPDATE/DELETE only
  proposalID := uuid()
  ch := make(chan WriteDecision, 1)
  session.pending[proposalID] = ch
  auditID, _ := policy.WriteAudit(... status:"proposed" sql=decl.SQL explain=expl ...)
  emit(EventWriteProposed{proposalID, decl, expl})

  select {
  case d := <-ch:
    if !d.Accept {
      policy.UpdateAuditStatus(auditID, "cancelled", nil, "")
      return jsonResult(`{"status":"cancelled"}`), nil
    }
    // re-check policy at execute time
    dec2 := policy.Check(...)
    if !dec2.Allowed {
      policy.UpdateAuditStatus(auditID, "denied", nil, dec2.Reason)
      return jsonErr("policy_denied", "permission was revoked before execute"), nil
    }
    res, err := db.ExecContext(ctx, decl.SQL)
    if err != nil {
      policy.UpdateAuditStatus(auditID, "failed", nil, err.Error())
      emit(EventWriteFailed{proposalID, err.Error()})
      return jsonResult(fmt.Sprintf(`{"status":"failed","error":%q}`, err.Error())), nil
    }
    n, _ := res.RowsAffected()
    policy.UpdateAuditStatus(auditID, "executed", &n, "")
    emit(EventWriteExecuted{proposalID, n})
    return jsonResult(fmt.Sprintf(`{"status":"executed","rows_affected":%d}`, n)), nil
  case <-ctx.Done():
    policy.UpdateAuditStatus(auditID, "cancelled", nil, "session closed")
    return jsonResult(`{"status":"cancelled"}`), nil
  }
```

### 6.6 WebSocket protocol — `internal/api/chat.go`

New `chatMsg` types alongside existing `text` / `tool_use` / `tool_result` / `done` / `error`:

- Server → client: `write_proposed`, `write_executed`, `write_failed`, `write_cancelled`
- Client → server: `execute_write` with `{proposal_id, accept}`

WS handler reads incoming envelopes in a loop; on `execute_write` it looks up the session's `pending[proposal_id]` and pushes the decision; orchestrator resumes.

### 6.7 HTTP endpoints — `internal/api/ai_policy.go` (new)

| Method | Path                              | Body / Query                                              | Response                                 |
|--------|-----------------------------------|----------------------------------------------------------|------------------------------------------|
| GET    | `/api/auth/ai-writes`             | –                                                        | `{enabled: bool}`                        |
| PUT    | `/api/auth/ai-writes`             | `{enabled: bool}`                                        | `{enabled: bool}`                        |
| GET    | `/api/auth/ai-policy`             | `?conn=1&db=fatgame_development`                         | `{configured: TablePolicy[], unconfigured: string[]}` |
| PUT    | `/api/auth/ai-policy`             | `{conn, db, table, policy:{insert,update,delete,ddl}}`   | `{table: TablePolicy}`                   |
| PUT    | `/api/auth/ai-policy/batch`       | `{conn, db, tables: string[], policy:{...}}`             | `{updated: number}`                      |
| DELETE | `/api/auth/ai-policy`             | `?conn=1&db=...&table=...`                               | `{ok: true}`                             |
| GET    | `/api/auth/ai-audit`              | `?limit=50`                                              | `AuditRow[]`                             |

All endpoints are user-scoped: `user_id` always comes from session, never from request body — clients cannot read/write someone else's policy. `GET /api/auth/ai-policy` performs the cross-tabulation server-side: it queries MySQL `information_schema.tables WHERE table_schema = ?` for the full table list and LEFT JOINs against `ai_write_policy` to split into `configured` / `unconfigured`.

## 7. Frontend

### 7.1 Settings page — `web/src/routes/Settings.tsx`

Add a new section `AIWritesSection` above the API-keys section. Master switch sits at top; the rest is hidden when the master is off.

Two new components in `web/src/components/`:

- **`AIWritePolicyTable.tsx`**: takes `connId`, `db`, renders two tabs: 已設定 + 未設定. Each table-row has 4 checkboxes (INS/UPD/DEL/DDL) plus a row-level "select all" that flips all 4. The 未設定 tab adds a leading row-multi-select column and a footer `[套用到選取的 N 個]` button calling the batch endpoint.
- **`AIWriteAuditList.tsx`**: scrollable last-50 audit list with status chip color (executed=green / denied=red / cancelled=gray / failed=orange / proposed=blue).

### 7.2 Chat proposal card — `web/src/components/WriteProposalCard.tsx`

The `ChatPanel` already maintains a flat ordered list of chat events. Extend the event union with `write_proposed` / `write_executed` / `write_failed` / `write_cancelled`. When a `write_proposed` arrives, append a `WriteProposalCard` in the conversation flow. The card carries its `proposal_id`, listens for matching status events to update its chip, and on click emits `execute_write` over the existing WS.

Visual treatment: card uses a warning-tone background — a new CSS variable `--bg-warning` (light reddish-amber in dark mode, light yellow in light mode) added to the global `:root` / `.light` selectors next to the existing `--bg-primary`, `--bg-secondary` family — so a user scrolling fast cannot miss it. The SQL block is rendered in a monospace `<pre>` with horizontal scroll. `EXPLAIN` rows render as a small table when present, or as an ⚠ banner if `explain_summary` carries an `error` field.

### 7.3 i18n keys — `web/src/i18n/messages.ts`

Adds keys under `settings.ai_writes.*` and `chat.proposal.*` (en + zh-TW).

## 8. Edge Cases

| # | Scenario | Behavior |
|---|---|---|
| 1 | LLM declares `op=INSERT` but SQL is `DELETE` | Reject `invalid_proposal`; LLM retries. |
| 2 | `UPDATE a JOIN b SET a.col = b.col` | Classifier picks `a` (first target); `b` is a read, no policy needed. |
| 3 | `INSERT INTO a SELECT * FROM b` | `a` checked for INSERT; `b` free to read. |
| 4 | `database` field missing in propose | Schema-validated; rejected before classifier. |
| 5 | Backtick / qualified-name variations | Classifier resolves `` `db`.`table` ``, `db.table`, bare `table` (uses pinned session DB if bare). |
| 6 | EXPLAIN itself errors | `explain_summary = {"error": msg}`; proposal still shown with ⚠ banner; execute may fail too — user's call. |
| 7 | WS disconnects with pending cards | `context.Done()` cancels all pending decisions → audit `cancelled` → tool_result `cancelled` flows back if/when WS reconnects (it generally won't; new session starts fresh). |
| 8 | Permission revoked between propose and execute | Re-check at execute returns `policy_denied`; card shows ✗ failed status; audit `denied`. |
| 9 | LLM smuggles DML into `run_sql` | Classifier-gated reject `run_sql_readonly`; no audit (no proposal occurred). |
| 10 | Multiple proposals concurrent | Each has its own `proposal_id` and channel; user can accept/reject in any order. |

## 9. Error Codes

| Code | LLM tool_result | UI visibility |
|------|-----------------|---------------|
| `master_disabled`  | Tool absent from list — LLM can't even call. | Section collapsed. |
| `policy_denied`    | `{error, database, table, operation, hint}` | None (no card emitted). |
| `invalid_proposal` | `{error, reason}` | None. |
| `multi_statement`  | `{error, reason}` | None. |
| `run_sql_readonly` | `{error, hint}` | None. |
| `explain_failed`   | Not an error per se. | ⚠ on the proposal card. |
| `execute_failed`   | `{status:"failed", error: <mysql err>}` | Card chip: ✗ 失敗. |

## 10. Testing

### 10.1 Go unit

- `internal/mysql/sqlclass_test.go`: 30+ cases including comments / multi-statement / quoted names / each verb / JOIN / `INSERT … SELECT` / unicode in identifiers / `CREATE` / `DROP` (forbidden).
- `internal/policy/policy_test.go`: master off / missing row / partial allow / DELETE flag covering TRUNCATE / DDL flag covering ALTER + RENAME / cross-user isolation.
- `internal/policy/audit_test.go`: insert + status transitions; pagination of `RecentAudit`.
- `internal/api/ai_policy_test.go`: HTTP handlers with auth, including denial on cross-user attempts.
- `internal/chat/orchestrator_propose_test.go`: with a stub LLM driving propose_write, exercise: deny / accept→execute / accept→reject-at-execute-recheck / cancel-via-context / multi-statement / classify mismatch.

### 10.2 Frontend unit (vitest)

- `AIWritePolicyTable.test.tsx`: row "select-all" toggle behaviour; batch-apply submits correct payload; switching tabs preserves draft state.
- `AIWriteAuditList.test.tsx`: empty / non-empty / status chip colors.
- `WriteProposalCard.test.tsx`: rendering with/without EXPLAIN; click handlers; state transitions on `write_executed` / `write_failed` / `write_cancelled`.

### 10.3 Integration

Reuse the existing WS-chat test harness with a fake `llm.LLMClient`. The fake emits `propose_write`, the test client sends `execute_write`, end-to-end against a SQLite stub for both MySQL ops and the `ai_write_*` tables.

## 11. Out of Scope (V1)

- `CREATE TABLE` / `DROP TABLE` from chat.
- Cross-user / org-wide default policy.
- Wildcards (`*` table or `*` DB).
- Time-window restrictions.
- Rate limits / write quotas.
- Diff or dry-run beyond `EXPLAIN`.
- Undo of executed writes.

## 12. Migration & Rollout

1. Land migration adding column + 2 tables — no behavior change for existing users (master defaults 0).
2. Land backend (classifier, policy, hardened `run_sql`, propose flow) gated by master.
3. Land HTTP endpoints + Settings UI.
4. Land Chat proposal card.
5. Verify on `dataseai.conray.top` with the existing `nas` connection: walk through master toggle → grant `categories` INSERT → ask AI to insert a row → execute → confirm audit row → revoke → ask AI to insert again → confirm denial.

Steps 1–4 can each ship as separate commits; step 5 is acceptance.
