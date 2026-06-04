package chat

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/conray/dataseai/internal/llm"
)

const defaultSystemPrompt = `You are a database assistant attached to a single MySQL connection. Use the provided tools (list_databases, list_tables, describe_table, query_table, run_sql) to answer questions about the user's data. Prefer narrowly-scoped queries with LIMIT and never run destructive DML/DDL — refuse if asked. Keep replies concise and quote query results inline when useful.`

type Deps struct {
	LLM           llm.LLMClient
	DB            *sql.DB
	MaxIterations int // safety: limit tool/LLM round trips (default 8)
	System        string
}

type Input struct {
	Messages []llm.Message
}

// Run drives the chat turn: call LLM, dispatch any tool_use, feed tool_result
// back, repeat until the LLM emits no more tool calls (or MaxIterations).
// The returned channel emits Events all the way through (including the tool
// dispatch events as EventToolUse/EventToolResult so the frontend can show
// progress) and closes after the final EventDone or EventError.
func Run(ctx context.Context, d Deps, in Input) (<-chan llm.Event, error) {
	if d.MaxIterations <= 0 {
		d.MaxIterations = 8
	}
	system := d.System
	if system == "" {
		system = defaultSystemPrompt
	}
	out := make(chan llm.Event, 32)
	go func() {
		defer close(out)
		msgs := append([]llm.Message{}, in.Messages...)
		for iter := 0; iter < d.MaxIterations; iter++ {
			events, err := d.LLM.Stream(ctx, llm.StreamRequest{
				System:   system,
				Messages: msgs,
				Tools:    Tools(),
			})
			if err != nil {
				out <- llm.Event{Type: llm.EventError, Message: err.Error()}
				return
			}
			// Aggregate the LLM's reply so we can append it back to messages
			// before the next turn (when tool calls happen).
			var assistant llm.Message
			assistant.Role = "assistant"
			var toolCalls []llm.ContentItem
			var textBuf string
			for ev := range events {
				out <- ev
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
			// Build the assistant message that we'll feed back next iteration.
			if textBuf != "" {
				assistant.Content = append(assistant.Content, llm.ContentItem{Type: "text", Text: textBuf})
			}
			assistant.Content = append(assistant.Content, toolCalls...)
			if len(toolCalls) == 0 {
				out <- llm.Event{Type: llm.EventDone}
				return
			}
			msgs = append(msgs, assistant)
			// Execute every tool call and emit results.
			toolMsg := llm.Message{Role: "tool"}
			for _, tc := range toolCalls {
				output, err := Execute(ctx, d.DB, tc.Name, tc.Input)
				if err != nil {
					output = fmt.Sprintf("ERROR: %v", err)
				}
				out <- llm.Event{
					Type: llm.EventToolResult, ToolUseID: tc.ID, Output: output,
				}
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
