# mysqlweb — Design Spec

**Date:** 2026-06-03
**Status:** Approved (after brainstorming session)
**Owner:** conray

A web-based MySQL administration tool with an integrated AI chat panel that talks to databases via MCP. Self-hosted, multi-user, container-first.

---

## 1. Goals & Scope

### 1.1 What we are building

A small-team (2–5 users) internal MySQL admin tool, similar in spirit to TablePlus / Sequel Ace, delivered as a single Docker container. Includes an AI chat panel that lets users query their databases conversationally via the Model Context Protocol (MCP).

### 1.2 Use case

- **Audience:** small team / intranet (2–5 users)
- **Distribution:** Docker container, runs on a single host
- **Network exposure:** internal only at V1 (HTTP); optional Cloudflare Tunnel for external HTTPS access later
- **Maintenance posture:** lightweight; minimal moving parts; one binary preferred

### 1.3 In scope (single delivery, no V1/V2 split)

| | Feature |
|---|---|
| a | Connection management (create/edit/delete saved DB connections, stored per-user) |
| b | Database / table sidebar |
| c | Table data browse (paginate, sort, filter) |
| d | Edit table data (cell edit, row insert/delete; primary-key required) |
| e | SQL editor + execute |
| f | Query history (per-user, retention cap) |
| g | Schema view (CREATE TABLE, indexes, foreign keys) |
| i | Import / export (CSV, SQL dump) |
| j | Multi-tab UI for open tables / queries |
| k | User management (registration, login, password change, session management) |
| chat | AI chat panel via MCP, supporting both Anthropic and OpenAI |

### 1.4 Out of scope

