package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestGeminiSearchHandlerSuccess(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())
	ctx := context.Background()

	requestPathCh := make(chan string, 1)
	requestBodyCh := make(chan []byte, 1)
	genaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		select {
		case requestPathCh <- r.URL.String():
		default:
		}
		select {
		case requestBodyCh <- body:
		default:
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates": [{
				"content": {
					"role": "model",
					"parts": [{"text": "grounded answer"}]
				},
				"groundingMetadata": {
					"webSearchQueries": ["go 1.23 release notes"],
					"groundingChunks": [
						{"web": {"title": "Go Blog", "uri": "https://go.dev/blog"}},
						{"web": {"title": "Go Blog duplicate", "uri": "https://go.dev/blog"}},
						{"retrievedContext": {"title": "Internal Docs", "uri": "https://docs.example/internal"}}
					]
				}
			}]
		}`))
	}))
	defer genaiServer.Close()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: "test-api-key",
		HTTPOptions: genai.HTTPOptions{
			BaseURL: genaiServer.URL,
		},
		HTTPClient: genaiServer.Client(),
	})
	require.NoError(t, err)

	s := &GeminiServer{
		config: &Config{
			GeminiSearchModel: "gemini-3-flash-preview",
			GeminiTemperature: 0.3,

			SearchThinkingLevel: "medium",
			ServiceTier:         "standard",
			MaxRetries:          0,
			HTTPTimeout:         100 * time.Millisecond,
		},
		client: client,
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "gemini_search",
			Arguments: map[string]any{
				"query":      "What changed in Go 1.23?",
				"start_time": "2026-01-01T00:00:00Z",
				"end_time":   "2026-01-31T23:59:59Z",
			},
		},
	}

	result, err := s.GeminiSearchHandler(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, toolResultText(t, result))

	var searchResp SearchResponse
	require.NoError(t, json.Unmarshal([]byte(toolResultText(t, result)), &searchResp))
	assert.Equal(t, "grounded answer", searchResp.Answer)
	assert.Equal(t, []string{"go 1.23 release notes"}, searchResp.SearchQueries)
	require.Len(t, searchResp.Sources, 2)
	assert.Equal(t, SourceInfo{Title: "Go Blog", Type: "web"}, searchResp.Sources[0])
	assert.Equal(t, SourceInfo{Title: "Internal Docs", Type: "retrieved_context"}, searchResp.Sources[1])

	var requestPath string
	select {
	case requestPath = <-requestPathCh:
	default:
		t.Fatal("expected outbound GenerateContent request path to be captured")
	}
	assert.True(t, strings.Contains(requestPath, ":generateContent"), "request path must target generateContent endpoint: %s", requestPath)

	var requestBody []byte
	select {
	case requestBody = <-requestBodyCh:
	default:
		t.Fatal("expected outbound GenerateContent request body to be captured")
	}
	body := string(requestBody)
	assert.Contains(t, body, "What changed in Go 1.23?")
	assert.Contains(t, body, "googleSearch")
	assert.Contains(t, body, "timeRangeFilter")
	assert.Contains(t, body, "2026-01-01T00:00:00Z")
	assert.Contains(t, body, "2026-01-31T23:59:59Z")
}

func TestGeminiSearchHandlerValidationAndClientErrors(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())

	s := &GeminiServer{
		config: &Config{
			GeminiSearchModel: "gemini-3-flash-preview",
			GeminiTemperature: 0.3,

			SearchThinkingLevel: "medium",
			ServiceTier:         "standard",
		},
	}

	tests := []struct {
		name          string
		args          map[string]any
		wantErrorText string
	}{
		{
			name:          "missing query",
			args:          map[string]any{},
			wantErrorText: "query must be a string and cannot be empty",
		},
		{
			name: "invalid partial time range",
			args: map[string]any{
				"query":      "find updates",
				"start_time": "2026-01-01T00:00:00Z",
			},
			wantErrorText: "both start_time and end_time must be provided",
		},
		{
			name: "non gemini model rejected",
			args: map[string]any{
				"query": "find updates",
				"model": "gpt-4.1",
			},
			wantErrorText: "not a recognized Gemini model",
		},
		{
			name: "client not initialized",
			args: map[string]any{
				"query": "find updates",
			},
			wantErrorText: "Internal error: Gemini client not properly initialized",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "gemini_search",
					Arguments: tc.args,
				},
			}
			result, err := s.GeminiSearchHandler(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError)
			assert.Contains(t, toolResultText(t, result), tc.wantErrorText)
		})
	}
}

func TestGeminiSearchHandlerAPIError(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())
	ctx := context.Background()

	genaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failure", http.StatusInternalServerError)
	}))
	defer genaiServer.Close()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: "test-api-key",
		HTTPOptions: genai.HTTPOptions{
			BaseURL: genaiServer.URL,
		},
		HTTPClient: genaiServer.Client(),
	})
	require.NoError(t, err)

	s := &GeminiServer{
		config: &Config{
			GeminiSearchModel: "gemini-3-flash-preview",
			GeminiTemperature: 0.3,

			SearchThinkingLevel: "medium",
			ServiceTier:         "standard",
			MaxRetries:          0,
			HTTPTimeout:         100 * time.Millisecond,
		},
		client: client,
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "gemini_search",
			Arguments: map[string]any{
				"query": "find updates",
			},
		},
	}

	result, err := s.GeminiSearchHandler(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, toolResultText(t, result), "Error from Gemini Search API")
}
