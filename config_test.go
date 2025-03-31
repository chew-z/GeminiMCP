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

	t.Run("missing API key returns error", func(t *testing.T) {
		os.Unsetenv("GEMINI_API_KEY")

		config, err := NewConfig()

		if err == nil {
			t.Error("Expected error when API key is missing, got nil")
		}
		if config != nil {
			t.Errorf("Expected nil config when API key is missing, got %+v", config)
		}
	})

	t.Run("valid API key creates config", func(t *testing.T) {
		os.Setenv("GEMINI_API_KEY", "test-api-key")
		os.Setenv("GEMINI_MODEL", "test-model")

		config, err := NewConfig()

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("Expected config to be created, got nil")
		}
		if config.GeminiAPIKey != "test-api-key" {
			t.Errorf("Expected API key 'test-api-key', got '%s'", config.GeminiAPIKey)
		}
		if config.GeminiModel != "test-model" {
			t.Errorf("Expected model 'test-model', got '%s'", config.GeminiModel)
		}
		if config.HTTPTimeout != 90*time.Second {
			t.Errorf("Expected timeout of 90s, got %v", config.HTTPTimeout)
		}
	})

	t.Run("missing model uses default", func(t *testing.T) {
		os.Setenv("GEMINI_API_KEY", "test-api-key")
		os.Unsetenv("GEMINI_MODEL")

		config, err := NewConfig()

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("Expected config to be created, got nil")
		}
		if config.GeminiModel != "gemini-pro" {
			t.Errorf("Expected default model 'gemini-pro', got '%s'", config.GeminiModel)
		}
	})

	t.Run("custom timeout", func(t *testing.T) {
		os.Setenv("GEMINI_API_KEY", "test-api-key")
		os.Setenv("GEMINI_TIMEOUT", "180")

		config, err := NewConfig()

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
		os.Setenv("GEMINI_INITIAL_BACKOFF", "2")
		os.Setenv("GEMINI_MAX_BACKOFF", "15")

		config, err := NewConfig()

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