- Schema modification UI (graphical add column / change type / add index) — users can do this via SQL editor instead
- Admin role / user role management — flat user model, all users are equal
- Email verification, CAPTCHA — small intranet, low-abuse environment
- Multi-turn autonomous agent loops in chat (one user-message → tool-calls → response cycle is enough)
- AI-driven schema modification (Claude can't `ALTER TABLE` you)
- Chart generation in chat
- E2E browser test automation (cost/benefit poor at this team size)

---

## 2. Architecture

```
┌──────────────────────────────────────────────────┐
│  Browser (React SPA, served from Go binary)      │
│  · Top tabs · Sidebar · Main area · Bottom tabs  │
└──────────────────────────────────────────────────┘
               │ HTTP/JSON + WebSocket
               ▼
┌──────────────────────────────────────────────────┐
│  Go binary (single container)                    │
│  ┌────────────────────────────────────────────┐  │
│  │ HTTP server (chi)                          │  │
│  │  ├─ /api/auth        (login/register)      │  │
│  │  ├─ /api/connections (CRUD)                │  │
│  │  ├─ /api/db/*        (browse/query/DML)    │  │
│  │  ├─ /ws/query        (long-running queries)│  │
│  │  └─ /ws/chat         (LLM chat stream)     │  │
│  └────────────────────────────────────────────┘  │
│  ┌─────────────┬─────────────┬───────────────┐  │
│  │ auth/user   │ conn store  │ query engine  │  │
│  │ (bcrypt)    │ (AES-GCM)   │ (mysql driver)│  │
│  └─────────────┴─────────────┴───────────────┘  │
│  ┌─────────────┬─────────────────────────────┐  │
│  │ LLM client  │ MCP HTTP client             │  │
│  │ (Anthropic +│ (talks to mcp-mysql sidecar)│  │
│  │  OpenAI)    │                             │  │
│  └─────────────┴─────────────────────────────┘  │
│       │              │               │           │
│       ▼              ▼               ▼           │
│   ┌────────┐    ┌────────┐      ┌──────────┐   │
│   │ sqlite │    │ sqlite │      │  MySQL   │   │
│   │ users  │    │ conns  │      │ (target) │   │
│   └────────┘    └────────┘      └──────────┘   │
│  (./data/mysqlweb.db, mounted volume)            │
└──────────────────────────────────────────────────┘
               │ HTTP                  ▲
               ▼                       │ runtime
┌─────────────────────────┐            │ add_connection
│ mcp-mysql sidecar       │────────────┘
│ (askdba/mysql-mcp-server│
│  with MYSQL_MCP_EXTENDED=1)
└─────────────────────────┘
               │
               ▼ Anthropic / OpenAI HTTPS
┌─────────────────────────┐
│ LLM provider            │
└─────────────────────────┘
```

### 2.1 Key architectural decisions

| Decision | Choice | Why |
|---|---|---|
| Backend language | **Go** | smallest Docker image (~30MB), lowest memory, single-binary deploy, strong concurrency for streaming |
| HTTP framework | **chi** | minimal router, idiomatic Go, fits scope |
| Frontend framework | **React + Vite + TypeScript** | richest ecosystem for CodeMirror, TanStack Table, chat-UI components |
| Frontend bundling | **embed React `dist/` into Go binary** (`//go:embed`) | one binary, zero runtime deps |
| State management (FE) | **Zustand** | small enough for app scope, simpler than Redux |
| Data grid | **TanStack Table** | virtualized rows, sort/filter/pagination, headless |
| SQL editor | **CodeMirror 6** | best modern editor, SQL syntax, autocomplete |
| Tool state DB | **sqlite** (mattn/go-sqlite3) | no extra server, simple, file-backed |
| Connection password encryption | **AES-GCM with master key from ENV** | reversible (needed to actually connect), authenticated |
| User password storage | **bcrypt (cost 10)** | one-way, brute-force resistant |
| Session strategy | **token in `sessions` table** (not stateless JWT) | enables multi-device tracking and per-session revocation |
| LLM provider | **Anthropic + OpenAI, both supported** | env-configurable |
| MCP server | **askdba/mysql-mcp-server** | Go-based, supports runtime `add_connection`, multi-DSN |
| Deployment unit | **docker-compose** (`mysqlweb` + `mcp-mysql`) | both services needed for chat |
| TLS termination | **upstream** (Cloudflare Tunnel or reverse proxy) | server stays HTTP for simplicity |
| Default listen port | **53306** | uncommon enough to avoid casual scanning, mnemonically tied to MySQL's 3306 |

---

## 3. Backend Module Layout

```
mysqlweb/
├── cmd/mysqlweb/main.go         # entrypoint
├── internal/
│   ├── config/                  # ENV loading (PORT, DB_PATH, MASTER_KEY, etc.)
│   ├── store/                   # sqlite layer (tool's own metadata)
│   │   ├── migrate.go           # schema migration on startup
│   │   ├── users.go             # bcrypt-backed user CRUD
│   │   ├── sessions.go          # token-based session table
│   │   ├── connections.go       # encrypted connection CRUD
│   │   └── history.go           # query history
│   ├── auth/                    # session issuance, validation, middleware
│   ├── crypto/                  # AES-GCM encrypt/decrypt helpers
│   ├── mysql/                   # operates on the user's target MySQL
│   │   ├── pool.go              # *sql.DB pool per (user, connection)
│   │   ├── browse.go            # SHOW DBs / TABLES, table data, pagination
│   │   ├── schema.go            # CREATE TABLE / indexes / FK introspection
│   │   ├── execute.go           # query execution + streaming results
│   │   └── dml.go               # cell edit / row insert/delete
│   ├── api/                     # chi handlers
│   │   ├── auth.go
│   │   ├── connections.go
│   │   ├── db.go
│   │   ├── query.go
│   │   ├── chat.go
│   │   └── middleware.go
│   ├── llm/                     # LLM provider abstraction
│   │   ├── client.go            # interface { Stream(ctx, req) → events }
│   │   ├── anthropic.go         # Claude implementation
│   │   └── openai.go            # OpenAI implementation
│   ├── chat/                    # chat orchestrator
│   │   ├── orchestrator.go      # turn the user message into LLM calls + tool dispatch
│   │   ├── tools.go             # MCP tool definitions exposed to LLM
│   │   └── mcp_client.go        # HTTP client for mcp-mysql
│   ├── importer/                # CSV / SQL dump import
│   └── exporter/                # CSV / SQL dump export
├── web/                         # React (Vite) source
│   ├── src/...
│   └── dist/                    # build output, embedded
├── embed.go                     # //go:embed web/dist
├── Dockerfile                   # multi-stage build
├── docker-compose.yml           # mysqlweb + mcp-mysql
├── .env.example
└── go.mod
```

**Conventions:**
- `internal/` keeps everything package-private; nothing exported outside the module
- `store/` operates on the tool's own sqlite; `mysql/` operates on the user's target MySQL — strict separation
- `internal/mysql/pool.go` keeps a `map[userID|connID] → *sql.DB` with idle-timeout cleanup
- HTTP handlers stay thin; logic lives in the layers below

---

## 4. Frontend Structure

```
web/src/
├── main.tsx                 # entry
├── App.tsx                  # router + auth guard
├── routes/
│   ├── Login.tsx
│   ├── Register.tsx
│   └── Workspace.tsx        # auth-required main shell
├── components/
│   ├── TopBar.tsx           # connection picker + open tabs + user menu
│   ├── Sidebar.tsx          # databases → tables tree
│   ├── MainArea.tsx         # renders content based on active bottom tab
│   ├── BottomTabs.tsx       # left group (table-scoped) + right group (DB-wide)
│   ├── DataGrid.tsx         # table data (TanStack Table)
│   ├── StructureView.tsx
│   ├── IndexesView.tsx
│   ├── ForeignKeysView.tsx
│   ├── SqlEditor.tsx        # CodeMirror 6
│   ├── ResultGrid.tsx       # query result
│   ├── QueryHistory.tsx
│   ├── ConnectionDialog.tsx
│   ├── ChatPanel.tsx        # AI chat
│   └── ChatMessage.tsx      # message bubbles, tool-call rendering
├── lib/
│   ├── api.ts               # fetch wrapper (Authorization: Bearer)
│   ├── ws.ts                # WebSocket client (query + chat)
│   └── auth.ts              # token storage (localStorage)
└── store/                   # Zustand
    ├── auth.ts
    ├── connections.ts
    ├── tabs.ts              # open top tabs
    ├── bottomTab.ts         # per-tab table-scoped + global DB-wide
    ├── editor.ts            # global SQL editor state
    └── chat.ts              # chat sessions + messages
```

### 4.1 UI behavior

- **Top tabs**: each tab is either an open table (`type: 'table'`) or a query/chat context. Closeable.
- **Sidebar**: shows the currently selected connection's databases → tables (collapsible tree, with text filter at the top).
- **Bottom tabs — left group (TABLE)**: `Data | Structure | Indexes | Foreign Keys` — scoped to the currently focused top-tab's table. Different top tabs remember their own last-active left-tab.
- **Bottom tabs — right group (DB-WIDE)**: `SQL Editor | AI Chat` — global instances; clicking them dims the active top-tab and shifts main area to the editor/chat.
- **SQL editor & chat are global, not per-tab** — switching top tabs preserves your editor draft and chat history.
- **Selected top tab dims (paused) when on a DB-wide bottom tab**; clicking any left-group tab resumes it.

---

## 5. Data Model (Tool's sqlite)

```sql
-- Users
CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,                    -- bcrypt cost 10
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Sessions (token-based; lets us list + revoke per-device)
CREATE TABLE sessions (
  token         TEXT PRIMARY KEY,                 -- crypto/rand 32-byte hex
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  last_used_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  user_agent    TEXT,
  expires_at    DATETIME                          -- sliding 30 days
);
CREATE INDEX idx_sessions_user ON sessions(user_id);

-- Connections (personal per user)
CREATE TABLE connections (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,                     -- "prod-db"
  host         TEXT NOT NULL,
  port         INTEGER NOT NULL DEFAULT 3306,
  username     TEXT NOT NULL,                     -- MySQL-side user
  password_enc BLOB NOT NULL,                     -- AES-GCM(nonce ‖ ciphertext ‖ tag)
  default_db   TEXT,
  tls          TEXT,                              -- 'disabled' | 'preferred' | 'required'
  color        TEXT,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(user_id, name)
);
CREATE INDEX idx_connections_user ON connections(user_id);

-- Query history (sql text + metadata only; no result rows)
CREATE TABLE query_history (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  connection_id INTEGER NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  database_name TEXT,
  sql_text      TEXT NOT NULL,
  duration_ms   INTEGER,
  rows_affected INTEGER,
  error_message TEXT,
  source        TEXT NOT NULL DEFAULT 'user',     -- 'user' | 'ai'
  executed_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_history_user_time ON query_history(user_id, executed_at DESC);

-- Migration tracking
CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**Notes:**
- `query_history` retention: per-user cap of `MYSQLWEB_HISTORY_MAX` (default 1000); oldest pruned at insert time.
- `password_enc` is binary: 12-byte nonce ‖ AES-GCM ciphertext (auth tag appended by Seal).
- Migrations are forward-only; each `CREATE TABLE` / `ALTER` script is keyed by an integer version applied once.
- Chat messages are NOT persisted server-side — chat history lives in the browser session only. If persistence is wanted later, add a `chat_messages` table; out of scope for this delivery.

---

## 6. API Surface

All responses are JSON. Authenticated routes require `Authorization: Bearer <token>`.

### 6.1 Auth

| Method | Path | Auth | Notes |
|---|---|---|---|
| POST | `/api/auth/register` | — | `{ username, password }` → `{ token, user }` (open registration) |
| POST | `/api/auth/login` | — | `{ username, password }` → `{ token, user }` |
| POST | `/api/auth/logout` | ✓ | revokes current token |
| GET | `/api/auth/me` | ✓ | current user |
| PUT | `/api/auth/password` | ✓ | `{ old, new }` |
| GET | `/api/auth/sessions` | ✓ | list other sessions (token prefix only, UA, last_used) |
| DELETE | `/api/auth/sessions/:id` | ✓ | revoke a specific session |

### 6.2 Connections

| Method | Path | Notes |
|---|---|---|
| GET | `/api/connections` | list user's connections (password omitted) |
| POST | `/api/connections` | create |
| GET | `/api/connections/:id` | one |
| PUT | `/api/connections/:id` | update |
| DELETE | `/api/connections/:id` | delete |
| POST | `/api/connections/:id/test` | connectivity probe |

### 6.3 DB Browse

| Method | Path | Notes |
|---|---|---|
| GET | `/api/db/:connId/databases` | list schemas |
| GET | `/api/db/:connId/databases/:db/tables` | tables + estimated row count + size |
| GET | `/api/db/:connId/databases/:db/tables/:t/data` | `?page=&pageSize=&sort=col:dir&filter=...` |
| GET | `/api/db/:connId/databases/:db/tables/:t/structure` | columns + CREATE TABLE |
| GET | `/api/db/:connId/databases/:db/tables/:t/indexes` | indexes |
| GET | `/api/db/:connId/databases/:db/tables/:t/fks` | foreign keys |

### 6.4 DML

| Method | Path | Notes |
|---|---|---|
| PATCH | `/api/db/:connId/databases/:db/tables/:t/rows` | `{ pkValue, column, newValue }` — table must have PK |
| POST | `/api/db/:connId/databases/:db/tables/:t/rows` | insert row |
| DELETE | `/api/db/:connId/databases/:db/tables/:t/rows` | `{ pkValue }` |

### 6.5 Query

| Method | Path | Notes |
|---|---|---|
| POST | `/api/query` | short queries: `{ connId, db, sql }` → result JSON. 5s timeout, 10MB response cap. Returns `408` or `413` with `{ hint: "use WebSocket" }` when exceeded. |
| WS | `/ws/query` | long queries: see protocol below |

WebSocket query protocol:

```
client → { type: 'exec',    queryId, connId, db, sql }
server → { type: 'columns', queryId, cols: [...] }
server → { type: 'rows',    queryId, batch: [...], offset }   // repeated; ~64KB or 100 rows per batch
server → { type: 'done',    queryId, total, durationMs }
server → { type: 'error',   queryId, message }
client → { type: 'cancel',  queryId }                          // triggers MySQL KILL QUERY
```

### 6.6 History

| Method | Path | Notes |
|---|---|---|
| GET | `/api/history` | `?limit=&offset=` |
| DELETE | `/api/history/:id` | one |
| DELETE | `/api/history` | clear all |

### 6.7 Import / Export

| Method | Path | Notes |
|---|---|---|
| POST | `/api/db/:connId/databases/:db/export` | `{ format: 'csv'|'sql', tables, where? }` → octet-stream |
| POST | `/api/db/:connId/databases/:db/import` | multipart file → `{ rows_inserted, errors }` |

### 6.8 Chat

| Method | Path | Notes |
|---|---|---|
| WS | `/ws/chat` | streaming chat with tool-call events |

Chat WebSocket protocol:

```
client → { type: 'user_message', connId, db, content }
server → { type: 'assistant_delta', tokens }            // streamed
server → { type: 'tool_call', tool, args }
server → { type: 'tool_result', tool, result }          // (may yield more assistant_delta)
server → { type: 'done', sessionId }
server → { type: 'error', message }
client → { type: 'cancel' }
```

### 6.9 Health

| Method | Path | Notes |
|---|---|---|
| GET | `/api/health` | `200 { ok: true, version, uptime_s }` |

---

## 7. Auth & Encryption

### 7.1 User passwords

- bcrypt cost 10 on register / password change
- Validation: min 8 chars, mixed letters + digits (front-end checks first, back-end re-validates)

### 7.2 Sessions

- Login → `crypto/rand` 32 bytes → 64-char hex token
- Stored in `sessions` table
- Bearer in `Authorization` header
- Middleware look-up cached in memory for 5 seconds to keep sqlite load low
- 30-day sliding expiry
- Logout deletes the row
- Password change revokes all other sessions for that user

### 7.3 Multi-device login

Explicitly supported. No limits. `GET /api/auth/sessions` shows other sessions; `DELETE` revokes one.

### 7.4 Connection password encryption

```go
// at startup
masterHex := os.Getenv("MYSQLWEB_MASTER_KEY")     // 64 hex chars → 32 bytes
key, _ := hex.DecodeString(masterHex)
block, _ := aes.NewCipher(key)
aead, _ := cipher.NewGCM(block)

// store
nonce := randBytes(12)
ct := aead.Seal(nil, nonce, plain, nil)            // tag appended
blob := append(nonce, ct...)

// load
nonce, ct := blob[:12], blob[12:]
plain, err := aead.Open(nil, nonce, ct, nil)       // err if key wrong or tampered
```

### 7.5 Master key handling

- ENV missing on first launch → generate `crypto/rand` 32 bytes, write hex to `./data/master.key`, log a warning instructing the operator to set the ENV.
- ENV provided but does not match what was used to encrypt existing rows → startup fails with a clear message.
- `mysqlweb rotate-key --old <hex> --new <hex>` CLI subcommand re-encrypts every `password_enc` row.

### 7.6 Rate limiting

- `/api/auth/login`: 5 req / min / IP, then 429
- `/api/auth/register`: 3 req / min / IP

### 7.7 CSRF

Not required — token lives in `Authorization` header, not a cookie, so browsers will not auto-attach it on cross-origin requests.

---

## 8. Query Execution Flow

### 8.1 Short-query path (`POST /api/query`)

```
client → middleware (auth, conn ownership)
  → pool for (user, connId)
  → USE <db> if specified
  → ctx with 5s timeout
  → db.QueryContext(ctx, sql)
  → drain rows up to 10MB cap
    · capped → 413  { hint: "use WebSocket" }
    · ctx deadline → 408 { hint: "use WebSocket" }
  → write to query_history (source='user')
  → return JSON { columns, rows, durationMs, rowsAffected }
```

### 8.2 Long-query path (`WS /ws/query`)

- Server records the MySQL `CONNECTION_ID()` for each running query.
- Reads rows in batches of ~64KB / 100 rows, streams them to the client.
- Frontend `ResultGrid` is virtualized; it can render incrementally.
- Cancellation: client sends `{ type: 'cancel', queryId }` or drops the WS → server issues `KILL QUERY <conn_id>` and emits `{ type: 'error', message: 'canceled' }`.

### 8.3 Cell edit (`PATCH /api/db/.../rows`)

```
server → check table has PK; if not → 422 "table has no primary key, edit disabled"
       → SQL: UPDATE `<table>` SET `<col>` = ? WHERE `<pk>` = ? LIMIT 1
                                                            ^^^^^^^^
                                  defensive — anomaly if affected > 1
       → return { affected }
```

### 8.4 Row insert / delete

- Insert: `INSERT INTO ... (cols) VALUES (?...)`, return the new row including any auto_increment id.
- Delete: PK in WHERE + `LIMIT 1`.

### 8.5 Identifier escaping

- Table and column names wrapped in backticks; embedded backticks doubled (`` `foo``bar` ``).
- Values always go through prepared statements; no string concatenation, ever.

### 8.6 Active query visibility

`GET /api/queries/active` — lists this user's running long queries from the in-memory registry, with `queryId`, `connId`, `sql_excerpt`, `started_at`. Allows manual kill from the UI.

---

## 9. Chat & MCP

### 9.1 High-level flow

```
client            chat orchestrator        LLM            MCP (askdba)
  │  user_message       │                   │                  │
  ├────────────────────→│                   │                  │
  │                     │ ensure DSN registered for (user,conn)│
  │                     ├──────────────────────────────────────→
  │                     │←──── ok ─────────────────────────────│
  │                     │ build tools schema from MCP server   │
  │                     │ + system prompt                       │
  │                     ├──── stream chat ─────→                │
  │                     │      ←── tokens ──────                │
  │ assistant_delta ←──┤                   │                  │
  │                     │      ←── tool_use_request ─────────  │
  │                     │ forward tool call to MCP             │
  │                     ├──────────────────────────────────────→
  │                     │←── tool result ──────────────────────│
  │ tool_call /         │                   │                  │
  │ tool_result ←──────┤      ←── follow-up tokens ───         │
  │ assistant_delta ←──┤                   │                  │
  │ done           ←───┤                   │                  │
```

### 9.2 Tool surface

Tools exposed to the LLM (mirroring what mcp-mysql provides plus our own constraints):

- `query_table(database, table, where?, limit?)` — returns rows; capped at 1000 rows / 1MB by orchestrator
- `describe_schema(database, table?)` — columns + indexes + FKs
- `run_sql(sql)` — arbitrary SELECT; orchestrator forbids DML in chat (configurable later)
- `list_tables(database)` / `list_databases()`

### 9.3 MCP integration

- One `mcp-mysql` sidecar container running `askdba/mysql-mcp-server` with `MYSQL_MCP_EXTENDED=1` over HTTP. The exact HTTP port and env var names should be verified against the upstream README at implementation time; defaults here are indicative.
- mysqlweb registers a DSN on demand: when alice opens chat with `prod-db` selected, the orchestrator calls MCP `add_connection` with a scoped name like `u{user_id}_c{conn_id}` and the decrypted credentials.
- **What the LLM sees vs what the orchestrator does:** the LLM is shown a fixed, sanitized tool surface (Section 9.2) that operates on "the user's currently selected connection." It never sees DSN names or administrative tools like `add_connection` / `remove_connection`. The orchestrator translates each LLM tool call into an MCP call against the correct DSN for the active (user, conn) pair.
- DSN cleanup: when the chat WS disconnects or the user switches connections, the orchestrator calls `remove_connection`. A background sweeper removes DSNs older than 1 h with no active chat as a safety net.
- **Security note:** the MCP DSN registry is process-global. mysqlweb's DSN naming convention (`u{user_id}_c{conn_id}`) is a defense-in-depth measure; the actual trust boundary is the orchestrator, which always pins tool calls to the caller's user. The MCP server itself is not relied upon for tenant isolation.

### 9.4 LLM provider abstraction

```go
package llm

type Client interface {
    Stream(ctx context.Context, req StreamRequest) (<-chan Event, error)
}

type StreamRequest struct {
    Model    string
    System   string
    Messages []Message
    Tools    []ToolDef
}

type Event struct {
    Type   string  // "text" | "tool_call" | "stop" | "error"
    ...
}
```

- `internal/llm/anthropic.go` and `internal/llm/openai.go` both implement `Client`.
- Provider selection per chat session: read from user preference (default `MYSQLWEB_LLM_DEFAULT`, e.g. `anthropic`); user can switch per session via UI.
- API keys come from ENV (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`); chat is available only when at least one is set.

