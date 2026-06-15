package mssql

import (
	"context"
	"database/sql"
	"encoding/csv"
	"io"
)

// ExportCSV writes the full contents of schema.table as CSV to w.
func ExportCSV(ctx context.Context, db *sql.DB, w io.Writer, schema, table string) error {
	m := MSSQL{}
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+qualifiedName(m, schema, table))
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return err
	}
	dbTypes := columnDatabaseTypes(colTypes)
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
			record[i] = normalizeValueForCSV(val, dbTypes[i])
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
