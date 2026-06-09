package mssql

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// StatementKind distinguishes read from write/DDL.
type StatementKind int

const (
	StmtSelect StatementKind = iota
	StmtExec
)

// Classify returns whether sql should be run via Query or Exec.
func Classify(s string) StatementKind {
	stripped := stripComments(s)
	word := strings.ToUpper(firstVerb(stripped))
	switch word {
	case "SELECT", "WITH", "VALUES", "TABLE":
		return StmtSelect
	}
	return StmtExec
}

// ExecResult is the unified result of running an ad-hoc statement.
type ExecResult struct {
	Kind         StatementKind `json:"-"`
	Columns      []string      `json:"columns,omitempty"`
	Rows         [][]any       `json:"rows,omitempty"`
	RowsAffected int64         `json:"rows_affected"`
	DurationMs   int64         `json:"duration_ms"`
	Truncated    bool          `json:"truncated"`
}

// RunOpts caps the result size.
type RunOpts struct {
	MaxRows  int
	Database string // ignored for MSSQL (database is set at connect time)
}

// Run executes one T-SQL statement and returns rows or rows_affected.
func Run(ctx context.Context, db *sql.DB, statement string, opts RunOpts) (ExecResult, error) {
	if opts.MaxRows <= 0 {
		opts.MaxRows = 10000
	}
	kind := Classify(statement)
	start := time.Now()

	if kind == StmtExec {
		res, err := db.ExecContext(ctx, statement)
		if err != nil {
			return ExecResult{Kind: kind}, err
		}
		n, _ := res.RowsAffected()
		return ExecResult{Kind: kind, RowsAffected: n, DurationMs: time.Since(start).Milliseconds()}, nil
	}

	rows, err := db.QueryContext(ctx, statement)
	if err != nil {
		return ExecResult{Kind: kind}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return ExecResult{Kind: kind}, err
	}
	out := ExecResult{Kind: kind, Columns: cols}
	for rows.Next() {
		if len(out.Rows) >= opts.MaxRows {
			out.Truncated = true
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return out, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		out.Rows = append(out.Rows, vals)
	}
	out.DurationMs = time.Since(start).Milliseconds()
	return out, rows.Err()
}
