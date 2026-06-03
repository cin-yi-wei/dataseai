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
