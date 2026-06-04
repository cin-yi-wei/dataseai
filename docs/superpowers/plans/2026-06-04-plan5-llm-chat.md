# Plan 5 — LLM + Chat UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Right-group "AI Chat" bottom tab lights up. User selects a connection + database, opens chat, asks a natural-language question; dataseai's backend orchestrates an Anthropic or OpenAI LLM, gives it MySQL tools (query_table, run_sql, list_databases, list_tables, describe_table), streams the assistant's answer + tool calls back to the browser.

**Architecture deviation from spec §9:** The design called for an external `mcp-mysql` sidecar container (askdba/mysql-mcp-server) that dataseai would forward LLM tool calls to. Plan 5 implements the **direct-tools** path instead: the Go backend exposes its own tool schema to the LLM (same names + JSON shapes the spec describes), and the orchestrator executes each tool call by calling existing `mysql.Run` / `mysql.ListDatabases` / etc. directly. Trade-off: we lose plug-and-play MCP ecosystem compatibility; we gain a zero-extra-container deployment that works the moment an API key is set. An "MCP shim" task is parked at the end of the plan as optional.

**Tech Stack:**
- Go: `net/http` for LLM HTTP requests (no SDKs — small surface area, easier to vendor than auth-token-juggling SDKs), existing `internal/mysql` for tool execution
- Frontend: existing Zustand + native WebSocket (same pattern as `/ws/query` in Plan 4)
- WebSocket auth via `?token=` query param (Plan 4 precedent)

**Spec reference:** `docs/superpowers/specs/2026-06-03-dataseai-design.md` Section 9 (Chat + MCP).

**Plan 4 carryover (still open):** browse 500 leaks raw driver error, spec query-param drift, middleware 5s cache, CodeMirror bundle.

---

## File Structure

```
dataseai/
├── internal/
│   ├── llm/                    # new
│   │   ├── client.go           # LLMClient interface + Tool, Message, Event types
│   │   ├── anthropic.go        # Anthropic Messages API client (streaming + tool use)
│   │   ├── openai.go           # OpenAI Chat Completions client (streaming + tools)
│   │   └── factory.go          # pick provider by name
│   ├── chat/                   # new
│   │   ├── tools.go            # tool schema definitions
│   │   ├── execute.go          # dispatch tool_use → mysql package
│   │   └── orchestrator.go     # the run-loop: user msg → LLM → tool → LLM → done
│   └── api/
│       ├── chat.go             # WS /ws/chat handler
│       └── router.go           # extended
├── cmd/dataseai/main.go        # wire LLM factory
├── web/
│   ├── src/
│   │   ├── store/chat.ts       # new
│   │   ├── lib/chatWs.ts       # new
│   │   ├── components/
│   │   │   ├── ChatPanel.tsx   # new
│   │   │   ├── ChatMessage.tsx # new
│   │   │   └── BottomTabs.tsx  # extended — enable chat tab
│   │   └── routes/Workspace.tsx # extended — route bottom 'chat' to ChatPanel
└── README.md                   # addendum
```

**Conventions reused:** module `github.com/conray/dataseai`; per-task TDD where it matters (tool execution, LLM event parsing); one git commit per task. Frontend components have no separate tests beyond typecheck (consistent with Plans 2-4).

---

## Task 1: LLM client interface + common types

**Files:**
- Create: `internal/llm/client.go`, `internal/llm/client_test.go`

- [ ] **Step 1: Tests**

```go
package llm

import "testing"

func TestToolJSONShape(t *testing.T) {
	tool := Tool{
		Name:        "list_databases",
		Description: "List schemas on the current MySQL connection",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
	}
	if tool.Name == "" || tool.InputSchema == nil {
		t.Fatal("tool malformed")
	}
}

func TestEventTypes(t *testing.T) {
	// Sanity: event Type constants stable for downstream consumers.
	if EventText != "text" || EventToolUse != "tool_use" || EventToolResult != "tool_result" || EventDone != "done" || EventError != "error" {
		t.Fatal("event type constants drifted")
	}
}
```

- [ ] **Step 2: Verify fail**

```bash
go test ./internal/llm/ -v
```

- [ ] **Step 3: Implement `internal/llm/client.go`**

```go
package llm

import (
	"context"
)

const (
	EventText       = "text"
	EventToolUse    = "tool_use"
	EventToolResult = "tool_result"
	EventDone       = "done"
	EventError      = "error"
)

// Tool is the LLM-facing description of a callable function.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// Message is one turn of conversation.
type Message struct {
	Role    string        `json:"role"` // "user" | "assistant" | "tool"
	Content []ContentItem `json:"content"`
}

// ContentItem is a part of a message — either plain text or a tool call/result.
type ContentItem struct {
	Type     string         `json:"type"`        // "text" | "tool_use" | "tool_result"
	Text     string         `json:"text,omitempty"`
	ID       string         `json:"id,omitempty"`        // tool_use id
	Name     string         `json:"name,omitempty"`      // tool_use name
	Input    map[string]any `json:"input,omitempty"`     // tool_use args
	ToolUseID string        `json:"tool_use_id,omitempty"` // tool_result back-ref
	Output   string         `json:"output,omitempty"`    // tool_result body
}

// Event is what Stream emits to its caller.
type Event struct {
	Type      string         // EventText/EventToolUse/EventToolResult/EventDone/EventError
	Text      string         // EventText (a delta chunk)
	ToolUseID string         // EventToolUse / EventToolResult
	ToolName  string         // EventToolUse
	ToolInput map[string]any // EventToolUse
	Output    string         // EventToolResult
	Message   string         // EventError
}

// StreamRequest is the input to a single LLM turn.
type StreamRequest struct {
	Model    string
	System   string
	Messages []Message
	Tools    []Tool
}

// LLMClient — providers implement Stream and emit Events on the returned channel.
// The channel is closed when the stream completes (after the EventDone or EventError event).
type LLMClient interface {
	Stream(ctx context.Context, req StreamRequest) (<-chan Event, error)
}
```

