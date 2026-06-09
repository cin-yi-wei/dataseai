package mssql

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// ImportCSV reads CSV from r and bulk-inserts rows into schema.table.
// Returns (inserted count, per-row errors, fatal error).
func ImportCSV(ctx context.Context, sqlDB *sql.DB, r io.Reader, schema, table string) (int, []string, error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	headers, err := cr.Read()
	if err != nil {
		return 0, nil, fmt.Errorf("read header: %w", err)
	}

	m := MSSQL{}
	quotedCols := make([]string, len(headers))
	placeholders := make([]string, len(headers))
	for i, h := range headers {
		quotedCols[i] = m.QuoteIdent(h)
		placeholders[i] = m.Placeholder(i + 1)
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qualifiedName(m, schema, table),
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)

	var inserted int
	var errs []string
	for {
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("row %d: parse error: %v", inserted+1, err))
			continue
		}
		args := make([]any, len(record))
		for i, v := range record {
			args[i] = v
		}
		if _, err := sqlDB.ExecContext(ctx, stmt, args...); err != nil {
			errs = append(errs, fmt.Sprintf("row %d: %v", inserted+1, err))
			continue
		}
		inserted++
	}
	return inserted, errs, nil
}
