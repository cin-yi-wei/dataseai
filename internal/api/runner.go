package api

import (
	"context"
	"database/sql"

	"github.com/conray/dataseai/internal/db"
	mssqldialect "github.com/conray/dataseai/internal/db/mssql"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
	pgdialect "github.com/conray/dataseai/internal/db/pg"
)

// dialectExecutor implements mysqldialect.Executor and routes Run() to the
// correct engine-specific Run function based on the dialect.
type dialectExecutor struct {
	dialect db.Dialect
	db      *sql.DB
}

func (e dialectExecutor) Run(ctx context.Context, stmt string, opts mysqldialect.RunOpts) (mysqldialect.ExecResult, error) {
	switch e.dialect.Engine() {
	case db.EnginePostgres:
		out, err := pgdialect.Run(ctx, e.db, stmt, pgdialect.RunOpts{
			MaxRows:  opts.MaxRows,
			Database: opts.Database,
		})
		if err != nil {
			return mysqldialect.ExecResult{}, err
		}
		return mysqldialect.ExecResult{
			Kind:         mysqldialect.StatementKind(out.Kind),
			Columns:      out.Columns,
			Rows:         out.Rows,
			RowsAffected: out.RowsAffected,
			DurationMs:   out.DurationMs,
			Truncated:    out.Truncated,
		}, nil
	case db.EngineMSSQL:
		out, err := mssqldialect.Run(ctx, e.db, stmt, mssqldialect.RunOpts{
			MaxRows:  opts.MaxRows,
			Database: opts.Database,
		})
		if err != nil {
			return mysqldialect.ExecResult{}, err
		}
		return mysqldialect.ExecResult{
			Kind:         mysqldialect.StatementKind(out.Kind),
			Columns:      out.Columns,
			Rows:         out.Rows,
			RowsAffected: out.RowsAffected,
			DurationMs:   out.DurationMs,
			Truncated:    out.Truncated,
		}, nil
	default:
		return mysqldialect.Run(ctx, e.db, stmt, opts)
	}
}