### 9.5 Audit

Every query the LLM runs through MCP gets written to `query_history` with `source = 'ai'`. The Query History UI shows this distinction with a small badge.

---

## 10. Docker, Compose, and Configuration

### 10.1 Dockerfile (multi-stage)

```dockerfile
# 1. frontend build
FROM node:20-alpine AS frontend
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. go build
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /web/dist ./web/dist
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always)" \
    -o /out/mysqlweb ./cmd/mysqlweb

# 3. final
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

Image size target: ~30 MB.

### 10.2 docker-compose.yml

```yaml
services:
  mysqlweb:
    image: conray/mysqlweb:latest
    ports: ["53306:53306"]
    volumes: ["./data:/data"]
    env_file: .env
    environment:
      - MCP_MYSQL_URL=http://mcp-mysql:8000
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - OPENAI_API_KEY=${OPENAI_API_KEY}
    depends_on: [mcp-mysql]
    restart: unless-stopped

  mcp-mysql:
    image: askdba/mysql-mcp-server:latest
    environment:
      - MYSQL_MCP_EXTENDED=1
      - MCP_HTTP_PORT=8000
    restart: unless-stopped

  # Optional, future:
  # cloudflared:
  #   image: cloudflare/cloudflared:latest
  #   command: tunnel --no-autoupdate run --token ${CF_TUNNEL_TOKEN}
  #   restart: unless-stopped