- [ ] **Step 4: Verify pass + commit**

```bash
go test ./internal/llm/ -v
git add internal/llm/
git commit -m "feat(llm): LLMClient interface + Tool/Message/Event types"
```

---

## Task 2: Anthropic provider

**Files:**
- Create: `internal/llm/anthropic.go`, `internal/llm/anthropic_test.go`

- [ ] **Step 1: Failing tests**

The Anthropic SSE format parses fairly mechanically. Most of the test value is in event-name dispatch — the actual HTTPS call is exercised manually.

```go
package llm

import (
	"strings"
	"testing"
)

func TestParseAnthropicEvent_ContentBlockText(t *testing.T) {
	raw := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n"
	events := parseAnthropicSSE(strings.NewReader(raw))
	got := <-events
	if got.Type != EventText || got.Text != "Hello" {
		t.Fatalf("got = %+v", got)
	}
}

func TestParseAnthropicEvent_ToolUse(t *testing.T) {
	// Anthropic emits content_block_start with type=tool_use, then a series of
	// input_json_delta inside content_block_delta, then content_block_stop.
	raw := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"list_databases","input":{}}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"x\":1}"}}`,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
	}, "\n") + "\n"
	events := parseAnthropicSSE(strings.NewReader(raw))
	var toolEv Event
	for ev := range events {
		if ev.Type == EventToolUse {
			toolEv = ev
			break
		}
	}
	if toolEv.ToolName != "list_databases" || toolEv.ToolUseID != "toolu_1" {
		t.Fatalf("toolEv = %+v", toolEv)
	}
	if v, _ := toolEv.ToolInput["x"]; v != float64(1) {
		t.Fatalf("input = %+v", toolEv.ToolInput)
	}
}
```

- [ ] **Step 2: Verify fail**

- [ ] **Step 3: Implement `internal/llm/anthropic.go`**

```go
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Anthropic struct {
	APIKey   string
	Model    string // default "claude-opus-4-7" — caller can override
	BaseURL  string // default "https://api.anthropic.com"
	Client   *http.Client
}

func (a *Anthropic) Stream(ctx context.Context, req StreamRequest) (<-chan Event, error) {
	model := req.Model
	if model == "" {
		model = a.Model
	}
	if model == "" {
		model = "claude-opus-4-7"
	}
	base := a.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	httpClient := a.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"stream":     true,
		"messages":   anthropicMessages(req.Messages),
	}
	if req.System != "" {
		body["system"] = req.System
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.InputSchema,
			}
		}
		body["tools"] = tools
	}
	bs, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", base+"/v1/messages", bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, string(body))
	}

	return streamFrom(parseAnthropicSSE(resp.Body), resp.Body), nil
}

func streamFrom(parsed <-chan Event, closer io.Closer) <-chan Event {
	out := make(chan Event, 16)
	go func() {
		defer close(out)
		defer closer.Close()
		for ev := range parsed {
			out <- ev
		}
	}()
	return out
}

func anthropicMessages(in []Message) []map[string]any {
	out := make([]map[string]any, len(in))
	for i, m := range in {
		// Anthropic accepts the role and content array directly.
		content := make([]map[string]any, len(m.Content))
		for j, c := range m.Content {
			ci := map[string]any{"type": c.Type}
			switch c.Type {
			case "text":
				ci["text"] = c.Text
			case "tool_use":
				ci["id"] = c.ID
				ci["name"] = c.Name
				ci["input"] = c.Input
			case "tool_result":
				ci["tool_use_id"] = c.ToolUseID
				ci["content"] = c.Output
			}
			content[j] = ci
		}
		role := m.Role
		if role == "tool" {
			role = "user" // Anthropic puts tool_result blocks inside a user message
		}
		out[i] = map[string]any{"role": role, "content": content}
	}
	return out
}

