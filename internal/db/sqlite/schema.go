package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// masterTable returns the sqlite_master table qualified by schema when needed.
func masterTable(schema string) string {
	if schema == "" || schema == "main" {
		return "sqlite_master"
	}
	return schema + ".sqlite_master"
}

// pragmaPrefix returns "<schema>." when schema is non-empty and not "main",
// so PRAGMA statements can target attached databases.
func pragmaPrefix(schema string) string {
	if schema == "" || schema == "main" {
		return ""
	}
	return schema + "."
}

func (s SQLite) ListDatabases(ctx context.Context, sqlDB *sql.DB, _ bool) ([]string, error) {
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

func (s SQLite) ListTables(ctx context.Context, sqlDB *sql.DB, schema string) ([]db.TableInfo, error) {
	q := fmt.Sprintf(
		"SELECT name FROM %s WHERE type IN ('table','view') ORDER BY name",
		masterTable(schema),
	)
	rows, err := sqlDB.QueryContext(ctx, q)
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

func (s SQLite) ListSchemaColumns(ctx context.Context, sqlDB *sql.DB, schema string) (map[string][]string, error) {
	tables, err := s.ListTables(ctx, sqlDB, schema)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string)
	for _, t := range tables {
		q := fmt.Sprintf("PRAGMA %stable_info(%s)", pragmaPrefix(schema), s.QuoteIdent(t.Name))
		rows, err := sqlDB.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				rows.Close()
				return nil, err
			}
			out[t.Name] = append(out[t.Name], name)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s SQLite) DescribeTable(ctx context.Context, sqlDB *sql.DB, schema, table string) (db.Structure, error) {
	q := fmt.Sprintf("PRAGMA %stable_info(%s)", pragmaPrefix(schema), s.QuoteIdent(table))
	rows, err := sqlDB.QueryContext(ctx, q)
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

	var createSQL sql.NullString
	err = sqlDB.QueryRowContext(ctx,
		fmt.Sprintf("SELECT sql FROM %s WHERE type='table' AND name=?", masterTable(schema)),
		table,
	).Scan(&createSQL)
	if err == nil && createSQL.Valid {
		out.CreateSQL = createSQL.String
	}
	return out, nil
}

func (s SQLite) ListIndexes(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.Index, error) {
	q := fmt.Sprintf("PRAGMA %sindex_list(%s)", pragmaPrefix(schema), s.QuoteIdent(table))
	rows, err := sqlDB.QueryContext(ctx, q)
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
		iq := fmt.Sprintf("PRAGMA %sindex_info(%s)", pragmaPrefix(schema), s.QuoteIdent(e.name))
		irows, err := sqlDB.QueryContext(ctx, iq)
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
		out = append(out, db.Index{
			Name:    e.name,
			Columns: cols,
			Unique:  e.unique != 0,
		})
	}
	return out, nil
}

func (s SQLite) ListForeignKeys(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.ForeignKey, error) {
	q := fmt.Sprintf("PRAGMA %sforeign_key_list(%s)", pragmaPrefix(schema), s.QuoteIdent(table))
	rows, err := sqlDB.QueryContext(ctx, q)
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
