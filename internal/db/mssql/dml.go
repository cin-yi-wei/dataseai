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

// --- 無主鍵表：整列所有欄位值比對 + COUNT 守衛（同 MySQL 的做法）。

// whereByMatch：非 NULL 用 `col = @pN`，NULL 用 `col IS NULL`；佔位符從 startN 起算。
func whereByMatch(m MSSQL, cols []string, vals []any, startN int) (string, []any) {
	parts := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols))
	n := startN
	for i, c := range cols {
		if vals[i] == nil {
			parts = append(parts, m.QuoteIdent(c)+" IS NULL")
			continue
		}
		parts = append(parts, m.QuoteIdent(c)+" = "+m.Placeholder(n))
		args = append(args, vals[i])
		n++
	}
	return strings.Join(parts, " AND "), args
}

func (m MSSQL) matchGuard(ctx context.Context, sqlDB *sql.DB, schema, table string, matchCols []string, matchVals []any) error {
	where, args := whereByMatch(m, matchCols, matchVals, 1)
	var cnt int64
	if err := sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+qualifiedName(m, schema, table)+" WHERE "+where, args...,
	).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		return errors.New("找不到符合的列（資料可能已被其他人變動）")
	}
	if cnt > 1 {
		return db.ErrAmbiguousRow
	}
	return nil
}

func (m MSSQL) UpdateCellByMatch(ctx context.Context, sqlDB *sql.DB, schema, table string, matchCols []string, matchVals []any, col string, newVal any) (int64, error) {
	if len(matchCols) == 0 || len(matchCols) != len(matchVals) {
		return 0, errors.New("no match columns")
	}
	if err := m.matchGuard(ctx, sqlDB, schema, table, matchCols, matchVals); err != nil {
		return 0, err
	}
	// newVal 用 @p1，比對條件從 @p2 起算。
	where, wargs := whereByMatch(m, matchCols, matchVals, 2)
	stmt := fmt.Sprintf("UPDATE %s SET %s = @p1 WHERE %s", qualifiedName(m, schema, table), m.QuoteIdent(col), where)
	res, err := sqlDB.ExecContext(ctx, stmt, append([]any{newVal}, wargs...)...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (m MSSQL) DeleteRowByMatch(ctx context.Context, sqlDB *sql.DB, schema, table string, matchCols []string, matchVals []any) (int64, error) {
	if len(matchCols) == 0 || len(matchCols) != len(matchVals) {
		return 0, errors.New("no match columns")
	}
	if err := m.matchGuard(ctx, sqlDB, schema, table, matchCols, matchVals); err != nil {
		return 0, err
	}
	where, args := whereByMatch(m, matchCols, matchVals, 1)
	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s", qualifiedName(m, schema, table), where)
	res, err := sqlDB.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
