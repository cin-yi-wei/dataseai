package chat

import "github.com/conray/mysqlweb/internal/llm"

// Tools returns the LLM tool schema for the current chat session. All tools
// implicitly act on the chat session's pinned connection + default db.
func Tools() []llm.Tool {
	return []llm.Tool{
		{
			Name:        "list_databases",
			Description: "List schemas (databases) visible on this MySQL connection.",
			InputSchema: map[string]any{
				"type": "object", "properties": map[string]any{}, "required": []string{},
			},
		},
		{
			Name:        "list_tables",
			Description: "List tables in a schema.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database": map[string]any{"type": "string", "description": "schema name"},
				},
				"required": []string{"database"},
			},
		},
		{
			Name:        "describe_table",
			Description: "Get columns + types + CREATE TABLE for a table.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database": map[string]any{"type": "string"},
					"table":    map[string]any{"type": "string"},
				},
				"required": []string{"database", "table"},
			},
		},
		{
			Name:        "query_table",
			Description: "Run a parameter-less SELECT on a fully-qualified table. Returns first 200 rows.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database": map[string]any{"type": "string"},
					"table":    map[string]any{"type": "string"},
					"where":    map[string]any{"type": "string", "description": "optional WHERE clause body, e.g. \"id > 100\""},
					"limit":    map[string]any{"type": "integer", "description": "default 200, max 1000"},
				},
				"required": []string{"database", "table"},
			},
		},
		{
			Name:        "run_sql",
			Description: "Run an arbitrary SQL statement. Read queries only — refusing DML/DDL is the caller's responsibility.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sql": map[string]any{"type": "string"},
				},
				"required": []string{"sql"},
			},
		},
	}
}
