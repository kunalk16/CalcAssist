package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidYAML(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `provider: openai
model: gpt-5-mini
api_key: test-key
base_url: https://example.test/v1
max_tokens: 1000
temperature: 0.7
max_tool_iterations: 3
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Provider != "openai" || cfg.Model != "gpt-5-mini" || cfg.APIKey != "test-key" || cfg.BaseURL != "https://example.test/v1" {
		t.Fatalf("Load() config = %+v", cfg)
	}
	if cfg.MaxTokens != 1000 || cfg.Temperature != 0.7 || cfg.MaxToolIterations != 3 {
		t.Fatalf("Load() limits = %+v", cfg)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `provider: openai
model: file-model
api_key: file-key
base_url: https://file.example/v1
max_tokens: 1000
temperature: 0.3
max_tool_iterations: 5
`)
	t.Setenv("CALCASSIST_PROVIDER", "anthropic")
	t.Setenv("CALCASSIST_MODEL", "env-model")
	t.Setenv("CALCASSIST_API_KEY", "env-key")
	t.Setenv("CALCASSIST_BASE_URL", "https://env.example")
	t.Setenv("CALCASSIST_MAX_TOKENS", "2222")
	t.Setenv("CALCASSIST_TEMPERATURE", "0.9")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Provider != "anthropic" || cfg.Model != "env-model" || cfg.APIKey != "env-key" || cfg.BaseURL != "https://env.example" {
		t.Fatalf("Load() config = %+v", cfg)
	}
	if cfg.MaxTokens != 2222 || cfg.Temperature != 0.9 || cfg.MaxToolIterations != 5 {
		t.Fatalf("Load() limits = %+v", cfg)
	}
}

func TestLoadProviderAPIKeyFallback(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		envName  string
	}{
		{name: "openai", provider: "openai", envName: "OPENAI_API_KEY"},
		{name: "azure", provider: "azure", envName: "AZURE_OPENAI_API_KEY"},
		{name: "anthropic", provider: "anthropic", envName: "ANTHROPIC_API_KEY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			path := writeConfig(t, `provider: `+tt.provider+`
model: model-name
base_url: https://example.openai.azure.com/openai/v1
`)
			t.Setenv(tt.envName, tt.name+"-fallback-key")

			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.APIKey != tt.name+"-fallback-key" {
				t.Fatalf("APIKey = %q, want fallback", cfg.APIKey)
			}
		})
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `model: gpt-5-mini
api_key: test-key
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", cfg.Provider)
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.MaxTokens != 4096 {
		t.Fatalf("MaxTokens = %d", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.2 {
		t.Fatalf("Temperature = %v", cfg.Temperature)
	}
	if cfg.MaxToolIterations != 12 {
		t.Fatalf("MaxToolIterations = %d", cfg.MaxToolIterations)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "unknown provider",
			yaml: `provider: wat
model: model
api_key: key
`,
			wantErr: "unsupported provider",
		},
		{
			name: "missing model",
			yaml: `provider: openai
api_key: key
`,
			wantErr: "model is required",
		},
		{
			name: "missing api key",
			yaml: `provider: openai
model: model
`,
			wantErr: "api_key is required",
		},
		{
			name: "azure missing base url",
			yaml: `provider: azure
model: deployment
api_key: key
`,
			wantErr: "azure base_url is required",
		},
		{
			name: "azure improper base url",
			yaml: `provider: azure
model: deployment
api_key: key
base_url: https://example.openai.azure.com/
`,
			wantErr: "azure base_url must be a v1 base",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			path := writeConfig(t, tt.yaml)

			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() error = nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultPath(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}

	wantSuffix := filepath.Join(".calcassist", "config.yaml")
	if !strings.HasSuffix(path, wantSuffix) {
		t.Fatalf("DefaultPath() = %q, want suffix %q", path, wantSuffix)
	}
}

func TestLoadMissingFileErrorMentionsPathAndDocs(t *testing.T) {
	clearEnv(t)
	path := filepath.Join(t.TempDir(), "missing.yaml")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "create") || !strings.Contains(err.Error(), "docs/README") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestRedactedAndStringDoNotExposeAPIKey(t *testing.T) {
	cfg := Config{Provider: "openai", Model: "model", APIKey: "secret-key", BaseURL: "https://api.openai.com/v1", MaxTokens: 1, Temperature: 0.2, MaxToolIterations: 1}

	redacted := cfg.Redacted()
	if redacted.APIKey == cfg.APIKey || !strings.HasPrefix(redacted.APIKey, "sec...") {
		t.Fatalf("Redacted APIKey = %q", redacted.APIKey)
	}
	if strings.Contains(cfg.String(), "secret-key") {
		t.Fatalf("String() exposed API key: %s", cfg.String())
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"CALCASSIST_PROVIDER",
		"CALCASSIST_MODEL",
		"CALCASSIST_API_KEY",
		"CALCASSIST_BASE_URL",
		"CALCASSIST_MAX_TOKENS",
		"CALCASSIST_TEMPERATURE",
		"OPENAI_API_KEY",
		"AZURE_OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
	} {
		t.Setenv(name, "")
	}
}
