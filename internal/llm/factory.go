package llm

import (
	"fmt"
	"net/http"
	"time"
)

type Config struct {
	Default         string // "anthropic" | "openai" | "gemini"
	AnthropicAPIKey string
	OpenAIAPIKey    string
	GeminiAPIKey    string
	AnthropicModel  string
	OpenAIModel     string
	GeminiModel     string
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
		// Use the local Claude Code OAuth token instead of an API key. Billing
		// rides on the user's Claude Pro/Max/Team subscription.
		tok, err := LoadClaudeCodeToken()
		if err != nil {
			return nil, fmt.Errorf("claude code: %w", err)
		}
		model := cfg.AnthropicModel
		if model == "" {
			// Haiku is the safest default: lower rate-limit pressure on the
			// shared Claude Code subscription pool. Users can override via
			// MYSQLWEB_ANTHROPIC_MODEL or the future per-user model setting.
			model = "claude-haiku-4-5"
		}
		return &Anthropic{APIKey: tok, Model: model, Client: httpClient, OAuth: true}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (expected anthropic|openai|gemini|claudecode)", provider)
	}
}
