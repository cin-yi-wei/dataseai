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
	q := MySQL{}.QuoteIdent
	cols := make([]string, len(header))
	placeholders := make([]string, len(header))
	for i, col := range header {
		cols[i] = q(strings.TrimSpace(col))
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
			// Treat empty cell as NULL — matches dataseai's own CSV export
			// (which writes "" for NULL) and how spreadsheets represent
			// missing values. Columns that legitimately want empty strings
			// will accept NULL on a nullable column or default; otherwise
			// the user will see a constraint error.
			if val == "" {
				args[i] = nil
				continue
			}
			args[i] = sanitizeCSVValue(val)
		}
		if _, err := db.ExecContext(ctx, stmt, args...); err != nil {
			errs = append(errs, fmt.Sprintf("row %d: %s", inserted+len(errs)+1, err.Error()))
			continue
		}
		inserted++
	}
	return inserted, errs, nil
}

// sanitizeCSVValue heals legacy CSVs that contain Go-default time format
// like "2026-05-18 15:26:51 +0000 UTC" — MySQL rejects the suffix. Drop
// trailing " +HHMM UTC" / " +HHMM" so the datetime parses.
func sanitizeCSVValue(s string) string {
	if len(s) < 20 {
		return s
	}
	// Look for " UTC" suffix; trim back through the offset token too.
	if strings.HasSuffix(s, " UTC") {
		s = strings.TrimSuffix(s, " UTC")
	}
	// Now possibly ends in " +0000" / " -0700".
	if i := strings.LastIndex(s, " "); i > 0 && i >= len(s)-6 {
		tail := s[i+1:]
		if len(tail) == 5 && (tail[0] == '+' || tail[0] == '-') {
			allDigits := true
			for j := 1; j < len(tail); j++ {
				if tail[j] < '0' || tail[j] > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				s = s[:i]
			}
		}
	}
	return s
}
