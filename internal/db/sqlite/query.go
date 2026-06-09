package sqlite

import (
	"context"
	"database/sql"
	"strings"
)

type StatementKind int

const (
	StmtSelect StatementKind = iota
	StmtExec
)

// Classify returns whether sql should be run via *sql.DB.Query (read) or
// *sql.DB.Exec (write/DDL). Strips leading whitespace and comments first.
func Classify(sql string) StatementKind {
	s := stripLeadingComments(sql)
	if s == "" {
		return StmtSelect
	}
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			end++
			continue
		}
		break
	}
	word := strings.ToUpper(s[:end])
	switch word {
	case "SELECT", "EXPLAIN", "PRAGMA", "WITH", "VALUES":
		return StmtSelect
	}
	return StmtExec
}

func stripLeadingComments(s string) string {
	for {
		s = strings.TrimLeft(s, " \t\r\n")
		if strings.HasPrefix(s, "--") {
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = s[i+1:]
				continue
			}
			return ""
		}
		if strings.HasPrefix(s, "/*") {
			if j := strings.Index(s, "*/"); j >= 0 {
				s = s[j+2:]
				continue
			}
			return ""
		}
		return s
	}
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

// RunOpts caps result size and optionally sets a schema prefix for the session.
// SQLite has no USE statement; Database is noted but not applied.
type RunOpts struct {
	MaxRows  int
	Database string
}

// Run executes one statement and returns rows or rows_affected. SQLite is
// file-based — there is no USE mechanism, so RunOpts.Database is ignored.
func Run(ctx context.Context, sdb *sql.DB, statement string, opts RunOpts) (ExecResult, error) {
	if opts.MaxRows <= 0 {
		opts.MaxRows = 10000
	}
	kind := Classify(statement)

	conn, err := sdb.Conn(ctx)
	if err != nil {
		return ExecResult{Kind: kind}, err
	}
	defer conn.Close()

	if kind == StmtExec {
		res, err := conn.ExecContext(ctx, statement)
		if err != nil {
			return ExecResult{Kind: kind}, err
		}
		n, _ := res.RowsAffected()
		return ExecResult{Kind: kind, RowsAffected: n}, nil
	}
	rows, err := conn.QueryContext(ctx, statement)
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
	return out, rows.Err()
}
