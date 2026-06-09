package mysql

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
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
	q := MySQL{}.QuoteIdent
	quotedCols := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = q(col)
	}
	prefix := "INSERT INTO " + q(table) + " (" + strings.Join(quotedCols, ", ") + ") VALUES "
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
	case []byte:
		return "'" + strings.ReplaceAll(string(x), "'", "''") + "'"
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'"
	default:
		return fmt.Sprint(x)
	}
}
