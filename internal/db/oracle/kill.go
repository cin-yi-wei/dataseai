package oracle

import (
	"context"
	"database/sql"
	"fmt"
)

// ConnectionIDQuery returns the current session SID.
func (Oracle) ConnectionIDQuery() string {
	return "SELECT TO_NUMBER(SYS_CONTEXT('USERENV','SID')) FROM dual"
}

// KillQuery attempts to kill the session by SID. Requires ALTER SYSTEM privilege.
// Looks up the serial# from v$session; fails gracefully if privilege is absent.
func (Oracle) KillQuery(ctx context.Context, sdb *sql.DB, connID int64) error {
	var serial int64
	err := sdb.QueryRowContext(ctx,
		`SELECT serial# FROM v$session WHERE sid = :1`, connID).Scan(&serial)
	if err != nil {
		return fmt.Errorf("oracle kill: cannot read v$session (need SELECT privilege): %w", err)
	}
	_, err = sdb.ExecContext(ctx,
		fmt.Sprintf("ALTER SYSTEM KILL SESSION '%d,%d' IMMEDIATE", connID, serial))
	return err
}
