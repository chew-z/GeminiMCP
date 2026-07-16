package main

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiAskHandlerIgnoresLegacyParameters(t *testing.T) {
	provider := &mockProvider{}
	s := &GeminiServer{config: &Config{GeminiModel: "test", HTTPTimeout: time.Second, GeminiTemperature: 0.5}, provider: provider}
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
