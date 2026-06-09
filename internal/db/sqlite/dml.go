package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func qualifiedName(s SQLite, schema, table string) string {
	if schema == "" || schema == "main" {
		return s.QuoteIdent(table)
	}
	return schema + "." + s.QuoteIdent(table)
}

func whereByPK(s SQLite, pkCols []string, pkVals []any) (string, []any) {
	parts := make([]string, len(pkCols))
	args := make([]any, len(pkCols))
	for i, col := range pkCols {
		parts[i] = s.QuoteIdent(col) + " = ?"
		args[i] = pkVals[i]
	}
	return strings.Join(parts, " AND "), args
}

// PrimaryKey returns the ordered primary-key column names for a table.
// SQLite encodes PK membership in PRAGMA table_info as pk > 0; the pk value
// is the 1-based position within the composite key.
func (s SQLite) PrimaryKey(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]string, error) {
	type pkRow struct {
		name string
		pos  int
	}
	q := "PRAGMA " + pragmaPrefix(schema) + "table_info(" + s.QuoteIdent(table) + ")"
	rows, err := sqlDB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pks []pkRow
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		if pk > 0 {
			pks = append(pks, pkRow{name: name, pos: pk})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Sort by pk position (already in order for single-col PK; sort for composite)
	for i := 1; i < len(pks); i++ {
		for j := i; j > 0 && pks[j].pos < pks[j-1].pos; j-- {
			pks[j], pks[j-1] = pks[j-1], pks[j]
		}
	}
	out := make([]string, len(pks))
	for i, pk := range pks {
		out[i] = pk.name
	}
	return out, nil
}

func (s SQLite) UpdateCell(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error) {
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

func (s SQLite) InsertRow(ctx context.Context, sqlDB *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
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

func (s SQLite) DeleteRow(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error) {
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