// parseAnthropicSSE reads the streaming response body, decodes the SSE event
// stream, and emits high-level Events. It buffers tool_use accumulation
// across content_block_start/delta/stop messages.
func parseAnthropicSSE(r io.Reader) <-chan Event {
	out := make(chan Event, 16)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		var event, data string
		// In-flight tool block being accumulated.
		var tool struct {
			id, name string
			argBuf   strings.Builder
			active   bool
		}
		flushTool := func() {
			if !tool.active {
				return
			}
			var input map[string]any
			if tool.argBuf.Len() > 0 {
				_ = json.Unmarshal([]byte(tool.argBuf.String()), &input)
			}
			out <- Event{
				Type: EventToolUse, ToolUseID: tool.id, ToolName: tool.name, ToolInput: input,
			}
			tool = struct {
				id, name string
				argBuf   strings.Builder
				active   bool
			}{}
		}
		dispatch := func() {
			if data == "" {
				return
			}
			var msg map[string]any
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				return
			}
			t, _ := msg["type"].(string)
			switch t {
			case "content_block_start":
				if cb, ok := msg["content_block"].(map[string]any); ok {
					if ct, _ := cb["type"].(string); ct == "tool_use" {
						id, _ := cb["id"].(string)
						name, _ := cb["name"].(string)
						tool.id = id
						tool.name = name
						tool.argBuf.Reset()
						tool.active = true
					}
				}
			case "content_block_delta":
				if d, ok := msg["delta"].(map[string]any); ok {
					dt, _ := d["type"].(string)
					switch dt {
					case "text_delta":
						txt, _ := d["text"].(string)
						out <- Event{Type: EventText, Text: txt}
					case "input_json_delta":
						pj, _ := d["partial_json"].(string)
						tool.argBuf.WriteString(pj)
					}
				}
			case "content_block_stop":
				flushTool()
			case "message_stop":
				out <- Event{Type: EventDone}
			case "error":
				if e, ok := msg["error"].(map[string]any); ok {
					m, _ := e["message"].(string)
					out <- Event{Type: EventError, Message: m}
				}
			}
		}
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				dispatch()
				event, data = "", ""
				continue
			}
			if strings.HasPrefix(line, "event:") {
				event = strings.TrimSpace(line[len("event:"):])
				_ = event
				continue
			}
			if strings.HasPrefix(line, "data:") {
				data = strings.TrimSpace(line[len("data:"):])
				continue
			}
		}
	}()
	return out
}
```

- [ ] **Step 4: Verify pass + commit**

```bash
go test ./internal/llm/ -v
git add internal/llm/
git commit -m "feat(llm): Anthropic Messages streaming client (text + tool_use)"
```

---

## Task 3: OpenAI provider

**Files:**
- Create: `internal/llm/openai.go`, `internal/llm/openai_test.go`

- [ ] **Step 1: Failing test**

```go
package llm

import (
	"strings"
	"testing"
)

func TestParseOpenAIEvent_TextDelta(t *testing.T) {
	raw := "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n"
	events := parseOpenAISSE(strings.NewReader(raw))
	for ev := range events {
		if ev.Type == EventText && ev.Text == "Hi" {
			return
		}
	}
	t.Fatal("text delta not parsed")
}

func TestParseOpenAIEvent_ToolCall(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"list_databases","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"finish_reason":"tool_calls"}]}`,
		``,
	}, "\n") + "\n"
	events := parseOpenAISSE(strings.NewReader(raw))
	var got Event
	for ev := range events {
		if ev.Type == EventToolUse {
			got = ev
			break
		}
	}
	if got.ToolName != "list_databases" || got.ToolUseID != "call_1" {
		t.Fatalf("got = %+v", got)
	}
}
```

- [ ] **Step 2: Verify fail**

- [ ] **Step 3: Implement `internal/llm/openai.go`**

```go
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OpenAI struct {
	APIKey  string
	Model   string // default "gpt-4o"
	BaseURL string // default "https://api.openai.com/v1"
	Client  *http.Client
}

func (o *OpenAI) Stream(ctx context.Context, req StreamRequest) (<-chan Event, error) {
	model := req.Model
	if model == "" {
		model = o.Model
	}
	if model == "" {
		model = "gpt-4o"
	}
	base := o.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	httpClient := o.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	msgs := openAIMessages(req.System, req.Messages)
	body := map[string]any{
		"model":    model,
		"stream":   true,
		"messages": msgs,
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.InputSchema,
				},
			}
		}
		body["tools"] = tools
	}
	bs, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", base+"/chat/completions", bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.APIKey)
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, string(body))
	}
	return streamFrom(parseOpenAISSE(resp.Body), resp.Body), nil
}

func openAIMessages(system string, in []Message) []map[string]any {
	out := []map[string]any{}
	if system != "" {
		out = append(out, map[string]any{"role": "system", "content": system})
	}
	for _, m := range in {
		role := m.Role
		// Aggregate the message content.
		switch role {
		case "tool":
			// Each tool_result block becomes its own role:tool message.
			for _, c := range m.Content {
				if c.Type == "tool_result" {
					out = append(out, map[string]any{
						"role":         "tool",
						"tool_call_id": c.ToolUseID,
						"content":      c.Output,
					})
				}
			}
		default:
			var text string
			var toolCalls []map[string]any
			for _, c := range m.Content {
				switch c.Type {
				case "text":
					text += c.Text
				case "tool_use":
					inp, _ := json.Marshal(c.Input)
					toolCalls = append(toolCalls, map[string]any{
						"id": c.ID, "type": "function",
						"function": map[string]any{"name": c.Name, "arguments": string(inp)},
					})
				}
			}
			msg := map[string]any{"role": role}
			if text != "" {
				msg["content"] = text
			}
			if len(toolCalls) > 0 {
				msg["tool_calls"] = toolCalls
			}
			out = append(out, msg)
		}
	}
	return out
}

