package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name     string
		vendor   string
		wantType any
		wantErr  bool
	}{
		{"deepseek", "deepseek", &openaiProvider{}, false},
		{"qwen", "qwen", &responsesProvider{}, false},
		{"empty vendor", "", nil, true},
		{"gemini removed", "gemini", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(&Config{Provider: ProviderConfig{Vendor: tt.vendor}}, NewLogger(LevelError))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.IsType(t, tt.wantType, provider)
		})
	}
}

// TestNewProviderThinkingForcedWiring guards the slices.Contains wiring that
// sets the dialect's thinkingForced flag: thinking-only models must get it,
// and stable models must not (a silent miss means effort=none and a 400).
func TestNewProviderThinkingForcedWiring(t *testing.T) {
	for _, tt := range []struct {
		model  string
		forced bool
	}{
		{"qwen3.8-max-preview", true},
		{"qwen3.7-max", false},
		{"qwen3.7-plus", false},
	} {
		t.Run(tt.model, func(t *testing.T) {
			cfg := &Config{Provider: ProviderConfig{Vendor: "qwen", APIKey: "key", BaseURL: "https://qwen.example", Model: tt.model}}
			p, err := NewProvider(cfg, NewLogger(LevelError))
			require.NoError(t, err)
			rp, ok := p.(*responsesProvider)
			require.True(t, ok, "qwen vendor must produce a responsesProvider")
			dialect, ok := rp.dialect.(qwenResponsesDialect)
			require.True(t, ok, "qwen vendor must use qwenResponsesDialect")
			assert.Equal(t, tt.forced, dialect.thinkingForced)
		})
	}
}
