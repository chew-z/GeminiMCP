package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiAskHandlerIgnoresLegacyParameters(t *testing.T) {
	provider := &mockProvider{}
	s := &GeminiServer{config: &Config{Provider: ProviderConfig{Model: "test"}, HTTPTimeout: time.Second, GeminiTemperature: 0.5}, provider: provider}
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"query": "hello", "model": "old", "thinking_level": "low"}}}
	_, err := s.GeminiAskHandler(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, provider.requests(), 1)
}

func TestConvertResponseToMCPResult(t *testing.T) {
	tests := []struct {
		name     string
		response *GenerationResponse
		want     string
		isError  bool
	}{
		{"abnormal finish", &GenerationResponse{Text: "cut", FinishReason: "MAX_TOKENS"}, "[WARN finish_reason=MAX_TOKENS]\ncut", false},
		{"empty text", &GenerationResponse{FinishReason: "STOP"}, "Please try rephrasing", false},
		{"nil", nil, "provider returned an empty response", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertResponseToMCPResult(tt.response, NewLogger(LevelError))
			assert.Equal(t, tt.isError, result.IsError)
			assert.Contains(t, toolResultText(t, result), tt.want)
		})
	}
}

func newTestServerWithProvider(provider Provider) *GeminiServer {
	return &GeminiServer{
		config: &Config{
			Provider:       ProviderConfig{Model: "test"},
			HTTPTimeout:    time.Second,
			MaxRetries:     1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
		},
		provider: provider,
	}
}

// TestProcessWithoutFilesProviderError verifies the query-only path surfaces
// the underlying provider error instead of masking it with the generic
// empty-response message.
func TestProcessWithoutFilesProviderError(t *testing.T) {
	provider := &mockProvider{
		generateFn: func(context.Context, GenerationRequest) (*GenerationResponse, error) {
			return nil, errors.New("boom: rate limit")
		},
	}
	s := newTestServerWithProvider(provider)

	result, err := s.processWithoutFiles(context.Background(), mcp.CallToolRequest{}, "hello", categoryGeneral, "system")
	require.NoError(t, err)
	require.True(t, result.IsError)
	text := toolResultText(t, result)
	assert.Contains(t, text, "boom: rate limit")
	assert.NotContains(t, text, "provider returned an empty response")
	assert.Len(t, provider.requests(), 1, "non-retryable error must not be retried")
}

// TestProcessWithoutFilesEmptyResponse verifies a successful but empty
// generation yields the friendly empty-response guidance, not an error.
func TestProcessWithoutFilesEmptyResponse(t *testing.T) {
	provider := &mockProvider{
		generateFn: func(context.Context, GenerationRequest) (*GenerationResponse, error) {
			return &GenerationResponse{Text: "", FinishReason: "stop"}, nil
		},
	}
	s := newTestServerWithProvider(provider)

	result, err := s.processWithoutFiles(context.Background(), mcp.CallToolRequest{}, "hello", categoryGeneral, "system")
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, toolResultText(t, result), "The model returned an empty response")
}
