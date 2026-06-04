# dataseai

Self-hosted MySQL administration tool for small teams. Browse, query, and edit your databases from a browser; an integrated AI chat panel (Plan 5) lets you talk to your DB via MCP.

All 5 plans landed (foundation, auth, connections, DB browse, SQL editor + history + schema views, DML editing, import/export, multi-tab, WebSocket query streaming, and AI Chat with Anthropic/OpenAI tool calling).

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
go build -o ./bin/dataseai ./cmd/dataseai
MYSQLWEB_DB_PATH=./data/dataseai.db ./bin/dataseai
```

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `MYSQLWEB_PORT` | `53306` | HTTP listen port |
| `MYSQLWEB_DB_PATH` | `/data/dataseai.db` | sqlite location (mount this for persistence) |
| `MYSQLWEB_MASTER_KEY` | (auto-generated) | 64-char hex (32 bytes), used to encrypt stored DB connection passwords in later plans. Generate with `openssl rand -hex 32`. |
| `MYSQLWEB_REGISTRATION` | `open` | `open` / `closed` |
| `MYSQLWEB_HISTORY_MAX` | `1000` | per-user query history cap (Plan 3) |
| `MYSQLWEB_QUERY_TIMEOUT_S` | `5` | short-query timeout (Plan 3) |
| `MYSQLWEB_QUERY_HTTP_MAX_MB` | `10` | short-query response cap (Plan 3) |

## What's in this plan (Plan 1)

- POST `/api/auth/register`, `/api/auth/login` (rate-limited per IP)
- POST `/api/auth/logout`, GET `/api/auth/me`
- PUT `/api/auth/password` (revokes other sessions)
- GET `/api/auth/sessions`, DELETE `/api/auth/sessions/:id`
- GET `/api/health`
- React SPA: login / register / workspace placeholder / settings
- AES-GCM crypto package and master-key bootstrap (used in Plan 2 onward)

## What's in this plan (Plan 2)

- POST/GET/PUT/DELETE `/api/connections` and `/api/connections/:id`
- POST `/api/connections/:id/test`
- GET `/api/db/:connId/databases`
- GET `/api/db/:connId/databases/:db/tables`
- GET `/api/db/:connId/databases/:db/tables/:t/data` (page / per_page / sort_col / sort_dir)
- AES-GCM-encrypted connection passwords (uses master key from Plan 1)
- Per-`(user, conn)` `*sql.DB` pool with 5-minute idle eviction
- Frontend: TopBar, ConnectionPicker, ConnectionsManager + Dialog (CRUD + test), Sidebar (databases/tables tree), DataGrid (TanStack Table, paginate + sort), BottomTabs (Data enabled; Structure/Indexes/FK stubbed for Plan 3)

## What's in this plan (Plan 3)

- GET `/api/db/:connId/databases/:db/tables/:t/structure` — columns + CREATE TABLE
- GET `/api/db/:connId/databases/:db/tables/:t/indexes`
- GET `/api/db/:connId/databases/:db/tables/:t/fks`
- POST `/api/query` — ad-hoc SQL, 5 s timeout, 10 000-row cap, writes to history
- GET `/api/history` (?limit=&offset=), DELETE `/api/history/:id`, DELETE `/api/history` (clear all)
- Frontend: CodeMirror 6 SQL editor (Ctrl+↵ to run), ResultPanel, QueryHistory modal,
  StructureView / IndexesView / ForeignKeysView in the bottom-left tabs

## What's in this plan (Plan 4)

- PATCH `/api/db/:connId/databases/:db/tables/:t/rows` — edit one cell by primary key.
- POST `/api/db/:connId/databases/:db/tables/:t/rows` — insert a row.
- DELETE `/api/db/:connId/databases/:db/tables/:t/rows` — delete a row by primary key.
- GET `/api/db/:connId/databases/:db/tables/:t/export?format=csv|sql` — export CSV or SQL dump.
- POST `/api/db/:connId/databases/:db/tables/:t/import` — multipart CSV import.
- GET `/api/queries/active` — list this user's active streaming queries.
- WebSocket `/ws/query?token=...` — stream long query results in batches and support cancel.
- Frontend: editable DataGrid, inline add-row panel, delete-row action, Import/Export dialog, top tab bar, and SQL editor WebSocket fallback.

## What's in this plan (Plan 5)

- WebSocket `/ws/chat?token=…` — LLM-orchestrated chat with tool calls (streamed)
- LLM providers: Anthropic Messages API + OpenAI Chat Completions (switch via `MYSQLWEB_LLM_DEFAULT` or per-message `provider` field)
- Built-in tools (no external MCP server required): `list_databases`, `list_tables`, `describe_table`, `query_table`, `run_sql` (read-only — orchestrator's system prompt instructs the model to refuse DML/DDL)
- Frontend: ChatPanel in the right-group **🤖 AI Chat** tab — streamed text, tool-call expandable details, clear-history button

> Architecture note: chat has **two execution paths**. By default (no `MYSQLWEB_MCP_COMMAND` set) it uses **direct tools** — `internal/chat/execute.go` calls `internal/mysql` in-process. When `MYSQLWEB_MCP_COMMAND` is set, dataseai spawns the MCP server as a subprocess at startup, registers each user's DSN via askdba's `add_connection` tool when chat opens, and forwards every LLM tool call through MCP `tools/call`. The LLM-facing tool schema is the same in both paths; only the wire-level dispatch changes.

### Env vars (Plan 5)

| Variable | Required? | Notes |
|---|---|---|
| `ANTHROPIC_API_KEY` | one of these two | enables Anthropic provider |
| `OPENAI_API_KEY` | one of these two | enables OpenAI provider |
| `MYSQLWEB_LLM_DEFAULT` | no | `anthropic` (default) or `openai` |
| `MYSQLWEB_MCP_COMMAND` | no | shell-tokenised command that dataseai runs as the MCP subprocess. Example: `npx -y @askdba/mcp-server-mysql` or the path to a compiled binary. `MYSQL_MCP_EXTENDED=1` is added to the child env automatically so `add_connection` / `remove_connection` are available. If unset, chat uses the direct-tools fallback. |

Without an API key the chat tab still renders but every request returns an error.

## Manual checklist for first deploy

1. `docker compose up -d --build`
2. Open http://localhost:53306
3. Register → land in the Workspace placeholder
4. Open Settings → change password → confirm "other sessions were revoked"
5. From a private window: log in with the new password → confirm 2 sessions in Settings → revoke the other one
6. Log out → confirm the login page

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

1. Log in (Plan 1 register/login).
2. Click "manage" in the top bar → "+ new":
   - name `local`, host `host.docker.internal` (or your host IP), port `13306`, user `root`, password `rootpw`, default db `demo`, tls `disabled`. Save.
3. Open the connection in the dialog → click "test" → expect "connected ✓".
4. Pick `local` in the top-bar selector.
5. Sidebar lists the `demo` database; click to expand and see the `users` table.
6. Click `users` → DataGrid shows the 3 seeded rows.
7. Click the `name` header to sort; flip direction with another click.
8. Pagination footer shows "page 1 / 1 · 3 rows total".
9. Tear down: `docker stop smoke-mysql`.

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

### Manual DML / import-export / tabs smoke (Plan 4)

Continuing from the Plan 2 smoke (`smoke-mysql` container is still up):

1. Click the `users` table → Data tab shows rows.
2. Double-click `alice` in the `name` column → type `ALICE` → press Enter → row refreshes.
3. Click `+ row` → fill `name=dave`, `email=d@x.io` → insert → row appears.
4. Click `delete` on `dave`'s row → confirm → row disappears.
5. Click `import/export` → choose CSV → download → file contains the table data.
6. In the same dialog choose SQL dump → download → file contains CREATE/INSERT SQL.
7. Prepare a CSV with header `name,email` and one row → import it → row count refreshes.
8. Click several tables in the sidebar → each opens as a top tab.
9. Click `+ SQL` in the top tab bar → SQL tab opens without losing table tabs.
10. Run a slow query in SQL Editor. The short HTTP path times out, then the editor falls back to WebSocket streaming and shows a cancel button.

### Manual chat smoke (Plan 5 — direct-tools path)

1. Before launch: `export ANTHROPIC_API_KEY=sk-ant-...` (or `OPENAI_API_KEY=sk-...`).
2. Open the workspace, pick the `local` connection, expand `demo`.
3. Click `🤖 AI Chat` in the right group.
4. Type "list the databases I can see" → expect a `list_databases` tool call expandable block + a short summary.
5. Type "describe the users table in demo" → expect `describe_table` + columns list.
6. Type "show me the first 3 rows of demo.users" → expect `query_table` or `run_sql` + 3 rows summarised.
7. Click "clear" in the chat toolbar → transcript empties.

### Manual chat smoke (Plan 5 — MCP path)

This exercises the spec §9 architecture (askdba/mysql-mcp-server as a subprocess).

1. Install the MCP server somewhere dataseai's host can run it. For askdba:
   ```bash
   # one-shot via npx (requires Node 20+)
   npx -y @askdba/mcp-server-mysql --help
   ```
2. Export the chat-relevant env vars before starting dataseai:
   ```bash
   export ANTHROPIC_API_KEY=sk-ant-...
   export MYSQLWEB_MCP_COMMAND="npx -y @askdba/mcp-server-mysql"
   ./bin/dataseai
   ```
   On startup dataseai logs `MCP subprocess running: …`. If the spawn fails it
   logs `⚠ MCP spawn failed … — chat will use direct-tools fallback` and
   continues; chat will still work, just not via MCP.
3. Repeat steps 2-7 from the direct-tools smoke. The tool-call expandable
   blocks should look identical — only the wire layer changes.
4. To confirm MCP is actually in use, watch the MCP subprocess's stderr (it
   writes there). You should see one `tools/call` per LLM tool call plus the
   initial `add_connection` for each new chat session.

## Tests

- Go: `go test ./...`
- Web: `cd web && npm test`

## Project layout

See [`docs/superpowers/specs/2026-06-03-dataseai-design.md`](docs/superpowers/specs/2026-06-03-dataseai-design.md) for the full design and [`docs/superpowers/plans/`](docs/superpowers/plans/) for plans.
