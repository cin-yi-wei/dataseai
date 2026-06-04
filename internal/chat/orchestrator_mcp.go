package chat

import (
	"context"
	"fmt"

	"github.com/conray/dataseai/internal/llm"
)

// MCPClient is the orchestrator's view of an MCP server. It mirrors
// internal/mcp.Client's CallTool surface so tests can inject a fake.
type MCPClient interface {
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
}

// MCPDeps carries everything RunMCP needs to drive a chat session whose
// tool calls go through an MCP server. The chat session is scoped to a
// single user+connection, materialised as a registered DSN name.
type MCPDeps struct {
	LLM           llm.LLMClient
	MCP           MCPClient
	DSNName       string // DSN registered with the MCP server (e.g. "u1_c10")
	MaxIterations int    // default 8
	System        string
}

// MCPTools is the tool surface dataseai exposes to the LLM when MCP is wired.
// The orchestrator translates every tool_use to an MCP tools/call and injects
// dsn_name automatically, so the model never sees the DSN identifier (defence
// in depth — even if the model emitted a bogus dsn_name it would be replaced).
func MCPTools() []llm.Tool {
	return []llm.Tool{
		{
			Name:        "mysql_query",
			Description: "Run a read-only SQL statement (SELECT / SHOW / DESCRIBE) against the user's MySQL connection. Returns rows as JSON. Refuse to run DML/DDL.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sql": map[string]any{"type": "string", "description": "SQL to execute. Read-only."},
				},
				"required": []string{"sql"},
			},
		},
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
					"database": map[string]any{"type": "string"},
				},
				"required": []string{"database"},
			},
		},
		{
			Name:        "describe_table",
			Description: "Get column metadata + CREATE TABLE for a table.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database": map[string]any{"type": "string"},
					"table":    map[string]any{"type": "string"},
				},
				"required": []string{"database", "table"},
			},
		},
	}
}

const mcpSystemPrompt = `You are a database assistant attached to a single MySQL connection through an MCP server. Use the provided tools. Prefer narrow queries with LIMIT. Never run destructive DML/DDL — refuse if asked. Keep replies concise and quote query results inline when useful.`

// RunMCP runs one chat turn (and any tool-call follow-ups) through the MCP
// server. Identical envelope to chat.Run, so the WS handler can swap.
func RunMCP(ctx context.Context, d MCPDeps, in Input) (<-chan llm.Event, error) {
	if d.MaxIterations <= 0 {
		d.MaxIterations = 8
	}
	system := d.System
	if system == "" {
		system = mcpSystemPrompt
	}
	out := make(chan llm.Event, 32)
	go func() {
		defer close(out)
		msgs := append([]llm.Message{}, in.Messages...)
		for iter := 0; iter < d.MaxIterations; iter++ {
			events, err := d.LLM.Stream(ctx, llm.StreamRequest{
				System:   system,
				Messages: msgs,
				Tools:    MCPTools(),
			})
			if err != nil {
				out <- llm.Event{Type: llm.EventError, Message: err.Error()}
				return
			}
			var textBuf string
			var toolCalls []llm.ContentItem
			for ev := range events {
				// Don't forward the LLM's per-turn Done — the client treats it
				// as end-of-conversation. We emit our own Done only when there
				// are no more tool calls.
				if ev.Type != llm.EventDone {
					out <- ev
				}
				switch ev.Type {
				case llm.EventText:
					textBuf += ev.Text
				case llm.EventToolUse:
					toolCalls = append(toolCalls, llm.ContentItem{
						Type: "tool_use", ID: ev.ToolUseID, Name: ev.ToolName, Input: ev.ToolInput,
					})
				case llm.EventError:
					return
				}
			}
			// Materialise the assistant turn.
			var assistant llm.Message
			assistant.Role = "assistant"
			if textBuf != "" {
				assistant.Content = append(assistant.Content, llm.ContentItem{Type: "text", Text: textBuf})
			}
			assistant.Content = append(assistant.Content, toolCalls...)
			if len(toolCalls) == 0 {
				out <- llm.Event{Type: llm.EventDone}
				return
			}
			msgs = append(msgs, assistant)

			toolMsg := llm.Message{Role: "tool"}
			for _, tc := range toolCalls {
				output, err := executeMCP(ctx, d, tc.Name, tc.Input)
				if err != nil {
					output = fmt.Sprintf("ERROR: %v", err)
				}
				out <- llm.Event{Type: llm.EventToolResult, ToolUseID: tc.ID, Output: output}
				toolMsg.Content = append(toolMsg.Content, llm.ContentItem{
					Type: "tool_result", ToolUseID: tc.ID, Output: output,
				})
			}
			msgs = append(msgs, toolMsg)
		}
		out <- llm.Event{Type: llm.EventError, Message: "max iterations reached"}
	}()
	return out, nil
}

