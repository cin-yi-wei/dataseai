package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func qualifiedName(s Snowflake, schema, table string) string {
	if schema == "" {
		return s.QuoteIdent(table)
	}
	return s.QuoteIdent(schema) + "." + s.QuoteIdent(table)
}

func whereByPK(s Snowflake, pkCols []string, pkVals []any) (string, []any) {
	parts := make([]string, len(pkCols))
	args := make([]any, len(pkCols))
	for i, col := range pkCols {
		parts[i] = s.QuoteIdent(col) + " = ?"
		args[i] = pkVals[i]
	}
	return strings.Join(parts, " AND "), args
}

func (s Snowflake) PrimaryKey(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT k.COLUMN_NAME
		 FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS c
		 JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE k
		   ON k.CONSTRAINT_NAME = c.CONSTRAINT_NAME AND k.CONSTRAINT_SCHEMA = c.CONSTRAINT_SCHEMA
		 WHERE c.CONSTRAINT_TYPE = 'PRIMARY KEY'
		   AND c.TABLE_SCHEMA = ? AND c.TABLE_NAME = ?
		 ORDER BY k.ORDINAL_POSITION`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
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

func (s Snowflake) UpdateCell(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where, args := whereByPK(s, pkCols, pkVals)
	res, err := sqlDB.ExecContext(
		ctx,
		"UPDATE "+qualifiedName(s, schema, table)+" SET "+s.QuoteIdent(col)+" = ? WHERE "+where,
		append([]any{newVal}, args...)...,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s Snowflake) InsertRow(ctx context.Context, sqlDB *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
	if len(cols) == 0 || len(cols) != len(vals) {
		return 0, errors.New("cols/vals empty or mismatched")
	}
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = s.QuoteIdent(col)
		placeholders[i] = "?"
	}
	res, err := sqlDB.ExecContext(
		ctx,
		"INSERT INTO "+qualifiedName(s, schema, table)+
			" ("+strings.Join(quotedCols, ", ")+") VALUES ("+strings.Join(placeholders, ", ")+")",
		vals...,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s Snowflake) DeleteRow(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where, args := whereByPK(s, pkCols, pkVals)
	res, err := sqlDB.ExecContext(
		ctx,
		"DELETE FROM "+qualifiedName(s, schema, table)+" WHERE "+where,
		args...,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
