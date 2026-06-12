package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/conray/dataseai/internal/db"
	mssqldialect "github.com/conray/dataseai/internal/db/mssql"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
	pgdialect "github.com/conray/dataseai/internal/db/pg"
)

// scopeMismatch returns a tool_result JSON describing a database-scope
// violation. The chat session pins itself to ec.DefaultDB at start; tools
// must target that database only.
func scopeMismatch(requested string) (string, error) {
	return marshal(map[string]any{
		"error":        "db_scope_denied",
		"reason":       "this chat session is pinned to a single database; tools must target it",
		"requested_db": requested,
	})
}

// Execute dispatches a single tool call. Returns a JSON string (the tool result
// body) or an error if the tool name is unknown / args bad. The result is what's
// fed back to the LLM as a tool_result block.
func Execute(ctx context.Context, ec ExecCtx, name string, input map[string]any) (string, error) {
	switch name {
	case "list_databases":
		// When session is pinned to a database, only that one is visible.
		if ec.DefaultDB != "" {
			return marshal(map[string]any{"databases": []string{ec.DefaultDB}})
		}
		exec, err := ec.executor()
		if err != nil {
			return "", err
		}
		names, err := listDatabasesViaExecutor(ctx, exec, ec.Engine, false)
		if err != nil {
			return "", err
		}
		return marshal(map[string]any{"databases": names})

	case "list_tables":
		schema, _ := input["database"].(string)
		if schema == "" {
			return "", fmt.Errorf("database required")
		}
		if ec.DefaultDB != "" && !strings.EqualFold(schema, ec.DefaultDB) {
			return scopeMismatch(schema)
		}
		exec, err := ec.executor()
		if err != nil {
			return "", err
		}
		tables, err := listTablesViaExecutor(ctx, exec, ec.Engine, schema)
		if err != nil {
			return "", err
		}
		return marshal(map[string]any{"tables": tables})

	case "describe_table":
		schema, _ := input["database"].(string)
		table, _ := input["table"].(string)
		if schema == "" || table == "" {
			return "", fmt.Errorf("database/table required")
		}
		if ec.DefaultDB != "" && !strings.EqualFold(schema, ec.DefaultDB) {
			return scopeMismatch(schema)
		}
		exec, err := ec.executor()
		if err != nil {
			return "", err
		}
		s, err := describeTableViaExecutor(ctx, exec, ec.Engine, schema, table)
		if err != nil {
			return "", err
		}
		return marshal(s)

	case "query_table":
		schema, _ := input["database"].(string)
		table, _ := input["table"].(string)
		if schema == "" || table == "" {
			return "", fmt.Errorf("database/table required")
		}
		if ec.DefaultDB != "" && !strings.EqualFold(schema, ec.DefaultDB) {
			return scopeMismatch(schema)
		}
		where, _ := input["where"].(string)
		limitF, _ := input["limit"].(float64)
		limit := int(limitF)
		if limit <= 0 {
			limit = 200
		}
		if limit > 1000 {
			limit = 1000
		}
		var q string
		switch {
		case engIsMSSQL(ec.Engine):
			qn := mssqldialect.MSSQL{}.QuoteIdent(chatSchema(ec.Engine, schema)) + "." + mssqldialect.MSSQL{}.QuoteIdent(table)
			q = fmt.Sprintf("SELECT TOP %d * FROM %s", limit, qn)
			if where != "" {
				q += " WHERE " + where
			}
		case engIsPG(ec.Engine):
			qn := pgdialect.PG{}.QuoteIdent(chatSchema(ec.Engine, schema)) + "." + pgdialect.PG{}.QuoteIdent(table)
			q = "SELECT * FROM " + qn
			if where != "" {
				q += " WHERE " + where
			}
			q += fmt.Sprintf(" LIMIT %d", limit)
		default:
			q = "SELECT * FROM " + mysqldialect.MySQL{}.QuoteIdent(schema) + "." + mysqldialect.MySQL{}.QuoteIdent(table)
			if where != "" {
				q += " WHERE " + where
			}
			q += fmt.Sprintf(" LIMIT %d", limit)
		}
		exec, err := ec.executor()
		if err != nil {
			return "", err
		}
		out, err := exec.Run(ctx, q, mysqldialect.RunOpts{MaxRows: limit, Database: schema})
		if err != nil {
			return "", err
		}
		return marshal(map[string]any{"columns": out.Columns, "rows": out.Rows})

	case "run_sql":
		sqlStr, _ := input["sql"].(string)
		if sqlStr == "" {
			return "", fmt.Errorf("sql required")
		}
		cls, _ := mysqldialect.MySQL{}.ClassifySQL(sqlStr)
		if cls.Op != db.OpSelect && cls.Op != db.OpReadMeta {
			return marshal(map[string]any{
				"error": "run_sql_readonly",
				"hint":  "use propose_write for any INSERT/UPDATE/DELETE/TRUNCATE/ALTER/RENAME; only SELECT/SHOW/DESCRIBE/EXPLAIN are allowed via run_sql",
			})
		}
		// Reject SQL whose classifier extracted a different db qualifier than
		// the pinned session DB. We can't catch every cross-db SELECT (e.g.
		// JOINs into another schema) but the common case — explicit
		// `other_db.table` — is caught here.
		if ec.DefaultDB != "" && cls.DB != "" && !strings.EqualFold(cls.DB, ec.DefaultDB) {
			return scopeMismatch(cls.DB)
		}
		exec, err := ec.executor()
		if err != nil {
			return "", err
		}
		out, err := exec.Run(ctx, sqlStr, mysqldialect.RunOpts{MaxRows: 1000, Database: ec.DefaultDB})
		if err != nil {
			return "", err
		}
		return marshal(map[string]any{
			"columns":       out.Columns,
			"rows":          out.Rows,
			"rows_affected": out.RowsAffected,
			"truncated":     out.Truncated,
		})

	case "propose_write":
		return handleProposeWrite(ctx, ec, input)

	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

func marshal(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func engIsMSSQL(e string) bool { return e == "mssql" || e == "sqlserver" }

func engIsPG(e string) bool {
	switch e {
	case "postgres", "postgresql", "cockroachdb", "redshift":
		return true
	}
	return false
}

// chatSchema maps the tool's `database` argument to the SQL schema used in
// metadata queries. MySQL: database == schema. PostgreSQL/SQL Server pin the
// connection to the database and keep objects in public/dbo.
func chatSchema(engine, database string) string {
	if engIsPG(engine) {
		return "public"
	}
	if engIsMSSQL(engine) {
		return "dbo"
	}
	return database
}

func listDatabasesViaExecutor(ctx context.Context, exec mysqldialect.Executor, engine string, includeSystem bool) ([]string, error) {
	query := "SHOW DATABASES"
	switch {
	case engIsPG(engine):
		query = `SELECT datname FROM pg_database WHERE datistemplate = false AND datallowconn = true ORDER BY datname`
	case engIsMSSQL(engine):
		query = `SELECT name FROM sys.databases ORDER BY name`
	}
	out, err := exec.Run(ctx, query, mysqldialect.RunOpts{})
	if err != nil {
		return nil, err
	}
	excluded := map[string]bool{
		"mysql": true, "information_schema": true, "performance_schema": true, "sys": true,
		"master": true, "model": true, "msdb": true, "tempdb": true,
	}
	var names []string
	for _, row := range out.Rows {
		if len(row) == 0 {
			continue
		}
		name := fmt.Sprint(row[0])
		if !includeSystem && excluded[name] {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func listTablesViaExecutor(ctx context.Context, exec mysqldialect.Executor, engine, schema string) ([]db.TableInfo, error) {
	var sql string
	switch {
	case engIsPG(engine):
		sql = `SELECT table_name, 0, 0 FROM information_schema.tables` +
			` WHERE table_schema = ` + sqlString(chatSchema(engine, schema)) +
			` AND table_type IN ('BASE TABLE','VIEW') ORDER BY table_name`
	case engIsMSSQL(engine):
		sql = `SELECT TABLE_NAME, 0, 0 FROM INFORMATION_SCHEMA.TABLES` +
			` WHERE TABLE_SCHEMA = ` + sqlString(chatSchema(engine, schema)) +
			` AND TABLE_TYPE IN ('BASE TABLE','VIEW') ORDER BY TABLE_NAME`
	default:
		sql = `SELECT table_name,
		        COALESCE(table_rows, 0),
		        COALESCE(ROUND((data_length + index_length) / 1024 / 1024), 0)
		 FROM information_schema.tables
		 WHERE table_schema = ` + sqlString(schema) + `
		 ORDER BY table_name`
	}
	out, err := exec.Run(ctx, sql, mysqldialect.RunOpts{})
	if err != nil {
		return nil, err
	}
	tables := make([]db.TableInfo, 0, len(out.Rows))
	for _, row := range out.Rows {
		if len(row) < 3 {
			continue
		}
		tables = append(tables, db.TableInfo{
			Name:    fmt.Sprint(row[0]),
			RowsEst: anyInt64(row[1]),
			SizeMB:  anyInt64(row[2]),
		})
	}
	return tables, nil
}

func describeTableViaExecutor(ctx context.Context, exec mysqldialect.Executor, engine, schema, table string) (db.Structure, error) {
	var sql string
	switch {
	case engIsPG(engine):
		sql = `SELECT column_name, data_type, is_nullable, COALESCE(column_default,''), '', '', ''` +
			` FROM information_schema.columns` +
			` WHERE table_schema = ` + sqlString(chatSchema(engine, schema)) +
			` AND table_name = ` + sqlString(table) +
			` ORDER BY ordinal_position`
	case engIsMSSQL(engine):
		sch := sqlString(chatSchema(engine, schema))
		tbl := sqlString(table)
		sql = `SELECT c.COLUMN_NAME, c.DATA_TYPE, c.IS_NULLABLE, ISNULL(c.COLUMN_DEFAULT,''), '', '',` +
			` CASE WHEN pk.COLUMN_NAME IS NOT NULL THEN 'PRI' ELSE '' END` +
			` FROM INFORMATION_SCHEMA.COLUMNS c` +
			` LEFT JOIN (SELECT kcu.COLUMN_NAME FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc` +
			`   JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu ON kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME` +
			`    AND kcu.TABLE_SCHEMA = tc.TABLE_SCHEMA AND kcu.TABLE_NAME = tc.TABLE_NAME` +
			`   WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY' AND tc.TABLE_SCHEMA = ` + sch + ` AND tc.TABLE_NAME = ` + tbl +
			` ) pk ON pk.COLUMN_NAME = c.COLUMN_NAME` +
			` WHERE c.TABLE_SCHEMA = ` + sch + ` AND c.TABLE_NAME = ` + tbl +
			` ORDER BY c.ORDINAL_POSITION`
	default:
		sql = `SELECT column_name, column_type, is_nullable, IFNULL(column_default,''),
		        IFNULL(extra,''), IFNULL(column_comment,''), IFNULL(column_key,'')
		 FROM information_schema.columns
		 WHERE table_schema = ` + sqlString(schema) + `
		   AND table_name = ` + sqlString(table) + `
		 ORDER BY ordinal_position`
	}
	out, err := exec.Run(ctx, sql, mysqldialect.RunOpts{})
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
	// SHOW CREATE TABLE is MySQL-only; skip it for other engines.
	if !engIsPG(engine) && !engIsMSSQL(engine) {
		createOut, err := exec.Run(ctx, "SHOW CREATE TABLE "+mysqldialect.MySQL{}.QuoteIdent(schema)+"."+mysqldialect.MySQL{}.QuoteIdent(table), mysqldialect.RunOpts{})
		if err != nil {
			return structure, err
		}
		if len(createOut.Rows) > 0 && len(createOut.Rows[0]) > 1 {
			structure.CreateSQL = fmt.Sprint(createOut.Rows[0][1])
		}
	}
	return structure, nil
}

func anyInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
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
