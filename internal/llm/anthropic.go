package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type Anthropic struct {
	APIKey  string
	Model   string // default "claude-opus-4-7" — caller can override
	BaseURL string // default "https://api.anthropic.com"
	Client  *http.Client
	// OAuth, when true, marks the credential as a Claude Code OAuth access
	// token (sk-ant-oat01-...). The extra `anthropic-beta: oauth-2025-04-20`
	// header is required by Anthropic for those tokens to hit /v1/messages.
	OAuth bool
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
	if a.OAuth {
		httpReq.Header.Set("anthropic-beta", "oauth-2025-04-20")
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, friendlyHTTPError("Anthropic", resp.StatusCode, body)
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
				// Flush any previously-buffered event before starting a new one
				// (some streams omit the blank-line separator).
				if data != "" {
					dispatch()
					event, data = "", ""
				}
				event = strings.TrimSpace(line[len("event:"):])
				_ = event
				continue
			}
			if strings.HasPrefix(line, "data:") {
				data = strings.TrimSpace(line[len("data:"):])
				continue
			}
		}
		// Flush any trailing event without a closing blank line (test inputs
		// may end immediately after the data line).
		dispatch()
	}()
	return out
}
