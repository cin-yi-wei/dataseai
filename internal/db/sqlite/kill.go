package sqlite

import (
	"context"
	"database/sql"
)

// ConnectionIDQuery returns a query that yields the session connection id.
// SQLite has no server-side connection concept; we return a constant 0 so
// the pool's kill machinery stays compatible without doing anything.
func (SQLite) ConnectionIDQuery() string { return "SELECT 0" }

// KillQuery is a no-op for SQLite. There is no server-side query to cancel;
// context cancellation on the sql.DB side is the only mechanism available.
func (SQLite) KillQuery(_ context.Context, _ *sql.DB, _ int64) error { return nil }
