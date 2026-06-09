package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func (c CH) ListDatabases(ctx context.Context, sqlDB *sql.DB, _ bool) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT name FROM system.databases
		 WHERE name NOT IN ('system','information_schema','INFORMATION_SCHEMA','_temporary_and_external_tables')
		 ORDER BY name`)
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

func (c CH) ListTables(ctx context.Context, sqlDB *sql.DB, schema string) ([]db.TableInfo, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT name, toInt64(ifNull(total_rows, 0)), toInt64(toUInt64(ifNull(total_bytes, 0)) / 1048576)
		 FROM system.tables
		 WHERE database = ?
		 ORDER BY name`,
		schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.TableInfo
	for rows.Next() {
		var ti db.TableInfo
		if err := rows.Scan(&ti.Name, &ti.RowsEst, &ti.SizeMB); err != nil {
			return nil, err
		}
		out = append(out, ti)
	}
	return out, rows.Err()
}

func (c CH) ListSchemaColumns(ctx context.Context, sqlDB *sql.DB, schema string) (map[string][]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT table, name FROM system.columns
		 WHERE database = ?
		 ORDER BY table, position`,
		schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]string)
	for rows.Next() {
		var t, col string
		if err := rows.Scan(&t, &col); err != nil {
			return nil, err
		}
		out[t] = append(out[t], col)
	}
	return out, rows.Err()
}

func (c CH) DescribeTable(ctx context.Context, sqlDB *sql.DB, schema, table string) (db.Structure, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT name, type, default_kind, default_expression, comment
		 FROM system.columns
		 WHERE database = ? AND table = ?
		 ORDER BY position`,
		schema, table)
	if err != nil {
		return db.Structure{}, err
	}
	var out db.Structure
	for rows.Next() {
		var col db.Column
		var defaultKind, comment string
		if err := rows.Scan(&col.Name, &col.Type, &defaultKind, &col.Default, &comment); err != nil {
			rows.Close()
			return db.Structure{}, err
		}
		col.Comment = comment
		col.Extra = defaultKind
		col.Nullable = strings.Contains(col.Type, "Nullable(")
		out.Columns = append(out.Columns, col)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return db.Structure{}, err
	}
	if createSQL, err := showCreateTable(ctx, sqlDB, c, schema, table); err == nil {
		out.CreateSQL = createSQL
	}
	return out, nil
}

func showCreateTable(ctx context.Context, sqlDB *sql.DB, c CH, schema, table string) (string, error) {
	var name, ddl string
	err := sqlDB.QueryRowContext(ctx,
		fmt.Sprintf("SHOW CREATE TABLE %s.%s", c.QuoteIdent(schema), c.QuoteIdent(table))).
		Scan(&name, &ddl)
	if err != nil {
		return "", err
	}
	return ddl, nil
}

func (CH) ListIndexes(_ context.Context, _ *sql.DB, _, _ string) ([]db.Index, error) {
	return nil, nil
}

func (CH) ListForeignKeys(_ context.Context, _ *sql.DB, _, _ string) ([]db.ForeignKey, error) {
	return nil, nil
}
