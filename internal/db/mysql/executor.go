package mysql

import (
	"context"
	"database/sql"
)

// Executor runs ad-hoc SQL through either a direct database pool or an
// alternate transport such as a connected local agent.
type Executor interface {
	Run(ctx context.Context, statement string, opts RunOpts) (ExecResult, error)
}

type DirectExecutor struct {
	DB *sql.DB
}

func (e DirectExecutor) Run(ctx context.Context, statement string, opts RunOpts) (ExecResult, error) {
	return Run(ctx, e.DB, statement, opts)
}
