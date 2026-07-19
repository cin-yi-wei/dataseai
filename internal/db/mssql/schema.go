package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// useDB returns a "USE [database];" prefix so a metadata query runs in the
// context of the chosen database, the way TablePlus switches databases on one
// connection. Returns "" when no database is given (use the current context).
func (m MSSQL) useDB(database string) string {
	if database == "" {
		return ""
	}
	return "USE " + m.QuoteIdent(database) + ";\n"
}

// ListDatabases returns every database on the server (TablePlus-style), so the
// sidebar can switch between them. System databases are excluded unless
// includeSystem is set.
func (m MSSQL) ListDatabases(ctx context.Context, sqlDB *sql.DB, includeSystem bool) ([]string, error) {
	q := "SELECT name FROM sys.databases"
	if !includeSystem {
		// database_id 1-4 are master/tempdb/model/msdb.
		q += " WHERE database_id > 4"
	}
	q += " ORDER BY name"
	rows, err := sqlDB.QueryContext(ctx, q)
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

// ListTables lists tables in the connected database's dbo schema. The database
// arg identifies the sidebar entry (the connected DB) but the connection is
// already scoped to it, so we filter by the dbo schema rather than the arg.
func (m MSSQL) ListTables(ctx context.Context, sqlDB *sql.DB, database string) ([]db.TableInfo, error) {
	rows, err := sqlDB.QueryContext(ctx,
		m.useDB(database)+
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
	rows, err := sqlDB.QueryContext(ctx,
		m.useDB(database)+
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
		m.useDB(database)+
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
	// INFORMATION_SCHEMA.COLUMNS has no key indicator; mark primary-key columns
	// (Key == "PRI") and foreign-key columns (Key == "MUL") so the UI can flag
	// them the way it does for MySQL.
	pkCols, _ := m.PrimaryKey(ctx, sqlDB, database, table)
	fks, _ := m.ListForeignKeys(ctx, sqlDB, database, table)
	pkSet := make(map[string]bool, len(pkCols))
	for _, c := range pkCols {
		pkSet[c] = true
	}
	fkSet := make(map[string]bool, len(fks))
	for _, fk := range fks {
		fkSet[fk.Column] = true
	}
	for i := range out.Columns {
		switch {
		case pkSet[out.Columns[i].Name]:
			out.Columns[i].Key = "PRI"
		case fkSet[out.Columns[i].Name]:
			out.Columns[i].Key = "MUL"
		}
	}
	out.CreateSQL = synthesizeCreateTable(m, table, out.Columns, pkCols, fks)
	return out, nil
}

// BuildCreateTable synthesizes a CREATE TABLE statement from columns + PK + FKs.
// Exported so the connector (agent) structure path can produce the same output
// as the direct path (SQL Server has no SHOW CREATE TABLE).
func BuildCreateTable(table string, cols []db.Column, pkCols []string, fks []db.ForeignKey) string {
	return synthesizeCreateTable(MSSQL{}, table, cols, pkCols, fks)
}

func synthesizeCreateTable(m MSSQL, table string, cols []db.Column, pkCols []string, fks []db.ForeignKey) string {
	if len(cols) == 0 {
		return ""
	}
	// Each line becomes one CREATE TABLE body entry; joined with ",\n".
	var lines []string
	for _, c := range cols {
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
		lines = append(lines, fmt.Sprintf("  %s %s%s%s", m.QuoteIdent(c.Name), colType, nullStr, defStr))
	}

	if len(pkCols) > 0 {
		quoted := make([]string, len(pkCols))
		for i, c := range pkCols {
			quoted[i] = m.QuoteIdent(c)
		}
		lines = append(lines, "  PRIMARY KEY ("+strings.Join(quoted, ", ")+")")
	}

	// Group FK rows by constraint name (composite FKs span multiple rows).
	type fkGroup struct {
		cols, refCols []string
		refTable      string
	}
	var order []string
	groups := map[string]*fkGroup{}
	for _, fk := range fks {
		g, ok := groups[fk.Name]
		if !ok {
			g = &fkGroup{refTable: fk.RefTable}
			groups[fk.Name] = g
			order = append(order, fk.Name)
		}
		g.cols = append(g.cols, m.QuoteIdent(fk.Column))
		g.refCols = append(g.refCols, m.QuoteIdent(fk.RefColumn))
	}
	for _, name := range order {
		g := groups[name]
		lines = append(lines, fmt.Sprintf("  CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s.%s (%s)",
			m.QuoteIdent(name), strings.Join(g.cols, ", "),
			m.QuoteIdent(defaultSchema), m.QuoteIdent(g.refTable), strings.Join(g.refCols, ", ")))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE TABLE %s.%s (\n", m.QuoteIdent(defaultSchema), m.QuoteIdent(table))
	sb.WriteString(strings.Join(lines, ",\n"))
	sb.WriteString("\n);")
	return sb.String()
}

func (m MSSQL) ListIndexes(ctx context.Context, sqlDB *sql.DB, database, table string) ([]db.Index, error) {
	rows, err := sqlDB.QueryContext(ctx,
		m.useDB(database)+
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
	rows, err := sqlDB.QueryContext(ctx,
		m.useDB(database)+
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
