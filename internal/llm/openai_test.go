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
