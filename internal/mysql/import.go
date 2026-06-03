package mysql

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

func ImportCSV(ctx context.Context, db *sql.DB, r io.Reader, schema, table string) (int, []string, error) {
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err != nil {
		return 0, nil, fmt.Errorf("read header: %w", err)
	}
	if len(header) == 0 {
		return 0, nil, fmt.Errorf("empty csv")
	}
	cols := make([]string, len(header))
	placeholders := make([]string, len(header))
	for i, col := range header {
		cols[i] = QuoteIdent(strings.TrimSpace(col))
		placeholders[i] = "?"
	}
	stmt := "INSERT INTO " + qualifiedName(schema, table) + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"

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
