package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
)

func (CH) ConnectionIDQuery() string { return "SELECT connection_id()" }

func (CH) KillQuery(ctx context.Context, db *sql.DB, connID int64) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("KILL QUERY WHERE connection_id = %d ASYNC", connID))
	return err
}