```

### 10.3 Environment variables

| Variable | Required | Default | Notes |
|---|---|---|---|
| `MYSQLWEB_PORT` | no | 53306 | listen port |
| `MYSQLWEB_DB_PATH` | no | `/data/mysqlweb.db` | sqlite location |
| `MYSQLWEB_MASTER_KEY` | yes (auto-generated on first launch if absent) | — | 64-char hex (32 bytes) |
| `MYSQLWEB_REGISTRATION` | no | `open` | `open` / `closed` |
| `MYSQLWEB_HISTORY_MAX` | no | 1000 | per-user cap |
| `MYSQLWEB_QUERY_TIMEOUT_S` | no | 5 | short-query timeout |
| `MYSQLWEB_QUERY_HTTP_MAX_MB` | no | 10 | short-query response cap |
| `MYSQLWEB_LLM_DEFAULT` | no | `anthropic` | `anthropic` or `openai` |
| `ANTHROPIC_API_KEY` | no | — | needed for Anthropic chat |
| `OPENAI_API_KEY` | no | — | needed for OpenAI chat |
| `MCP_MYSQL_URL` | no | — | when set, chat is available |
| `TZ` | no | `Asia/Taipei` | for query history timestamps |

If neither API key is set, the AI Chat tab is visible but shows a "not configured" state and disables input.

---

## 11. Testing Strategy

| Layer | Approach |
|---|---|
| `internal/crypto` | unit (encrypt/decrypt roundtrip, fuzz with 100 random plaintexts) |
| `internal/store` | unit with in-memory sqlite (CRUD + migration apply/rollback) |
| `internal/auth` | unit (bcrypt cost, token uniqueness, expiry sliding) |
| `internal/mysql` | integration with `testcontainers-go` running MySQL 8.0 |
| `internal/chat` | unit with mock `llm.Client` and mock MCP HTTP server |
| `internal/api` | handler test (`httptest.Server`) + integration spanning `api → store → mysql` |
| `internal/importer`, `internal/exporter` | unit + small fixture files |
| Frontend | Vitest component tests for DataGrid sort/filter and SqlEditor shortcuts. No E2E. |
| Manual | README checklist: first-time deploy, add connection, browse, edit, query, CSV export, chat round-trip. |

### CI

GitHub Actions:
- on push: `golangci-lint`, `eslint`, unit tests, integration tests (testcontainers)
- on tag (`v*`): build & push Docker image to ghcr.io / Docker Hub

---

## 12. Implementation Sequence

Estimated 6 weeks of solo work. The phases are sequential within the same delivery; the milestones produce intermediate value.

```
Week 1 — Skeleton & infra
  · Go project + chi + config + store + crypto + auth
  · React + Vite + login/register/workspace shell
  · Dockerfile + docker-compose (mysqlweb only)
  ✓ Milestone: log in, container boots

