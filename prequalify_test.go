package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrequalifyQuery(t *testing.T) {
	provider := &mockProvider{generateFn: func(_ context.Context, req GenerationRequest) (*GenerationResponse, error) {
		return &GenerationResponse{Text: `{"category":"analyze"}`, FinishReason: "STOP"}, nil
	}}
	s := &GeminiServer{config: &Config{Provider: ProviderConfig{Model: "test"}}, provider: provider}
	category, err := s.prequalifyQuery(context.Background(), "explain this", "")
	require.NoError(t, err)
	assert.Equal(t, categoryAnalyze, category)
	requests := provider.requests()
	require.Len(t, requests, 1)
	assert.Equal(t, "json_object", requests[0].ResponseFormat)
	assert.False(t, requests[0].Thinking.Enabled)
}

func TestPrequalifyQueryUnknownCategory(t *testing.T) {
	s := &GeminiServer{config: &Config{}, provider: &mockProvider{generateFn: func(context.Context, GenerationRequest) (*GenerationResponse, error) {
		return &GenerationResponse{Text: `"unknown"`}, nil
	}}}
	_, err := s.prequalifyQuery(context.Background(), "question", "")
	require.Error(t, err)
}

func TestParsePrequalifyResponse(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		want    queryCategory
		wantErr bool
	}{
		{"category object", `{"category":"analyze"}`, categoryAnalyze, false},
		{"single key object", `{"result":"debug"}`, categoryDebug, false},
		{"quoted string", `"review"`, categoryReview, false},
		{"bare word", "tests", categoryTests, false},
		{"unknown value", `{"category":"other"}`, "", true},
		{"non-string category", `{"category":42}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category, raw, err := parsePrequalifyResponse(&GenerationResponse{Text: tt.text})
			assert.Equal(t, tt.text, raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, category)
		})
	}
}

func TestResolveSystemPromptAsyncFallback(t *testing.T) {
	t.Run("disabled uses general", func(t *testing.T) {
		s := &GeminiServer{config: &Config{Prequalify: false}, provider: &mockProvider{}}
		got := <-s.resolveSystemPromptAsync(context.Background(), mcp.CallToolRequest{}, "q", NewLogger(LevelError))
		assert.Equal(t, categoryGeneral, got.Category)
		assert.Equal(t, systemPromptGeneral, got.SystemPrompt)
	})
	t.Run("error with github context uses analyze", func(t *testing.T) {
		s := &GeminiServer{config: &Config{Prequalify: true}, provider: &mockProvider{generateFn: func(context.Context, GenerationRequest) (*GenerationResponse, error) { return nil, errors.New("down") }}}
		req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"github_repo": "o/r", "github_files": []any{"a.go"}}}}
		got := <-s.resolveSystemPromptAsync(context.Background(), req, "q", NewLogger(LevelError))
		assert.Equal(t, categoryAnalyze, got.Category)
	})
}
