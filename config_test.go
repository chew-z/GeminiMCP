package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHTTPPublicURL(t *testing.T) {
	for _, tt := range []struct {
		input, want string
		wantErr     bool
	}{
		{"", "", false}, {"https://example.com/gemini/", "https://example.com/gemini", false}, {"http://localhost:8080", "http://localhost:8080", false}, {"http://example.com", "", true},
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
