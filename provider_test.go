package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name    string
		vendor  string
		wantErr bool
	}{
		{"deepseek", "deepseek", false},
		{"qwen", "qwen", false},
		{"empty vendor", "", true},
		{"gemini removed", "gemini", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(&Config{Provider: ProviderConfig{Vendor: tt.vendor}}, NewLogger(LevelError))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.IsType(t, &openaiProvider{}, provider)
		})
	}
}
