package main

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupEnv is a test helper to set environment variables for a test and
// clean them up afterward.
func setupEnv(t *testing.T, env map[string]string) {
	t.Helper()
	originalEnv := make(map[string]string)

	for key, value := range env {
		originalValue, wasSet := os.LookupEnv(key)
		if wasSet {
			originalEnv[key] = originalValue
		} else {
			originalEnv[key] = "" // Mark for unsetting
		}
		os.Setenv(key, value)
	}

	t.Cleanup(func() {
		for key := range env {
			originalValue, wasSet := originalEnv[key]
			if wasSet && originalValue != "" {
				os.Setenv(key, originalValue)
			} else {
				os.Unsetenv(key)
			}
		}
	})
}

func TestNewConfig(t *testing.T) {
	logger := NewLogger(LevelDebug)

	testCases := []struct {
		name      string
		env       map[string]string
		expectErr bool
		check     func(t *testing.T, cfg *Config)
	}{
		{
			name:      "error on missing API key",
			env:       map[string]string{"GEMINI_API_KEY": ""},
			expectErr: true,
		},
		{
			name: "defaults are set correctly",
			env:  map[string]string{"GEMINI_API_KEY": "test-key"},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "test-key", cfg.GeminiAPIKey)
				assert.Equal(t, defaultGeminiModel, cfg.GeminiModel)
				assert.Equal(t, 90*time.Second, cfg.HTTPTimeout)
				assert.Equal(t, 2, cfg.MaxRetries)
				assert.True(t, cfg.EnableCaching)
				assert.Equal(t, defaultThinkingBudgetLevel, cfg.ThinkingBudgetLevel)
				assert.Equal(t, getThinkingBudgetFromLevel(defaultThinkingBudgetLevel), cfg.ThinkingBudget)
			},
		},
		{
			name: "custom values override defaults",
			env: map[string]string{
				"GEMINI_API_KEY":           "custom-key",
				"GEMINI_MODEL":             "gemini-1.5-pro",
				"GEMINI_TIMEOUT":           "120s",
				"GEMINI_MAX_RETRIES":       "5",
				"GEMINI_INITIAL_BACKOFF":   "2s",
				"GEMINI_MAX_BACKOFF":       "20s",
				"GEMINI_ENABLE_CACHING":    "false",
				"GEMINI_DEFAULT_CACHE_TTL": "1h",
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "custom-key", cfg.GeminiAPIKey)
				assert.Equal(t, "gemini-1.5-pro", cfg.GeminiModel)
				assert.Equal(t, 120*time.Second, cfg.HTTPTimeout)
				assert.Equal(t, 5, cfg.MaxRetries)
				assert.Equal(t, 2*time.Second, cfg.InitialBackoff)
				assert.Equal(t, 20*time.Second, cfg.MaxBackoff)
				assert.False(t, cfg.EnableCaching)
				assert.Equal(t, 1*time.Hour, cfg.DefaultCacheTTL)
			},
		},
		{
			name:      "invalid temperature > 1.0",
			env:       map[string]string{"GEMINI_API_KEY": "key", "GEMINI_TEMPERATURE": "1.5"},
			expectErr: true,
		},
		{
			name:      "invalid temperature < 0.0",
			env:       map[string]string{"GEMINI_API_KEY": "key", "GEMINI_TEMPERATURE": "-0.5"},
			expectErr: true,
		},
		{
			name: "valid temperature",
			env:  map[string]string{"GEMINI_API_KEY": "key", "GEMINI_TEMPERATURE": "0.8"},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 0.8, cfg.GeminiTemperature)
			},
		},
		{
			name: "custom file settings",
			env: map[string]string{
				"GEMINI_API_KEY":            "key",
				"GEMINI_MAX_FILE_SIZE":      "1048576", // 1 MB
				"GEMINI_ALLOWED_FILE_TYPES": "text/plain,application/pdf",
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, int64(1048576), cfg.MaxFileSize)
				assert.Equal(t, []string{"text/plain", "application/pdf"}, cfg.AllowedFileTypes)
			},
		},
		{
			name: "custom thinking settings",
			env: map[string]string{
				"GEMINI_API_KEY":               "key",
				"GEMINI_THINKING_BUDGET_LEVEL": "high",
				"GEMINI_THINKING_BUDGET":       "9999", // Explicit budget overrides level
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "high", cfg.ThinkingBudgetLevel)
				assert.Equal(t, 9999, cfg.ThinkingBudget)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use a clean environment for each test case
			os.Clearenv()
			setupEnv(t, tc.env)

			config, err := NewConfig(logger)

			if tc.expectErr {
				require.Error(t, err)
				assert.Nil(t, config)
			} else {
				require.NoError(t, err)
				require.NotNil(t, config)
				if tc.check != nil {
					tc.check(t, config)
				}
			}
		})
	}
}
