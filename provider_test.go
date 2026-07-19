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

// TestPrequalifyModelsAreKnown guards prequalifyModelForVendor against
// referencing a model no allowlist knows about — a silent typo there would
// only surface as a provider error at startup.
func TestPrequalifyModelsAreKnown(t *testing.T) {
	for vendor, model := range prequalifyModelForVendor {
		var allowlist []string
		switch vendor {
		case "deepseek":
			allowlist = deepseekModels
		case "qwen":
			allowlist = qwenModels
		default:
			t.Fatalf("prequalifyModelForVendor has unknown vendor %q", vendor)
		}
		assert.Contains(t, allowlist, model, "prequalify model %q for vendor %q must be in the allowlist", model, vendor)
	}
}

// TestNewPrequalifyProvider verifies the prequalify provider pins the
// vendor's cheap model and never inherits the main model (running
// prequalification on a thinking-forced preview model wedges the follow-up
// generation in production).
func TestNewPrequalifyProvider(t *testing.T) {
	t.Run("qwen pins qwen3.7-plus", func(t *testing.T) {
		cfg := &Config{Provider: ProviderConfig{Vendor: "qwen", APIKey: "key", BaseURL: "https://qwen.example", Model: "qwen3.8-max-preview"}}
		p, err := NewPrequalifyProvider(cfg, NewLogger(LevelError))
		require.NoError(t, err)
		rp, ok := p.(*responsesProvider)
		require.True(t, ok)
		assert.Equal(t, "qwen3.7-plus", rp.model)
		dialect, ok := rp.dialect.(qwenResponsesDialect)
		require.True(t, ok)
		assert.False(t, dialect.thinkingForced, "prequalify dialect must not be thinking-forced")
	})
	t.Run("deepseek pins deepseek-v4-flash", func(t *testing.T) {
		cfg := &Config{Provider: ProviderConfig{Vendor: "deepseek", APIKey: "key", BaseURL: "https://ds.example", Model: "deepseek-v4-pro"}}
		p, err := NewPrequalifyProvider(cfg, NewLogger(LevelError))
		require.NoError(t, err)
		op, ok := p.(*openaiProvider)
		require.True(t, ok)
		assert.Equal(t, "deepseek-v4-flash", op.model)
	})
	t.Run("unknown vendor errors", func(t *testing.T) {
		cfg := &Config{Provider: ProviderConfig{Vendor: "nope"}}
		_, err := NewPrequalifyProvider(cfg, NewLogger(LevelError))
		require.Error(t, err)
	})
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
