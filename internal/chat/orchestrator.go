package chat

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
	"github.com/conray/dataseai/internal/llm"
	"github.com/conray/dataseai/internal/store"
)

const defaultSystemPrompt = `You are a database assistant attached to a single MySQL connection. Use the provided tools (list_databases, list_tables, describe_table, query_table, run_sql) to answer questions about the user's data. Prefer narrowly-scoped queries with LIMIT and never run destructive DML/DDL — refuse if asked. Keep replies concise and quote query results inline when useful.`

type Deps struct {
	LLM           llm.LLMClient
	DB            *sql.DB
	Executor      mysqldialect.Executor
	MaxIterations int // safety: limit tool/LLM round trips (default 8)
	System        string

	// New for propose_write (T9/T10/T11):
	Store     *store.Store
	Gateway   ProposalGateway
	UserID    int64
	ConnID    int64
	DefaultDB string

	// IncludeProposeWrite controls whether the propose_write tool is exposed to
	// the LLM. WS handler sets this from the user's ai_writes_enabled flag at
	// session start.
	IncludeProposeWrite bool
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
	// When this chat is pinned to a single database, prepend an explicit scope
	// instruction so the LLM doesn't waste tool calls trying other DBs.
	if d.DefaultDB != "" {
		system += fmt.Sprintf("\n\nIMPORTANT SCOPE: this chat session is scoped to database %q on the current connection. Treat it as the only database that exists — do not call tools with any other `database` value, and do not write SQL that references other schemas. If the user asks about data in a different database, refuse and tell them to switch the DB picker.", d.DefaultDB)
	} else {
		// No DB pinned: don't let the LLM go fishing across every database.
		// Make it ask the user which one to use first.
		system += "\n\nIMPORTANT SCOPE: no database has been selected for this chat. Before calling any data tool (list_tables, describe_table, query_table, run_sql, propose_write), you MUST ask the user which database to use. You MAY call list_databases once to show them the available options. Do NOT pick a database on your own."
	}
	out := make(chan llm.Event, 32)
	go func() {
		defer close(out)
		msgs := append([]llm.Message{}, in.Messages...)
		for iter := 0; iter < d.MaxIterations; iter++ {
			events, err := d.LLM.Stream(ctx, llm.StreamRequest{
				System:   system,
				Messages: msgs,
				Tools:    Tools(ToolOpts{IncludeProposeWrite: d.IncludeProposeWrite}),
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
			log.Printf("[chat] iter=%d, tool calls=%d", iter, len(toolCalls))
			toolMsg := llm.Message{Role: "tool"}
			for _, tc := range toolCalls {
				log.Printf("[chat]   executing tool: %s, input=%v, id=%s", tc.Name, tc.Input, tc.ID)
				output, err := Execute(ctx, ExecCtx{
					DB:        d.DB,
					Executor:  d.Executor,
					Store:     d.Store,
					Gateway:   d.Gateway,
					UserID:    d.UserID,
					ConnID:    d.ConnID,
					DefaultDB: d.DefaultDB,
				}, tc.Name, tc.Input)
				if err != nil {
					log.Printf("[chat]   tool error: %v", err)
					output = fmt.Sprintf("ERROR: %v", err)
				} else {
					log.Printf("[chat]   tool output len=%d", len(output))
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
