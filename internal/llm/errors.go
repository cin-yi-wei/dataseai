package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// friendlyHTTPError converts a raw provider HTTP error body into a short,
// human-readable message suitable for surfacing in chat. The provider's
// own error code (Anthropic / OpenAI / Google) is preserved as a prefix
// so logs still tell us which side failed.
//
// Recognised cases (each provider's body shape):
//   - 429 → rate limit / quota exhausted, with retry hint if present
//   - 401 / 403 → invalid or missing API key
//   - 400 → surfaces the provider's `error.message` only (no JSON dump)
//   - everything else → status + short message (or first 200 bytes of body)
func friendlyHTTPError(provider string, status int, body []byte) error {
	msg, retry := parseProviderError(body)

	switch {
	case status == 429:
		base := fmt.Sprintf("%s API 額度已達上限（HTTP 429）。", provider)
		if retry != "" {
			base += fmt.Sprintf(" 請於 %s 後再試", retry)
		}
		if msg != "" {
			base += "。原始訊息：" + truncate(msg, 200)
		}
		return fmt.Errorf("%s", base)

	case status == 401 || status == 403:
		base := fmt.Sprintf("%s API key 無效或無權限（HTTP %d）。請至 Settings 確認金鑰。", provider, status)
		if msg != "" {
			base += " 原始訊息：" + truncate(msg, 200)
		}
		return fmt.Errorf("%s", base)

	case status == 400:
		if msg != "" {
			return fmt.Errorf("%s 請求格式錯誤（HTTP 400）：%s", provider, truncate(msg, 300))
		}
		return fmt.Errorf("%s 請求格式錯誤（HTTP 400）", provider)

	case status >= 500:
		base := fmt.Sprintf("%s 服務端錯誤（HTTP %d），請稍後再試", provider, status)
		if msg != "" {
			base += "。" + truncate(msg, 200)
		}
		return fmt.Errorf("%s", base)

	default:
		if msg != "" {
			return fmt.Errorf("%s HTTP %d: %s", provider, status, truncate(msg, 300))
		}
		return fmt.Errorf("%s HTTP %d: %s", provider, status, truncate(string(body), 300))
	}
}

// parseProviderError best-effort pulls (message, retryAfter) from any of the
// three providers' error envelopes:
//
//	Google Gemini : {"error":{"code":429,"message":"...", "details":[{"@type":".../RetryInfo","retryDelay":"33s"}, ...]}}
//	Anthropic     : {"type":"error","error":{"type":"...","message":"..."}}
//	OpenAI        : {"error":{"message":"...","type":"...","code":"..."}}
func parseProviderError(body []byte) (message, retryAfter string) {
	var env struct {
		Error struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
			Type    string `json:"type"`
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err == nil {
		message = env.Error.Message
		for _, d := range env.Error.Details {
			if strings.Contains(d.Type, "RetryInfo") && d.RetryDelay != "" {
				retryAfter = d.RetryDelay
				break
			}
		}
	}
	return
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
