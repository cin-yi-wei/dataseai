package mysql

import (
	"context"
	"database/sql"
)

// ListDatabases returns visible database names excluding MySQL/system schemas.
func ListDatabases(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT schema_name
		 FROM information_schema.schemata
		 WHERE schema_name NOT IN ('mysql','information_schema','performance_schema','sys')
		 ORDER BY schema_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

type TableInfo struct {
	Name    string `json:"name"`
	RowsEst int64  `json:"rows_est"`
	SizeMB  int64  `json:"size_mb"`
}

func ListTables(ctx context.Context, db *sql.DB, schema string) ([]TableInfo, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT table_name,
		        COALESCE(table_rows, 0),
		        COALESCE(ROUND((data_length + index_length) / 1024 / 1024), 0)
		 FROM information_schema.tables
		 WHERE table_schema = ?
		 ORDER BY table_name`,
		schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.RowsEst, &t.SizeMB); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