// parseOpenAISSE handles OpenAI's `data: {json}` SSE format. Tool call
// arguments arrive as a series of partial strings keyed by index — we
// accumulate per-index then emit a single EventToolUse when finish_reason
// signals "tool_calls".
func parseOpenAISSE(r io.Reader) <-chan Event {
	out := make(chan Event, 16)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		type tc struct {
			id, name, args string
		}
		acc := map[int]*tc{}
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(line[len("data:"):])
			if data == "[DONE]" {
				out <- Event{Type: EventDone}
				return
			}
			if data == "" {
				continue
			}
			var msg map[string]any
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				continue
			}
			choices, _ := msg["choices"].([]any)
			if len(choices) == 0 {
				continue
			}
			ch, _ := choices[0].(map[string]any)
			delta, _ := ch["delta"].(map[string]any)
			if content, ok := delta["content"].(string); ok && content != "" {
				out <- Event{Type: EventText, Text: content}
			}
			if tcs, ok := delta["tool_calls"].([]any); ok {
				for _, tcAny := range tcs {
					tcMap, _ := tcAny.(map[string]any)
					idxF, _ := tcMap["index"].(float64)
					idx := int(idxF)
					if acc[idx] == nil {
						acc[idx] = &tc{}
					}
					if id, ok := tcMap["id"].(string); ok && id != "" {
						acc[idx].id = id
					}
					if fn, ok := tcMap["function"].(map[string]any); ok {
						if name, ok := fn["name"].(string); ok && name != "" {
							acc[idx].name = name
						}
						if args, ok := fn["arguments"].(string); ok {
							acc[idx].args += args
						}
					}
				}
			}
			if reason, _ := ch["finish_reason"].(string); reason == "tool_calls" {
				for _, t := range acc {
					var input map[string]any
					_ = json.Unmarshal([]byte(t.args), &input)
					out <- Event{Type: EventToolUse, ToolUseID: t.id, ToolName: t.name, ToolInput: input}
				}
				acc = map[int]*tc{}
			}
		}
	}()
	return out
}
```

- [ ] **Step 4: Verify pass + commit**

```bash
go test ./internal/llm/ -v
git add internal/llm/
git commit -m "feat(llm): OpenAI Chat Completions streaming client (text + tool_calls)"
```

---

## Task 4: LLM factory

**Files:**
- Create: `internal/llm/factory.go`

- [ ] **Step 1: Implement**

```go
package llm

import (
	"fmt"
	"net/http"
	"time"
)

type Config struct {
	Default         string // "anthropic" | "openai"
	AnthropicAPIKey string
	OpenAIAPIKey    string
	AnthropicModel  string
	OpenAIModel     string
}

// Pick returns the configured client. provider == "" → Default.
func Pick(cfg Config, provider string) (LLMClient, error) {
	if provider == "" {
		provider = cfg.Default
	}
	httpClient := &http.Client{Timeout: 5 * time.Minute}
	switch provider {
	case "anthropic":
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("anthropic api key not set")
		}
		return &Anthropic{APIKey: cfg.AnthropicAPIKey, Model: cfg.AnthropicModel, Client: httpClient}, nil
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("openai api key not set")
		}
		return &OpenAI{APIKey: cfg.OpenAIAPIKey, Model: cfg.OpenAIModel, Client: httpClient}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (expected anthropic|openai)", provider)
	}
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./internal/llm/
git add internal/llm/factory.go
git commit -m "feat(llm): factory — pick provider by name from config"
```

---

## Task 5: Chat tools (schema + dispatcher)

**Files:**
- Create: `internal/chat/tools.go`, `internal/chat/execute.go`, `internal/chat/execute_test.go`

- [ ] **Step 1: Failing tests at `internal/chat/execute_test.go`**

```go
package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestExecute_ListDatabases(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	// Without information_schema, this will fail — capture that.
	out, err := Execute(context.Background(), db, "list_databases", map[string]any{})
	if err == nil {
		t.Skip("sqlite happens to have information_schema-like view")
	}
	if out != "" {
		t.Fatalf("out should be empty on error, got %q", out)
	}
}

func TestExecute_RunSQL(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	_, _ = db.Exec("CREATE TABLE t(id INT, n TEXT); INSERT INTO t VALUES(1,'a'),(2,'b')")
	out, err := Execute(context.Background(), db, "run_sql", map[string]any{"sql": "SELECT * FROM t ORDER BY id"})
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if parsed["rows"] == nil {
		t.Fatalf("missing rows: %s", out)
	}
}

