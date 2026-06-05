package llm

// Codex / ChatGPT-subscription LLM client. Talks to the same `responses`
// backend that `codex` CLI does, authenticated with an OAuth access token
// from a logged-in ChatGPT account. Tool / function calls are not yet
// supported in this V1 — text completion only.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type Codex struct {
	AccessToken string
	AccountID   string
	Model       string // default "gpt-5.5"
	BaseURL     string // default "https://chatgpt.com/backend-api/codex"
	Client      *http.Client
}

func (c *Codex) Stream(ctx context.Context, req StreamRequest) (<-chan Event, error) {
	model := req.Model
	if model == "" {
		model = c.Model
	}
	if model == "" {
		model = "gpt-5.5"
	}
	base := c.BaseURL
	if base == "" {
		base = "https://chatgpt.com/backend-api/codex"
	}
	httpClient := c.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	system := req.System
	if system == "" {
		system = "You are a helpful assistant."
	}
	body := map[string]any{
		"model":        model,
		"instructions": system,
		"store":        false,
		"stream":       true,
		"input":        codexInputs(req.Messages),
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]any{
				"type":        "function",
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			}
		}
		body["tools"] = tools
	}

	bs, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", base+"/responses", bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.AccessToken)
	httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
	if c.AccountID != "" {
		httpReq.Header.Set("chatgpt-account-id", c.AccountID)
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, friendlyHTTPError("Codex", resp.StatusCode, body)
	}
	return streamFrom(parseCodexSSE(resp.Body), resp.Body), nil
}

// codexInputs maps the generic Message list to the Responses-API `input`
// shape. Each message becomes an envelope with content blocks (input_text /
// output_text), and tool turns are flattened to top-level `function_call`
// and `function_call_output` items — those are siblings of messages in the
// input array, not nested in their content.
func codexInputs(in []Message) []map[string]any {
	out := []map[string]any{}
	for _, m := range in {
		// Tool role: emit one function_call_output per content item, no message.
		if m.Role == "tool" {
			for _, c := range m.Content {
				if c.Type == "tool_result" {
					out = append(out, map[string]any{
						"type":    "function_call_output",
						"call_id": c.ToolUseID,
						"output":  c.Output,
					})
				}
			}
			continue
		}

		blockType := "input_text"
		if m.Role == "assistant" {
			blockType = "output_text"
		}
		textBlocks := []map[string]any{}
		toolCalls := []map[string]any{}
		for _, c := range m.Content {
			switch c.Type {
			case "text":
				if c.Text != "" {
					textBlocks = append(textBlocks, map[string]any{"type": blockType, "text": c.Text})
				}
			case "tool_use":
				argsJSON, _ := json.Marshal(c.Input)
				toolCalls = append(toolCalls, map[string]any{
					"type":      "function_call",
					"call_id":   c.ID,
					"name":      c.Name,
					"arguments": string(argsJSON),
				})
			}
		}
		if len(textBlocks) > 0 {
			out = append(out, map[string]any{
				"type":    "message",
				"role":    m.Role,
				"content": textBlocks,
			})
		}
		out = append(out, toolCalls...)
	}
	return out
}

// parseCodexSSE reads Responses-API SSE events. The events we care about:
//   - response.output_text.delta — token chunk
//   - response.completed         — end of stream
//   - response.failed / .error   — error surface
func parseCodexSSE(r io.Reader) <-chan Event {
	out := make(chan Event, 16)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		var data string
		flush := func() {
			if data == "" {
				return
			}
			dispatchCodex(out, data)
			data = ""
		}
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				flush()
				continue
			}
			if strings.HasPrefix(line, "data:") {
				data = strings.TrimSpace(line[len("data:"):])
				continue
			}
			// Ignore `event:` lines — `type` field inside the JSON body
			// already gives us the event kind.
		}
		flush()
	}()
	return out
}

func dispatchCodex(out chan<- Event, data string) {
	var msg map[string]any
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return
	}
	t, _ := msg["type"].(string)
	switch t {
	case "response.output_text.delta":
		if delta, _ := msg["delta"].(string); delta != "" {
			out <- Event{Type: EventText, Text: delta}
		}
	case "response.output_item.done":
		// A complete function_call (with finalized arguments) arrives here.
		// Earlier `response.function_call_arguments.delta` events stream the
		// args piecewise but the `done` event ships the full string so we can
		// avoid stitching.
		item, _ := msg["item"].(map[string]any)
		if it, _ := item["type"].(string); it == "function_call" {
			id, _ := item["call_id"].(string)
			name, _ := item["name"].(string)
			argsStr, _ := item["arguments"].(string)
			var argsMap map[string]any
			_ = json.Unmarshal([]byte(argsStr), &argsMap)
			out <- Event{Type: EventToolUse, ToolUseID: id, ToolName: name, ToolInput: argsMap}
		}
	case "response.completed":
		out <- Event{Type: EventDone}
	case "response.failed", "response.error", "error":
		errMsg := "response failed"
		if e, _ := msg["error"].(map[string]any); e != nil {
			if m, _ := e["message"].(string); m != "" {
				errMsg = m
			}
		} else if m, _ := msg["message"].(string); m != "" {
			errMsg = m
		}
		out <- Event{Type: EventError, Message: errMsg}
	}
}

// ExtractCodexAccountID pulls chatgpt_account_id out of the OAuth access
// token's JWT payload (claim path: https://api.openai.com/auth.chatgpt_account_id).
func ExtractCodexAccountID(jwtToken string) (string, error) {
	parts := strings.Split(jwtToken, ".")
	if len(parts) < 2 {
		return "", errors.New("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// some encoders pad — try standard.
		if payload, err = base64.StdEncoding.DecodeString(parts[1]); err != nil {
			return "", err
		}
	}
	var p struct {
		Auth struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", err
	}
	if p.Auth.ChatGPTAccountID == "" {
		return "", errors.New("no chatgpt_account_id claim in JWT")
	}
	return p.Auth.ChatGPTAccountID, nil
}
