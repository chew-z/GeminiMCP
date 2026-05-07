package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// captureLogger captures (level, message) tuples for assertions in tests.
type captureLogger struct {
	mu      sync.Mutex
	entries []capturedLogEntry
}

type capturedLogEntry struct {
	level   string
	message string
}

func (c *captureLogger) record(level, format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, capturedLogEntry{
		level:   level,
		message: fmt.Sprintf(format, args...),
	})
}

func (c *captureLogger) Debug(format string, args ...any) { c.record("DEBUG", format, args...) }
func (c *captureLogger) Info(format string, args ...any)  { c.record("INFO", format, args...) }
func (c *captureLogger) Warn(format string, args ...any)  { c.record("WARN", format, args...) }
func (c *captureLogger) Warnf(format string, args ...any) { c.record("WARN", format, args...) }
func (c *captureLogger) Error(format string, args ...any) { c.record("ERROR", format, args...) }

func (c *captureLogger) snapshot() []capturedLogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]capturedLogEntry, len(c.entries))
	copy(out, c.entries)
	return out
}

func TestLogGeminiAPIError(t *testing.T) {
	background := context.Background()

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	deadlineCtx, cancelDeadline := context.WithDeadline(context.Background(), time.Unix(0, 1))
	defer cancelDeadline()
	// Allow the deadline to actually fire so ctx.Err() == DeadlineExceeded.
	<-deadlineCtx.Done()

	tests := []struct {
		name        string
		ctx         context.Context
		err         error
		wantLevel   string
		wantContain string
	}{
		{
			name:        "wrapped DeadlineExceeded with arbitrary ctx err",
			ctx:         background,
			err:         fmt.Errorf("upstream: %w", context.DeadlineExceeded),
			wantLevel:   "INFO",
			wantContain: "deadline exceeded",
		},
		{
			name:        "wrapped Canceled with ctx DeadlineExceeded → server deadline",
			ctx:         deadlineCtx,
			err:         fmt.Errorf("transport: %w", context.Canceled),
			wantLevel:   "INFO",
			wantContain: "canceled by server deadline",
		},
		{
			name:        "wrapped Canceled with ctx Canceled → caller cancel",
			ctx:         canceledCtx,
			err:         fmt.Errorf("transport: %w", context.Canceled),
			wantLevel:   "INFO",
			wantContain: "canceled by caller",
		},
		{
			name:        "wrapped Canceled with uncancelled ctx → caller cancel",
			ctx:         background,
			err:         fmt.Errorf("transport: %w", context.Canceled),
			wantLevel:   "INFO",
			wantContain: "canceled by caller",
		},
		{
			name:        "generic error → ERROR",
			ctx:         background,
			err:         errors.New("boom"),
			wantLevel:   "ERROR",
			wantContain: "Gemini API",
		},
		{
			name:        "nil ctx (defensive) does not panic",
			ctx:         nil,
			err:         fmt.Errorf("transport: %w", context.Canceled),
			wantLevel:   "INFO",
			wantContain: "canceled by caller",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := &captureLogger{}
			require.NotPanics(t, func() {
				logGeminiAPIError(tc.ctx, logger, "Gemini API", tc.err)
			})
			entries := logger.snapshot()
			require.Len(t, entries, 1)
			assert.Equal(t, tc.wantLevel, entries[0].level)
			assert.Contains(t, entries[0].message, tc.wantContain)
		})
	}
}

