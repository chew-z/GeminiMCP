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

// --- truncateDiff ---

func TestTruncateDiff(t *testing.T) {
	t.Run("empty input returns untouched", func(t *testing.T) {
		got, trunc := truncateDiff("", 1024)
		assert.Equal(t, "", got)
		assert.False(t, trunc)
	})

	t.Run("under limit returns untouched", func(t *testing.T) {
		patch := "diff --git a/a b/a\n@@ -1,1 +1,1 @@\n-foo\n+bar\n"
		got, trunc := truncateDiff(patch, 1024)
		assert.Equal(t, patch, got)
		assert.False(t, trunc)
	})

	t.Run("cuts at hunk boundary and appends marker", func(t *testing.T) {
		patch := "diff --git a/f b/f\n" +
			"@@ -1,2 +1,2 @@\n-a\n+b\n" +
			"@@ -10,2 +10,2 @@\n-c\n+d\n" +
			"@@ -20,2 +20,2 @@\n-e\n+f\n"
		// Keep only the header + first hunk (~40 bytes).
		got, trunc := truncateDiff(patch, 60)
		assert.True(t, trunc, "expected truncation flag")
		assert.Contains(t, got, "diff --git a/f b/f")
		assert.Contains(t, got, "[truncated:")
		// Second hunk must not appear.
		assert.NotContains(t, got, "-c")
	})

	t.Run("no hunk markers falls back to byte cut", func(t *testing.T) {
		patch := strings.Repeat("x", 200)
		got, trunc := truncateDiff(patch, 50)
		assert.True(t, trunc)
		assert.Contains(t, got, "[truncated")
		assert.LessOrEqual(t, len(got), 200)
	})
}

// --- buildContextInventoryAddendum ---

func TestBuildContextInventoryAddendum(t *testing.T) {
	t.Run("empty inventory yields empty addendum", func(t *testing.T) {
		var inv contextInventory
		assert.Equal(t, "", buildContextInventoryAddendum(&inv))
	})

	t.Run("files only mentions files only", func(t *testing.T) {
		inv := contextInventory{
			Repo:  "openai/openai-go",
			Files: fileInventory{Count: 3, Ref: "main"},
		}
		got := buildContextInventoryAddendum(&inv)
		assert.Contains(t, got, "github.com/openai/openai-go")
		assert.Contains(t, got, "3 source file(s)")
		assert.Contains(t, got, "ref main")
		assert.NotContains(t, got, "commit")
		assert.NotContains(t, got, "Pull request")
		assert.NotContains(t, got, "comparison")
	})

	t.Run("mixed sources mention every attached block", func(t *testing.T) {
		inv := contextInventory{
			Repo:    "openai/openai-go",
			Files:   fileInventory{Count: 2, Ref: "main"},
			Commits: []commitInventory{{SHA: "abc1234", Subject: "fix"}},
			Diff:    &diffInventory{Base: "main", Head: "feat", Truncated: true},
			PR:      &prInventory{Number: 42, Title: "big pr", ReviewCount: 5},
		}
		got := buildContextInventoryAddendum(&inv)
		assert.Contains(t, got, "2 source file(s)")
		assert.Contains(t, got, "1 commit patch(es)")
		assert.Contains(t, got, "main and feat")
		assert.Contains(t, got, "Pull request #42")
		assert.Contains(t, got, "5 review comment(s)")
		assert.Contains(t, got, "truncated")
	})
}

// --- merged github-context integration test ---

