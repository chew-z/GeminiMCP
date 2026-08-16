package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeepSeekLive exercises the configured DeepSeek backend when explicitly
// enabled: thinking-enabled generation, thinking-disabled (effort none), and
// the prequalify provider on deepseek-v4-flash with JSON mode.
func TestDeepSeekLive(t *testing.T) {
	if os.Getenv("PROVIDER") != "deepseek" || os.Getenv("PROVIDER_API_KEY") == "" {
		t.Skip("set PROVIDER=deepseek and PROVIDER_API_KEY to run the DeepSeek live test")
	}
	cfg, err := NewConfig(NewLogger(LevelError))
	require.NoError(t, err)
	p, err := NewProvider(cfg, NewLogger(LevelError))
	require.NoError(t, err)

	t.Run("thinking enabled", func(t *testing.T) {
		resp, err := p.Generate(context.Background(), GenerationRequest{Parts: []ContentPart{{Text: "Say OK"}}, Thinking: ThinkingSpec{Enabled: true}})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Text)
	})
	t.Run("thinking disabled", func(t *testing.T) {
		resp, err := p.Generate(context.Background(), GenerationRequest{Parts: []ContentPart{{Text: "Say OK"}}})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Text)
	})
	t.Run("prequalify provider", func(t *testing.T) {
		pq, err := NewPrequalifyProvider(cfg, NewLogger(LevelError))
		require.NoError(t, err)
		resp, err := pq.Generate(context.Background(), GenerationRequest{
			SystemPrompt:   `Respond with JSON in exactly this form: {"category": "general"}`,
			Parts:          []ContentPart{{Text: "hello"}},
			ResponseFormat: "json_object",
			Temperature:    0,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Text)
	})
}

// TestQwenLive exercises the configured Qwen backend when explicitly enabled.
func TestQwenLive(t *testing.T) {
	if os.Getenv("PROVIDER") != "qwen" || os.Getenv("PROVIDER_API_KEY") == "" {
		t.Skip("set PROVIDER=qwen and PROVIDER_API_KEY to run the Qwen live test")
	}
	cfg, err := NewConfig(NewLogger(LevelError))
	require.NoError(t, err)
	p, err := NewProvider(cfg, NewLogger(LevelError))
	require.NoError(t, err)
	resp, err := p.Generate(context.Background(), GenerationRequest{Parts: []ContentPart{{Text: "Say OK"}}})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Text)
}
