# Test Environment — feat/db-dialect-abstraction

Branch under test: `feat/db-dialect-abstraction`

## What changed
- `internal/mysql/` removed; all DB-engine behavior lives in `internal/db/`
  with a per-engine implementation under `internal/db/<engine>/`.
- MySQL dialect is the sole implementation; `engine` column on connections
  is locked to `mysql` (migration 0017 backfills existing rows).
- Connection form and connection-list card show a read-only "MySQL" engine
  badge. No engine selector — locked to MySQL.
- Connection API DTO now carries `engine` field (only `"mysql"` accepted on
  create/update; empty value defaults to `"mysql"` server-side).
- API dispatches the right `db.Dialect` per connection based on
  `connection.engine`, so future Postgres / MSSQL dialects slot in without
  touching consumers.
- No connector binary changes — that comes in a separate plan.

## Backend verification checklist
- [ ] `go test ./...` green on CI (244 tests across 14 packages).
- [ ] Migration 0017 applies cleanly on a copy of prod metadata DB; existing
      `connections` rows acquire `engine='mysql'`.
- [ ] Existing connections (created on `main`) load and reconnect after the
      0017 migration runs.
- [ ] Direct connect to staging MySQL (no SSH): list databases → list tables
      → describe schema → browse rows → run a SELECT → edit a cell → kill a
      long query.
- [ ] SSH-tunneled connect to staging MySQL: repeat the same flow.
- [ ] Chat orchestrator: propose + execute a SELECT against staging.
- [ ] Active-queries panel (`/api/queries/active`) returns only the current
      user's in-flight queries.
- [ ] AI policy editor reads the per-table catalog correctly.

## Frontend verification checklist
- [ ] `npm test -- --run` green (vitest, 16 files / 37 cases).
- [ ] `npm run build` produces a clean Vite bundle.
- [ ] New connection wizard shows the "MySQL" badge under the name field.
- [ ] Connection card in the sidebar shows the "MySQL" tag next to the SSH
      tag (when present).
- [ ] Editing a connection retains its engine value.

## Rollback
- Revert the merge commit. Migration 0017 is additive; the `engine` column
  is harmless when the prior code path runs against a DB that already has
  it (the field is simply unused).
- Frontend rollback removes the badge but does not break flows.

## Smoke-test commands

From a workstation with repo checked out:

```bash
git fetch origin feat/db-dialect-abstraction
git checkout feat/db-dialect-abstraction

# Backend
go build ./...
go test ./...

# Frontend
cd web
npm ci
npm test -- --run
npm run build
```

Expected: green build + green tests across both sides.
