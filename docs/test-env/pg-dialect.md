# Test Environment — feat/db-pg-dialect

Branch under test: `feat/db-pg-dialect`

## What changed

- New `internal/db/pg/` package: full PostgreSQL dialect implementation.
- PG engine registered via `init()` blank import in `cmd/dataseai/main.go`.
- `connection.engine = "postgres"` now accepted by the API and store.
- PG design: **schemas treated as "databases"** in the sidebar (MySQL UI parity).
  The connection's `default_db` field specifies which PG database to connect to;
  the schema list appears as the second sidebar level.
- PG-specific SQL quirks: `$N` positional placeholders, double-quote identifiers,
  `INSERT ... RETURNING <pk>` for auto-increment, `pg_cancel_backend($1)` for KILL.
- SSH tunnelling for PG: uses `pgx.ConnConfig.DialFunc` injection +
  `stdlib.RegisterConnConfig(cfg)` returning an opaque DSN reference.
- Frontend connection dialog now has an **engine dropdown** (MySQL / PostgreSQL)
  with automatic default port switch (3306 ↔ 5432) when changed.
- `agent`-based (connector) connections reject `engine=postgres` with HTTP 400
  (direct connection only; connector PG support is a separate plan).

## Backend verification checklist

- [ ] `go test ./...` green (309 tests across 15 packages).
- [ ] `go build ./...` clean.
- [ ] Direct PG connection (no SSH): create connection with engine=postgres →
      list schemas → list tables → describe schema → browse rows → run SELECT →
      edit cell → kill a long query.
- [ ] SSH-tunnelled PG connection: repeat the same flow.
- [ ] Chat orchestrator: propose + execute a SELECT against a PG connection.
- [ ] Active-queries panel (`/api/queries/active`) shows PG queries.
- [ ] Existing MySQL connections unaffected.

## Frontend verification checklist

- [ ] `npm test -- --run` green (38 test cases).
- [ ] `npm run build` clean Vite bundle.
- [ ] New connection dialog shows engine dropdown with MySQL and PostgreSQL options.
- [ ] Switching to PostgreSQL auto-sets port to 5432 (if port was at MySQL default).
- [ ] Switching back to MySQL auto-sets port to 3306 (if port was at PG default).
- [ ] Connection card in sidebar shows "PostgreSQL" engine tag for PG connections.
- [ ] Editing a PG connection retains engine=postgres.

## Rollback

- Revert the merge commit. The `engine` column migration (0017) is additive;
  no PG rows should exist in prod at merge time.
- Frontend rollback removes the dropdown but does not break MySQL flows.

## Smoke-test commands

```bash
git fetch origin feat/db-pg-dialect
git checkout feat/db-pg-dialect

# Backend
go build ./...
go test ./...

# Frontend
cd web
npm ci
npm test -- --run
npm run build
```

Expected: green build + green tests on both sides.
