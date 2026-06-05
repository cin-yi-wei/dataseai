package llm

import (
	"strings"
	"testing"
)

func TestFriendlyHTTPErrorGemini429(t *testing.T) {
	body := []byte(`{"error":{"code":429,"message":"You exceeded your current quota","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"33s"}]}}`)
	err := friendlyHTTPError("Gemini", 429, body)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "額度已達上限") {
		t.Errorf("missing quota hint: %s", msg)
	}
	if !strings.Contains(msg, "33s") {
		t.Errorf("missing retry hint: %s", msg)
	}
	if strings.Contains(msg, `"@type"`) {
		t.Errorf("raw JSON leaked: %s", msg)
	}
}

func TestFriendlyHTTPErrorAnthropic401(t *testing.T) {
	body := []byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	err := friendlyHTTPError("Anthropic", 401, body)
	msg := err.Error()
	if !strings.Contains(msg, "API key 無效") {
		t.Errorf("missing auth hint: %s", msg)
	}
	if !strings.Contains(msg, "invalid x-api-key") {
		t.Errorf("expected provider message: %s", msg)
	}
}

func TestFriendlyHTTPErrorOpenAI400(t *testing.T) {
	body := []byte(`{"error":{"message":"model 'gpt-9' is unknown","type":"invalid_request_error","code":"model_not_found"}}`)
	err := friendlyHTTPError("OpenAI", 400, body)
	msg := err.Error()
	if !strings.Contains(msg, "請求格式錯誤") {
		t.Errorf("missing 400 hint: %s", msg)
	}
	if !strings.Contains(msg, "model 'gpt-9' is unknown") {
		t.Errorf("expected provider message: %s", msg)
	}
}

func TestFriendlyHTTPErrorUnstructuredBody(t *testing.T) {
	body := []byte(`<html>upstream timeout</html>`)
	err := friendlyHTTPError("Gemini", 504, body)
	msg := err.Error()
	if !strings.Contains(msg, "服務端錯誤") {
		t.Errorf("expected 5xx hint: %s", msg)
	}
}

func TestParseProviderErrorRetryDelay(t *testing.T) {
	body := []byte(`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"7s"}]}}`)
	_, retry := parseProviderError(body)
	if retry != "7s" {
		t.Errorf("retry=%q want 7s", retry)
	}
}
