package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// DuckDB supports SQLite-compatible PRAGMA statements as well as standard
// information_schema. We use PRAGMA for column/index/FK introspection and
// information_schema for ListTables / ListSchemaColumns.

func (d DuckDB) ListDatabases(ctx context.Context, sqlDB *sql.DB, _ bool) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx, "PRAGMA database_list")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return nil, err
		}
		if name != "temp" {
			out = append(out, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		out = []string{"main"}
	}
	return out, nil
}

func (d DuckDB) ListTables(ctx context.Context, sqlDB *sql.DB, schema string) ([]db.TableInfo, error) {
	if schema == "" {
		schema = "main"
	}
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = ? AND table_type IN ('BASE TABLE','VIEW')
		 ORDER BY table_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.TableInfo
	for rows.Next() {
		var t db.TableInfo
		if err := rows.Scan(&t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (d DuckDB) ListSchemaColumns(ctx context.Context, sqlDB *sql.DB, schema string) (map[string][]string, error) {
	if schema == "" {
		schema = "main"
	}
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT table_name, column_name FROM information_schema.columns
		 WHERE table_schema = ?
		 ORDER BY table_name, ordinal_position`,
		schema,
	)
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

func (d DuckDB) DescribeTable(ctx context.Context, sqlDB *sql.DB, schema, table string) (db.Structure, error) {
	// Use PRAGMA table_info (DuckDB is SQLite-compatible here)
	var pragma string
	if schema != "" && schema != "main" {
		pragma = fmt.Sprintf("PRAGMA %s.table_info(%s)", schema, d.QuoteIdent(table))
	} else {
		pragma = fmt.Sprintf("PRAGMA table_info(%s)", d.QuoteIdent(table))
	}
	rows, err := sqlDB.QueryContext(ctx, pragma)
	if err != nil {
		return db.Structure{}, err
	}
	var out db.Structure
	for rows.Next() {
		var cid, notnull, pk int
		var c db.Column
		var dflt sql.NullString
		if err := rows.Scan(&cid, &c.Name, &c.Type, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return db.Structure{}, err
		}
		c.Nullable = notnull == 0 && pk == 0
		if dflt.Valid {
			c.Default = dflt.String
		}
		if pk > 0 {
			c.Key = "PRI"
		}
		out.Columns = append(out.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return db.Structure{}, err
	}
	out.CreateSQL = synthesizeCreateTable(schema, table, out.Columns)
	return out, nil
}

func synthesizeCreateTable(schema, table string, cols []db.Column) string {
	if len(cols) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE TABLE %q.%q (\n", schema, table)
	for i, c := range cols {
		nullStr := " NOT NULL"
		if c.Nullable {
			nullStr = ""
		}
		defStr := ""
		if c.Default != "" {
			defStr = " DEFAULT " + c.Default
		}
		comma := ","
		if i == len(cols)-1 {
			comma = ""
		}
		fmt.Fprintf(&sb, "  %q %s%s%s%s\n", c.Name, c.Type, nullStr, defStr, comma)
	}
	sb.WriteString(");")
	return sb.String()
}

func (d DuckDB) ListIndexes(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.Index, error) {
	var pragma string
	if schema != "" && schema != "main" {
		pragma = fmt.Sprintf("PRAGMA %s.index_list(%s)", schema, d.QuoteIdent(table))
	} else {
		pragma = fmt.Sprintf("PRAGMA index_list(%s)", d.QuoteIdent(table))
	}
	rows, err := sqlDB.QueryContext(ctx, pragma)
	if err != nil {
		return nil, err
	}
	type entry struct {
		name   string
		unique int
	}
	var entries []entry
	for rows.Next() {
		var seq, partial int
		var origin string
		var e entry
		if err := rows.Scan(&seq, &e.name, &e.unique, &origin, &partial); err != nil {
			rows.Close()
			return nil, err
		}
		entries = append(entries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]db.Index, 0, len(entries))
	for _, e := range entries {
		var ipr string
		if schema != "" && schema != "main" {
			ipr = fmt.Sprintf("PRAGMA %s.index_info(%s)", schema, d.QuoteIdent(e.name))
		} else {
			ipr = fmt.Sprintf("PRAGMA index_info(%s)", d.QuoteIdent(e.name))
		}
		irows, err := sqlDB.QueryContext(ctx, ipr)
		if err != nil {
			return nil, err
		}
		var cols []string
		for irows.Next() {
			var seqno, cid int
			var colName string
			if err := irows.Scan(&seqno, &cid, &colName); err != nil {
				irows.Close()
				return nil, err
			}
			cols = append(cols, colName)
		}
		irows.Close()
		if err := irows.Err(); err != nil {
			return nil, err
		}
		out = append(out, db.Index{Name: e.name, Columns: cols, Unique: e.unique != 0})
	}
	return out, nil
}

func (d DuckDB) ListForeignKeys(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.ForeignKey, error) {
	var pragma string
	if schema != "" && schema != "main" {
		pragma = fmt.Sprintf("PRAGMA %s.foreign_key_list(%s)", schema, d.QuoteIdent(table))
	} else {
		pragma = fmt.Sprintf("PRAGMA foreign_key_list(%s)", d.QuoteIdent(table))
	}
	rows, err := sqlDB.QueryContext(ctx, pragma)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.ForeignKey
	for rows.Next() {
		var id, seq int
		var refTable, fromCol, toCol, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpdate, &onDelete, &match); err != nil {
			return nil, err
		}
		out = append(out, db.ForeignKey{
			Name:      fmt.Sprintf("fk_%s_%s_%d", strings.ToLower(table), strings.ToLower(fromCol), id),
			Column:    fromCol,
			RefTable:  refTable,
			RefColumn: toCol,
			OnDelete:  onDelete,
			OnUpdate:  onUpdate,
		})
	}
	return out, rows.Err()
}