func TestExtractArgumentStringArray(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		key      string
		expected []string
	}{
		{
			name: "array from client",
			args: map[string]any{
				"github_files": []any{"a.go", "b.go", 42},
			},
			key:      "github_files",
			expected: []string{"a.go", "b.go"},
		},
		{
			name: "json string array",
			args: map[string]any{
				"github_files": `["x.go","y.go"]`,
			},
			key:      "github_files",
			expected: []string{"x.go", "y.go"},
		},
		{
			name: "plain string value",
			args: map[string]any{
				"github_files": "single.go",
			},
			key:      "github_files",
			expected: []string{"single.go"},
		},
		{
			name: "malformed json falls back to plain string",
			args: map[string]any{
				"github_files": "[bad-json",
			},
			key:      "github_files",
			expected: []string{"[bad-json"},
		},
		{
			name: "empty string returns empty slice",
			args: map[string]any{
				"github_files": "",
			},
			key:      "github_files",
			expected: nil,
		},
		{
			name:     "missing key returns empty slice",
			args:     map[string]any{},
			key:      "github_files",
			expected: nil,
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

			got := extractArgumentStringArray(req, tc.key)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestServiceTierFromString(t *testing.T) {
	assert.Equal(t, genai.ServiceTierFlex, serviceTierFromString("flex"))
	assert.Equal(t, genai.ServiceTierPriority, serviceTierFromString("priority"))
	assert.Equal(t, genai.ServiceTierStandard, serviceTierFromString("standard"))
	assert.Equal(t, genai.ServiceTierStandard, serviceTierFromString("unknown-tier"))
}

func TestValidateFilePathArray(t *testing.T) {
	t.Run("github paths reject traversal and absolute", func(t *testing.T) {
		require.NoError(t, validateFilePathArray([]string{"src/main.go", "README.md"}))
		assert.Error(t, validateFilePathArray([]string{"../secret.txt"}))
		assert.Error(t, validateFilePathArray([]string{"/etc/passwd"}))
	})
}

func TestValidateTimeRange(t *testing.T) {
	t.Run("requires both start and end", func(t *testing.T) {
		_, _, err := validateTimeRange("2026-01-01T00:00:00Z", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both start_time and end_time must be provided")
	})

	t.Run("accepts empty range", func(t *testing.T) {
		start, end, err := validateTimeRange("", "")
		require.NoError(t, err)
		assert.Nil(t, start)
		assert.Nil(t, end)
	})

	t.Run("parses valid range", func(t *testing.T) {
		start, end, err := validateTimeRange("2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z")
		require.NoError(t, err)
		require.NotNil(t, start)
		require.NotNil(t, end)
		assert.True(t, start.Before(*end))
	})

	t.Run("rejects reverse range", func(t *testing.T) {
		_, _, err := validateTimeRange("2026-01-02T00:00:00Z", "2026-01-01T00:00:00Z")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start_time must be before or equal to end_time")
	})

	t.Run("rejects invalid format", func(t *testing.T) {
		_, _, err := validateTimeRange(time.Now().String(), "2026-01-01T00:00:00Z")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid start_time format")
	})
}

func TestBuildSearchResponse(t *testing.T) {
	t.Run("builds valid search response JSON", func(t *testing.T) {
		result, err := buildSearchResponse(
			"answer text",
			[]SourceInfo{{Title: "Source A", Type: "web"}},
			[]string{"query a"},
		)
		require.NoError(t, err)
		require.NotNil(t, result)

		var payload SearchResponse
		require.NoError(t, json.Unmarshal([]byte(toolResultText(t, result)), &payload))
		assert.Equal(t, "answer text", payload.Answer)
		assert.Equal(t, []SourceInfo{{Title: "Source A", Type: "web"}}, payload.Sources)
		assert.Equal(t, []string{"query a"}, payload.SearchQueries)
	})

	t.Run("empty answer gets fallback message", func(t *testing.T) {
		result, err := buildSearchResponse("", nil, nil)
		require.NoError(t, err)

		var payload SearchResponse
		require.NoError(t, json.Unmarshal([]byte(toolResultText(t, result)), &payload))
		assert.Contains(t, payload.Answer, "returned an empty response")
	})
}

func TestProcessSearchResponse(t *testing.T) {
	t.Run("extracts answer, deduplicates sources, and captures search queries", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: genai.NewContentFromText("grounded answer", genai.RoleModel),
					GroundingMetadata: &genai.GroundingMetadata{
						WebSearchQueries: []string{"go release notes"},
						GroundingChunks: []*genai.GroundingChunk{
							{Web: &genai.GroundingChunkWeb{Title: "Go Blog", URI: "https://go.dev/blog"}},
							{Web: &genai.GroundingChunkWeb{Title: "Go Blog duplicate", URI: "https://go.dev/blog"}},
							{RetrievedContext: &genai.GroundingChunkRetrievedContext{Title: "Internal Docs", URI: "https://docs.example/internal"}},
						},
					},
				},
			},
		}

		var sources []SourceInfo
		var queries []string
		seenURLs := map[string]bool{}

		text := processSearchResponse(resp, &sources, &queries, seenURLs)

		assert.Equal(t, "grounded answer", text)
		assert.Equal(t, []string{"go release notes"}, queries)
		assert.Equal(t, []SourceInfo{
			{Title: "Go Blog", Type: "web"},
			{Title: "Internal Docs", Type: "retrieved_context"},
		}, sources)
	})

	t.Run("does not overwrite existing search queries", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: genai.NewContentFromText("answer", genai.RoleModel),
					GroundingMetadata: &genai.GroundingMetadata{
						WebSearchQueries: []string{"new query"},
					},
				},
			},
		}

		var sources []SourceInfo
		queries := []string{"existing query"}
		seenURLs := map[string]bool{}

		_ = processSearchResponse(resp, &sources, &queries, seenURLs)
		assert.Equal(t, []string{"existing query"}, queries)
	})
}