func TestExecute_UnknownTool(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	_, err := Execute(context.Background(), db, "no_such_tool", map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Verify fail**

- [ ] **Step 3: Implement `internal/chat/tools.go`**

```go
package chat

import "github.com/conray/dataseai/internal/llm"

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
```

- [ ] **Step 4: Implement `internal/chat/execute.go`**

```go
package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/conray/dataseai/internal/mysql"
)

// Execute dispatches a single tool call against db. Returns a JSON string
// (the tool result body) or an error if the tool name is unknown / args bad.
// The result is what's fed back to the LLM as a tool_result block.
func Execute(ctx context.Context, db *sql.DB, name string, input map[string]any) (string, error) {
	switch name {
	case "list_databases":
		names, err := mysql.ListDatabases(ctx, db)
		if err != nil {
			return "", err
		}
		return marshal(map[string]any{"databases": names})

	case "list_tables":
		schema, _ := input["database"].(string)
		if schema == "" {
			return "", fmt.Errorf("database required")
		}
		tables, err := mysql.ListTables(ctx, db, schema)
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
		s, err := mysql.DescribeTable(ctx, db, schema, table)
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
		out, err := mysql.Run(ctx, db, q, mysql.RunOpts{MaxRows: limit})
		if err != nil {
			return "", err
		}
		return marshal(map[string]any{"columns": out.Columns, "rows": out.Rows})

	case "run_sql":
		sqlStr, _ := input["sql"].(string)
		if sqlStr == "" {
			return "", fmt.Errorf("sql required")
		}
		out, err := mysql.Run(ctx, db, sqlStr, mysql.RunOpts{MaxRows: 1000})
		if err != nil {
			return "", err
		}
		return marshal(map[string]any{
			"columns":       out.Columns,
			"rows":          out.Rows,
			"rows_affected": out.RowsAffected,
			"truncated":     out.Truncated,
		})

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
```

- [ ] **Step 5: Verify pass + commit**

```bash
go test ./internal/chat/ -v
git add internal/chat/
git commit -m "feat(chat): tool schema + Execute dispatcher (list/describe/query/run_sql)"
```

---

## Task 6: Chat orchestrator

**Files:**
- Create: `internal/chat/orchestrator.go`, `internal/chat/orchestrator_test.go`

- [ ] **Step 1: Failing test**

The orchestrator runs the LLM loop. We exercise it with a stub LLMClient.

```go
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
```

- [ ] **Step 2: Verify fail**

- [ ] **Step 3: Implement `internal/chat/orchestrator.go`**

```go
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
```

- [ ] **Step 4: Verify pass + commit**

```bash
go test ./internal/chat/ -v
git add internal/chat/
git commit -m "feat(chat): orchestrator — LLM ↔ tool loop with MaxIterations guard"
```

---

## Task 7: WS /ws/chat handler

**Files:**
- Create: `internal/api/chat.go`, `internal/api/chat_test.go`
- Modify: `internal/api/router.go`, `cmd/dataseai/main.go`

- [ ] **Step 1: Failing test**

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"
)

func TestWSChat_RequiresToken(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	srv := httptest.NewServer(r)
	defer srv.Close()
	wsURL := "ws" + srv.URL[len("http"):] + "/ws/chat"
	_, _, err := websocket.Dial(t.Context(), wsURL, nil)
	if err == nil {
		t.Fatal("expected dial to fail without token")
	}
}
```

- [ ] **Step 2: Verify fail**

- [ ] **Step 3: Implement `internal/api/chat.go`**

```go
package api

import (
	"context"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/conray/dataseai/internal/chat"
	"github.com/conray/dataseai/internal/llm"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
)

type chatExecReq struct {
	Type     string        `json:"type"`     // "exec" | "cancel"
	ConnID   int64         `json:"conn_id"`
	DB       string        `json:"db"`
	Provider string        `json:"provider"` // "anthropic" | "openai" | ""
	Messages []llm.Message `json:"messages"`
}

type chatMsg struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	ToolInput any    `json:"tool_input,omitempty"`
	Output    string `json:"output,omitempty"`
	Message   string `json:"message,omitempty"`
}

func handleWSChat(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.URL.Query().Get("token")
		if tok == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		sess, err := d.Store.GetSession(tok)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		u, err := d.Store.GetUserByID(sess.UserID)
		if err != nil {
			http.Error(w, "invalid user", http.StatusUnauthorized)
			return
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			return
		}
		defer c.CloseNow()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		var req chatExecReq
		if err := wsjson.Read(ctx, c, &req); err != nil {
			return
		}
		if req.Type != "exec" {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: "first envelope must be type:exec"})
			return
		}

		// Resolve the user's connection + open the *sql.DB.
		conn, err := d.Store.GetConnection(u.ID, req.ConnID)
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: "connection not found"})
			return
		}
		pw, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, req.ConnID)
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: "decrypt failed"})
			return
		}
		dsn := mysql.BuildDSN(mysql.DSNInput{
			Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
			DefaultDB: conn.DefaultDB, TLS: conn.TLS,
		})
		db, err := d.Pool.Get(mysql.PoolKey{UserID: u.ID, ConnID: req.ConnID}, dsn)
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: err.Error()})
			return
		}
		if req.DB != "" {
			sc, err := db.Conn(ctx)
			if err == nil {
				_, _ = sc.ExecContext(ctx, "USE "+mysql.QuoteIdent(req.DB))
				_ = sc.Close()
			}
		}

		llmClient, err := llm.Pick(d.LLMConfig, req.Provider)
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: err.Error()})
			return
		}
		events, err := chat.Run(ctx, chat.Deps{LLM: llmClient, DB: db}, chat.Input{
			Messages: req.Messages,
		})
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: err.Error()})
			return
		}
		for ev := range events {
			out := chatMsg{Type: ev.Type, Text: ev.Text, ToolUseID: ev.ToolUseID,
				ToolName: ev.ToolName, ToolInput: ev.ToolInput, Output: ev.Output, Message: ev.Message}
			if err := wsjson.Write(ctx, c, out); err != nil {
				return
			}
			if ev.Type == llm.EventDone || ev.Type == llm.EventError {
				return
			}
		}
		_ = store.Connection{} // keep store import live
	}
}
```

- [ ] **Step 4: Extend Deps with LLMConfig**

In `internal/api/router.go`:

```go
import (
	"github.com/conray/dataseai/internal/llm"
	// ... existing ...
)

