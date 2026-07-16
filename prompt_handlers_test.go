package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// promptText runs a workflow prompt handler and returns the single text
// instruction it emits, failing the test on any structural surprise.
func promptText(t *testing.T, h mcpPromptHandlerFunc, name string, args map[string]string) string {
	t.Helper()
	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name:      name,
			Arguments: args,
		},
	}
	result, err := h(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 1)
	text, ok := result.Messages[0].Content.(mcp.TextContent)
	require.True(t, ok, "expected TextContent")
	return text.Text
}

func TestBuildReviewPRHandler(t *testing.T) {
	h := buildReviewPRHandler(nil)

	t.Run("happy path with focus", func(t *testing.T) {
		out := promptText(t, h, "review_pr", map[string]string{
			"owner":     "octo",
			"repo":      "hello",
			"pr_number": "42",
			"focus":     "concurrency",
		})
		assert.Contains(t, out, `"octo/hello"`)
		assert.Contains(t, out, "`github_pr`: 42")
		assert.Contains(t, out, "Review pull request #42 in octo/hello.")
		assert.Contains(t, out, "Focus on: concurrency.")
	})

	t.Run("focus omitted", func(t *testing.T) {
		out := promptText(t, h, "review_pr", map[string]string{
			"owner":     "octo",
			"repo":      "hello",
			"pr_number": "42",
		})
		assert.NotContains(t, out, "Focus on:")
	})

	t.Run("focus is HTML-escaped", func(t *testing.T) {
		out := promptText(t, h, "review_pr", map[string]string{
			"owner":     "octo",
			"repo":      "hello",
			"pr_number": "1",
			"focus":     "auth & <xss>",
		})
		assert.Contains(t, out, "&amp;")
		assert.Contains(t, out, "&lt;xss&gt;")
		assert.NotContains(t, out, "<xss>")
	})

	for _, missing := range []string{"owner", "repo", "pr_number"} {
		t.Run("missing "+missing, func(t *testing.T) {
			args := map[string]string{"owner": "o", "repo": "r", "pr_number": "1"}
			delete(args, missing)
			_, err := h(context.Background(), mcp.GetPromptRequest{
				Params: mcp.GetPromptParams{Name: "review_pr", Arguments: args},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required argument: "+missing)
		})
	}
}

func TestBuildExplainCommitHandler(t *testing.T) {
	h := buildExplainCommitHandler(nil)

	t.Run("happy path with question", func(t *testing.T) {
		out := promptText(t, h, "explain_commit", map[string]string{
			"owner":    "octo",
			"repo":     "hello",
			"sha":      "abc123",
			"question": "why the lock?",
		})
		assert.Contains(t, out, `"octo/hello"`)
		assert.Contains(t, out, `"abc123"`)
		assert.Contains(t, out, "Explain what commit abc123 in octo/hello does and why.")
		assert.Contains(t, out, "why the lock?")
	})

	t.Run("question omitted", func(t *testing.T) {
		out := promptText(t, h, "explain_commit", map[string]string{
			"owner": "octo",
			"repo":  "hello",
			"sha":   "abc123",
		})
		assert.Contains(t, out, "Explain what commit abc123 in octo/hello does and why.")
	})

	for _, missing := range []string{"owner", "repo", "sha"} {
		t.Run("missing "+missing, func(t *testing.T) {
			args := map[string]string{"owner": "o", "repo": "r", "sha": "s"}
			delete(args, missing)
			_, err := h(context.Background(), mcp.GetPromptRequest{
				Params: mcp.GetPromptParams{Name: "explain_commit", Arguments: args},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required argument: "+missing)
		})
	}
}

func TestBuildCompareRefsHandler(t *testing.T) {
	h := buildCompareRefsHandler(nil)

	t.Run("happy path with question", func(t *testing.T) {
		out := promptText(t, h, "compare_refs", map[string]string{
			"owner":    "octo",
			"repo":     "hello",
			"base":     "v1.0",
			"head":     "v2.0",
			"question": "any breaking changes?",
		})
		assert.Contains(t, out, `"octo/hello"`)
		assert.Contains(t, out, `"v1.0"`)
		assert.Contains(t, out, `"v2.0"`)
		assert.Contains(t, out, "Summarize the changes between v1.0 and v2.0 in octo/hello.")
		assert.Contains(t, out, "any breaking changes?")
	})

	t.Run("question omitted", func(t *testing.T) {
		out := promptText(t, h, "compare_refs", map[string]string{
			"owner": "octo",
			"repo":  "hello",
			"base":  "v1.0",
			"head":  "v2.0",
		})
		assert.Contains(t, out, "Summarize the changes between v1.0 and v2.0 in octo/hello.")
	})

	for _, missing := range []string{"owner", "repo", "base", "head"} {
		t.Run("missing "+missing, func(t *testing.T) {
			args := map[string]string{"owner": "o", "repo": "r", "base": "b", "head": "h"}
			delete(args, missing)
			_, err := h(context.Background(), mcp.GetPromptRequest{
				Params: mcp.GetPromptParams{Name: "compare_refs", Arguments: args},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required argument: "+missing)
		})
	}
}
