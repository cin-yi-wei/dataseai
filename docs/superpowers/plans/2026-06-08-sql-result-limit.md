# SQL Result Limit Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the SQL editor result area control the maximum number of rows fetched per query.

**Architecture:** Add a result row limit to the editor store and expose it in `ResultPanel`. Send that limit from `SqlEditor` to `/api/query` and `/ws/query`; clamp and apply it in backend query execution.

**Tech Stack:** Go HTTP/websocket API, MySQL query executor, React, Zustand, Vitest.

---

### Task 1: Backend REST Query Limit

**Files:**
- Modify: `internal/api/query.go`
- Test: `internal/api/query_test.go`

**Steps:**
1. Add a failing test that posts `/api/query` with `max_rows: 1` against a query returning multiple rows and expects one row plus `truncated: true`.
2. Run `go test ./internal/api -run TestQuery_RespectsMaxRows -count=1` and confirm it fails.
3. Add `MaxRows int json:"max_rows"` to the request struct, clamp it, and pass it to `mysql.RunOpts`.
4. Re-run the focused test and confirm it passes.

### Task 2: Backend Websocket Limit

**Files:**
- Modify: `internal/api/ws.go`
- Test: `internal/api/ws_test.go`

**Steps:**
1. Add a failing websocket test that sends `maxRows: 1` for a multi-row query and expects only one row plus a truncated final event.
2. Run `go test ./internal/api -run TestWS_ExecSelectRespectsMaxRows -count=1` and confirm it fails.
3. Add `MaxRows int json:"maxRows"` to websocket messages, clamp it, pass it to streaming, and stop streaming after the cap.
4. Re-run the focused test and confirm it passes.

### Task 3: Frontend Limit State and Controls

**Files:**
- Modify: `web/src/store/editor.ts`
- Modify: `web/src/components/ResultPanel.tsx`
- Test: `web/src/store/editor.test.ts`
- Test: `web/src/components/ResultPanel.test.tsx`

**Steps:**
1. Add failing tests for changing the result limit in store/UI.
2. Run focused Vitest commands and confirm failure.
3. Add `resultLimit` and `setResultLimit` to the editor store.
4. Render a compact limit selector in `ResultPanel`.
5. Re-run focused tests and confirm they pass.

### Task 4: Frontend Request Wiring

**Files:**
- Modify: `web/src/components/SqlEditor.tsx`
- Modify: `web/src/lib/wsQuery.ts`
- Test: `web/src/components/SqlEditor.test.tsx` or existing focused frontend tests

**Steps:**
1. Add failing tests proving REST receives `max_rows` and websocket exec receives `maxRows`.
2. Wire `resultLimit` into REST and websocket requests.
3. Re-run focused tests and full frontend tests.

### Task 5: Verification

**Commands:**
- `go test ./internal/api -count=1`
- `go test ./... -count=1`
- `cd web && npm test`
- `cd web && npm run build`
