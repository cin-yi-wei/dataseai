package chat

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/conray/dataseai/internal/llm"
	_ "github.com/mattn/go-sqlite3"
)

type fakeLLM struct {
	scripts [][]llm.Event
}

func (f *fakeLLM) Stream(ctx context.Context, req llm.StreamRequest) (<-chan llm.Event, error) {
	ch := make(chan llm.Event, 16)
	go func() {
		defer close(ch)
		if len(f.scripts) == 0 {
			ch <- llm.Event{Type: llm.EventDone}
			return
		}
		batch := f.scripts[0]
		f.scripts = f.scripts[1:]
		for _, e := range batch {
			ch <- e
		}
	}()
	return ch, nil
}

func TestRun_PlainTextResponse(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	stub := &fakeLLM{scripts: [][]llm.Event{
		{
			{Type: llm.EventText, Text: "Hello"},
			{Type: llm.EventText, Text: " world"},
			{Type: llm.EventDone},
		},
	}}
	events, err := Run(context.Background(), Deps{LLM: stub, DB: db}, Input{
		Messages: []llm.Message{{Role: "user", Content: []llm.ContentItem{{Type: "text", Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for e := range events {
		if e.Type == llm.EventText {
			text += e.Text
		}
	}
	if text != "Hello world" {
		t.Fatalf("text = %q", text)
	}
}

func TestRun_ToolLoop(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	_, _ = db.Exec("CREATE TABLE t(id INT); INSERT INTO t VALUES(1),(2)")
	stub := &fakeLLM{scripts: [][]llm.Event{
		// First call: LLM asks to run a tool.
		{
			{Type: llm.EventToolUse, ToolUseID: "t1", ToolName: "run_sql", ToolInput: map[string]any{"sql": "SELECT count(*) AS n FROM t"}},
			{Type: llm.EventDone},
		},
		// Second call (after tool_result fed back): LLM emits final text.
		{
			{Type: llm.EventText, Text: "There are 2 rows"},
			{Type: llm.EventDone},
		},
	}}
	events, err := Run(context.Background(), Deps{LLM: stub, DB: db, MaxIterations: 5}, Input{
		Messages: []llm.Message{{Role: "user", Content: []llm.ContentItem{{Type: "text", Text: "count"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	var sawTool, sawResult, sawText bool
	for {
		select {
		case <-deadline:
			t.Fatal("timeout")
		case e, ok := <-events:
			if !ok {
				if !(sawTool && sawResult && sawText) {
					t.Fatalf("missed an event: tool=%v result=%v text=%v", sawTool, sawResult, sawText)
				}
				return
			}
			switch e.Type {
			case llm.EventToolUse:
				sawTool = true
			case llm.EventToolResult:
				sawResult = true
			case llm.EventText:
				if e.Text == "There are 2 rows" {
					sawText = true
				}
			}
		}
	}
}
