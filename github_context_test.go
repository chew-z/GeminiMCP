package main

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func TestParseGitHubContextSpec(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"github_pr": float64(42), "github_commits": []any{"a", "b"}, "github_diff_base": "main", "github_diff_head": "feature",
	}}}
	spec := parseGitHubContextSpec(req)
	assert.True(t, spec.hasPR)
	assert.Equal(t, 42, spec.prNumber)
	assert.Equal(t, []string{"a", "b"}, spec.commits)
	assert.True(t, spec.wantsDiff)
}

func TestConsolidatedContextError(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"github_files": []any{"missing.go"}}}}
	result := consolidatedContextError(req, githubContextSpec{}, 0, []string{"missing"})
	assert.True(t, result.IsError)
	assert.Contains(t, toolResultText(t, result), "Failed to fetch any")
}
