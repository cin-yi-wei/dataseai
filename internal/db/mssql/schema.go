package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// ListDatabases returns the connected database name (e.g. "dev_db") as the
// single sidebar entry, matching how tools like TablePlus surface the database
// rather than its schemas. The actual database is fixed at connection time via
// the DSN; tables underneath are resolved in the dbo schema (see defaultSchema).
func (m MSSQL) ListDatabases(ctx context.Context, sqlDB *sql.DB, _ bool) ([]string, error) {
	var name string
	if err := sqlDB.QueryRowContext(ctx, `SELECT DB_NAME()`).Scan(&name); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, nil
	}
	return []string{name}, nil
}

// ListTables lists tables in the connected database's dbo schema. The database
// arg identifies the sidebar entry (the connected DB) but the connection is
// already scoped to it, so we filter by the dbo schema rather than the arg.
func (m MSSQL) ListTables(ctx context.Context, sqlDB *sql.DB, database string) ([]db.TableInfo, error) {
	_ = database
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES
		 WHERE TABLE_SCHEMA = @p1
		 ORDER BY TABLE_NAME`,
		defaultSchema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.TableInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, db.TableInfo{Name: name})
	}
	return out, rows.Err()
}

func (m MSSQL) ListSchemaColumns(ctx context.Context, sqlDB *sql.DB, database string) (map[string][]string, error) {
	_ = database
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT TABLE_NAME, COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS
		 WHERE TABLE_SCHEMA = @p1
		 ORDER BY TABLE_NAME, ORDINAL_POSITION`,
		defaultSchema)
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

func (m MSSQL) DescribeTable(ctx context.Context, sqlDB *sql.DB, database, table string) (db.Structure, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE,
		        ISNULL(COLUMN_DEFAULT,''),
		        ISNULL(CAST(CHARACTER_MAXIMUM_LENGTH AS VARCHAR(20)),'')
		 FROM INFORMATION_SCHEMA.COLUMNS
		 WHERE TABLE_SCHEMA = @p1 AND TABLE_NAME = @p2
		 ORDER BY ORDINAL_POSITION`,
		defaultSchema, table)
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
	out.CreateSQL = synthesizeCreateTable(m, table, out.Columns)
	return out, nil
}

func synthesizeCreateTable(m MSSQL, table string, cols []db.Column) string {
	if len(cols) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE TABLE %s.%s (\n", m.QuoteIdent(defaultSchema), m.QuoteIdent(table))
	for i, c := range cols {
		colType := c.Type
		if strings.HasPrefix(c.Extra, "max_length=") {
			ml := strings.TrimPrefix(c.Extra, "max_length=")
			colType = fmt.Sprintf("%s(%s)", colType, ml)
		}
		nullStr := " NOT NULL"
		if c.Nullable {
			nullStr = " NULL"
		}
		defStr := ""
		if c.Default != "" {
			defStr = " DEFAULT " + c.Default
		}
		comma := ","
		if i == len(cols)-1 {
			comma = ""
		}
		fmt.Fprintf(&sb, "  %s %s%s%s%s\n", m.QuoteIdent(c.Name), colType, nullStr, defStr, comma)
	}
	sb.WriteString(");")
	return sb.String()
}

func (m MSSQL) ListIndexes(ctx context.Context, sqlDB *sql.DB, database, table string) ([]db.Index, error) {
	_ = database
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT i.name, i.is_unique, i.type_desc,
		        STRING_AGG(c.name, ',') WITHIN GROUP (ORDER BY ic.key_ordinal) AS cols
		 FROM sys.indexes i
		 JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
		 JOIN sys.columns c ON c.object_id = i.object_id AND c.column_id = ic.column_id
		 JOIN sys.tables t ON t.object_id = i.object_id
		 JOIN sys.schemas s ON s.schema_id = t.schema_id
		 WHERE s.name = @p1 AND t.name = @p2 AND i.name IS NOT NULL
		 GROUP BY i.name, i.is_unique, i.type_desc
		 ORDER BY i.name`,
		defaultSchema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.Index
	for rows.Next() {
		var name, typeDesc, colsStr string
		var isUnique bool
		if err := rows.Scan(&name, &isUnique, &typeDesc, &colsStr); err != nil {
			return nil, err
		}
		out = append(out, db.Index{
			Name:      name,
			Columns:   strings.Split(colsStr, ","),
			Unique:    isUnique,
			IndexType: strings.ToLower(typeDesc),
		})
	}
	return out, rows.Err()
}

func (m MSSQL) ListForeignKeys(ctx context.Context, sqlDB *sql.DB, database, table string) ([]db.ForeignKey, error) {
	_ = database
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT
		   fk.name,
		   c.name AS column_name,
		   tp.name AS ref_table,
		   rc.name AS ref_column,
		   fk.update_referential_action_desc,
		   fk.delete_referential_action_desc
		 FROM sys.foreign_keys fk
		 JOIN sys.foreign_key_columns fkc ON fkc.constraint_object_id = fk.object_id
		 JOIN sys.columns c ON c.object_id = fkc.parent_object_id AND c.column_id = fkc.parent_column_id
		 JOIN sys.tables tp ON tp.object_id = fkc.referenced_object_id
		 JOIN sys.columns rc ON rc.object_id = fkc.referenced_object_id AND rc.column_id = fkc.referenced_column_id
		 JOIN sys.tables t ON t.object_id = fk.parent_object_id
		 JOIN sys.schemas s ON s.schema_id = t.schema_id
		 WHERE s.name = @p1 AND t.name = @p2
		 ORDER BY fk.name`,
		defaultSchema, table)
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