Week 2 — Connections & DB browse
  · /api/connections CRUD + test
  · internal/mysql pool + browse
  · TopBar, Sidebar, ConnectionDialog
  · DataGrid with TanStack Table
  · BottomTabs left group
  ✓ Milestone: add a connection, browse a table

Week 3 — SQL editor & history & schema views
  · /api/query (short path) with cap + timeout
  · SqlEditor (CodeMirror 6)
  · ResultGrid + history write
  · Structure / Indexes / FK views
  · Identifier escape audit
  ✓ Milestone: write SQL, inspect schema

Week 4 — DML, long query, import/export, multi-tab
  · Cell edit / row insert / row delete (PK gating)
  · /ws/query (streaming + cancel via KILL QUERY)
  · CSV import/export
  · SQL dump export
  · Tabs Zustand store
  · Settings (password change, session management)
  ✓ Milestone: admin tool feature-complete

Week 5 — LLM abstraction + chat backend + MCP
  · internal/llm with Anthropic & OpenAI implementations
  · internal/chat orchestrator + tool dispatch
  · docker-compose: add mcp-mysql sidecar
  · MCP client with dynamic add_connection / remove_connection
  · DSN naming + scope filtering
  · /ws/chat backend
  ✓ Milestone: round-trip a chat through MCP to MySQL via curl

Week 6 — Chat UI & polish
  · ChatPanel + ChatMessage (streaming, tool-call rendering)
  · Source='ai' badge in history
  · /api/health
  · README + deploy docs
  · Manual checklist pass
  ✓ Milestone: full feature delivery
```

Parallelization opportunities:
- Frontend in week 2 can run against a mocked backend
- Chat UI shell in week 6 can be sketched in week 5 using a fake LLM stream

---

## 13. Open Questions

None as of design freeze.

## 14. Glossary

- **Connection** — a saved MySQL endpoint definition (host/port/user/password/db). Owned by one user.
- **Top tab** — a tab in the top bar; either a table view (`type:'table'`) or a SQL/chat context.
- **Bottom tab** — second-level tab strip at the bottom of the main area. Left group is table-scoped; right group is DB-wide.
- **MCP** — Model Context Protocol; standard for exposing tools/resources to LLMs. We use `askdba/mysql-mcp-server`.
- **DSN** — Data Source Name; the MCP-side name for a registered MySQL connection. We use `u{user_id}_c{conn_id}` as the convention.
