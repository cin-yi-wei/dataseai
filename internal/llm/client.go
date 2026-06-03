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
