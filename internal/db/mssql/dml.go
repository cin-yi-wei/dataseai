package mssql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// qualifiedName builds a fully-qualified identifier for a table. The first arg
// is a database name (the sidebar lists databases for MSSQL, not schemas), so
// the result is [database].[dbo].[table]; objects are assumed to live in dbo.
// When database is empty it falls back to [dbo].[table] in the connected DB.
func qualifiedName(m MSSQL, database, table string) string {
	if database == "" {
		return m.QuoteIdent(defaultSchema) + "." + m.QuoteIdent(table)
	}
	return m.QuoteIdent(database) + "." + m.QuoteIdent(defaultSchema) + "." + m.QuoteIdent(table)
}

func whereByPK(m MSSQL, pkCols []string, pkVals []any, startN int) (string, []any) {
	parts := make([]string, len(pkCols))
	args := make([]any, len(pkCols))
	for i, col := range pkCols {
		parts[i] = m.QuoteIdent(col) + " = " + m.Placeholder(startN+i)
		args[i] = pkVals[i]
	}
	return strings.Join(parts, " AND "), args
}

func (m MSSQL) PrimaryKey(ctx context.Context, sqlDB *sql.DB, database, table string) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		m.useDB(database)+
			`SELECT kcu.COLUMN_NAME
		 FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		 JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
		   ON kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
		  AND kcu.TABLE_SCHEMA = tc.TABLE_SCHEMA
		  AND kcu.TABLE_NAME = tc.TABLE_NAME
		 WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
		   AND tc.TABLE_SCHEMA = @p1 AND tc.TABLE_NAME = @p2
		 ORDER BY kcu.ORDINAL_POSITION`,
		defaultSchema, table)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, db.ErrNoPrimaryKey
	}
	return out, nil
}

func (m MSSQL) UpdateCell(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where, whereArgs := whereByPK(m, pkCols, pkVals, 2)
	stmt := fmt.Sprintf("UPDATE %s SET %s = @p1 WHERE %s",
		qualifiedName(m, schema, table),
		m.QuoteIdent(col),
		where,
	)
	args := append([]any{newVal}, whereArgs...)
	res, err := sqlDB.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// InsertRow inserts a row and returns the value of the first PK column via
// the OUTPUT clause. Falls back to plain INSERT if there is no primary key.
func (m MSSQL) InsertRow(ctx context.Context, sqlDB *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
	if len(cols) == 0 || len(cols) != len(vals) {
		return 0, errors.New("cols/vals empty or mismatched")
	}

	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = m.QuoteIdent(col)
		placeholders[i] = m.Placeholder(i + 1)
	}

	pkCols, pkErr := m.PrimaryKey(ctx, sqlDB, schema, table)
	if pkErr != nil {
		if !errors.Is(pkErr, db.ErrNoPrimaryKey) {
			return 0, pkErr
		}
		stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			qualifiedName(m, schema, table),
			strings.Join(quotedCols, ", "),
			strings.Join(placeholders, ", "),
		)
		_, err := sqlDB.ExecContext(ctx, stmt, vals...)
		return 0, err
	}

	stmt := fmt.Sprintf("INSERT INTO %s (%s) OUTPUT INSERTED.%s VALUES (%s)",
		qualifiedName(m, schema, table),
		strings.Join(quotedCols, ", "),
		m.QuoteIdent(pkCols[0]),
		strings.Join(placeholders, ", "),
	)
	var id int64
	err := sqlDB.QueryRowContext(ctx, stmt, vals...).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (m MSSQL) DeleteRow(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where, args := whereByPK(m, pkCols, pkVals, 1)
	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s", qualifiedName(m, schema, table), where)
	res, err := sqlDB.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
