package oracle

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func qualifiedName(o Oracle, schema, table string) string {
	if schema == "" {
		return o.QuoteIdent(table)
	}
	return o.QuoteIdent(schema) + "." + o.QuoteIdent(table)
}

func (o Oracle) PrimaryKey(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT col.column_name
		 FROM all_constraints con
		 JOIN all_cons_columns col
		   ON col.owner = con.owner AND col.constraint_name = con.constraint_name
		 WHERE con.constraint_type = 'P'
		   AND con.owner = :1 AND con.table_name = :2
		 ORDER BY col.position`,
		schema, table)
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

func (o Oracle) UpdateCell(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	// :1 = newVal, :2.. = pk values
	pkParts := make([]string, len(pkCols))
	pkArgs := make([]any, len(pkCols))
	for i, c := range pkCols {
		pkParts[i] = o.QuoteIdent(c) + " = " + o.Placeholder(i+2)
		pkArgs[i] = pkVals[i]
	}

	args := append([]any{newVal}, pkArgs...)
	res, err := sqlDB.ExecContext(ctx,
		"UPDATE "+qualifiedName(o, schema, table)+
			" SET "+o.QuoteIdent(col)+" = "+o.Placeholder(1)+
			" WHERE "+strings.Join(pkParts, " AND "),
		args...,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (o Oracle) InsertRow(ctx context.Context, sqlDB *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
	if len(cols) == 0 || len(cols) != len(vals) {
		return 0, errors.New("cols/vals empty or mismatched")
	}
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, c := range cols {
		quotedCols[i] = o.QuoteIdent(c)
		placeholders[i] = o.Placeholder(i + 1)
	}
	_, err := sqlDB.ExecContext(ctx,
		"INSERT INTO "+qualifiedName(o, schema, table)+
			" ("+strings.Join(quotedCols, ", ")+") VALUES ("+strings.Join(placeholders, ", ")+")",
		vals...,
	)
	return 0, err
}

func (o Oracle) DeleteRow(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	pkParts := make([]string, len(pkCols))
	for i, c := range pkCols {
		pkParts[i] = o.QuoteIdent(c) + " = " + o.Placeholder(i+1)
	}
	res, err := sqlDB.ExecContext(ctx,
		"DELETE FROM "+qualifiedName(o, schema, table)+" WHERE "+strings.Join(pkParts, " AND "),
		pkVals...,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
