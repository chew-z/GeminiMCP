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

func TestParseHTTPPublicURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty input is allowed", input: "", want: ""},
		{name: "https no path", input: "https://example.com", want: "https://example.com"},
		{name: "https with path", input: "https://example.com/gemini", want: "https://example.com/gemini"},
		{name: "trailing slash trimmed", input: "https://example.com/gemini/", want: "https://example.com/gemini"},
		{name: "loopback localhost http", input: "http://localhost:8080", want: "http://localhost:8080"},
		{name: "loopback 127.0.0.1 http", input: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{name: "loopback ipv6 http", input: "http://[::1]:8080", want: "http://[::1]:8080"},
		{name: "non-loopback http rejected", input: "http://example.com", wantErr: true},
		{name: "non-loopback http with path rejected", input: "http://example.com/gemini", wantErr: true},
		{name: "ftp scheme rejected", input: "ftp://example.com", wantErr: true},
		{name: "missing scheme rejected", input: "example.com/gemini", wantErr: true},
		{name: "empty host rejected", input: "https://", wantErr: true},
		{name: "malformed url rejected", input: "://example.com", wantErr: true},
		{name: "query rejected", input: "https://example.com?foo=bar", wantErr: true},
		{name: "fragment rejected", input: "https://example.com#frag", wantErr: true},
		{name: "whitespace trimmed", input: "   https://example.com   ", want: "https://example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHTTPPublicURL(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "GEMINI_HTTP_PUBLIC_URL")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
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
				assert.Equal(t, 360*time.Second, cfg.HTTPWriteTimeout, "default HTTPWriteTimeout = HTTPTimeout + 60s slack")
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
				assert.Equal(t, 180*time.Second, cfg.HTTPWriteTimeout, "HTTPWriteTimeout follows HTTPTimeout + 60s when unset")
				assert.Equal(t, 5, cfg.MaxRetries)
				assert.Equal(t, 2*time.Second, cfg.InitialBackoff)
				assert.Equal(t, 20*time.Second, cfg.MaxBackoff)
			},
		},
		{
			name: "explicit GEMINI_HTTP_WRITE_TIMEOUT override",
			env: map[string]string{
				"GEMINI_API_KEY":            "key",
				"GEMINI_TIMEOUT":            "30s",
				"GEMINI_HTTP_WRITE_TIMEOUT": "5m",
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 30*time.Second, cfg.HTTPTimeout)
				assert.Equal(t, 5*time.Minute, cfg.HTTPWriteTimeout, "explicit value wins over the default formula")
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
		{
			name: "http public URL stored verbatim when valid",
			env: map[string]string{
				"GEMINI_API_KEY":         "key",
				"GEMINI_HTTP_PUBLIC_URL": "https://example.test/gemini",
			},
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "https://example.test/gemini", cfg.HTTPPublicURL)
			},
		},
		{
			name: "trust forwarded proto opt-in",
			env: map[string]string{
				"GEMINI_API_KEY":                    "key",
				"GEMINI_HTTP_TRUST_FORWARDED_PROTO": "true",
			},
			check: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.HTTPTrustForwardedProto)
			},
		},
		{
			name: "invalid http public URL fails NewConfig",
			env: map[string]string{
				"GEMINI_API_KEY":         "key",
				"GEMINI_HTTP_PUBLIC_URL": "http://example.com",
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
