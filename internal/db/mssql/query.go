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
	Database string // when set, the statement runs in this database (USE [db]; prefix)
}

// Run executes one T-SQL statement and returns rows or rows_affected.
//
// The user's statement runs in the chosen database context the same way the
// browse/metadata paths do — a "USE [db];" prefix — so unqualified names
// resolve against the database the user picked in the sidebar. If a bare
// table name still fails with "Invalid object name", Run scans the server for
// where that table actually lives and retries with a fully-qualified path;
// when the name exists in more than one place it returns an error asking the
// user to qualify it.
func Run(ctx context.Context, db *sql.DB, statement string, opts RunOpts) (ExecResult, error) {
	if opts.MaxRows <= 0 {
		opts.MaxRows = 10000
	}
	kind := Classify(statement)

	res, err := runStatement(ctx, db, kind, MSSQL{}.useDB(opts.Database)+statement, opts)
	if err == nil {
		return res, nil
	}

	// Fallback: an unqualified table reference didn't resolve. Find it.
	name := parseInvalidObject(err)
	if name == "" {
		return res, err
	}
	locs := locateTable(ctx, db, name)
	switch len(locs) {
	case 0:
		return res, err // couldn't help; surface the original error
	case 1:
		qualified := qualifyName(statement, name, locs[0])
		if qualified == statement {
			return res, err
		}
		return runStatement(ctx, db, kind, MSSQL{}.useDB(opts.Database)+qualified, opts)
	default:
		return ExecResult{Kind: kind}, ambiguousError(name, locs)
	}
}

// runStatement executes one already-prepared statement (query or exec).
func runStatement(ctx context.Context, db *sql.DB, kind StatementKind, execSQL string, opts RunOpts) (ExecResult, error) {
	start := time.Now()

	if kind == StmtExec {
		res, err := db.ExecContext(ctx, execSQL)
		if err != nil {
			return ExecResult{Kind: kind}, err
		}
		n, _ := res.RowsAffected()
		return ExecResult{Kind: kind, RowsAffected: n, DurationMs: time.Since(start).Milliseconds()}, nil
	}

	rows, err := db.QueryContext(ctx, execSQL)
	if err != nil {
		return ExecResult{Kind: kind}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return ExecResult{Kind: kind}, err
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return ExecResult{Kind: kind}, err
	}
	dbTypes := columnDatabaseTypes(colTypes)
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
			vals[i] = normalizeValue(v, dbTypes[i])
		}
		out.Rows = append(out.Rows, vals)
	}
	out.DurationMs = time.Since(start).Milliseconds()
	return out, rows.Err()
}
