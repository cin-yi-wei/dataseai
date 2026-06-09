package clickhouse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func qualifiedName(c CH, schema, table string) string {
	if schema == "" {
		return c.QuoteIdent(table)
	}
	return c.QuoteIdent(schema) + "." + c.QuoteIdent(table)
}

// PrimaryKey always returns ErrNoPrimaryKey — ClickHouse ORDER BY/PRIMARY KEY
// is a physical sort hint, not a row identity constraint.
func (CH) PrimaryKey(_ context.Context, _ *sql.DB, _, _ string) ([]string, error) {
	return nil, db.ErrNoPrimaryKey
}

func (CH) UpdateCell(_ context.Context, _ *sql.DB, _, _ string, _ []string, _ []any, _ string, _ any) (int64, error) {
	return 0, errors.New("ClickHouse cell updates require ALTER TABLE mutations — not supported via this interface")
}

func (c CH) InsertRow(ctx context.Context, sqlDB *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
	if len(cols) == 0 || len(cols) != len(vals) {
		return 0, errors.New("cols/vals empty or mismatched")
	}
	quotedCols := make([]string, len(cols))
	phs := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = c.QuoteIdent(col)
		phs[i] = "?"
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qualifiedName(c, schema, table),
		strings.Join(quotedCols, ", "),
		strings.Join(phs, ", "),
	)
	_, err := sqlDB.ExecContext(ctx, stmt, vals...)
	return 0, err
}

func (CH) DeleteRow(_ context.Context, _ *sql.DB, _, _ string, _ []string, _ []any) (int64, error) {
	return 0, errors.New("ClickHouse row deletes require ALTER TABLE mutations — not supported via this interface")
}
