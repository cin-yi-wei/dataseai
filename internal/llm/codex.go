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
// shape: an array of message envelopes each carrying typed content blocks.
// Tool_use / tool_result content is flattened to text in this V1 because
// the Codex provider does not yet expose tools.
func codexInputs(in []Message) []map[string]any {
	out := []map[string]any{}
	for _, m := range in {
		role := m.Role
		if role == "tool" {
			role = "user"
		}
		blocks := []map[string]any{}
		blockType := "input_text"
		if role == "assistant" {
			blockType = "output_text"
		}
		for _, c := range m.Content {
			switch c.Type {
			case "text":
				if c.Text != "" {
					blocks = append(blocks, map[string]any{"type": blockType, "text": c.Text})
				}
			case "tool_use":
				// Surface the LLM's tool intent as plain text so the Codex
				// model can at least see what was attempted in prior turns.
				blocks = append(blocks, map[string]any{"type": blockType, "text": "(tool_use: " + c.Name + ")"})
			case "tool_result":
				blocks = append(blocks, map[string]any{"type": "input_text", "text": "(tool_result: " + c.Output + ")"})
			}
		}
		if len(blocks) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"type":    "message",
			"role":    role,
			"content": blocks,
		})
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
