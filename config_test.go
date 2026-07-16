package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHTTPPublicURL(t *testing.T) {
	for _, tt := range []struct {
		input, want string
		wantErr     bool
	}{
		{"", "", false},
		{"https://example.com/gemini/", "https://example.com/gemini", false},
		{"http://localhost:8080", "http://localhost:8080", false},
		{"http://example.com", "", true},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseHTTPPublicURL(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewConfigProviderSelection(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
		vendor  string
	}{
		{"deepseek missing key", map[string]string{"PROVIDER": "deepseek", "PROVIDER_MODEL": "deepseek-v4-pro"}, "PROVIDER_API_KEY", ""},
		{"deepseek missing model", map[string]string{"PROVIDER": "deepseek", "PROVIDER_API_KEY": "key"}, "PROVIDER_MODEL", ""},
		{"deepseek rejected model", map[string]string{"PROVIDER": "deepseek", "PROVIDER_API_KEY": "key", "PROVIDER_MODEL": "foo"}, "allowed values", ""},
		{
			"deepseek defaults without gemini",
			map[string]string{"PROVIDER": "deepseek", "PROVIDER_API_KEY": "key", "PROVIDER_MODEL": "deepseek-v4-pro"},
			"",
			"deepseek",
		},
		{"gemini default", map[string]string{"GEMINI_API_KEY": "key"}, "", "gemini"},
		{
			"qwen missing key",
			map[string]string{"PROVIDER": "qwen", "PROVIDER_MODEL": "qwen3.7-max", "PROVIDER_BASE_URL": "https://qwen.example"},
			"PROVIDER_API_KEY", "",
		},
		{"qwen missing base URL", map[string]string{"PROVIDER": "qwen", "PROVIDER_API_KEY": "key", "PROVIDER_MODEL": "qwen3.7-max"}, "PROVIDER_BASE_URL", ""},
		{
			"qwen rejected model",
			map[string]string{"PROVIDER": "qwen", "PROVIDER_API_KEY": "key", "PROVIDER_MODEL": "foo", "PROVIDER_BASE_URL": "https://qwen.example"},
			"allowed values", "",
		},
		{
			"qwen happy path",
			map[string]string{"PROVIDER": "qwen", "PROVIDER_API_KEY": "key", "PROVIDER_MODEL": "qwen3.7-max", "PROVIDER_BASE_URL": "https://qwen.example"},
			"", "qwen",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCleanEnv(t)
			setupEnv(t, tt.env)
			cfg, err := NewConfig(NewLogger(LevelError))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), tt.wantErr))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.vendor, cfg.Provider.Vendor)
			if tt.vendor == "deepseek" {
				assert.Equal(t, defaultDeepSeekBaseURL, cfg.Provider.BaseURL)
				assert.Equal(t, "key", cfg.Provider.APIKey)
			}
			if tt.vendor == "qwen" {
				assert.Equal(t, "https://qwen.example", cfg.Provider.BaseURL)
				assert.Equal(t, "key", cfg.Provider.APIKey)
			}
		})
	}
}

func TestNewConfigProviderMaxTokens(t *testing.T) {
	tests := []struct {
		name, value string
		want        int32
	}{{"unset", "", 0}, {"set", "1234", 1234}, {"negative", "-1", 0}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCleanEnv(t)
			setupEnv(t, map[string]string{"GEMINI_API_KEY": "key", "PROVIDER_MAX_TOKENS": tt.value})
			cfg, err := NewConfig(NewLogger(LevelError))
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.ProviderMaxTokens)
		})
	}
}

func TestConfigActiveModel(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"gemini", Config{Provider: ProviderConfig{Vendor: "gemini"}, GeminiModel: "gemini-model"}, "gemini-model"},
		{"deepseek", Config{Provider: ProviderConfig{Vendor: "deepseek", Model: "deepseek-model"}, GeminiModel: "gemini-model"}, "deepseek-model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.want, tt.cfg.ActiveModel()) })
	}
}
