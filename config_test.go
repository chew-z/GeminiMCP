package main

import (
	"os"
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	// Save original environment and restore it after the test
	originalAPIKey := os.Getenv("GEMINI_API_KEY")
	originalModel := os.Getenv("GEMINI_MODEL")
	originalTimeout := os.Getenv("GEMINI_TIMEOUT")
	defer func() {
		os.Setenv("GEMINI_API_KEY", originalAPIKey)
		os.Setenv("GEMINI_MODEL", originalModel)
		os.Setenv("GEMINI_TIMEOUT", originalTimeout)
	}()

	logger := NewLogger(LevelInfo)

	t.Run("missing API key returns error", func(t *testing.T) {
		os.Unsetenv("GEMINI_API_KEY")

		config, err := NewConfig(logger)

		if err == nil {
			t.Error("Expected error when API key is missing, got nil")
		}
		if config != nil {
			t.Errorf("Expected nil config when API key is missing, got %+v", config)
		}
	})

	t.Run("valid API key creates config", func(t *testing.T) {
		os.Setenv("GEMINI_API_KEY", "test-api-key")
		os.Setenv("GEMINI_MODEL", "gemini-2.5-pro") // Use a valid model from models.go

		config, err := NewConfig(logger)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("Expected config to be created, got nil")
		}
		if config.GeminiAPIKey != "test-api-key" {
			t.Errorf("Expected API key 'test-api-key', got '%s'", config.GeminiAPIKey)
		}
		if config.GeminiModel != "gemini-2.5-pro" {
			t.Errorf("Expected model 'gemini-2.5-pro', got '%s'", config.GeminiModel)
		}
		if config.HTTPTimeout != 90*time.Second {
			t.Errorf("Expected timeout of 90s, got %v", config.HTTPTimeout)
		}
	})

	t.Run("missing model uses default", func(t *testing.T) {
		os.Setenv("GEMINI_API_KEY", "test-api-key")
		os.Unsetenv("GEMINI_MODEL")

		config, err := NewConfig(logger)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("Expected config to be created, got nil")
		}
		if config.GeminiModel != "gemini-2.5-pro" {
			t.Errorf("Expected default model 'gemini-2.5-pro', got '%s'", config.GeminiModel)
		}
	})

	t.Run("custom timeout", func(t *testing.T) {
		os.Setenv("GEMINI_API_KEY", "test-api-key")
		os.Setenv("GEMINI_TIMEOUT", "180s")

		config, err := NewConfig(logger)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("Expected config to be created, got nil")
		}
		if config.HTTPTimeout != 180*time.Second {
			t.Errorf("Expected timeout of 120s, got %v", config.HTTPTimeout)
		}
	})

	t.Run("custom retry settings", func(t *testing.T) {
		os.Setenv("GEMINI_API_KEY", "test-api-key")
		os.Setenv("GEMINI_MAX_RETRIES", "3")
		os.Setenv("GEMINI_INITIAL_BACKOFF", "2s")
		os.Setenv("GEMINI_MAX_BACKOFF", "15s")

		config, err := NewConfig(logger)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("Expected config to be created, got nil")
		}
		if config.MaxRetries != 3 {
			t.Errorf("Expected max retries of 3, got %d", config.MaxRetries)
		}
		if config.InitialBackoff != 2*time.Second {
			t.Errorf("Expected initial backoff of 2s, got %v", config.InitialBackoff)
		}
		if config.MaxBackoff != 15*time.Second {
			t.Errorf("Expected max backoff of 15s, got %v", config.MaxBackoff)
		}
	})
}
func TestConfigDefaults(t *testing.T) {
	os.Clearenv()
	os.Setenv("GEMINI_API_KEY", "key")
	logger := NewLogger(LevelInfo)
	cfg, err := NewConfig(logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GeminiModel != defaultGeminiModel {
		t.Errorf("default model: want %q, got %q", defaultGeminiModel, cfg.GeminiModel)
	}
	if cfg.GeminiSearchModel != defaultGeminiSearchModel {
		t.Errorf("default search model: want %q, got %q", defaultGeminiSearchModel, cfg.GeminiSearchModel)
	}
	if cfg.GeminiTemperature != defaultGeminiTemperature {
		t.Errorf("default temperature: want %v, got %v", defaultGeminiTemperature, cfg.GeminiTemperature)
	}
	if cfg.HTTPTimeout != 90*time.Second {
		t.Errorf("default timeout: want 90s, got %v", cfg.HTTPTimeout)
	}
	if cfg.MaxRetries != 2 {
		t.Errorf("default max retries: want 2, got %d", cfg.MaxRetries)
	}
	if cfg.InitialBackoff != 1*time.Second {
		t.Errorf("default initial backoff: want 1s, got %v", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != 10*time.Second {
		t.Errorf("default max backoff: want 10s, got %v", cfg.MaxBackoff)
	}
	if cfg.MaxFileSize != defaultMaxFileSize {
		t.Errorf("default max file size: want %d, got %d", defaultMaxFileSize, cfg.MaxFileSize)
	}
	if !cfg.EnableCaching {
		t.Errorf("default caching: want true, got false")
	}
	if cfg.DefaultCacheTTL != defaultDefaultCacheTTL {
		t.Errorf("default cache TTL: want %v, got %v", defaultDefaultCacheTTL, cfg.DefaultCacheTTL)
	}
	if !cfg.EnableThinking {
		t.Errorf("default thinking: want true, got false")
	}
	if cfg.ThinkingBudgetLevel != defaultThinkingBudgetLevel {
		t.Errorf("default thinking level: want %q, got %q", defaultThinkingBudgetLevel, cfg.ThinkingBudgetLevel)
	}
	expBudget := getThinkingBudgetFromLevel(defaultThinkingBudgetLevel)
	if cfg.ThinkingBudget != expBudget {
		t.Errorf("default thinking budget: want %d, got %d", expBudget, cfg.ThinkingBudget)
	}
	if cfg.EnableHTTP {
		t.Errorf("default HTTP: want false, got true")
	}
	if cfg.HTTPAddress != defaultHTTPAddress {
		t.Errorf("default HTTP address: want %q, got %q", defaultHTTPAddress, cfg.HTTPAddress)
	}
	if cfg.HTTPPath != defaultHTTPPath {
		t.Errorf("default HTTP path: want %q, got %q", defaultHTTPPath, cfg.HTTPPath)
	}
	if cfg.HTTPStateless {
		t.Errorf("default HTTP stateless: want false, got true")
	}
	if cfg.HTTPHeartbeat != defaultHTTPHeartbeat {
		t.Errorf("default HTTP heartbeat: want %v, got %v", defaultHTTPHeartbeat, cfg.HTTPHeartbeat)
	}
	if !cfg.HTTPCORSEnabled {
		t.Errorf("default CORS: want true, got false")
	}
	if len(cfg.HTTPCORSOrigins) != 1 || cfg.HTTPCORSOrigins[0] != "*" {
		t.Errorf("default CORS origins: want [\"*\"], got %v", cfg.HTTPCORSOrigins)
	}
}

func TestInvalidTemperature(t *testing.T) {
	os.Clearenv()
	os.Setenv("GEMINI_API_KEY", "key")
	os.Setenv("GEMINI_TEMPERATURE", "1.5")
	logger := NewLogger(LevelInfo)
	_, err := NewConfig(logger)
	if err == nil {
		t.Fatal("expected error for GEMINI_TEMPERATURE > 1.0, got nil")
	}
}

func TestValidTemperature(t *testing.T) {
	os.Clearenv()
	os.Setenv("GEMINI_API_KEY", "key")
	os.Setenv("GEMINI_TEMPERATURE", "0.8")
	logger := NewLogger(LevelInfo)
	cfg, err := NewConfig(logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GeminiTemperature != 0.8 {
		t.Errorf("override temperature: want 0.8, got %v", cfg.GeminiTemperature)
	}
}

func TestFileSettings(t *testing.T) {
	os.Clearenv()
	os.Setenv("GEMINI_API_KEY", "key")
	os.Setenv("GEMINI_MAX_FILE_SIZE", "2097152") // 2 MB
	os.Setenv("GEMINI_ALLOWED_FILE_TYPES", "text/foo,application/bar")
	logger := NewLogger(LevelInfo)
	cfg, err := NewConfig(logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxFileSize != 2097152 {
		t.Errorf("max file size: want 2097152, got %d", cfg.MaxFileSize)
	}
	wantTypes := []string{"text/foo", "application/bar"}
	if len(cfg.AllowedFileTypes) != 2 ||
		cfg.AllowedFileTypes[0] != wantTypes[0] ||
		cfg.AllowedFileTypes[1] != wantTypes[1] {
		t.Errorf("allowed types: want %v, got %v", wantTypes, cfg.AllowedFileTypes)
	}
}

func TestCacheSettings(t *testing.T) {
	os.Clearenv()
	os.Setenv("GEMINI_API_KEY", "key")
	os.Setenv("GEMINI_ENABLE_CACHING", "false")
	os.Setenv("GEMINI_DEFAULT_CACHE_TTL", "30m")
	logger := NewLogger(LevelInfo)
	cfg, err := NewConfig(logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.EnableCaching {
		t.Errorf("enable caching: want false, got true")
	}
	if cfg.DefaultCacheTTL != 30*time.Minute {
		t.Errorf("cache TTL: want 30m, got %v", cfg.DefaultCacheTTL)
	}
}

func TestThinkingSettings(t *testing.T) {
	// override level only
	os.Clearenv()
	os.Setenv("GEMINI_API_KEY", "key")
	os.Setenv("GEMINI_THINKING_BUDGET_LEVEL", "high")
	logger := NewLogger(LevelInfo)
	cfg, err := NewConfig(logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ThinkingBudgetLevel != "high" {
		t.Errorf("thinking level: want high, got %s", cfg.ThinkingBudgetLevel)
	}
	if cfg.ThinkingBudget != getThinkingBudgetFromLevel("high") {
		t.Errorf("thinking budget: want %d, got %d",
			getThinkingBudgetFromLevel("high"), cfg.ThinkingBudget)
	}
	// explicit budget override
	os.Clearenv()
	os.Setenv("GEMINI_API_KEY", "key")
	os.Setenv("GEMINI_THINKING_BUDGET_LEVEL", "medium")
	os.Setenv("GEMINI_THINKING_BUDGET", "1000")
	cfg2, err := NewConfig(logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg2.ThinkingBudget != 1000 {
		t.Errorf("explicit thinking budget: want 1000, got %d", cfg2.ThinkingBudget)
	}
}

func TestHTTPSettings(t *testing.T) {
	os.Clearenv()
	os.Setenv("GEMINI_API_KEY", "key")
	os.Setenv("GEMINI_ENABLE_HTTP", "true")
	os.Setenv("GEMINI_HTTP_ADDRESS", ":9090")
	os.Setenv("GEMINI_HTTP_PATH", "/test")
	os.Setenv("GEMINI_HTTP_STATELESS", "true")
	os.Setenv("GEMINI_HTTP_HEARTBEAT", "5s")
	os.Setenv("GEMINI_HTTP_CORS_ENABLED", "false")
	os.Setenv("GEMINI_HTTP_CORS_ORIGINS", "https://a,https://b")
	logger := NewLogger(LevelInfo)
	cfg, err := NewConfig(logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.EnableHTTP {
		t.Error("enable HTTP: want true, got false")
	}
	if cfg.HTTPAddress != ":9090" {
		t.Errorf("HTTP address: want :9090, got %s", cfg.HTTPAddress)
	}
	if cfg.HTTPPath != "/test" {
		t.Errorf("HTTP path: want /test, got %s", cfg.HTTPPath)
	}
	if !cfg.HTTPStateless {
		t.Error("HTTP stateless: want true, got false")
	}
	if cfg.HTTPHeartbeat != 5*time.Second {
		t.Errorf("HTTP heartbeat: want 5s, got %v", cfg.HTTPHeartbeat)
	}
	if cfg.HTTPCORSEnabled {
		t.Error("CORS enabled: want false, got true")
	}
	if len(cfg.HTTPCORSOrigins) != 2 ||
		cfg.HTTPCORSOrigins[0] != "https://a" ||
		cfg.HTTPCORSOrigins[1] != "https://b" {
		t.Errorf("CORS origins: want [https://a https://b], got %v", cfg.HTTPCORSOrigins)
	}
}