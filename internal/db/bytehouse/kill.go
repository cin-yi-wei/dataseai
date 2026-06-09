package bytehouse

import (
	"context"
	"database/sql"
	"fmt"
)

// ConnectionIDQuery returns the SQL that yields the current connection id.
// ByteHouse exposes connection_id() for MySQL-protocol compatibility.
func (BH) ConnectionIDQuery() string { return "SELECT connection_id()" }

// KillQuery sends KILL QUERY WHERE connection_id = connID ASYNC.
// ByteHouse supports this via ClickHouse's KILL QUERY system.
func (BH) KillQuery(ctx context.Context, db *sql.DB, connID int64) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("KILL QUERY WHERE connection_id = %d ASYNC", connID))
	return err
}
