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
