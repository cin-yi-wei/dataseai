package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func qualifiedName(p PG, schema, table string) string {
	if schema == "" {
		return p.QuoteIdent(table)
	}
	return p.QuoteIdent(schema) + "." + p.QuoteIdent(table)
}

func whereByPK(p PG, pkCols []string, pkVals []any, startN int) (string, []any) {
	parts := make([]string, len(pkCols))
	args := make([]any, len(pkCols))
	for i, col := range pkCols {
		parts[i] = p.QuoteIdent(col) + " = " + p.Placeholder(startN+i)
		args[i] = pkVals[i]
	}
	return strings.Join(parts, " AND "), args
}

// PrimaryKey returns the ordered primary-key column names for a table using
// the pg_catalog system tables.
func (p PG) PrimaryKey(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]string, error) {
	const q = `
SELECT a.attname
FROM pg_index ix
JOIN pg_class t  ON t.oid = ix.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN pg_attribute a ON a.attrelid = t.oid
     AND a.attnum = ANY(ix.indkey)
WHERE n.nspname = $1
  AND t.relname = $2
  AND ix.indisprimary = true
ORDER BY array_position(ix.indkey, a.attnum::smallint)
`
	rows, err := sqlDB.QueryContext(ctx, q, schema, table)
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

// UpdateCell updates a single cell identified by one or more primary key columns.
func (p PG) UpdateCell(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	// SET col = $1, WHERE pkCol1 = $2 [AND pkCol2 = $3 ...]
	where, whereArgs := whereByPK(p, pkCols, pkVals, 2)
	stmt := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s",
		qualifiedName(p, schema, table),
		p.QuoteIdent(col),
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

// InsertRow inserts a row and returns the value of the first primary-key
// column via RETURNING. If the table has no primary key the plain INSERT is
// executed and 0 is returned.
func (p PG) InsertRow(ctx context.Context, sqlDB *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
	if len(cols) == 0 || len(cols) != len(vals) {
		return 0, errors.New("cols/vals empty or mismatched")
	}

	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = p.QuoteIdent(col)
		placeholders[i] = p.Placeholder(i + 1)
	}

	base := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qualifiedName(p, schema, table),
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)

	// Try to get the primary key so we can use RETURNING.
	pkCols, pkErr := p.PrimaryKey(ctx, sqlDB, schema, table)
	if pkErr != nil {
		if !errors.Is(pkErr, db.ErrNoPrimaryKey) {
			return 0, pkErr
		}
		// No PK — plain INSERT, return 0.
		_, err := sqlDB.ExecContext(ctx, base, vals...)
		if err != nil {
			return 0, err
		}
		return 0, nil
	}

	stmt := base + " RETURNING " + p.QuoteIdent(pkCols[0])
	var id int64
	err := sqlDB.QueryRowContext(ctx, stmt, vals...).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// DeleteRow deletes the row(s) identified by the supplied primary key columns.
func (p PG) DeleteRow(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where, args := whereByPK(p, pkCols, pkVals, 1)
	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s", qualifiedName(p, schema, table), where)
	res, err := sqlDB.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
