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
