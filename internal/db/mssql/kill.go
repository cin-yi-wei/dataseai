package mssql

import (
	"context"
	"database/sql"
	"fmt"
)

func (MSSQL) ConnectionIDQuery() string { return "SELECT @@SPID" }

// KillQuery cancels the server-side query running on the session identified
// by connID via KILL. KILL cannot be parameterized so the value is formatted
// directly — connID comes from @@SPID which is always a server-assigned int.
func (MSSQL) KillQuery(ctx context.Context, db *sql.DB, connID int64) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("KILL %d", connID))
	return err
}
