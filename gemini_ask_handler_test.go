package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func toolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "expected text content in tool result")
	return text.Text
}

func TestAppendFileWarningNote(t *testing.T) {
	baseQuery := "summarize this code"

	t.Run("no warnings keeps query unchanged", func(t *testing.T) {
		got := appendFileWarningNote(baseQuery, nil)
		assert.Equal(t, baseQuery, got)
	})

	t.Run("caps warnings and appends overflow suffix", func(t *testing.T) {
		var warnings []string
		for i := 1; i <= maxReportedWarnings+2; i++ {
			warnings = append(warnings, fmt.Sprintf("file-%02d: could not be loaded", i))
		}

		got := appendFileWarningNote(baseQuery, warnings)
		assert.Contains(t, got, baseQuery)
		assert.Contains(t, got, "The following requested files could not be loaded")
		assert.Contains(t, got, "... and 2 other file(s)")

		for i := 0; i < maxReportedWarnings; i++ {
			assert.Contains(t, got, warnings[i])
		}
		assert.NotContains(t, got, warnings[maxReportedWarnings])
		assert.NotContains(t, got, warnings[maxReportedWarnings+1])
	})
}

func TestGeminiAskHandlerFileSourceBehavior(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())

	baseDir := t.TempDir()
	require.NoError(t, os.WriteFile(baseDir+"/ok.txt", []byte("hello"), 0644))

	s := &GeminiServer{
		config: &Config{
			GeminiModel:        "gemini-pro",
			GeminiSystemPrompt: "system prompt",
			GeminiTemperature:  0.3,
			EnableThinking:     true,
			ThinkingLevel:      "high",
			ServiceTier:        "standard",
			FileReadBaseDir:    baseDir,
			MaxFileSize:        1024,
			MaxGitHubFiles:     10,
			MaxGitHubFileSize:  1024,
			HTTPTimeout:        50 * time.Millisecond,
		},
	}

	tests := []struct {
		name            string
		args            map[string]interface{}
		wantErrorText   string
		wantInternalErr bool
		httpTransport   bool
	}{
		{
			name: "rejects mixed local and github sources",
			args: map[string]interface{}{
				"query":        "test query",
				"file_paths":   []any{"ok.txt"},
				"github_files": []any{"README.md"},
				"github_repo":  "owner/repo",
			},
			wantErrorText: "Cannot use both 'file_paths' and 'github_files'",
		},
		{
			name: "rejects github_files without github_repo",
			args: map[string]interface{}{
				"query":        "test query",
				"github_files": []any{"README.md"},
			},
			wantErrorText: "'github_repo' is required when using 'github_files'",
		},
		{
			name: "hard-fails when file_paths requested but none gathered",
			args: map[string]interface{}{
				"query":      "test query",
				"file_paths": []any{"missing.txt"},
			},
			wantErrorText: "Failed to retrieve any of the requested files",
		},
		{
			name: "rejects local file_paths over HTTP transport",
			args: map[string]interface{}{
				"query":      "test query",
				"file_paths": []any{"ok.txt"},
			},
			wantErrorText: "'file_paths' is not supported in HTTP transport mode",
			httpTransport: true,
		},
		{
			name: "valid local file reaches client validation path",
			args: map[string]interface{}{
				"query":      "test query",
				"file_paths": []any{"ok.txt"},
			},
			wantErrorText:   "Internal error: Gemini client not properly initialized",
			wantInternalErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "gemini_ask",
					Arguments: tc.args,
				},
			}

			ctx := context.Background()
			if tc.httpTransport {
				ctx = withHTTPTransport(ctx)
			}

			result, err := s.GeminiAskHandler(ctx, req)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError)

			text := toolResultText(t, result)
			assert.Contains(t, text, tc.wantErrorText)

			if tc.wantInternalErr {
				assert.Contains(t, text, "Gemini client not properly initialized")
			}
		})
	}
}

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
