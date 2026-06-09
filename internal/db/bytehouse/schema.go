package bytehouse

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// ListDatabases returns ByteHouse database names, excluding system databases.
func (b BH) ListDatabases(ctx context.Context, sqlDB *sql.DB, _ bool) ([]string, error) {
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

func (b BH) ListTables(ctx context.Context, sqlDB *sql.DB, schema string) ([]db.TableInfo, error) {
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

func (b BH) ListSchemaColumns(ctx context.Context, sqlDB *sql.DB, schema string) (map[string][]string, error) {
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
		var t, c string
		if err := rows.Scan(&t, &c); err != nil {
			return nil, err
		}
		out[t] = append(out[t], c)
	}
	return out, rows.Err()
}

func (b BH) DescribeTable(ctx context.Context, sqlDB *sql.DB, schema, table string) (db.Structure, error) {
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
		var c db.Column
		var defaultKind, comment string
		if err := rows.Scan(&c.Name, &c.Type, &defaultKind, &c.Default, &comment); err != nil {
			rows.Close()
			return db.Structure{}, err
		}
		c.Comment = comment
		c.Extra = defaultKind // e.g. "DEFAULT", "MATERIALIZED", "ALIAS"
		// ClickHouse types don't use standard NULLABLE; Nullable(T) wraps the type
		c.Nullable = strings.Contains(c.Type, "Nullable(")
		out.Columns = append(out.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return db.Structure{}, err
	}

	// Use SHOW CREATE TABLE for the DDL representation.
	createSQL, err := showCreateTable(ctx, sqlDB, b, schema, table)
	if err == nil {
		out.CreateSQL = createSQL
	}
	return out, nil
}

func showCreateTable(ctx context.Context, sqlDB *sql.DB, b BH, schema, table string) (string, error) {
	var name, sql string
	err := sqlDB.QueryRowContext(ctx,
		fmt.Sprintf("SHOW CREATE TABLE %s.%s", b.QuoteIdent(schema), b.QuoteIdent(table))).
		Scan(&name, &sql)
	if err != nil {
		return "", err
	}
	return sql, nil
}

// ListIndexes returns empty — ClickHouse/ByteHouse uses MergeTree ORDER BY
// keys as "indexes" but they don't map to the traditional index model.
func (b BH) ListIndexes(_ context.Context, _ *sql.DB, _, _ string) ([]db.Index, error) {
	return nil, nil
}

// ListForeignKeys returns empty — ClickHouse/ByteHouse has no FK constraints.
func (b BH) ListForeignKeys(_ context.Context, _ *sql.DB, _, _ string) ([]db.ForeignKey, error) {
	return nil, nil
}
