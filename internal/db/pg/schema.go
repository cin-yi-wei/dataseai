package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func (p PG) DescribeTable(ctx context.Context, sqlDB *sql.DB, schema, table string) (db.Structure, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable,
		        COALESCE(column_default, ''), COALESCE(character_maximum_length::text, '')
		 FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2
		 ORDER BY ordinal_position`,
		schema, table,
	)
	if err != nil {
		return db.Structure{}, err
	}
	var out db.Structure
	for rows.Next() {
		var c db.Column
		var nullable, charMaxLen string
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &c.Default, &charMaxLen); err != nil {
			rows.Close()
			return db.Structure{}, err
		}
		c.Nullable = nullable == "YES"
		if charMaxLen != "" {
			c.Extra = "max_length=" + charMaxLen
		}
		out.Columns = append(out.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return db.Structure{}, err
	}

	// Synthesize a best-effort CREATE TABLE statement from column info.
	out.CreateSQL = synthesizeCreateTable(schema, table, out.Columns)
	return out, nil
}

// synthesizeCreateTable builds a pseudo CREATE TABLE DDL from column metadata.
// PostgreSQL has no SHOW CREATE TABLE, so this is a best-effort approximation.
func synthesizeCreateTable(schema, table string, cols []db.Column) string {
	if len(cols) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE TABLE %q.%q (\n", schema, table)
	for i, c := range cols {
		colType := c.Type
		if strings.HasPrefix(c.Extra, "max_length=") {
			ml := strings.TrimPrefix(c.Extra, "max_length=")
			colType = fmt.Sprintf("%s(%s)", colType, ml)
		}
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
		fmt.Fprintf(&sb, "  %q %s%s%s%s\n", c.Name, colType, nullStr, defStr, comma)
	}
	sb.WriteString(");")
	return sb.String()
}

func (p PG) ListIndexes(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.Index, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT i.relname AS index_name,
		        ix.indisunique AS is_unique,
		        ix.indisprimary AS is_primary,
		        array_to_string(array_agg(a.attname ORDER BY k.n), ',') AS columns
		 FROM pg_class t
		 JOIN pg_namespace n ON n.oid = t.relnamespace
		 JOIN pg_index ix ON ix.indrelid = t.oid
		 JOIN pg_class i ON i.oid = ix.indexrelid
		 JOIN LATERAL unnest(ix.indkey) WITH ORDINALITY AS k(attnum, n) ON TRUE
		 JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum
		 WHERE n.nspname = $1 AND t.relname = $2
		 GROUP BY i.relname, ix.indisunique, ix.indisprimary
		 ORDER BY i.relname`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.Index
	for rows.Next() {
		var name, colsStr string
		var isUnique, isPrimary bool
		if err := rows.Scan(&name, &isUnique, &isPrimary, &colsStr); err != nil {
			return nil, err
		}
		idxType := "btree" // default for pg
		if isPrimary {
			idxType = "btree"
		}
		out = append(out, db.Index{
			Name:      name,
			Columns:   strings.Split(colsStr, ","),
			Unique:    isUnique,
			IndexType: idxType,
		})
	}
	return out, rows.Err()
}

func (p PG) ListForeignKeys(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.ForeignKey, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT
		   kcu.constraint_name,
		   kcu.column_name,
		   ccu.table_name AS foreign_table,
		   ccu.column_name AS foreign_column,
		   rc.update_rule,
		   rc.delete_rule
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.key_column_usage kcu
		   ON kcu.constraint_name = tc.constraint_name
		  AND kcu.table_schema = tc.table_schema
		  AND kcu.table_name = tc.table_name
		 JOIN information_schema.referential_constraints rc
		   ON rc.constraint_name = tc.constraint_name
		  AND rc.constraint_schema = tc.constraint_schema
		 JOIN information_schema.constraint_column_usage ccu
		   ON ccu.constraint_name = tc.constraint_name
		  AND ccu.table_schema = tc.constraint_schema
		 WHERE tc.constraint_type = 'FOREIGN KEY'
		   AND tc.table_schema = $1
		   AND tc.table_name = $2
		 ORDER BY kcu.constraint_name, kcu.ordinal_position`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.ForeignKey
	for rows.Next() {
		var fk db.ForeignKey
		if err := rows.Scan(&fk.Name, &fk.Column, &fk.RefTable, &fk.RefColumn, &fk.OnUpdate, &fk.OnDelete); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}
