package main

import (
	"os"
	"strings"
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
	wasSetMap := make(map[string]bool)

	for key, value := range env {
		originalValue, wasSet := os.LookupEnv(key)
		if wasSet {
			originalEnv[key] = originalValue
		}
		wasSetMap[key] = wasSet
		require.NoError(t, os.Setenv(key, value))
	}

	t.Cleanup(func() {
		for key := range env {
			if wasSetMap[key] {
				require.NoError(t, os.Setenv(key, originalEnv[key]))
			} else {
				require.NoError(t, os.Unsetenv(key))
			}
		}
	})
}

func withCleanEnv(t *testing.T) {
	t.Helper()

	original := append([]string(nil), os.Environ()...)
	os.Clearenv()

	t.Cleanup(func() {
		os.Clearenv()
		for _, kv := range original {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				continue
			}
			require.NoError(t, os.Setenv(parts[0], parts[1]))
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
				assert.Equal(t, 300*time.Second, cfg.HTTPTimeout)
				assert.Equal(t, 2, cfg.MaxRetries)
				assert.Equal(t, defaultThinkingLevel, cfg.ThinkingLevel)
			},
		},
		{
			name: "custom values override defaults",
			env: map[string]string{
				"GEMINI_API_KEY":         "custom-key",
				"GEMINI_MODEL":           "gemini-1.5-pro",
				"GEMINI_TIMEOUT":         "120s",
				"GEMINI_MAX_RETRIES":     "5",
				"GEMINI_INITIAL_BACKOFF": "2s",
				"GEMINI_MAX_BACKOFF":     "20s",
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "custom-key", cfg.GeminiAPIKey)
				assert.Equal(t, "gemini-1.5-pro", cfg.GeminiModel)
				assert.Equal(t, 120*time.Second, cfg.HTTPTimeout)
				assert.Equal(t, 5, cfg.MaxRetries)
				assert.Equal(t, 2*time.Second, cfg.InitialBackoff)
				assert.Equal(t, 20*time.Second, cfg.MaxBackoff)
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
			name: "custom thinking settings",
			env: map[string]string{
				"GEMINI_API_KEY":        "key",
				"GEMINI_THINKING_LEVEL": "low",
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "low", cfg.ThinkingLevel)
			},
		},
		{
			name: "custom service tier",
			env: map[string]string{
				"GEMINI_API_KEY":      "key",
				"GEMINI_SERVICE_TIER": "priority",
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "priority", cfg.ServiceTier)
			},
		},
		{
			name: "invalid service tier falls back to default",
			env: map[string]string{
				"GEMINI_API_KEY":      "key",
				"GEMINI_SERVICE_TIER": "invalid-tier",
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, defaultServiceTier, cfg.ServiceTier)
			},
		},
		{
			name: "auth enabled requires secret key",
			env: map[string]string{
				"GEMINI_API_KEY":      "key",
				"GEMINI_AUTH_ENABLED": "true",
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use a clean environment for each test case and restore process env after it.
			withCleanEnv(t)
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
