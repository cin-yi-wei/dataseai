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
	msg, retry, quota := parseProviderError(body)

	switch {
	case status == 429:
		var base string
		switch {
		case quota.Period != "" && quota.Value != "":
			model := quota.Model
			if model == "" {
				model = "free tier"
			}
			base = fmt.Sprintf("%s %s %s免費額度已達上限（%s 次/%s）。",
				provider, model, quota.Period, quota.Value, strings.TrimPrefix(quota.Period, "每"))
		default:
			base = fmt.Sprintf("%s API 額度已達上限（HTTP 429）。", provider)
		}
		if retry != "" {
			base += fmt.Sprintf(" 請於 %s 後再試", retry)
		} else if quota.Period == "每日" {
			base += " 明天 UTC 0:00 後重置"
		}
		base += "；長期建議在 Settings 設定付費 API key。"
		if msg != "" {
			base += "原始訊息：" + truncate(msg, 160)
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

// quotaInfo describes a single Gemini QuotaFailure violation.
type quotaInfo struct {
	Period string // "每分鐘" | "每小時" | "每日" | "" (unknown)
	Value  string // e.g. "20"
	Model  string // e.g. "gemini-2.5-flash"
}

// parseProviderError best-effort pulls (message, retryAfter, quota) from any
// of the three providers' error envelopes:
//
//	Google Gemini : {"error":{"code":429,"message":"...", "details":[
//	                 {"@type":".../QuotaFailure","violations":[{"quotaId":"...PerDay...","quotaValue":"20","quotaDimensions":{"model":"gemini-2.5-flash"}}]},
//	                 {"@type":".../RetryInfo","retryDelay":"33s"}
//	               ]}}
//	Anthropic     : {"type":"error","error":{"type":"...","message":"..."}}
//	OpenAI        : {"error":{"message":"...","type":"...","code":"..."}}
func parseProviderError(body []byte) (message, retryAfter string, quota quotaInfo) {
	var env struct {
		Error struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
			Type    string `json:"type"`
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
				Violations []struct {
					QuotaID         string            `json:"quotaId"`
					QuotaValue      string            `json:"quotaValue"`
					QuotaDimensions map[string]string `json:"quotaDimensions"`
				} `json:"violations"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return
	}
	message = env.Error.Message
	for _, d := range env.Error.Details {
		if strings.Contains(d.Type, "RetryInfo") && d.RetryDelay != "" {
			retryAfter = d.RetryDelay
		}
		if strings.Contains(d.Type, "QuotaFailure") && len(d.Violations) > 0 {
			v := d.Violations[0]
			quota.Value = v.QuotaValue
			quota.Period = quotaPeriodFromID(v.QuotaID)
			if v.QuotaDimensions != nil {
				quota.Model = v.QuotaDimensions["model"]
			}
		}
	}
	return
}

// quotaPeriodFromID maps Gemini's quotaId substrings to a zh-TW period label.
//
//	"GenerateRequestsPerDayPerProjectPerModel-FreeTier"    → 每日
//	"GenerateRequestsPerMinutePerProjectPerModel-FreeTier" → 每分鐘
//	"GenerateContentInputTokensPerHourPerProject..."       → 每小時
func quotaPeriodFromID(id string) string {
	switch {
	case strings.Contains(id, "PerDay"):
		return "每日"
	case strings.Contains(id, "PerMinute"):
		return "每分鐘"
	case strings.Contains(id, "PerHour"):
		return "每小時"
	}
	return ""
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
