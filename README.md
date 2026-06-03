# mysqlweb

Self-hosted MySQL administration tool for small teams. Browse, query, and edit your databases from a browser; an integrated AI chat panel (Plan 5) lets you talk to your DB via MCP.

Plans 1 + 2 + 3 are landed (foundation, auth, connection management, DB browse, SQL editor + history + schema views). Import/export and chat arrive in plans 4-5.

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

Then in the running mysqlweb:

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

## Tests

- Go: `go test ./...`
- Web: `cd web && npm test`

## Project layout

See [`docs/superpowers/specs/2026-06-03-mysqlweb-design.md`](docs/superpowers/specs/2026-06-03-mysqlweb-design.md) for the full design and [`docs/superpowers/plans/`](docs/superpowers/plans/) for plans.
