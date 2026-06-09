package pg

import (
	"context"
	"database/sql"
)

// ConnectionIDQuery returns the SQL that yields the current session's
// server-side backend pid as a single int column.
func (PG) ConnectionIDQuery() string { return "SELECT pg_backend_pid()" }

// KillQuery cancels the server-side query running on connID via
// pg_cancel_backend. If the backend is not found (returns false) we treat it
// as a no-op — the query may have already finished.
func (PG) KillQuery(ctx context.Context, db *sql.DB, connID int64) error {
	var cancelled bool
	_ = db.QueryRowContext(ctx, "SELECT pg_cancel_backend($1)", connID).Scan(&cancelled)
	return nil
}
