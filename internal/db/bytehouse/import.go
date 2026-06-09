package bytehouse

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// ImportCSV reads CSV from r and bulk-inserts rows into schema.table.
func ImportCSV(ctx context.Context, sqlDB *sql.DB, r io.Reader, schema, table string) (int, []string, error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	headers, err := cr.Read()
	if err != nil {
		return 0, nil, fmt.Errorf("read header: %w", err)
	}

	b := BH{}
	quotedCols := make([]string, len(headers))
	phs := make([]string, len(headers))
	for i, h := range headers {
		quotedCols[i] = b.QuoteIdent(h)
		phs[i] = "?"
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qualifiedName(b, schema, table),
		strings.Join(quotedCols, ", "),
		strings.Join(phs, ", "),
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
