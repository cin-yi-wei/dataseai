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

// Gemini implements LLMClient using Google's Generative Language API.
// Streams via the streamGenerateContent endpoint with SSE.
type Gemini struct {
	APIKey  string
	Model   string // default "gemini-2.0-flash"
	BaseURL string // default "https://generativelanguage.googleapis.com/v1beta"
	Client  *http.Client
}

func (g *Gemini) Stream(ctx context.Context, req StreamRequest) (<-chan Event, error) {
	model := req.Model
	if model == "" {
		model = g.Model
	}
	if model == "" {
		model = "gemini-2.0-flash"
	}
	base := g.BaseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	httpClient := g.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	body := map[string]any{
		"contents": geminiContents(req.Messages),
	}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": req.System}},
		}
	}
	if len(req.Tools) > 0 {
		decls := make([]map[string]any, len(req.Tools))
		for i, t := range req.Tools {
			decls[i] = map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  cleanSchemaForGemini(t.InputSchema),
			}
		}
		body["tools"] = []map[string]any{{"functionDeclarations": decls}}
	}

	bs, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", base, model, g.APIKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gemini: HTTP %d: %s", resp.StatusCode, string(body))
	}
	return streamFrom(parseGeminiSSE(resp.Body), resp.Body), nil
}

// geminiContents converts our Message format to Gemini's contents array.
// Roles: "user" → "user", "assistant" → "model", "tool" → "user" with functionResponse.
func geminiContents(in []Message) []map[string]any {
	out := []map[string]any{}
	for _, m := range in {
		switch m.Role {
		case "tool":
			parts := []map[string]any{}
			for _, c := range m.Content {
				if c.Type == "tool_result" {
					var response any
					if err := json.Unmarshal([]byte(c.Output), &response); err != nil {
						response = map[string]any{"result": c.Output}
					}
					parts = append(parts, map[string]any{
						"functionResponse": map[string]any{
							"name":     toolNameForResult(c.ToolUseID, in),
							"response": map[string]any{"result": response},
						},
					})
				}
			}
			if len(parts) > 0 {
				out = append(out, map[string]any{"role": "user", "parts": parts})
			}
		case "assistant":
			parts := []map[string]any{}
			for _, c := range m.Content {
				switch c.Type {
				case "text":
					if c.Text != "" {
						parts = append(parts, map[string]any{"text": c.Text})
					}
				case "tool_use":
					parts = append(parts, map[string]any{
						"functionCall": map[string]any{
							"name": c.Name,
							"args": c.Input,
						},
					})
				}
			}
			if len(parts) > 0 {
				out = append(out, map[string]any{"role": "model", "parts": parts})
			}
		default: // user
			parts := []map[string]any{}
			for _, c := range m.Content {
				if c.Type == "text" && c.Text != "" {
					parts = append(parts, map[string]any{"text": c.Text})
				}
			}
			if len(parts) > 0 {
				out = append(out, map[string]any{"role": "user", "parts": parts})
			}
		}
	}
	return out
}

// toolNameForResult finds the tool name that produced this tool_use_id by
// scanning prior assistant messages.
func toolNameForResult(toolUseID string, msgs []Message) string {
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		for _, c := range m.Content {
			if c.Type == "tool_use" && c.ID == toolUseID {
				return c.Name
			}
		}
	}
	return ""
}

// cleanSchemaForGemini removes JSON Schema fields Gemini rejects.
// Gemini accepts: type, properties, required, items, enum, description, format, nullable.
func cleanSchemaForGemini(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	allowed := map[string]bool{
		"type": true, "properties": true, "required": true,
		"items": true, "enum": true, "description": true,
		"format": true, "nullable": true,
	}
	out := map[string]any{}
	for k, v := range schema {
		if !allowed[k] {
			continue
		}
		switch typed := v.(type) {
		case map[string]any:
			// Special case: "properties" is a map of name -> schema, so recurse on each entry.
			if k == "properties" {
				cleaned := map[string]any{}
				for name, propV := range typed {
					if propMap, ok := propV.(map[string]any); ok {
						cleaned[name] = cleanSchemaForGemini(propMap)
					} else {
						cleaned[name] = propV
					}
				}
				out[k] = cleaned
			} else {
				out[k] = cleanSchemaForGemini(typed)
			}
		case []any:
			out[k] = typed
		default:
			out[k] = v
		}
	}
	return out
}

// parseGeminiSSE handles Gemini's SSE format. Each event is `data: {json}`.
// Gemini doesn't generate tool_use_ids; we synthesize them from name+index.
func parseGeminiSSE(r io.Reader) <-chan Event {
	out := make(chan Event, 16)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		toolCallIdx := 0
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(line[len("data:"):])
			if data == "" {
				continue
			}
			var msg struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text         string         `json:"text,omitempty"`
							FunctionCall *functionCall  `json:"functionCall,omitempty"`
						} `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason,omitempty"`
				} `json:"candidates"`
			}
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				continue
			}
			if len(msg.Candidates) == 0 {
				continue
			}
			cand := msg.Candidates[0]
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					out <- Event{Type: EventText, Text: part.Text}
				}
				if part.FunctionCall != nil {
					toolCallIdx++
					id := fmt.Sprintf("call_%d_%s", toolCallIdx, part.FunctionCall.Name)
					out <- Event{
						Type:      EventToolUse,
						ToolUseID: id,
						ToolName:  part.FunctionCall.Name,
						ToolInput: part.FunctionCall.Args,
					}
				}
			}
			if cand.FinishReason != "" && cand.FinishReason != "FINISH_REASON_UNSPECIFIED" {
				out <- Event{Type: EventDone}
				return
			}
		}
		out <- Event{Type: EventDone}
	}()
	return out
}

type functionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}
