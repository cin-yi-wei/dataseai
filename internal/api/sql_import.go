package api

import (
	"context"
	"database/sql"
	"fmt"
	"io"
)

// splitSQL splits a SQL script into statements on top-level semicolons,
// tracking single/double/backtick quoted strings and line/block comments.
// It is intentionally simple — meant to handle dataseai's own SQL dump
// format plus most well-formed user dumps. Not a full SQL parser.
func splitSQL(text string) []string {
	out := make([]string, 0, 64)
	cur := make([]byte, 0, 256)
	inSingle, inDouble, inBacktick := false, false, false
	inLine, inBlock := false, false
	for i := 0; i < len(text); i++ {
		c := text[i]
		var n byte
		if i+1 < len(text) {
			n = text[i+1]
		}
		switch {
		case inLine:
			cur = append(cur, c)
			if c == '\n' {
				inLine = false
			}
		case inBlock:
			cur = append(cur, c)
			if c == '*' && n == '/' {
				cur = append(cur, n)
				i++
				inBlock = false
			}
		case inSingle:
			cur = append(cur, c)
			if c == '\\' && n != 0 {
				cur = append(cur, n)
				i++
				continue
			}
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			cur = append(cur, c)
			if c == '\\' && n != 0 {
				cur = append(cur, n)
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
		case inBacktick:
			cur = append(cur, c)
			if c == '`' {
				inBacktick = false
			}
		case c == '-' && n == '-':
			inLine = true
			cur = append(cur, c)
		case c == '/' && n == '*':
			inBlock = true
			cur = append(cur, c)
		case c == '\'':
			inSingle = true
			cur = append(cur, c)
		case c == '"':
			inDouble = true
			cur = append(cur, c)
		case c == '`':
			inBacktick = true
			cur = append(cur, c)
		case c == ';':
			s := trimSQL(cur)
			if len(s) > 0 {
				out = append(out, s)
			}
			cur = cur[:0]
		default:
			cur = append(cur, c)
		}
	}
	if s := trimSQL(cur); len(s) > 0 {
		out = append(out, s)
	}
	return out
}

func trimSQL(b []byte) string {
	start, end := 0, len(b)
	for start < end {
		c := b[start]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			start++
			continue
		}
		break
	}
	for end > start {
		c := b[end-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			end--
			continue
		}
		break
	}
	return string(b[start:end])
}

// importSQL reads a SQL script from r, splits it into statements, and
// executes them sequentially against sqlDB. Returns the number of
// statements that executed successfully and a list of any per-statement
// errors. A fatal read/begin/commit error is returned as the second
// error return; per-statement errors do not abort the import.
func importSQL(ctx context.Context, sqlDB *sql.DB, r io.Reader) (int, []string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, nil, err
	}
	stmts := splitSQL(string(data))
	if len(stmts) == 0 {
		return 0, nil, nil
	}
	executed := 0
	errs := make([]string, 0)
	for i, s := range stmts {
		if _, err := sqlDB.ExecContext(ctx, s); err != nil {
			errs = append(errs, fmt.Sprintf("#%d: %s", i+1, err.Error()))
			continue
		}
		executed++
	}
	return executed, errs, nil
}
