package snowflake

import (
	"context"
	"database/sql"
)

func (Snowflake) ConnectionIDQuery() string                               { return "SELECT CURRENT_SESSION()" }
func (Snowflake) KillQuery(_ context.Context, _ *sql.DB, _ int64) error  { return nil }
