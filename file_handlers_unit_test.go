package main

import (
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
