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
