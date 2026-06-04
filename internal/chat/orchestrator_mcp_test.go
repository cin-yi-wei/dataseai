package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/conray/dataseai/internal/llm"
)

type fakeMCP struct {
	last struct {
		name string
		args map[string]any
	}
	reply string
}

func (f *fakeMCP) CallTool(_ context.Context, name string, args map[string]any) (string, error) {
	f.last.name = name
	f.last.args = args
	return f.reply, nil
}

func TestRunMCP_DSNInjectedOnMysqlQuery(t *testing.T) {
	mcp := &fakeMCP{reply: `{"rows":[[1]]}`}
	stub := &fakeLLM{scripts: [][]llm.Event{
		{
			{Type: llm.EventToolUse, ToolUseID: "t1", ToolName: "mysql_query",
				ToolInput: map[string]any{"sql": "SELECT 1"}},
			{Type: llm.EventDone},
		},
		{
			{Type: llm.EventText, Text: "Done."},
			{Type: llm.EventDone},
		},
	}}
	events, err := RunMCP(context.Background(),
		MCPDeps{LLM: stub, MCP: mcp, DSNName: "u1_c10", MaxIterations: 4},
		Input{Messages: []llm.Message{{Role: "user", Content: []llm.ContentItem{{Type: "text", Text: "q"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout")
		case ev, ok := <-events:
			if !ok {
				goto done
			}
			_ = ev
		}
	}
done:
	if mcp.last.name != "mysql_query" {
		t.Fatalf("expected mysql_query tool call, got %q", mcp.last.name)
	}
	if dsn, _ := mcp.last.args["dsn_name"].(string); dsn != "u1_c10" {
		t.Fatalf("dsn_name not injected: %+v", mcp.last.args)
	}
	if sql, _ := mcp.last.args["sql"].(string); sql != "SELECT 1" {
		t.Fatalf("sql args = %q", sql)
	}
}

func TestRunMCP_ListTablesTranslatesToMySQLQuery(t *testing.T) {
	mcp := &fakeMCP{reply: `[]`}
	stub := &fakeLLM{scripts: [][]llm.Event{
		{
			{Type: llm.EventToolUse, ToolUseID: "t1", ToolName: "list_tables",
				ToolInput: map[string]any{"database": "demo"}},
			{Type: llm.EventDone},
		},
		{
			{Type: llm.EventText, Text: "ok"},
			{Type: llm.EventDone},
		},
	}}
	events, _ := RunMCP(context.Background(),
		MCPDeps{LLM: stub, MCP: mcp, DSNName: "u1_c10"},
		Input{Messages: []llm.Message{{Role: "user", Content: []llm.ContentItem{{Type: "text", Text: "tables?"}}}}})
	for range events {
	}
	if mcp.last.name != "mysql_query" {
		t.Fatalf("expected translation to mysql_query, got %q", mcp.last.name)
	}
	sql, _ := mcp.last.args["sql"].(string)
	if !strings.Contains(sql, "information_schema.tables") || !strings.Contains(sql, "'demo'") {
		t.Fatalf("translated SQL doesn't look right: %s", sql)
	}
}

func TestBacktickEscapesBackticks(t *testing.T) {
	if got := backtick("a`b"); got != "`a``b`" {
		t.Fatalf("got %q", got)
	}
}
