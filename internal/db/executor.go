package db

import (
	"context"
	"database/sql"
)

// ExecResult is the minimal shape a query response carries beyond raw rows.
// Engine-specific rich results live in streaming helpers per dialect.
type ExecResult struct {
	RowsAffected int64
}

// Executor runs an ad-hoc SQL statement. The interface stays engine-agnostic
// so the chat orchestrator and agent layer can be wired to either a direct
// pool or a connector-backed transport without caring about the dialect.
type Executor interface {
	Run(ctx context.Context, statement string) (ExecResult, error)
}

// DirectExecutor wires Executor straight to a *sql.DB. Used by API
// handlers that talk directly to the target DB.
type DirectExecutor struct {
	DB *sql.DB
}

func (e DirectExecutor) Run(ctx context.Context, statement string) (ExecResult, error) {
	res, err := e.DB.ExecContext(ctx, statement)
	if err != nil {
		return ExecResult{}, err
	}
	n, _ := res.RowsAffected()
	return ExecResult{RowsAffected: n}, nil
}
