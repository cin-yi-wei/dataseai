package pg

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// ImportCSV reads a CSV from r (first row = header = column names) and
// inserts each subsequent row into schema.table using $N positional
// placeholders. Returns the number of rows inserted, any per-row error
// strings, and a fatal error if the CSV header could not be read.
func ImportCSV(ctx context.Context, db *sql.DB, r io.Reader, schema, table string) (int, []string, error) {
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err != nil {
		return 0, nil, fmt.Errorf("read header: %w", err)
	}
	if len(header) == 0 {
		return 0, nil, fmt.Errorf("empty csv")
	}
	p := PG{}
	cols := make([]string, len(header))
	placeholders := make([]string, len(header))
	for i, col := range header {
		cols[i] = p.QuoteIdent(strings.TrimSpace(col))
		placeholders[i] = p.Placeholder(i + 1)
	}
	stmt := "INSERT INTO " + qualifiedName(p, schema, table) +
		" (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"

	inserted := 0
	var errs []string
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errs = append(errs, "csv: "+err.Error())
			continue
		}
		args := make([]any, len(row))
		for i, val := range row {
			args[i] = val
		}
		if _, err := db.ExecContext(ctx, stmt, args...); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		inserted++
	}
	return inserted, errs, nil
}
