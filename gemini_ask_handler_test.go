package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAskRequest(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())

	s := &GeminiServer{
		config: &Config{
			GeminiModel:        "gemini-pro",
			GeminiSystemPrompt: "system prompt",
			GeminiTemperature:  0.3,
			EnableThinking:     true,
			ThinkingLevel:      "high",
			ServiceTier:        "standard",
		},
	}

	tests := []struct {
		name          string
		req           mcp.CallToolRequest
		wantQuery     string
		wantModelName string
		wantErr       bool
		errContains   string
	}{
		{
			name: "valid request uses tier default and resolves to concrete family",
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "gemini_ask",
					Arguments: map[string]interface{}{
						"query": "test query",
					},
				},
			},
			wantQuery:     "test query",
			wantModelName: "gemini-3.1-pro-preview",
			wantErr:       false,
		},
		{
			name: "explicit model family resolves to preferred version",
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "gemini_ask",
					Arguments: map[string]interface{}{
						"query": "test query",
						"model": "gemini-3-flash-preview",
					},
				},
			},
			wantQuery:     "test query",
			wantModelName: "gemini-3-flash-preview-0502",
			wantErr:       false,
		},
		{
			name: "unknown gemini model redirects to best available tier model",
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "gemini_ask",
					Arguments: map[string]interface{}{
						"query": "test query",
						"model": "gemini-9-pro-preview",
					},
				},
			},
			wantQuery:     "test query",
			wantModelName: "gemini-3.1-pro-preview",
			wantErr:       false,
		},
		{
			name: "non-gemini model is rejected",
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "gemini_ask",
					Arguments: map[string]interface{}{
						"query": "test query",
						"model": "gpt-4.1",
					},
				},
			},
			wantErr:     true,
			errContains: "not a recognized Gemini model",
		},
		{
			name: "missing query",
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "gemini_ask",
					Arguments: map[string]interface{}{
						"query": "",
					},
				},
			},
			wantErr:     true,
			errContains: "query must be a string and cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, cfg, modelName, err := s.parseAskRequest(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantQuery, query)
			assert.Equal(t, tt.wantModelName, modelName)
			require.NotNil(t, cfg)
		})
	}
}
