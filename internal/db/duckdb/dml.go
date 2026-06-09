package duckdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func qualifiedName(d DuckDB, schema, table string) string {
	if schema == "" || schema == "main" {
		return d.QuoteIdent(table)
	}
	return schema + "." + d.QuoteIdent(table)
}

func whereByPK(d DuckDB, pkCols []string, pkVals []any) (string, []any) {
	parts := make([]string, len(pkCols))
	args := make([]any, len(pkCols))
	for i, col := range pkCols {
		parts[i] = d.QuoteIdent(col) + " = ?"
		args[i] = pkVals[i]
	}
	return strings.Join(parts, " AND "), args
}

func (d DuckDB) PrimaryKey(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]string, error) {
	var pragma string
	if schema != "" && schema != "main" {
		pragma = fmt.Sprintf("PRAGMA %s.table_info(%s)", schema, d.QuoteIdent(table))
	} else {
		pragma = fmt.Sprintf("PRAGMA table_info(%s)", d.QuoteIdent(table))
	}
	rows, err := sqlDB.QueryContext(ctx, pragma)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type pkRow struct {
		name string
		pos  int
	}
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

func (d DuckDB) UpdateCell(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where, args := whereByPK(d, pkCols, pkVals)
	res, err := sqlDB.ExecContext(
		ctx,
		"UPDATE "+qualifiedName(d, schema, table)+" SET "+d.QuoteIdent(col)+" = ? WHERE "+where,
		append([]any{newVal}, args...)...,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (d DuckDB) InsertRow(ctx context.Context, sqlDB *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
	if len(cols) == 0 || len(cols) != len(vals) {
		return 0, errors.New("cols/vals empty or mismatched")
	}
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = d.QuoteIdent(col)
		placeholders[i] = "?"
	}
	res, err := sqlDB.ExecContext(
		ctx,
		"INSERT INTO "+qualifiedName(d, schema, table)+
			" ("+strings.Join(quotedCols, ", ")+") VALUES ("+strings.Join(placeholders, ", ")+")",
		vals...,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (d DuckDB) DeleteRow(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where, args := whereByPK(d, pkCols, pkVals)
	res, err := sqlDB.ExecContext(
		ctx,
		"DELETE FROM "+qualifiedName(d, schema, table)+" WHERE "+where,
		args...,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
