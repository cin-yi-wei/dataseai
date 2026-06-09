package duckdb

import (
	"context"
	"database/sql"
)

func (DuckDB) ConnectionIDQuery() string                                   { return "SELECT 0" }
func (DuckDB) KillQuery(_ context.Context, _ *sql.DB, _ int64) error      { return nil }
