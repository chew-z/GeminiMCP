package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubRepo(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
		wantErr   string
	}{
		{
			name:      "owner repo shorthand",
			input:     "openai/gpt-oss",
			wantOwner: "openai",
			wantRepo:  "gpt-oss",
		},
		{
			name:      "https github url",
			input:     "https://github.com/openai/openai-go",
			wantOwner: "openai",
			wantRepo:  "openai-go",
		},
		{
			name:      "https url with .git suffix",
			input:     "https://github.com/openai/openai-python.git",
			wantOwner: "openai",
			wantRepo:  "openai-python",
		},
		{
			name:      "ssh url",
			input:     "git@github.com:openai/evals.git",
			wantOwner: "openai",
			wantRepo:  "evals",
		},
		{
			name:      "git ssh syntax owner/repo",
			input:     "git@github.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:    "git ssh syntax with port is unsupported",
			input:   "git@github.com:22:owner/repo.git",
			wantErr: "unsupported github_repo SSH format",
		},
		{
			name:    "traversal in shorthand is invalid",
			input:   "../repo",
			wantErr: "invalid github_repo format",
		},
		{
			name:    "invalid shorthand",
			input:   "openai",
			wantErr: "invalid github_repo format",
		},
		{
			name:    "invalid url path",
			input:   "https://github.com/openai",
			wantErr: "invalid github_repo URL path",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := parseGitHubRepo(tc.input)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantOwner, owner)
			assert.Equal(t, tc.wantRepo, repo)
		})
	}
}

func TestRateLimitResponseRetryAfterRemainingNonZeroIsRetryable(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("payload")),
	}
	resp.Header.Set("X-RateLimit-Remaining", "5")
	resp.Header.Set("Retry-After", "1")

	ctxErr, retryErr := handleRateLimitResponse(context.Background(), resp, NewLogger(LevelDebug), "file.txt")
	assert.Nil(t, ctxErr)
	assert.EqualError(t, retryErr, "rate limit exceeded")
}
