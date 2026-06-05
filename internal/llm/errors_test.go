package llm

import (
	"strings"
	"testing"
)

func TestFriendlyHTTPErrorGemini429PerDay(t *testing.T) {
	body := []byte(`{"error":{"code":429,"message":"You exceeded your current quota","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaMetric":"x","quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier","quotaDimensions":{"location":"global","model":"gemini-2.5-flash"},"quotaValue":"20"}]},{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"33s"}]}}`)
	err := friendlyHTTPError("Gemini", 429, body)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{"每日", "20 次/日", "gemini-2.5-flash", "33s"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in: %s", want, msg)
		}
	}
	if strings.Contains(msg, `"@type"`) {
		t.Errorf("raw JSON leaked: %s", msg)
	}
}

func TestFriendlyHTTPErrorGemini429PerMinute(t *testing.T) {
	body := []byte(`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerMinutePerProjectPerModel-FreeTier","quotaDimensions":{"model":"gemini-2.5-flash"},"quotaValue":"10"}]}]}}`)
	err := friendlyHTTPError("Gemini", 429, body)
	msg := err.Error()
	if !strings.Contains(msg, "每分鐘") || !strings.Contains(msg, "10 次/分鐘") {
		t.Errorf("expected per-minute phrasing: %s", msg)
	}
}

func TestFriendlyHTTPErrorGemini429Generic(t *testing.T) {
	body := []byte(`{"error":{"code":429,"message":"quota exceeded"}}`)
	err := friendlyHTTPError("Gemini", 429, body)
	msg := err.Error()
	if !strings.Contains(msg, "額度已達上限") {
		t.Errorf("missing generic 429 hint: %s", msg)
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
	_, retry, _ := parseProviderError(body)
	if retry != "7s" {
		t.Errorf("retry=%q want 7s", retry)
	}
}

func TestParseProviderErrorQuota(t *testing.T) {
	body := []byte(`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier","quotaValue":"20","quotaDimensions":{"model":"gemini-2.5-flash"}}]}]}}`)
	_, _, q := parseProviderError(body)
	if q.Period != "每日" || q.Value != "20" || q.Model != "gemini-2.5-flash" {
		t.Errorf("quota=%+v", q)
	}
}