type Deps struct {
	// ... existing ...
	LLMConfig llm.Config
}
```

Wire the route OUTSIDE the auth-middleware group (WS token comes via query param):

```go
	r.HandleFunc("/ws/chat", handleWSChat(d))
```

- [ ] **Step 5: Wire from `cmd/dataseai/main.go`**

```go
import "github.com/conray/dataseai/internal/llm"

// after pool := ...
llmCfg := llm.Config{
	Default:         cfg.LLMDefault,
	AnthropicAPIKey: cfg.AnthropicAPIKey,
	OpenAIAPIKey:    cfg.OpenAIAPIKey,
}
```

And add to `api.Deps{}`:

```go
LLMConfig: llmCfg,
```

- [ ] **Step 6: Verify pass + commit**

```bash
go test ./internal/api/ -v
git add internal/ cmd/
git commit -m "feat(api): WebSocket /ws/chat — LLM-orchestrated streaming with tool calls"
```

---

## Task 8: Frontend chat store

**Files:**
- Create: `web/src/store/chat.ts`, `web/src/store/chat.test.ts`

- [ ] **Step 1: Failing test**

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useChat } from './chat'

describe('useChat', () => {
  beforeEach(() => {
    useChat.setState({ messages: [], busy: false, error: null })
  })

  it('appends user message', () => {
    useChat.getState().pushUser('hello')
    expect(useChat.getState().messages).toHaveLength(1)
    expect(useChat.getState().messages[0].role).toBe('user')
  })

  it('appends assistant text incrementally', () => {
    useChat.getState().pushAssistant()
    useChat.getState().appendText('hi ')
    useChat.getState().appendText('there')
    const m = useChat.getState().messages
    expect(m[m.length - 1].role).toBe('assistant')
    expect(m[m.length - 1].text).toBe('hi there')
  })
})
```

- [ ] **Step 2: Implement `web/src/store/chat.ts`**

```ts
import { create } from 'zustand'

export interface ToolCall {
  id: string
  name: string
  input: any
  output?: string
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  text: string
  toolCalls: ToolCall[]
}

interface State {
  messages: ChatMessage[]
  busy: boolean
  error: string | null
  pushUser: (text: string) => void
  pushAssistant: () => void
  appendText: (chunk: string) => void
  addToolCall: (tc: ToolCall) => void
  setToolOutput: (id: string, output: string) => void
  reset: () => void
  setBusy: (b: boolean) => void
  setError: (e: string | null) => void
}

export const useChat = create<State>((set, get) => ({
  messages: [],
  busy: false,
  error: null,
  pushUser: (text) => set({ messages: [...get().messages, { role: 'user', text, toolCalls: [] }] }),
  pushAssistant: () => set({ messages: [...get().messages, { role: 'assistant', text: '', toolCalls: [] }] }),
  appendText: (chunk) => {
    const msgs = get().messages.slice()
    if (msgs.length === 0 || msgs[msgs.length - 1].role !== 'assistant') {
      msgs.push({ role: 'assistant', text: '', toolCalls: [] })
    }
    msgs[msgs.length - 1] = { ...msgs[msgs.length - 1], text: msgs[msgs.length - 1].text + chunk }
    set({ messages: msgs })
  },
  addToolCall: (tc) => {
    const msgs = get().messages.slice()
    if (msgs.length === 0 || msgs[msgs.length - 1].role !== 'assistant') {
      msgs.push({ role: 'assistant', text: '', toolCalls: [] })
    }
    const last = msgs[msgs.length - 1]
    msgs[msgs.length - 1] = { ...last, toolCalls: [...last.toolCalls, tc] }
    set({ messages: msgs })
  },
  setToolOutput: (id, output) => {
    const msgs = get().messages.map((m) => ({
      ...m,
      toolCalls: m.toolCalls.map((tc) => (tc.id === id ? { ...tc, output } : tc)),
    }))
    set({ messages: msgs })
  },
  reset: () => set({ messages: [], busy: false, error: null }),
  setBusy: (b) => set({ busy: b }),
  setError: (e) => set({ error: e }),
}))
```

- [ ] **Step 3: Test pass + commit**

```bash
cd web && npm test -- chat.test.ts && cd ..
git add web/src/store/chat.ts web/src/store/chat.test.ts
git commit -m "feat(web): chat Zustand store (messages + incremental text + tool calls)"
```

---

## Task 9: Frontend chat WS client

**Files:**
- Create: `web/src/lib/chatWs.ts`

- [ ] **Step 1: Implement**

