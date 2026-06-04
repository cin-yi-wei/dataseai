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
	default:
		return nil, fmt.Errorf("unknown provider %q (expected anthropic|openai|gemini)", provider)
	}
}