func TestConvertGenaiResponseToMCPResult(t *testing.T) {
	logger := NewLogger(LevelError)

	tests := []struct {
		name         string
		resp         *genai.GenerateContentResponse
		wantIsError  bool
		wantPrefix   string
		wantContains string
	}{
		{
			name:        "nil response returns error",
			resp:        nil,
			wantIsError: true,
		},
		{
			name: "finish reason STOP has no warning prefix",
			resp: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{
					Content:      genai.NewContentFromText("all good", genai.RoleModel),
					FinishReason: genai.FinishReasonStop,
				}},
			},
			wantContains: "all good",
		},
		{
			name: "finish reason MAX_TOKENS is surfaced as prefix",
			resp: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{
					Content:      genai.NewContentFromText("truncated answer", genai.RoleModel),
					FinishReason: genai.FinishReasonMaxTokens,
				}},
			},
			wantPrefix:   "[WARN finish_reason=MAX_TOKENS]\n",
			wantContains: "truncated answer",
		},
		{
			name: "finish reason SAFETY is surfaced as prefix",
			resp: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{
					Content:      genai.NewContentFromText("redacted", genai.RoleModel),
					FinishReason: genai.FinishReasonSafety,
				}},
			},
			wantPrefix:   "[WARN finish_reason=SAFETY]\n",
			wantContains: "redacted",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := convertGenaiResponseToMCPResult(tc.resp, logger)
			require.NotNil(t, result)
			if tc.wantIsError {
				assert.True(t, result.IsError)
				return
			}
			text := toolResultText(t, result)
			if tc.wantPrefix != "" {
				assert.True(t, strings.HasPrefix(text, tc.wantPrefix),
					"expected prefix %q, got %q", tc.wantPrefix, text)
			} else {
				assert.False(t, strings.HasPrefix(text, "[WARN"),
					"unexpected WARN prefix in %q", text)
			}
			if tc.wantContains != "" {
				assert.Contains(t, text, tc.wantContains)
			}
		})
	}
}

func TestTierDefaultThinkingLevel(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())

	assert.Equal(t, "high", tierDefaultThinkingLevel("gemini-3.1-pro-preview", "fallback"))
	assert.Equal(t, "medium", tierDefaultThinkingLevel("gemini-3-flash-preview", "fallback"))
	assert.Equal(t, "medium", tierDefaultThinkingLevel("gemini-3.1-flash-lite", "fallback"))
	assert.Equal(t, "fallback", tierDefaultThinkingLevel("not-a-gemini-model", "fallback"))
}

func TestSafeWriter(t *testing.T) {
	writer := NewSafeWriter(NewLogger(LevelError))
	writer.Write("hello %s", "world")
	assert.False(t, writer.Failed())
	assert.Equal(t, "hello world", writer.String())
}
