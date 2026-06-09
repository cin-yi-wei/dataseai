package mysql

import (
	"context"
	"database/sql"
	"strconv"
)

// KillQuery cancels the server-side query running on connID. MySQL uses
// the KILL QUERY statement with an integer connection id. The id must be
// an integer literal — MySQL does not accept a placeholder here — so we
// format it via strconv rather than passing it as a query arg.
func (MySQL) KillQuery(ctx context.Context, sqlDB *sql.DB, connID int64) error {
	_, err := sqlDB.ExecContext(ctx, "KILL QUERY "+strconv.FormatInt(connID, 10))
	return err
}

// ConnectionIDQuery returns the SQL that yields the current session's
// server-side connection id as a single int column. For MySQL: CONNECTION_ID().
func (MySQL) ConnectionIDQuery() string { return "SELECT CONNECTION_ID()" }
