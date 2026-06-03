package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var ErrNoPrimaryKey = errors.New("table has no primary key, edit disabled")

func qualifiedName(schema, table string) string {
	if schema == "" {
		return QuoteIdent(table)
	}
	return QuoteIdent(schema) + "." + QuoteIdent(table)
}

// PrimaryKey returns the ordered primary-key column names for a table.
// MySQL uses information_schema; sqlite is supported as a lightweight test stub.
func PrimaryKey(ctx context.Context, db *sql.DB, schema, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ? AND column_key = 'PRI'
		ORDER BY ordinal_position
	`, schema, table)
	if err == nil {
		defer rows.Close()
		var out []string
		for rows.Next() {
			var col string
			if err := rows.Scan(&col); err != nil {
				return nil, err
			}
			out = append(out, col)
		}
		return out, rows.Err()
	}
	if !strings.Contains(err.Error(), "no such table") {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, "PRAGMA table_info("+QuoteIdent(table)+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var (
			cid     int
			name    string
			typ     string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		if pk > 0 {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

func whereByPK(pkCols []string, pkVals []any) (string, []any) {
	parts := make([]string, len(pkCols))
	args := make([]any, len(pkCols))
	for i, col := range pkCols {
		parts[i] = QuoteIdent(col) + " = ?"
		args[i] = pkVals[i]
	}
	return strings.Join(parts, " AND "), args
}

func UpdateCell(ctx context.Context, db *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, ErrNoPrimaryKey
	}
	where, args := whereByPK(pkCols, pkVals)
	res, err := db.ExecContext(
		ctx,
		"UPDATE "+qualifiedName(schema, table)+" SET "+QuoteIdent(col)+" = ? WHERE "+where,
		append([]any{newVal}, args...)...,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func InsertRow(ctx context.Context, db *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
	if len(cols) == 0 || len(cols) != len(vals) {
		return 0, errors.New("cols/vals empty or mismatched")
	}
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = QuoteIdent(col)
		placeholders[i] = "?"
	}
	res, err := db.ExecContext(
		ctx,
		"INSERT INTO "+qualifiedName(schema, table)+" ("+strings.Join(quotedCols, ", ")+") VALUES ("+strings.Join(placeholders, ", ")+")",
		vals...,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func DeleteRow(ctx context.Context, db *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, ErrNoPrimaryKey
	}
	where, args := whereByPK(pkCols, pkVals)
	res, err := db.ExecContext(ctx, "DELETE FROM "+qualifiedName(schema, table)+" WHERE "+where, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