```ts
export interface ChatEvent {
  type: 'text' | 'tool_use' | 'tool_result' | 'done' | 'error'
  text?: string
  tool_use_id?: string
  tool_name?: string
  tool_input?: any
  output?: string
  message?: string
}

interface Message {
  role: 'user' | 'assistant' | 'tool'
  content: any[]
}

export function chatStream(args: {
  token: string
  connId: number
  db: string
  provider?: string
  messages: Message[]
  onEvent: (e: ChatEvent) => void
  onClose?: () => void
}): { cancel: () => void } {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const url = `${proto}://${location.host}/ws/chat?token=${encodeURIComponent(args.token)}`
  const ws = new WebSocket(url)
  ws.onopen = () => {
    ws.send(JSON.stringify({
      type: 'exec',
      conn_id: args.connId,
      db: args.db,
      provider: args.provider ?? '',
      messages: args.messages,
    }))
  }
  ws.onmessage = (m) => {
    try {
      const ev = JSON.parse(m.data) as ChatEvent
      args.onEvent(ev)
      if (ev.type === 'done' || ev.type === 'error') ws.close()
    } catch {
      args.onEvent({ type: 'error', message: 'bad stream message' })
      ws.close()
    }
  }
  ws.onclose = () => args.onClose?.()
  return {
    cancel: () => {
      try { ws.close() } catch {}
    },
  }
}
```

- [ ] **Step 2: Typecheck + commit**

```bash
cd web && npx tsc --noEmit && cd ..
git add web/src/lib/chatWs.ts
git commit -m "feat(web): chatStream — WebSocket /ws/chat client"
```

---

## Task 10: ChatPanel component

**Files:**
- Create: `web/src/components/ChatPanel.tsx`

- [ ] **Step 1: Implement**

```tsx
import { FormEvent, useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { useChat } from '../store/chat'
import { useActiveConn } from '../store/activeConn'
import { chatStream } from '../lib/chatWs'

interface Props {
  database?: string
}

export default function ChatPanel({ database }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const messages = useChat((s) => s.messages)
  const busy = useChat((s) => s.busy)
  const error = useChat((s) => s.error)
  const pushUser = useChat((s) => s.pushUser)
  const appendText = useChat((s) => s.appendText)
  const addToolCall = useChat((s) => s.addToolCall)
  const setToolOutput = useChat((s) => s.setToolOutput)
  const reset = useChat((s) => s.reset)
  const setBusy = useChat((s) => s.setBusy)
  const setError = useChat((s) => s.setError)
  const [input, setInput] = useState('')
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const cancelRef = useRef<(() => void) | null>(null)

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [messages])

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!input.trim() || connId == null) return
    const text = input.trim()
    setInput('')
    pushUser(text)
    setBusy(true)
    setError(null)
    // Build the messages payload from store state. Tool results were already
    // captured at the previous turn, so we send the full transcript here.
    const transcript: any[] = []
    for (const m of [...messages, { role: 'user' as const, text, toolCalls: [] }]) {
      if (m.role === 'user') {
        transcript.push({ role: 'user', content: [{ type: 'text', text: m.text }] })
      } else {
        const content: any[] = []
        if (m.text) content.push({ type: 'text', text: m.text })
        for (const tc of m.toolCalls) {
          content.push({ type: 'tool_use', id: tc.id, name: tc.name, input: tc.input })
        }
        if (content.length) transcript.push({ role: 'assistant', content })
        const toolResults: any[] = m.toolCalls
          .filter((tc) => tc.output !== undefined)
          .map((tc) => ({ type: 'tool_result', tool_use_id: tc.id, output: tc.output! }))
        if (toolResults.length) transcript.push({ role: 'tool', content: toolResults })
      }
    }
    const token = localStorage.getItem('dataseai.token') ?? ''
    const s = chatStream({
      token,
      connId,
      db: database ?? '',
      messages: transcript,
      onEvent: (ev) => {
        if (ev.type === 'text') appendText(ev.text ?? '')
        else if (ev.type === 'tool_use') addToolCall({ id: ev.tool_use_id!, name: ev.tool_name ?? '', input: ev.tool_input })
        else if (ev.type === 'tool_result') setToolOutput(ev.tool_use_id!, ev.output ?? '')
        else if (ev.type === 'error') setError(ev.message ?? 'chat error')
      },
      onClose: () => {
        setBusy(false)
        cancelRef.current = null
      },
    })
    cancelRef.current = s.cancel
  }

  return (
    <div style={wrap}>
      <div style={bar}>
        <strong>🤖 AI Chat</strong>
        {database && <span style={{ fontSize: 12, color: '#666' }}>db: {database}</span>}
        <span style={{ flex: 1 }} />
        <button onClick={reset}>clear</button>
      </div>
      <div ref={scrollRef} style={msgList}>
        {messages.length === 0 && (
          <div style={{ color: '#999', padding: 16, textAlign: 'center' }}>
            Ask about your data. Try "list databases" or "show me the schema of users".
          </div>
        )}
        {messages.map((m, i) => (
          <div key={i} style={{ padding: 8, borderBottom: '1px solid #f0f0f0' }}>
            <div style={{ fontSize: 11, color: '#888', marginBottom: 2 }}>{m.role}</div>
            {m.text && <div style={{ whiteSpace: 'pre-wrap', fontSize: 14 }}>{m.text}</div>}
            {m.toolCalls.map((tc) => (
              <details key={tc.id} style={{ marginTop: 6, background: '#f5f7fa', borderRadius: 4, padding: 4 }}>
                <summary style={{ fontSize: 12 }}>🔧 {tc.name}({JSON.stringify(tc.input)})</summary>
                <pre style={{ fontSize: 11, margin: 4, whiteSpace: 'pre-wrap', maxHeight: 240, overflow: 'auto' }}>{tc.output ?? '(pending…)'}</pre>
              </details>
            ))}
          </div>
        ))}
        {busy && <div style={{ padding: 8, color: '#888', fontSize: 13 }}>thinking…</div>}
        {error && <div style={{ padding: 8, color: 'crimson', fontSize: 13 }}>{error}</div>}
      </div>
      <form onSubmit={submit} style={form}>
        <input
          autoFocus
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={connId == null ? 'pick a connection first' : 'Ask about your data…'}
          disabled={connId == null || busy}
          style={{ flex: 1, padding: '6px 8px' }}
        />
        <button disabled={connId == null || busy || !input.trim()}>send</button>
      </form>
    </div>
  )
}

