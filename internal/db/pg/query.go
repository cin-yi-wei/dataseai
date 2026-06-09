package pg

import (
	"context"
	"database/sql"
	"strings"
)

// StatementKind distinguishes read (SELECT-like) from write/DDL statements.
type StatementKind int

const (
	StmtSelect StatementKind = iota
	StmtExec
)

// Classify returns whether sql should be run via *sql.DB.Query (read) or
// *sql.DB.Exec (write/DDL). Strips leading whitespace and SQL comments
// (line `--` and block `/* */`) before inspecting the first keyword.
// The logic mirrors mysql.Classify; PG also recognises SHOW as a read verb.
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
	case "SELECT", "SHOW", "EXPLAIN", "WITH", "VALUES", "TABLE":
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

// RunOpts caps the result size and optionally sets the search_path for the
// connection (PG equivalent of MySQL's "USE <db>").
type RunOpts struct {
	MaxRows  int    // hard row cap; 0 = 10000
	Database string // if set, SET search_path = "<db>" is executed first
}

// Run executes one statement, classifies it, and returns rows or rows_affected.
// Uses ctx for timeout. Acquires a dedicated *sql.Conn so that search_path +
// query share session state. Caller owns the pool.
func Run(ctx context.Context, db *sql.DB, statement string, opts RunOpts) (ExecResult, error) {
	if opts.MaxRows <= 0 {
		opts.MaxRows = 10000
	}
	kind := Classify(statement)

	conn, err := db.Conn(ctx)
	if err != nil {
		return ExecResult{Kind: kind}, err
	}
	defer conn.Close()

	if opts.Database != "" {
		p := PG{}
		if _, err := conn.ExecContext(ctx, "SET search_path = "+p.QuoteIdent(opts.Database)); err != nil {
			return ExecResult{Kind: kind}, err
		}
	}

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
