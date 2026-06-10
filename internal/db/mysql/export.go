package mysql

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"
)

func ExportCSV(ctx context.Context, db *sql.DB, w io.Writer, schema, table string) error {
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+qualifiedName(schema, table))
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		record := make([]string, len(cols))
		for i, val := range vals {
			record[i] = anyToCSV(val)
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func ExportSQL(ctx context.Context, db *sql.DB, w io.Writer, schema, table string) error {
	q := MySQL{}.QuoteIdent
	// Emit USE so the dump replays into a connection without a default schema.
	if schema != "" {
		if _, err := fmt.Fprintln(w, "USE "+q(schema)+";"); err != nil {
			return err
		}
	}
	var tableName, createStmt string
	if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE "+qualifiedName(schema, table)).Scan(&tableName, &createStmt); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, createStmt+";"); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+qualifiedName(schema, table))
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	quotedCols := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = q(col)
	}
	prefix := "INSERT INTO " + qualifiedName(schema, table) + " (" + strings.Join(quotedCols, ", ") + ") VALUES "
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		literals := make([]string, len(cols))
		for i, val := range vals {
			literals[i] = anyToSQLLiteral(val)
		}
		if _, err := fmt.Fprintln(w, prefix+"("+strings.Join(literals, ", ")+");"); err != nil {
			return err
		}
	}
	return rows.Err()
}

func anyToCSV(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}

func anyToSQLLiteral(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case time.Time:
		// MySQL DATETIME literal — no zone, microsecond precision when present.
		if x.Nanosecond() != 0 {
			return "'" + x.UTC().Format("2006-01-02 15:04:05.000000") + "'"
		}
		return "'" + x.UTC().Format("2006-01-02 15:04:05") + "'"
	case bool:
		if x {
			return "1"
		}
		return "0"
	case []byte:
		return "'" + escapeMySQLString(string(x)) + "'"
	case string:
		return "'" + escapeMySQLString(x) + "'"
	default:
		return fmt.Sprint(x)
	}
}

// escapeMySQLString escapes characters that break MySQL string literals.
// Handles the cases the dump can hit even without ANSI_QUOTES mode.
func escapeMySQLString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case 0:
			b.WriteString(`\0`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case 0x1a:
			b.WriteString(`\Z`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
