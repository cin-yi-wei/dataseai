package oracle

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type StatementKind int

const (
	StmtSelect StatementKind = iota
	StmtExec
)

func Classify(s string) StatementKind {
	stripped := stripLeadingComments(s)
	end := 0
	for end < len(stripped) {
		c := stripped[end]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			end++
			continue
		}
		break
	}
	switch strings.ToUpper(stripped[:end]) {
	case "SELECT", "WITH", "SHOW", "DESCRIBE", "EXPLAIN":
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

type ExecResult struct {
	Kind         StatementKind `json:"-"`
	Columns      []string      `json:"columns,omitempty"`
	Rows         [][]any       `json:"rows,omitempty"`
	RowsAffected int64         `json:"rows_affected"`
	DurationMs   int64         `json:"duration_ms"`
	Truncated    bool          `json:"truncated"`
}

type RunOpts struct {
	MaxRows  int
	Database string
}

func Run(ctx context.Context, sdb *sql.DB, statement string, opts RunOpts) (ExecResult, error) {
	if opts.MaxRows <= 0 {
		opts.MaxRows = 10000
	}
	kind := Classify(statement)
	start := time.Now()

	if kind == StmtExec {
		res, err := sdb.ExecContext(ctx, statement)
		if err != nil {
			return ExecResult{Kind: kind}, err
		}
		n, _ := res.RowsAffected()
		return ExecResult{Kind: kind, RowsAffected: n, DurationMs: time.Since(start).Milliseconds()}, nil
	}

	rows, err := sdb.QueryContext(ctx, statement)
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
