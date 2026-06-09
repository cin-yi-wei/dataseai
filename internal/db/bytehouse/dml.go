package bytehouse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func qualifiedName(b BH, schema, table string) string {
	if schema == "" {
		return b.QuoteIdent(table)
	}
	return b.QuoteIdent(schema) + "." + b.QuoteIdent(table)
}

// PrimaryKey always returns ErrNoPrimaryKey for ByteHouse/ClickHouse.
// ClickHouse uses ORDER BY / PRIMARY KEY for physical sorting, not for
// row-level identity. Cell-level updates require ALTER TABLE mutations.
func (b BH) PrimaryKey(_ context.Context, _ *sql.DB, _, _ string) ([]string, error) {
	return nil, db.ErrNoPrimaryKey
}

// UpdateCell is not supported for ByteHouse. ClickHouse updates are async
// ALTER TABLE ... UPDATE mutations and cannot be expressed as a single-cell
// targeted write.
func (b BH) UpdateCell(_ context.Context, _ *sql.DB, _, _ string, _ []string, _ []any, _ string, _ any) (int64, error) {
	return 0, errors.New("ByteHouse/ClickHouse cell updates require ALTER TABLE mutations — not supported via this interface")
}

// InsertRow inserts a row using standard INSERT syntax.
func (b BH) InsertRow(ctx context.Context, sqlDB *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
	if len(cols) == 0 || len(cols) != len(vals) {
		return 0, errors.New("cols/vals empty or mismatched")
	}
	quotedCols := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = b.QuoteIdent(col)
	}
	phs := make([]string, len(cols))
	for i := range phs {
		phs[i] = "?"
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qualifiedName(b, schema, table),
		strings.Join(quotedCols, ", "),
		strings.Join(phs, ", "),
	)
	_, err := sqlDB.ExecContext(ctx, stmt, vals...)
	return 0, err
}

// DeleteRow is not supported for ByteHouse. ClickHouse deletes are async
// ALTER TABLE ... DELETE mutations.
func (b BH) DeleteRow(_ context.Context, _ *sql.DB, _, _ string, _ []string, _ []any) (int64, error) {
	return 0, errors.New("ByteHouse/ClickHouse row deletes require ALTER TABLE mutations — not supported via this interface")
}
