package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/mysql"
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
		names, err := mysql.ListDatabases(ctx, ec.DB, false)
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
		tables, err := mysql.ListTables(ctx, ec.DB, schema)
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
		s, err := mysql.DescribeTable(ctx, ec.DB, schema, table)
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
		q := "SELECT * FROM " + mysql.QuoteIdent(schema) + "." + mysql.QuoteIdent(table)
		if where != "" {
			q += " WHERE " + where
		}
		q += fmt.Sprintf(" LIMIT %d", limit)
		out, err := mysql.Run(ctx, ec.DB, q, mysql.RunOpts{MaxRows: limit})
		if err != nil {
			return "", err
		}
		return marshal(map[string]any{"columns": out.Columns, "rows": out.Rows})

	case "run_sql":
		sqlStr, _ := input["sql"].(string)
		if sqlStr == "" {
			return "", fmt.Errorf("sql required")
		}
		cls, _ := mysql.ClassifySQL(sqlStr)
		if cls.Op != mysql.OpSelect && cls.Op != mysql.OpReadMeta {
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
		out, err := mysql.Run(ctx, ec.DB, sqlStr, mysql.RunOpts{MaxRows: 1000})
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
