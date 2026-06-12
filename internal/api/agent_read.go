package api

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/conray/dataseai/internal/db"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
)

func isPGDialect(d db.Dialect) bool {
	switch d.Engine() {
	case db.EnginePostgres, "postgresql", "cockroachdb", "redshift":
		return true
	}
	return false
}

func isMSSQLDialect(d db.Dialect) bool {
	switch d.Engine() {
	case db.EngineMSSQL, "sqlserver":
		return true
	}
	return false
}

// agentPGSchema returns the SQL schema name for agent-path browse queries.
// On the agent path the sidebar "database" level maps to a real database, so
// the schema parameter holds a database name. Engines that namespace tables
// under a fixed schema resolve to it: PostgreSQL -> "public", SQL Server ->
// "dbo" (matching the direct-connection MSSQL adapter). Everything else (e.g.
// MySQL, where database == schema) keeps the supplied value.
func agentPGSchema(schema string, d db.Dialect) string {
	if isPGDialect(d) {
		return "public"
	}
	if isMSSQLDialect(d) {
		return "dbo"
	}
	return schema
}

func listDatabasesViaExecutor(ctx context.Context, exec mysqldialect.Executor, includeSystem bool, dialect db.Dialect) ([]string, error) {
	var query string
	switch {
	case isPGDialect(dialect):
		query = `SELECT datname FROM pg_database` +
			` WHERE datistemplate = false AND datallowconn = true` +
			` ORDER BY datname`
	case isMSSQLDialect(dialect):
		// List every database on the server (TablePlus-style). Selecting one
		// makes the connector reconnect with that database as its context, so
		// the table/column queries below resolve against it.
		query = `SELECT name FROM sys.databases ORDER BY name`
	default:
		query = "SHOW DATABASES"
	}
	out, err := exec.Run(ctx, query, mysqldialect.RunOpts{})
	if err != nil {
		return nil, err
	}
	excluded := map[string]bool{
		"mysql": true, "information_schema": true, "performance_schema": true, "sys": true,
		// MSSQL system databases
		"master": true, "model": true, "msdb": true, "tempdb": true,
	}
	var names []string
	for _, row := range out.Rows {
		if len(row) == 0 {
			continue
		}
		name := fmt.Sprint(row[0])
		if !isPGDialect(dialect) && !includeSystem && excluded[name] {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func listTablesViaExecutor(ctx context.Context, exec mysqldialect.Executor, schema string, dialect db.Dialect) ([]db.TableInfo, error) {
	var query string
	switch {
	case isPGDialect(dialect):
		query = `SELECT table_name FROM information_schema.tables` +
			` WHERE table_schema = ` + sqlString(agentPGSchema(schema, dialect)) +
			` AND table_type IN ('BASE TABLE', 'VIEW') ORDER BY table_name`
	case isMSSQLDialect(dialect):
		query = `SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES` +
			` WHERE TABLE_SCHEMA = ` + sqlString(agentPGSchema(schema, dialect)) +
			` AND TABLE_TYPE IN ('BASE TABLE', 'VIEW') ORDER BY TABLE_NAME`
	default:
		query = `SELECT table_name,` +
			` COALESCE(table_rows, 0),` +
			` COALESCE(ROUND((data_length + index_length) / 1024 / 1024), 0)` +
			` FROM information_schema.tables` +
			` WHERE table_schema = ` + sqlString(schema) +
			` ORDER BY table_name`
	}
	out, err := exec.Run(ctx, query, mysqldialect.RunOpts{})
	if err != nil {
		return nil, err
	}
	tables := make([]db.TableInfo, 0, len(out.Rows))
	for _, row := range out.Rows {
		if len(row) < 1 {
			continue
		}
		ti := db.TableInfo{Name: fmt.Sprint(row[0])}
		if !isPGDialect(dialect) && len(row) >= 3 {
			ti.RowsEst = anyInt64(row[1])
			ti.SizeMB = anyInt64(row[2])
		}
		tables = append(tables, ti)
	}
	return tables, nil
}

func listSchemaColumnsViaExecutor(ctx context.Context, exec mysqldialect.Executor, schema string, dialect db.Dialect) (map[string][]string, error) {
	sql := `SELECT table_name, column_name
		 FROM information_schema.columns
		 WHERE table_schema = ` + sqlString(agentPGSchema(schema, dialect)) + `
		 ORDER BY table_name, ordinal_position`
	out, err := exec.Run(ctx, sql, mysqldialect.RunOpts{})
	if err != nil {
		return nil, err
	}
	tables := make(map[string][]string)
	for _, row := range out.Rows {
		if len(row) < 2 {
			continue
		}
		table := fmt.Sprint(row[0])
		col := fmt.Sprint(row[1])
		tables[table] = append(tables[table], col)
	}
	return tables, nil
}

func fetchTableRowsViaExecutor(ctx context.Context, exec mysqldialect.Executor, o db.RowsOpts, dialect db.Dialect) (db.RowsPage, error) {
	if o.Page < 1 {
		o.Page = 1
	}
	if o.PerPage < 1 {
		o.PerPage = 50
	}
	if o.PerPage > db.MaxRowsPerPage {
		o.PerPage = db.MaxRowsPerPage
	}
	offset := (o.Page - 1) * o.PerPage
	qualified := dialect.QuoteIdent(agentPGSchema(o.Schema, dialect)) + "." + dialect.QuoteIdent(o.Table)
	whereSQL := buildWhereSQLForDialect(o.Filters, dialect)
	orderBy := ""
	if o.SortCol != "" {
		dir := "ASC"
		if o.SortDir == "desc" {
			dir = "DESC"
		}
		orderBy = " ORDER BY " + dialect.QuoteIdent(o.SortCol) + " " + dir
	}
	var rowsSQL string
	if isMSSQLDialect(dialect) {
		// MSSQL has no LIMIT; use OFFSET/FETCH, which requires an ORDER BY.
		ob := orderBy
		if ob == "" {
			ob = " ORDER BY (SELECT NULL)"
		}
		rowsSQL = "SELECT * FROM " + qualified + whereSQL + ob +
			" OFFSET " + strconv.Itoa(offset) + " ROWS FETCH NEXT " + strconv.Itoa(o.PerPage) + " ROWS ONLY"
	} else {
		rowsSQL = "SELECT * FROM " + qualified + whereSQL + orderBy + " LIMIT " + strconv.Itoa(o.PerPage) + " OFFSET " + strconv.Itoa(offset)
	}
	// Fetch the page first so a slow COUNT(*) on a huge table can't block the
	// rows from loading.
	rowsOut, err := exec.Run(ctx, rowsSQL, mysqldialect.RunOpts{MaxRows: o.PerPage})
	if err != nil {
		return db.RowsPage{}, err
	}
	// Total is best-effort: COUNT(*) is unbounded and can exceed the request
	// deadline on large tables. -1 signals "unknown" to the UI.
	total := int64(-1)
	countSQL := "SELECT COUNT(*) FROM " + qualified + whereSQL
	if countOut, cErr := exec.Run(ctx, countSQL, mysqldialect.RunOpts{}); cErr == nil &&
		len(countOut.Rows) > 0 && len(countOut.Rows[0]) > 0 {
		total = anyInt64(countOut.Rows[0][0])
	}
	return db.RowsPage{
		Columns: rowsOut.Columns,
		Rows:    rowsOut.Rows,
		Total:   total,
		Page:    o.Page,
		PerPage: o.PerPage,
	}, nil
}

func buildWhereSQLForDialect(filters []db.Filter, dialect db.Dialect) string {
	if len(filters) == 0 {
		return ""
	}
	var conds []string
	for _, f := range filters {
		if f.Column == "" {
			continue
		}
		col := dialect.QuoteIdent(f.Column)
		switch f.Operator {
		case "=", "<>", "<", ">", "<=", ">=":
			conds = append(conds, col+" "+f.Operator+" "+sqlString(f.Value))
		case "LIKE":
			conds = append(conds, col+" LIKE "+sqlString(f.Value))
		case "Contains":
			conds = append(conds, col+" LIKE "+sqlString("%"+f.Value+"%"))
		case "Not contains":
			conds = append(conds, col+" NOT LIKE "+sqlString("%"+f.Value+"%"))
		case "Has prefix":
			conds = append(conds, col+" LIKE "+sqlString(f.Value+"%"))
		case "Has suffix":
			conds = append(conds, col+" LIKE "+sqlString("%"+f.Value))
		case "IS NULL":
			conds = append(conds, col+" IS NULL")
		case "IS NOT NULL":
			conds = append(conds, col+" IS NOT NULL")
		case "IN", "NOT IN":
			parts := splitFilterCSV(f.Value)
			if len(parts) == 0 {
				continue
			}
			vals := make([]string, 0, len(parts))
			for _, p := range parts {
				vals = append(vals, sqlString(p))
			}
			op := "IN"
			if f.Operator == "NOT IN" {
				op = "NOT IN"
			}
			conds = append(conds, col+" "+op+" ("+strings.Join(vals, ", ")+")")
		case "BETWEEN", "NOT BETWEEN":
			parts := splitFilterCSV(f.Value)
			if len(parts) != 2 {
				continue
			}
			op := "BETWEEN"
			if f.Operator == "NOT BETWEEN" {
				op = "NOT BETWEEN"
			}
			conds = append(conds, col+" "+op+" "+sqlString(parts[0])+" AND "+sqlString(parts[1]))
		}
	}
	if len(conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conds, " AND ")
}

func splitFilterCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func describeTableViaExecutor(ctx context.Context, exec mysqldialect.Executor, schema, table string, dialect db.Dialect) (db.Structure, error) {
	var query string
	switch {
	case isPGDialect(dialect):
		query = `SELECT column_name, data_type, is_nullable, COALESCE(column_default,''), '', '', ''` +
			` FROM information_schema.columns` +
			` WHERE table_schema = ` + sqlString(agentPGSchema(schema, dialect)) +
			` AND table_name = ` + sqlString(table) +
			` ORDER BY ordinal_position`
	case isMSSQLDialect(dialect):
		// INFORMATION_SCHEMA.COLUMNS has no key column, so LEFT JOIN the primary
		// key columns and emit 'PRI' for them — the grid needs this to edit rows.
		sch := sqlString(agentPGSchema(schema, dialect))
		tbl := sqlString(table)
		query = `SELECT c.COLUMN_NAME, c.DATA_TYPE, c.IS_NULLABLE, ISNULL(c.COLUMN_DEFAULT,''), '', '',` +
			` CASE WHEN pk.COLUMN_NAME IS NOT NULL THEN 'PRI' ELSE '' END` +
			` FROM INFORMATION_SCHEMA.COLUMNS c` +
			` LEFT JOIN (` +
			`   SELECT kcu.COLUMN_NAME` +
			`   FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc` +
			`   JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu` +
			`     ON kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME` +
			`    AND kcu.TABLE_SCHEMA = tc.TABLE_SCHEMA` +
			`    AND kcu.TABLE_NAME = tc.TABLE_NAME` +
			`   WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'` +
			`     AND tc.TABLE_SCHEMA = ` + sch + ` AND tc.TABLE_NAME = ` + tbl +
			` ) pk ON pk.COLUMN_NAME = c.COLUMN_NAME` +
			` WHERE c.TABLE_SCHEMA = ` + sch + ` AND c.TABLE_NAME = ` + tbl +
			` ORDER BY c.ORDINAL_POSITION`
	default:
		query = `SELECT column_name, column_type, is_nullable, IFNULL(column_default,''),` +
			` IFNULL(extra,''), IFNULL(column_comment,''), IFNULL(column_key,'')` +
			` FROM information_schema.columns` +
			` WHERE table_schema = ` + sqlString(schema) +
			` AND table_name = ` + sqlString(table) +
			` ORDER BY ordinal_position`
	}
	out, err := exec.Run(ctx, query, mysqldialect.RunOpts{})
	if err != nil {
		return db.Structure{}, err
	}
	var structure db.Structure
	for _, row := range out.Rows {
		if len(row) < 7 {
			continue
		}
		nullable := fmt.Sprint(row[2])
		structure.Columns = append(structure.Columns, db.Column{
			Name:     fmt.Sprint(row[0]),
			Type:     fmt.Sprint(row[1]),
			Nullable: nullable == "YES",
			Default:  fmt.Sprint(row[3]),
			Extra:    fmt.Sprint(row[4]),
			Comment:  fmt.Sprint(row[5]),
			Key:      fmt.Sprint(row[6]),
		})
	}
	if !isPGDialect(dialect) && !isMSSQLDialect(dialect) {
		createOut, err := exec.Run(ctx, "SHOW CREATE TABLE "+dialect.QuoteIdent(schema)+"."+dialect.QuoteIdent(table), mysqldialect.RunOpts{})
		if err != nil {
			return structure, err
		}
		if len(createOut.Rows) > 0 && len(createOut.Rows[0]) > 1 {
			structure.CreateSQL = fmt.Sprint(createOut.Rows[0][1])
		}
	}
	return structure, nil
}

func listIndexesViaExecutor(ctx context.Context, exec mysqldialect.Executor, schema, table string, dialect db.Dialect) ([]db.Index, error) {
	var query string
	switch {
	case isPGDialect(dialect):
		query = `SELECT i.relname, a.attname, NOT ix.indisunique::bool, am.amname` +
			` FROM pg_class t` +
			` JOIN pg_index ix ON t.oid = ix.indrelid` +
			` JOIN pg_class i ON ix.indexrelid = i.oid` +
			` JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)` +
			` JOIN pg_namespace n ON n.oid = t.relnamespace` +
			` JOIN pg_am am ON am.oid = i.relam` +
			` WHERE n.nspname = ` + sqlString(agentPGSchema(schema, dialect)) +
			` AND t.relname = ` + sqlString(table) +
			` ORDER BY i.relname, a.attnum`
	case isMSSQLDialect(dialect):
		query = `SELECT i.name, c.name,` +
			` CASE WHEN i.is_unique = 1 THEN 0 ELSE 1 END, i.type_desc` +
			` FROM sys.indexes i` +
			` JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id` +
			` JOIN sys.columns c ON c.object_id = i.object_id AND c.column_id = ic.column_id` +
			` JOIN sys.tables t ON t.object_id = i.object_id` +
			` JOIN sys.schemas s ON s.schema_id = t.schema_id` +
			` WHERE s.name = ` + sqlString(agentPGSchema(schema, dialect)) +
			` AND t.name = ` + sqlString(table) +
			` AND i.name IS NOT NULL` +
			` ORDER BY i.name, ic.key_ordinal`
	default:
		query = `SELECT index_name, column_name, non_unique, IFNULL(index_type,'BTREE')` +
			` FROM information_schema.statistics` +
			` WHERE table_schema = ` + sqlString(schema) +
			` AND table_name = ` + sqlString(table) +
			` ORDER BY index_name, seq_in_index`
	}
	out, err := exec.Run(ctx, query, mysqldialect.RunOpts{})
	if err != nil {
		return nil, err
	}
	type acc struct {
		Name      string
		Columns   []string
		NonUnique int64
		IndexType string
	}
	var ordered []*acc
	byName := map[string]*acc{}
	for _, row := range out.Rows {
		if len(row) < 4 {
			continue
		}
		name := fmt.Sprint(row[0])
		entry, ok := byName[name]
		if !ok {
			entry = &acc{Name: name, NonUnique: anyInt64(row[2]), IndexType: fmt.Sprint(row[3])}
			byName[name] = entry
			ordered = append(ordered, entry)
		}
		entry.Columns = append(entry.Columns, fmt.Sprint(row[1]))
	}
	indexes := make([]db.Index, 0, len(ordered))
	for _, entry := range ordered {
		indexes = append(indexes, db.Index{
			Name:      entry.Name,
			Columns:   entry.Columns,
			Unique:    entry.NonUnique == 0,
			IndexType: entry.IndexType,
		})
	}
	return indexes, nil
}

func listForeignKeysViaExecutor(ctx context.Context, exec mysqldialect.Executor, schema, table string, dialect db.Dialect) ([]db.ForeignKey, error) {
	var query string
	if isMSSQLDialect(dialect) {
		query = `SELECT fk.name, c.name, tp.name, rc.name,` +
			` fk.delete_referential_action_desc, fk.update_referential_action_desc` +
			` FROM sys.foreign_keys fk` +
			` JOIN sys.foreign_key_columns fkc ON fkc.constraint_object_id = fk.object_id` +
			` JOIN sys.columns c ON c.object_id = fkc.parent_object_id AND c.column_id = fkc.parent_column_id` +
			` JOIN sys.tables tp ON tp.object_id = fkc.referenced_object_id` +
			` JOIN sys.columns rc ON rc.object_id = fkc.referenced_object_id AND rc.column_id = fkc.referenced_column_id` +
			` JOIN sys.tables t ON t.object_id = fk.parent_object_id` +
			` JOIN sys.schemas s ON s.schema_id = t.schema_id` +
			` WHERE s.name = ` + sqlString(agentPGSchema(schema, dialect)) +
			` AND t.name = ` + sqlString(table) +
			` ORDER BY fk.name`
	} else if isPGDialect(dialect) {
		query = `SELECT k.constraint_name, k.column_name,` +
			` k.referenced_table_name, k.referenced_column_name,` +
			` COALESCE(c.delete_rule,''), COALESCE(c.update_rule,'')` +
			` FROM information_schema.key_column_usage k` +
			` LEFT JOIN information_schema.referential_constraints c` +
			`   ON c.constraint_schema = k.constraint_schema` +
			`  AND c.constraint_name = k.constraint_name` +
			` WHERE k.table_schema = ` + sqlString(agentPGSchema(schema, dialect)) +
			` AND k.table_name = ` + sqlString(table) +
			` AND k.referenced_table_name IS NOT NULL` +
			` ORDER BY k.constraint_name, k.ordinal_position`
	} else {
		query = `SELECT k.constraint_name, k.column_name,` +
			` k.referenced_table_name, k.referenced_column_name,` +
			` IFNULL(c.delete_rule,''), IFNULL(c.update_rule,'')` +
			` FROM information_schema.key_column_usage k` +
			` LEFT JOIN information_schema.referential_constraints c` +
			`   ON c.constraint_schema = k.constraint_schema` +
			`  AND c.constraint_name = k.constraint_name` +
			` WHERE k.table_schema = ` + sqlString(schema) +
			` AND k.table_name = ` + sqlString(table) +
			` AND k.referenced_table_name IS NOT NULL` +
			` ORDER BY k.constraint_name, k.ordinal_position`
	}
	out, err := exec.Run(ctx, query, mysqldialect.RunOpts{})
	if err != nil {
		return nil, err
	}
	fks := make([]db.ForeignKey, 0, len(out.Rows))
	for _, row := range out.Rows {
		if len(row) < 6 {
			continue
		}
		fks = append(fks, db.ForeignKey{
			Name:      fmt.Sprint(row[0]),
			Column:    fmt.Sprint(row[1]),
			RefTable:  fmt.Sprint(row[2]),
			RefColumn: fmt.Sprint(row[3]),
			OnDelete:  fmt.Sprint(row[4]),
			OnUpdate:  fmt.Sprint(row[5]),
		})
	}
	return fks, nil
}

func primaryKeyViaExecutor(ctx context.Context, exec mysqldialect.Executor, schema, table string, dialect db.Dialect) ([]string, error) {
	var query string
	switch {
	case isPGDialect(dialect):
		query = `SELECT a.attname` +
			` FROM pg_index i` +
			` JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)` +
			` JOIN pg_class c ON c.oid = i.indrelid` +
			` JOIN pg_namespace n ON n.oid = c.relnamespace` +
			` WHERE n.nspname = ` + sqlString(agentPGSchema(schema, dialect)) +
			` AND c.relname = ` + sqlString(table) +
			` AND i.indisprimary ORDER BY a.attnum`
	case isMSSQLDialect(dialect):
		query = `SELECT kcu.COLUMN_NAME` +
			` FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc` +
			` JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu` +
			`   ON kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME` +
			`  AND kcu.TABLE_SCHEMA = tc.TABLE_SCHEMA` +
			`  AND kcu.TABLE_NAME = tc.TABLE_NAME` +
			` WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'` +
			` AND tc.TABLE_SCHEMA = ` + sqlString(agentPGSchema(schema, dialect)) +
			` AND tc.TABLE_NAME = ` + sqlString(table) +
			` ORDER BY kcu.ORDINAL_POSITION`
	default:
		query = `SELECT column_name FROM information_schema.columns` +
			` WHERE table_schema = ` + sqlString(schema) +
			` AND table_name = ` + sqlString(table) +
			` AND column_key = 'PRI' ORDER BY ordinal_position`
	}
	out, err := exec.Run(ctx, query, mysqldialect.RunOpts{})
	if err != nil {
		return nil, err
	}
	cols := make([]string, 0, len(out.Rows))
	for _, row := range out.Rows {
		if len(row) > 0 {
			cols = append(cols, fmt.Sprint(row[0]))
		}
	}
	return cols, nil
}

func updateCellViaExecutor(ctx context.Context, exec mysqldialect.Executor, schema, table string, pkCols []string, pkVals []any, col string, newVal any, dialect db.Dialect) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where := whereByPKSQLForDialect(pkCols, pkVals, dialect)
	sql := "UPDATE " + dialect.QuoteIdent(agentPGSchema(schema, dialect)) + "." + dialect.QuoteIdent(table) +
		" SET " + dialect.QuoteIdent(col) + " = " + sqlLiteral(newVal) + " WHERE " + where
	out, err := exec.Run(ctx, sql, mysqldialect.RunOpts{})
	if err != nil {
		return 0, err
	}
	return out.RowsAffected, nil
}

func insertRowViaExecutor(ctx context.Context, exec mysqldialect.Executor, schema, table string, values map[string]any, dialect db.Dialect) (id int64, affected int64, err error) {
	if len(values) == 0 {
		return 0, 0, fmt.Errorf("values required")
	}
	cols := make([]string, 0, len(values))
	for col := range values {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	quotedCols := make([]string, 0, len(cols))
	literals := make([]string, 0, len(cols))
	for _, col := range cols {
		quotedCols = append(quotedCols, dialect.QuoteIdent(col))
		literals = append(literals, sqlLiteral(values[col]))
	}
	sql := "INSERT INTO " + dialect.QuoteIdent(agentPGSchema(schema, dialect)) + "." + dialect.QuoteIdent(table) +
		" (" + strings.Join(quotedCols, ", ") + ") VALUES (" + strings.Join(literals, ", ") + ")"
	out, err := exec.Run(ctx, sql, mysqldialect.RunOpts{})
	if err != nil {
		return 0, 0, err
	}
	return 0, out.RowsAffected, nil
}

func deleteRowViaExecutor(ctx context.Context, exec mysqldialect.Executor, schema, table string, pkCols []string, pkVals []any, dialect db.Dialect) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	sql := "DELETE FROM " + dialect.QuoteIdent(agentPGSchema(schema, dialect)) + "." + dialect.QuoteIdent(table) + " WHERE " + whereByPKSQLForDialect(pkCols, pkVals, dialect)
	out, err := exec.Run(ctx, sql, mysqldialect.RunOpts{})
	if err != nil {
		return 0, err
	}
	return out.RowsAffected, nil
}

func whereByPKSQLForDialect(pkCols []string, pkVals []any, dialect db.Dialect) string {
	parts := make([]string, 0, len(pkCols))
	for i, col := range pkCols {
		parts = append(parts, dialect.QuoteIdent(col)+" = "+sqlLiteral(pkVals[i]))
	}
	return strings.Join(parts, " AND ")
}

func sqlLiteral(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if x {
			return "1"
		}
		return "0"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return sqlString(x)
	default:
		return sqlString(fmt.Sprint(v))
	}
}

func anyInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case jsonNumber:
		i, _ := strconv.ParseInt(string(n), 10, 64)
		return i
	case []byte:
		i, _ := strconv.ParseInt(string(n), 10, 64)
		return i
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	default:
		return 0
	}
}

type jsonNumber string

func sqlString(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += "''"
			continue
		}
		out += string(r)
	}
	return out + "'"
}
