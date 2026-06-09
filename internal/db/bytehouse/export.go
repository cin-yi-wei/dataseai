package bytehouse

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
)

// ExportCSV writes the full contents of schema.table as CSV to w.
func ExportCSV(ctx context.Context, db *sql.DB, w io.Writer, schema, table string) error {
	b := BH{}
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+qualifiedName(b, schema, table))
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		record := make([]string, len(cols))
		for i, val := range vals {
			record[i] = anyToCSV(val)
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func anyToCSV(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}
