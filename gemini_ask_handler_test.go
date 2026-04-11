package main

import (
	"context"
	"encoding/json"
	"fmt"
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
		assert.Contains(t, got, "The following requested context could not be loaded")
		assert.Contains(t, got, "... and 2 other item(s)")

		for i := 0; i < maxReportedWarnings; i++ {
			assert.Contains(t, got, warnings[i])
		}
		assert.NotContains(t, got, warnings[maxReportedWarnings])
		assert.NotContains(t, got, warnings[maxReportedWarnings+1])
	})
}

func TestGeminiAskHandlerFileSourceBehavior(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())

	s := &GeminiServer{
		config: &Config{
			GeminiModel:        "gemini-pro",
			GeminiSystemPrompt: "system prompt",
			GeminiTemperature:  0.3,
			EnableThinking:     true,
			ThinkingLevel:      "high",
			ServiceTier:        "standard",
			MaxGitHubFiles:     10,
			MaxGitHubFileSize:  1024,
			HTTPTimeout:        50 * time.Millisecond,
		},
	}

	tests := []struct {
		name          string
		args          map[string]interface{}
		wantErrorText string
	}{
		{
			name: "rejects github_files without github_repo",
			args: map[string]interface{}{
				"query":        "test query",
				"github_files": []any{"README.md"},
			},
			wantErrorText: "'github_repo' is required when using 'github_files'",
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

			result, err := s.GeminiAskHandler(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError)

			text := toolResultText(t, result)
			assert.Contains(t, text, tc.wantErrorText)
		})
	}
}

func TestGeminiAskHandlerGitHubWarningTruncationInOutboundQuery(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())
	ctx := context.Background()

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/contents/path/ok.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("ok"))
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
	defer githubServer.Close()

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
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"mock ok"}]}}]}`))
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
			GeminiModel:        "gemini-pro",
			GeminiSystemPrompt: "system prompt",
			GeminiTemperature:  0.3,
			EnableThinking:     true,
			ThinkingLevel:      "high",
			ServiceTier:        "standard",
			MaxRetries:         0,
			HTTPTimeout:        100 * time.Millisecond,
			GitHubAPIBaseURL:   githubServer.URL,
			MaxGitHubFiles:     20,
			MaxGitHubFileSize:  1024,
		},
		client: client,
	}

	githubFiles := []any{"path/ok.txt"}
	for i := 1; i <= maxReportedWarnings+2; i++ {
		githubFiles = append(githubFiles, fmt.Sprintf("path/missing-%02d.txt", i))
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "gemini_ask",
			Arguments: map[string]interface{}{
				"query":        "analyze this repository",
				"github_repo":  "owner/repo",
				"github_files": githubFiles,
			},
		},
	}

	result, err := s.GeminiAskHandler(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, toolResultText(t, result))
	assert.Contains(t, toolResultText(t, result), "mock ok")

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

	var payload struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(requestBody, &payload))
	require.NotEmpty(t, payload.Contents)
	require.NotEmpty(t, payload.Contents[0].Parts)

	querySent := payload.Contents[0].Parts[len(payload.Contents[0].Parts)-1].Text
	assert.Contains(t, querySent, "analyze this repository")
	assert.Contains(t, querySent, "The following requested context could not be loaded")
	assert.Contains(t, querySent, "... and 2 other item(s)")

	for i := 1; i <= maxReportedWarnings; i++ {
		assert.Contains(t, querySent, fmt.Sprintf("path/missing-%02d.txt: could not be fetched from GitHub", i))
	}
	assert.NotContains(t, querySent, "path/missing-11.txt: could not be fetched from GitHub")
	assert.NotContains(t, querySent, "path/missing-12.txt: could not be fetched from GitHub")
}

func TestGeminiAskHandlerWithoutFilesUsesProcessWithoutFiles(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"no-file ok"}]}}]}`))
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
			GeminiModel:        "gemini-pro",
			GeminiSystemPrompt: "system prompt",
			GeminiTemperature:  0.3,
			EnableThinking:     true,
			ThinkingLevel:      "high",
			ServiceTier:        "standard",
			MaxRetries:         0,
			HTTPTimeout:        100 * time.Millisecond,
		},
		client: client,
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "gemini_ask",
			Arguments: map[string]interface{}{
				"query": "answer this directly",
			},
		},
	}

	result, err := s.GeminiAskHandler(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, toolResultText(t, result))
	assert.Contains(t, toolResultText(t, result), "no-file ok")

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

	var payload struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(requestBody, &payload))
	require.NotEmpty(t, payload.Contents)
	require.Len(t, payload.Contents[0].Parts, 1)
	assert.Equal(t, "answer this directly", payload.Contents[0].Parts[0].Text)
}

func TestGatherGitHubFiles(t *testing.T) {
	logger := NewLogger(LevelDebug)
	ctx := context.WithValue(context.Background(), loggerKey, logger)

	makeServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/owner/repo/contents/path/ok.txt":
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte("ok"))
			default:
				http.Error(w, "Not Found", http.StatusNotFound)
			}
		}))
	}

	t.Run("partial success returns exact warning list for missing files", func(t *testing.T) {
		server := makeServer()
		defer server.Close()

		s := &GeminiServer{
			config: &Config{
				GitHubAPIBaseURL:  server.URL,
				MaxGitHubFiles:    10,
				MaxGitHubFileSize: 1024,
				HTTPTimeout:       100 * time.Millisecond,
			},
		}

		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "gemini_ask",
				Arguments: map[string]interface{}{
					"github_repo": "owner/repo",
				},
			},
		}
		files := []string{"path/missing-a.txt", "path/ok.txt", "path/missing-b.txt"}

		uploads, warnings, errResult := s.gatherGitHubFiles(ctx, req, files)
		require.Nil(t, errResult)
		require.Len(t, uploads, 1)
		assert.Equal(t, "path/ok.txt", uploads[0].FileName)
		assert.Equal(t, []string{
			"path/missing-a.txt: could not be fetched from GitHub",
			"path/missing-b.txt: could not be fetched from GitHub",
		}, warnings)
	})

	t.Run("all files failing returns tool error result", func(t *testing.T) {
		server := makeServer()
		defer server.Close()

		s := &GeminiServer{
			config: &Config{
				GitHubAPIBaseURL:  server.URL,
				MaxGitHubFiles:    10,
				MaxGitHubFileSize: 1024,
				HTTPTimeout:       100 * time.Millisecond,
			},
		}

		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "gemini_ask",
				Arguments: map[string]interface{}{
					"github_repo": "owner/repo",
				},
			},
		}

		uploads, warnings, errResult := s.gatherGitHubFiles(ctx, req, []string{"path/missing.txt"})
		assert.Nil(t, uploads)
		assert.Nil(t, warnings)
		require.NotNil(t, errResult)
		assert.True(t, errResult.IsError)
		assert.Contains(t, toolResultText(t, errResult), "Error processing github files")
	})
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
