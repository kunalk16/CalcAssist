package llm

import (
	"fmt"
	"net/http"
	"time"
)

// New creates a Provider for cfg.Provider.
func New(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAI(cfg), nil
	case "azure":
		return NewAzure(cfg), nil
	case "anthropic":
		return NewAnthropic(cfg), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}
