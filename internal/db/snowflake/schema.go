package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// ListDatabases uses SHOW DATABASES and extracts the "name" column.
// Snowflake's INFORMATION_SCHEMA is per-database, so SHOW DATABASES is the
// simplest cross-database listing available.
func (s Snowflake) ListDatabases(ctx context.Context, sqlDB *sql.DB, _ bool) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	nameIdx := 1 // Snowflake: created_on, name, ...
	for i, c := range cols {
		if strings.EqualFold(c, "name") {
			nameIdx = i
			break
		}
	}

	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	var out []string
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		switch v := vals[nameIdx].(type) {
		case string:
			out = append(out, v)
		case []byte:
			out = append(out, string(v))
		}
	}
	return out, rows.Err()
}

func (s Snowflake) ListTables(ctx context.Context, sqlDB *sql.DB, schema string) ([]db.TableInfo, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES
		 WHERE TABLE_SCHEMA = ? AND TABLE_TYPE IN ('BASE TABLE','VIEW')
		 ORDER BY TABLE_NAME`,
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

func (s Snowflake) ListSchemaColumns(ctx context.Context, sqlDB *sql.DB, schema string) (map[string][]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT TABLE_NAME, COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS
		 WHERE TABLE_SCHEMA = ?
		 ORDER BY TABLE_NAME, ORDINAL_POSITION`,
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

func (s Snowflake) DescribeTable(ctx context.Context, sqlDB *sql.DB, schema, table string) (db.Structure, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COALESCE(COLUMN_DEFAULT, ''),
		        COALESCE(CHARACTER_MAXIMUM_LENGTH::TEXT, '')
		 FROM INFORMATION_SCHEMA.COLUMNS
		 WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		 ORDER BY ORDINAL_POSITION`,
		schema, table,
	)
	if err != nil {
		return db.Structure{}, err
	}
	var out db.Structure
	for rows.Next() {
		var c db.Column
		var nullable, charMax string
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &c.Default, &charMax); err != nil {
			rows.Close()
			return db.Structure{}, err
		}
		c.Nullable = nullable == "YES" || nullable == "Y"
		if charMax != "" {
			c.Extra = "max_length=" + charMax
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

func (s Snowflake) ListIndexes(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.Index, error) {
	// Snowflake is a columnar store; it does not expose traditional B-tree indexes.
	return nil, nil
}

func (s Snowflake) ListForeignKeys(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.ForeignKey, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT k.CONSTRAINT_NAME, k.COLUMN_NAME,
		        u.TABLE_NAME AS REF_TABLE, u.COLUMN_NAME AS REF_COLUMN,
		        r.UPDATE_RULE, r.DELETE_RULE
		 FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS c
		 JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE k
		   ON k.CONSTRAINT_NAME = c.CONSTRAINT_NAME AND k.CONSTRAINT_SCHEMA = c.CONSTRAINT_SCHEMA
		 JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS r
		   ON r.CONSTRAINT_NAME = c.CONSTRAINT_NAME AND r.CONSTRAINT_SCHEMA = c.CONSTRAINT_SCHEMA
		 JOIN INFORMATION_SCHEMA.CONSTRAINT_COLUMN_USAGE u
		   ON u.CONSTRAINT_NAME = c.CONSTRAINT_NAME AND u.CONSTRAINT_SCHEMA = c.CONSTRAINT_SCHEMA
		 WHERE c.CONSTRAINT_TYPE = 'FOREIGN KEY'
		   AND c.TABLE_SCHEMA = ? AND c.TABLE_NAME = ?
		 ORDER BY k.CONSTRAINT_NAME, k.ORDINAL_POSITION`,
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
