package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config contains runtime settings for calcassist.
type Config struct {
	Provider          string  `yaml:"provider"` // azure | openai | anthropic
	Model             string  `yaml:"model"`
	APIKey            string  `yaml:"api_key"`
	BaseURL           string  `yaml:"base_url"`
	MaxTokens         int     `yaml:"max_tokens"`
	Temperature       float64 `yaml:"temperature"`
	MaxToolIterations int     `yaml:"max_tool_iterations"`
}

// DefaultPath returns the standard calcassist configuration path.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home directory: %w", err)
	}

	return filepath.Join(home, ".calcassist", "config.yaml"), nil
}

// Load reads, overlays, defaults, and validates a calcassist YAML configuration.
func Load(path string) (*Config, error) {
	if path == "" {
		defaultPath, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = defaultPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file %q does not exist; create it first (see docs/README): %w", path, err)
		}
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}

	if err := cfg.applyEnvOverrides(); err != nil {
		return nil, err
	}
	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) applyEnvOverrides() error {
	if value := os.Getenv("CALCASSIST_PROVIDER"); value != "" {
		c.Provider = value
	}
	if value := os.Getenv("CALCASSIST_MODEL"); value != "" {
		c.Model = value
	}
	if value := os.Getenv("CALCASSIST_API_KEY"); value != "" {
		c.APIKey = value
	}
	if value := os.Getenv("CALCASSIST_BASE_URL"); value != "" {
		c.BaseURL = value
	}
	if value := os.Getenv("CALCASSIST_MAX_TOKENS"); value != "" {
		maxTokens, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse CALCASSIST_MAX_TOKENS %q: %w", value, err)
		}
		c.MaxTokens = maxTokens
	}
	if value := os.Getenv("CALCASSIST_TEMPERATURE"); value != "" {
		temperature, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("parse CALCASSIST_TEMPERATURE %q: %w", value, err)
		}
		c.Temperature = temperature
	}

	c.applyProviderAPIKeyFallback()

	return nil
}

func (c *Config) applyDefaults() {
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	if c.Provider == "" {
		c.Provider = "openai"
	}

	// A zero temperature is treated as unset and defaults to 0.2; use a small positive value if needed.
	if c.Temperature <= 0 {
		c.Temperature = 0.2
	}
	if c.MaxTokens == 0 {
		c.MaxTokens = 4096
	}
	if c.MaxToolIterations == 0 {
		c.MaxToolIterations = 12
	}
	if c.BaseURL == "" {
		switch c.Provider {
		case "openai":
			c.BaseURL = "https://api.openai.com/v1"
		case "anthropic":
			c.BaseURL = "https://api.anthropic.com"
		}
	}
	c.applyProviderAPIKeyFallback()
}

func (c *Config) applyProviderAPIKeyFallback() {
	if c.APIKey != "" {
		return
	}

	switch strings.ToLower(strings.TrimSpace(c.Provider)) {
	case "openai":
		c.APIKey = os.Getenv("OPENAI_API_KEY")
	case "azure":
		c.APIKey = os.Getenv("AZURE_OPENAI_API_KEY")
	case "anthropic":
		c.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
}

// Validate checks whether the configuration is complete and supported.
func (c *Config) Validate() error {
	switch c.Provider {
	case "azure", "openai", "anthropic":
	default:
		return fmt.Errorf("unsupported provider %q; must be one of azure, openai, anthropic", c.Provider)
	}
	if strings.TrimSpace(c.Model) == "" {
		return errors.New("model is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("api_key is required; set it in config or with an environment variable")
	}
	if c.Provider == "azure" {
		if strings.TrimSpace(c.BaseURL) == "" {
			return errors.New("azure base_url is required and must be e.g. https://<resource>.openai.azure.com/openai/v1")
		}
		if !strings.Contains(c.BaseURL, "/openai/v1") {
			return errors.New("azure base_url must be a v1 base, e.g. https://<resource>.openai.azure.com/openai/v1")
		}
	}

	return nil
}

// Redacted returns a copy safe for display without exposing the API key.
func (c *Config) Redacted() Config {
	copy := *c
	copy.APIKey = redactAPIKey(copy.APIKey)
	return copy
}

// String returns a redacted, human-readable representation of the configuration.
func (c *Config) String() string {
	redacted := c.Redacted()
	return fmt.Sprintf("provider=%s model=%s api_key=%s base_url=%s max_tokens=%d temperature=%g max_tool_iterations=%d",
		redacted.Provider,
		redacted.Model,
		redacted.APIKey,
		redacted.BaseURL,
		redacted.MaxTokens,
		redacted.Temperature,
		redacted.MaxToolIterations,
	)
}

func redactAPIKey(apiKey string) string {
	if apiKey == "" {
		return "<empty>"
	}
	if len(apiKey) <= 3 {
		return "<set>"
	}
	return apiKey[:3] + "..."
}