const wrap: CSSProperties = { display: 'flex', flexDirection: 'column', height: '100%', fontFamily: 'system-ui' }
const bar: CSSProperties = { display: 'flex', alignItems: 'center', gap: 8, padding: 6, borderBottom: '1px solid #ddd', background: '#fafafa' }
const msgList: CSSProperties = { flex: 1, overflow: 'auto' }
const form: CSSProperties = { display: 'flex', gap: 8, padding: 8, borderTop: '1px solid #ddd' }
```

- [ ] **Step 2: Typecheck + commit**

```bash
cd web && npx tsc --noEmit && cd ..
git add web/src/components/ChatPanel.tsx
git commit -m "feat(web): ChatPanel — message list + tool-call expandable details + input"
```

---

## Task 11: Enable chat tab + wire into Workspace

**Files:**
- Modify: `web/src/components/BottomTabs.tsx`, `web/src/routes/Workspace.tsx`

- [ ] **Step 1: Flip the chat tab to enabled**

Edit `web/src/components/BottomTabs.tsx`:

```ts
const RIGHT: { key: BottomTab; label: string; enabled: boolean }[] = [
  { key: 'sql', label: '⌨ SQL Editor', enabled: true },
  { key: 'chat', label: '🤖 AI Chat', enabled: true },
]
```

- [ ] **Step 2: Route bottom 'chat' to ChatPanel in `Workspace.tsx`**

In the central-area JSX, alongside the existing `bottom === 'sql'` block, add:

```tsx
{connId != null && bottom === 'chat' && (
  <ChatPanel database={selected?.db} />
)}
```

And update the "pick a table" hint to also exclude chat:

```tsx
{connId != null && selected == null && bottom !== 'sql' && bottom !== 'chat' && (
  <div style={center}>pick a table in the sidebar</div>
)}
```

Add the `ChatPanel` import at the top.

- [ ] **Step 3: Build + commit**

```bash
cd web && npm run build && cd ..
git add web/src/components/BottomTabs.tsx web/src/routes/Workspace.tsx
git commit -m "feat(web): enable AI Chat tab — route bottom 'chat' to ChatPanel"
```

---

## Task 12: README addendum

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Insert Plan 5 section**

After "## What's in this plan (Plan 4)":

```markdown
## What's in this plan (Plan 5)

- WebSocket `/ws/chat` — LLM-orchestrated chat with tool calls
- LLM providers: Anthropic Messages API + OpenAI Chat Completions (switch via `MYSQLWEB_LLM_DEFAULT` or per-message `provider` field)
- Built-in tools: `list_databases`, `list_tables`, `describe_table`, `query_table`, `run_sql` (read-only — the model is instructed to refuse DML/DDL)
- Frontend: ChatPanel in the right-group AI Chat tab — streamed text, tool-call call/result expandable blocks, clear-history button
```

Add the env requirements:

```markdown
### Env vars (Plan 5)

| Variable | Required? | Notes |
|---|---|---|
| `ANTHROPIC_API_KEY` | one of these two | enables Anthropic |
| `OPENAI_API_KEY` | one of these two | enables OpenAI |
| `MYSQLWEB_LLM_DEFAULT` | no | `anthropic` (default) or `openai` |

Without an API key the chat tab still renders but every request returns an error.
```

Append the manual smoke:

```markdown
### Manual chat smoke (Plan 5)

1. `export ANTHROPIC_API_KEY=sk-ant-...` (or `OPENAI_API_KEY=sk-...`) before starting dataseai.
2. Open the workspace, pick a connection, expand a database in the sidebar.
3. Click `🤖 AI Chat` in the right group.
4. Type: "list the databases I can see". Expect a tool call (`list_databases`) followed by a short summary.
5. Type: "show me the columns of the users table in the demo db". Expect `describe_table` then a column list.
6. Type: "select the first 5 rows from demo.users". Expect a `query_table` or `run_sql` call and the rows summarised.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README addendum for Plan 5 (LLM chat + env vars)"
```

---

## Task 13 (optional, deferred): MCP HTTP client + askdba integration

(Skipped for now per direct-tools choice. Stub paragraph in README + a tracking note in the design spec.)

---

## Plan 5 Done — milestone

After Task 12 the AI Chat right-group tab is live: pick a connection, open chat, ask in natural language, the LLM orchestrates calls to the same MySQL tools the spec described (just executed directly from the Go backend instead of routed through an external MCP server).

Total: 12 commits expected.

**Plan 5 deviation from spec:** No external mcp-mysql sidecar; tools are dispatched in-process via `internal/chat/execute.go`. If you need real MCP compatibility later (so e.g. Claude Desktop can also connect to your DB), a follow-up "Plan 6" can add `internal/mcp/client.go` + wire `chat.Execute` through it; the LLM-facing tool surface is identical.

**Backlog still open:** browse-handler raw-error 500 (Plan 2 carryover), spec query-param drift, middleware 5s cache, CodeMirror bundle size.
