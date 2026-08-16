package main

import (
	"context"
	"os"
	"strings"
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

	// Thinking-forced preview model at forced-low effort: the production
	// shape for qwen3.8-max-preview, independent of the configured model.
	t.Run("thinking forced low effort", func(t *testing.T) {
		pcfg := cfg.Provider
		pcfg.Model = "qwen3.8-max-preview"
		forced := newResponsesProvider(pcfg, qwenResponsesDialect{thinkingForced: true}, NewLogger(LevelError))
		resp, err := forced.Generate(context.Background(), GenerationRequest{
			Parts:    []ContentPart{{Text: "Say OK"}},
			Thinking: ThinkingSpec{Enabled: true},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Text)
	})

	// Session-cache audit: send the same >1024-token single-turn prompt
	// twice and report whether the second call hits the cache. A zero on
	// the second call means x-dashscope-session-cache buys nothing for
	// single-turn traffic and the header can be dropped.
	t.Run("session cache audit", func(t *testing.T) {
		var prompt strings.Builder
		for range 300 {
			prompt.WriteString("The quick brown fox jumps over the lazy dog while the server watches the token budget. ")
		}
		prompt.WriteString("Repeat the word: ")
		req := GenerationRequest{Parts: []ContentPart{{Text: prompt.String()}}}

		first, err := p.Generate(context.Background(), req)
		require.NoError(t, err)
		second, err := p.Generate(context.Background(), req)
		require.NoError(t, err)
		t.Logf("first call: total=%d cached=%d; second call: total=%d cached=%d",
			first.Usage.TotalTokens, first.Usage.CachedTokens,
			second.Usage.TotalTokens, second.Usage.CachedTokens)
	})
}
