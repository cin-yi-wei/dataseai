package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port            int
	DBPath          string
	MasterKeyHex    string // empty means "generate on first launch"
	Registration    string // "open" | "closed"
	HistoryMax      int
	QueryTimeoutS   int
	QueryHTTPMaxMB  int
	LLMDefault      string // "anthropic" | "openai" | "gemini"
	AnthropicAPIKey string
	OpenAIAPIKey    string
	GeminiAPIKey    string
	// ForgotPassword enables the self-serve password reset. OFF by default.
	// When a mailer is configured (MailAPIKey+MailFrom) the reset is the
	// email-code flow (Phase 2); when enabled without a mailer it's the
	// unconditional local reset (Phase 1, for the single-user desktop GUI).
	ForgotPassword bool
	// Mail* configure the transactional mailer (Resend). When both key and
	// from are set, email-verified reset is used instead of unconditional.
	MailAPIKey string
	MailFrom   string
}

func Load() (Config, error) {
	c := Config{
		Port:           53306,
		DBPath:         "/data/dataseai.db",
		Registration:   "open",
		HistoryMax:     1000,
		QueryTimeoutS:  5,
		QueryHTTPMaxMB: 10,
		LLMDefault:     "anthropic",
	}
	if v := os.Getenv("MYSQLWEB_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("MYSQLWEB_PORT: %w", err)
		}
		c.Port = p
	}
	if v := os.Getenv("MYSQLWEB_DB_PATH"); v != "" {
		c.DBPath = v
	}
	if v := os.Getenv("MYSQLWEB_MASTER_KEY"); v != "" {
		c.MasterKeyHex = v
	}
	if v := os.Getenv("MYSQLWEB_REGISTRATION"); v != "" {
		c.Registration = v
	}
	if v := os.Getenv("MYSQLWEB_HISTORY_MAX"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("MYSQLWEB_HISTORY_MAX: %w", err)
		}
		c.HistoryMax = n
	}
	if v := os.Getenv("MYSQLWEB_QUERY_TIMEOUT_S"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("MYSQLWEB_QUERY_TIMEOUT_S: %w", err)
		}
		c.QueryTimeoutS = n
	}
	if v := os.Getenv("MYSQLWEB_QUERY_HTTP_MAX_MB"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("MYSQLWEB_QUERY_HTTP_MAX_MB: %w", err)
		}
		c.QueryHTTPMaxMB = n
	}
	if v := os.Getenv("MYSQLWEB_LLM_DEFAULT"); v != "" {
		c.LLMDefault = v
	}
	c.AnthropicAPIKey = os.Getenv("ANTHROPIC_API_KEY")
	c.OpenAIAPIKey = os.Getenv("OPENAI_API_KEY")
	c.GeminiAPIKey = os.Getenv("GEMINI_API_KEY")
	if v := os.Getenv("MYSQLWEB_FORGOT_PASSWORD"); v == "1" || v == "true" {
		c.ForgotPassword = true
	}
	c.MailAPIKey = os.Getenv("MYSQLWEB_MAIL_API_KEY")
	c.MailFrom = os.Getenv("MYSQLWEB_MAIL_FROM")
	return c, nil
}
