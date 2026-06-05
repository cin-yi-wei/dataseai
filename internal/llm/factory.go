package llm

import (
	"fmt"
	"net/http"
	"time"
)

type Config struct {
	Default         string // "anthropic" | "openai" | "gemini" | "claudecode" | "codex"
	AnthropicAPIKey string
	OpenAIAPIKey    string
	GeminiAPIKey    string
	ClaudeCodeToken string // OAuth access token; per-user override comes through chat.go
	CodexToken      string // Codex / ChatGPT subscription OAuth access token
	CodexAccountID  string // chatgpt_account_id derived from the JWT
	AnthropicModel  string
	OpenAIModel     string
	GeminiModel     string
	CodexModel      string
}

// Pick returns the configured client. provider == "" → Default.
func Pick(cfg Config, provider string) (LLMClient, error) {
	if provider == "" {
		provider = cfg.Default
	}
	httpClient := &http.Client{Timeout: 5 * time.Minute}
	switch provider {
	case "anthropic":
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("anthropic api key not set")
		}
		return &Anthropic{APIKey: cfg.AnthropicAPIKey, Model: cfg.AnthropicModel, Client: httpClient}, nil
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("openai api key not set")
		}
		return &OpenAI{APIKey: cfg.OpenAIAPIKey, Model: cfg.OpenAIModel, Client: httpClient}, nil
	case "gemini":
		if cfg.GeminiAPIKey == "" {
			return nil, fmt.Errorf("gemini api key not set")
		}
		return &Gemini{APIKey: cfg.GeminiAPIKey, Model: cfg.GeminiModel, Client: httpClient}, nil
	case "claudecode":
		// Per-user OAuth token from Settings overrides the local file. Only
		// fall back to the server's ~/.claude/.credentials.json if no user
		// token was configured.
		tok := cfg.ClaudeCodeToken
		if tok == "" {
			t, err := LoadClaudeCodeToken()
			if err != nil {
				return nil, fmt.Errorf("claude code: %w (set a token in Settings → API keys, or run `claude` on the server)", err)
			}
			tok = t
		}
		model := cfg.AnthropicModel
		if model == "" {
			// Haiku has the most permissive rate limit on Claude Code
			// subscriptions; users can override per-installation via
			// MYSQLWEB_ANTHROPIC_MODEL.
			model = "claude-haiku-4-5"
		}
		return &Anthropic{APIKey: tok, Model: model, Client: httpClient, OAuth: true}, nil
	case "codex":
		if cfg.CodexToken == "" {
			return nil, fmt.Errorf("codex: not connected (Settings → API keys → ChatGPT)")
		}
		acct := cfg.CodexAccountID
		if acct == "" {
			if a, err := ExtractCodexAccountID(cfg.CodexToken); err == nil {
				acct = a
			}
		}
		model := cfg.CodexModel
		if model == "" {
			model = "gpt-5.5"
		}
		return &Codex{
			AccessToken: cfg.CodexToken,
			AccountID:   acct,
			Model:       model,
			Client:      httpClient,
		}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (expected anthropic|openai|gemini|claudecode|codex)", provider)
	}
}