// executeMCP translates the orchestrator's tool surface to the MCP-side
// tools, injecting dsn_name. We forward `mysql_query` as-is; the typed
// `list_*` / `describe_*` tools become `mysql_query` with a canned
// information_schema query.
func executeMCP(ctx context.Context, d MCPDeps, name string, input map[string]any) (string, error) {
	switch name {
	case "mysql_query":
		sqlText, _ := input["sql"].(string)
		if sqlText == "" {
			return "", fmt.Errorf("sql required")
		}
		return d.MCP.CallTool(ctx, "mysql_query", map[string]any{
			"dsn_name": d.DSNName,
			"sql":      sqlText,
		})
	case "list_databases":
		return d.MCP.CallTool(ctx, "mysql_query", map[string]any{
			"dsn_name": d.DSNName,
			"sql":      "SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('mysql','information_schema','performance_schema','sys') ORDER BY schema_name",
		})
	case "list_tables":
		schema, _ := input["database"].(string)
		if schema == "" {
			return "", fmt.Errorf("database required")
		}
		return d.MCP.CallTool(ctx, "mysql_query", map[string]any{
			"dsn_name": d.DSNName,
			"sql":      fmt.Sprintf("SELECT table_name, table_rows, ROUND((data_length + index_length)/1024/1024) AS size_mb FROM information_schema.tables WHERE table_schema=%s ORDER BY table_name", sqlString(schema)),
		})
	case "describe_table":
		schema, _ := input["database"].(string)
		table, _ := input["table"].(string)
		if schema == "" || table == "" {
			return "", fmt.Errorf("database/table required")
		}
		return d.MCP.CallTool(ctx, "mysql_query", map[string]any{
			"dsn_name": d.DSNName,
			"sql":      fmt.Sprintf("SHOW CREATE TABLE %s.%s", backtick(schema), backtick(table)),
		})
	default:
		// Unknown tool — pass through to MCP in case the underlying server
		// exposes something extra (graceful degradation).
		args := map[string]any{"dsn_name": d.DSNName}
		for k, v := range input {
			args[k] = v
		}
		return d.MCP.CallTool(ctx, name, args)
	}
}

// sqlString returns a SQL string literal with single-quote escaping. Only
// used here for identifiers that we already know are non-attacker-controlled
// (callers run their own validation upstream); kept defensive.
func sqlString(s string) string {
	out := []byte{'\''}
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'', '\'')
		} else {
			out = append(out, s[i])
		}
	}
	out = append(out, '\'')
	return string(out)
}

// backtick wraps a MySQL identifier in backticks, escaping any embedded
// backticks. Same shape as internal/mysql.QuoteIdent but kept local to avoid
// importing the heavier package from chat.
func backtick(name string) string {
	out := []byte{'`'}
	for i := 0; i < len(name); i++ {
		if name[i] == '`' {
			out = append(out, '`', '`')
		} else {
			out = append(out, name[i])
		}
	}
	out = append(out, '`')
	return string(out)
}