// TestGeminiAskHandlerMergesGitHubContext exercises the full merge path:
// github_files + github_commits + github_pr in a single request. Asserts the
// parts are sent in the stable order (commits → PR → files → query) and the
// system prompt gets an inventory addendum describing all three sources.
func TestGeminiAskHandlerMergesGitHubContext(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())
	ctx := context.Background()

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		switch {
		case r.URL.Path == "/repos/o/r/contents/README.md":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("readme-body"))
		case r.URL.Path == "/repos/o/r/pulls/7" && strings.Contains(accept, "diff"):
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("diff --git a/x b/x\n@@ -1,1 +1,1 @@\n-old\n+new\n"))
		case r.URL.Path == "/repos/o/r/pulls/7":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"number":7,"title":"pr title","body":"pr body","state":"open","user":{"login":"alice"},"base":{"sha":"b000000","ref":"main"},"head":{"sha":"h111111","ref":"feat"}}`))
		case strings.HasPrefix(r.URL.Path, "/repos/o/r/pulls/7/comments"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"user":{"login":"bob"},"path":"x","line":1,"body":"nit"}]`))
		case r.URL.Path == "/repos/o/r/commits/abc1234" && strings.Contains(accept, "diff"):
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("diff --git a/y b/y\n@@ -1,1 +1,1 @@\n-p\n+q\n"))
		case r.URL.Path == "/repos/o/r/commits/abc1234":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha":"abc1234567890","commit":{"message":"commit subject\n\nbody","author":{"name":"alice","date":"2026-01-01T00:00:00Z"}},"author":{"login":"alice"}}`))
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
	defer githubServer.Close()

	requestBodyCh := make(chan []byte, 1)
	genaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		select {
		case requestBodyCh <- body:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`))
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
			GeminiModel:               "gemini-pro",
			GeminiSystemPrompt:        "base system prompt",
			GeminiTemperature:         0.3,
			ServiceTier:               "standard",
			MaxRetries:                0,
			HTTPTimeout:               500 * time.Millisecond,
			GitHubAPIBaseURL:          githubServer.URL,
			MaxGitHubFiles:            10,
			MaxGitHubFileSize:         1024,
			MaxGitHubDiffBytes:        1024 * 64,
			MaxGitHubCommits:          10,
			MaxGitHubPRReviewComments: 10,
		},
		client: client,
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "gemini_ask",
			Arguments: map[string]any{
				"query":          "mixed context query",
				"github_repo":    "o/r",
				"github_files":   []any{"README.md"},
				"github_pr":      float64(7),
				"github_commits": []any{"abc1234"},
			},
		},
	}

	result, err := s.GeminiAskHandler(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, toolResultText(t, result))

	var body []byte
	select {
	case body = <-requestBodyCh:
	default:
		t.Fatal("expected outbound request to be captured")
	}

	var payload struct {
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	require.NotEmpty(t, payload.Contents)
	parts := payload.Contents[0].Parts
	require.GreaterOrEqual(t, len(parts), 4, "expected commit + pr + file + query parts")

	// Build a single flat string for ordering checks.
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
		sb.WriteString("\n")
	}
	all := sb.String()

	// Stable merge order: commits → diff → PR → files → query.
	commitIdx := strings.Index(all, "--- Commit abc1234")
	prIdx := strings.Index(all, "--- PR #7 ")
	fileIdx := strings.Index(all, "--- File: README.md ---")
	queryIdx := strings.Index(all, "mixed context query")

	assert.NotEqual(t, -1, commitIdx, "commit block missing")
	assert.NotEqual(t, -1, prIdx, "pr block missing")
	assert.NotEqual(t, -1, fileIdx, "file block missing")
	assert.NotEqual(t, -1, queryIdx, "query missing")

	assert.Less(t, commitIdx, prIdx, "commits must come before PR")
	assert.Less(t, prIdx, fileIdx, "PR must come before files")
	assert.Less(t, fileIdx, queryIdx, "files must come before query")

	// System prompt addendum should describe every attached source.
	require.NotEmpty(t, payload.SystemInstruction.Parts)
	systemText := payload.SystemInstruction.Parts[0].Text
	assert.Contains(t, systemText, "base system prompt")
	assert.Contains(t, systemText, "github.com/o/r")
	assert.Contains(t, systemText, "1 source file(s)")
	assert.Contains(t, systemText, "1 commit patch(es)")
	assert.Contains(t, systemText, "Pull request #7")
}

// TestGeminiAskHandlerRejectsLocalFilePathsWithGitHubContext verifies the
// local file_paths source is still exclusive with any github_* parameter.
func TestGeminiAskHandlerRejectsLocalFilePathsWithGitHubContext(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())
	s := &GeminiServer{
		config: &Config{
			GeminiModel:               "gemini-pro",
			GeminiSystemPrompt:        "sp",
			GeminiTemperature:         0.3,
			ServiceTier:               "standard",
			MaxGitHubFiles:            10,
			MaxGitHubFileSize:         1024,
			MaxGitHubDiffBytes:        1024,
			MaxGitHubCommits:          5,
			MaxGitHubPRReviewComments: 5,
			GitHubAPIBaseURL:          "http://127.0.0.1:1", // unreachable on purpose
			HTTPTimeout:               50 * time.Millisecond,
		},
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "gemini_ask",
			Arguments: map[string]any{
				"query":       "q",
				"github_repo": "o/r",
				"github_pr":   float64(1),
				"file_paths":  []any{"local.txt"},
			},
		},
	}

	result, err := s.GeminiAskHandler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, toolResultText(t, result), "'file_paths' cannot be combined")
}

// TestGeminiAskHandlerGitHubDiffRequiresBothRefs ensures supplying only one of
// the diff refs returns a clear validation error before any network call.
func TestGeminiAskHandlerGitHubDiffRequiresBothRefs(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())
	s := &GeminiServer{
		config: &Config{
			GeminiModel:        "gemini-pro",
			GeminiSystemPrompt: "sp",
			GeminiTemperature:  0.3,
			ServiceTier:        "standard",
			MaxGitHubDiffBytes: 1024,
			HTTPTimeout:        50 * time.Millisecond,
		},
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "gemini_ask",
			Arguments: map[string]any{
				"query":            "q",
				"github_repo":      "o/r",
				"github_diff_base": "main",
				// head intentionally missing
			},
		},
	}
	result, err := s.GeminiAskHandler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Contains(t, toolResultText(t, result), "must both be provided")
}

// --- extractArgumentInt coverage ---

func TestExtractArgumentInt(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
		ok   bool
	}{
		{"float64", float64(42), 42, true},
		{"int", 7, 7, true},
		{"int64", int64(9), 9, true},
		{"numeric string", "15", 15, true},
		{"blank string", "", 0, false},
		{"garbage string", "nope", 0, false},
		{"missing", nil, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "gemini_ask",
					Arguments: map[string]any{},
				},
			}
			if tc.in != nil {
				req.Params.Arguments.(map[string]any)["github_pr"] = tc.in
			}
			got, ok := extractArgumentInt(req, "github_pr")
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// sanity test for encodeRefForURL
func TestEncodeRefForURL(t *testing.T) {
	assert.Equal(t, "main", encodeRefForURL("main"))
	assert.Equal(t, "feature/foo", encodeRefForURL("feature/foo"))
	assert.Equal(t, "feat%20ure", encodeRefForURL("feat ure"))
}

// helper guard so go vet doesn't complain about unused fmt in slimmed builds.
var _ = fmt.Sprintf
