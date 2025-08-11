package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadLocalFiles(t *testing.T) {
	logger := NewLogger(LevelDebug)
	ctx := context.WithValue(context.Background(), loggerKey, logger)
	baseDir := t.TempDir()

	// Setup test files
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "small.txt"), []byte("small"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "large.txt"), []byte("this file is too large"), 0644))
	require.NoError(t, os.Symlink("small.txt", filepath.Join(baseDir, "goodlink.txt")))

	// Setup for traversal attack
	outsideDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret"), 0644))
	// Create a symlink that tries to escape the base directory
	// Note: The relative path from baseDir to outsideDir needs to be calculated.
	relPath, err := filepath.Rel(baseDir, filepath.Join(outsideDir, "secret.txt"))
	require.NoError(t, err)
	require.NoError(t, os.Symlink(relPath, filepath.Join(baseDir, "badlink.txt")))

	testCases := []struct {
		name            string
		paths           []string
		config          *Config
		expectedCount   int
		expectedContent map[string]string
		expectError     bool
	}{
		{
			name:            "read valid file",
			paths:           []string{"small.txt"},
			config:          &Config{FileReadBaseDir: baseDir, MaxFileSize: 100},
			expectedCount:   1,
			expectedContent: map[string]string{"small.txt": "small"},
		},
		{
			name:        "error on no base dir",
			paths:       []string{"small.txt"},
			config:      &Config{MaxFileSize: 100}, // No FileReadBaseDir
			expectError: true,
		},
		{
			name:            "partial success with non-existent file",
			paths:           []string{"small.txt", "nonexistent.txt"},
			config:          &Config{FileReadBaseDir: baseDir, MaxFileSize: 100},
			expectedCount:   1,
			expectedContent: map[string]string{"small.txt": "small"},
		},
		{
			name:        "error on path traversal",
			paths:       []string{"../small.txt"},
			config:      &Config{FileReadBaseDir: baseDir, MaxFileSize: 100},
			expectError: true,
		},
		{
			name:        "error on absolute path",
			paths:       []string{filepath.Join(baseDir, "small.txt")},
			config:      &Config{FileReadBaseDir: baseDir, MaxFileSize: 100},
			expectError: true,
		},
		{
			name:            "read valid symlink",
			paths:           []string{"goodlink.txt"},
			config:          &Config{FileReadBaseDir: baseDir, MaxFileSize: 100},
			expectedCount:   1,
			expectedContent: map[string]string{"goodlink.txt": "small"},
		},
		{
			name:          "error on symlink path traversal",
			paths:         []string{"badlink.txt"},
			config:        &Config{FileReadBaseDir: baseDir, MaxFileSize: 100},
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:            "skip file larger than max size",
			paths:           []string{"small.txt", "large.txt"},
			config:          &Config{FileReadBaseDir: baseDir, MaxFileSize: 10}, // large.txt is > 10 bytes
			expectedCount:   1,
			expectedContent: map[string]string{"small.txt": "small"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			files, err := readLocalFiles(ctx, tc.paths, tc.config)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Len(t, files, tc.expectedCount)
				for _, f := range files {
					expected, ok := tc.expectedContent[f.FileName]
					assert.True(t, ok, "found unexpected file in result: %s", f.FileName)
					assert.Equal(t, expected, string(f.Content))
				}
			}
		})
	}
}

func TestFetchFromGitHub(t *testing.T) {
	logger := NewLogger(LevelDebug)
	ctx := context.WithValue(context.Background(), loggerKey, logger)
	s := &GeminiServer{config: &Config{
		MaxGitHubFiles:    10,
		MaxGitHubFileSize: 1024 * 1024,
	}}

	// Mock GitHub API response for file content
	type githubContentResponse struct {
		Name     string `json:"name"`
		Path     string `json:"path"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}

	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/contents/path/to/file.go":
			resp := githubContentResponse{
				Name:     "file.go",
				Path:     "path/to/file.go",
				Content:  base64.StdEncoding.EncodeToString([]byte("package main")),
				Encoding: "base64",
			}
			json.NewEncoder(w).Encode(resp)
		case "/repos/owner/repo/contents/path/to/bad-base64.txt":
			resp := githubContentResponse{
				Name:     "bad-base64.txt",
				Path:     "path/to/bad-base64.txt",
				Content:  "not-base64-$$",
				Encoding: "base64",
			}
			json.NewEncoder(w).Encode(resp)
		case "/repos/owner/repo/contents/path/to/not-found.txt":
			http.Error(w, "Not Found", http.StatusNotFound)
		case "/repos/owner/repo/contents/path/to/server-error.txt":
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Override the githubBaseURL to point to our mock server
	originalURL := githubBaseURL
	githubBaseURL = server.URL
	s.config.GitHubAPIBaseURL = server.URL
	defer func() { githubBaseURL = originalURL }()

	t.Run("happy path - fetch single file", func(t *testing.T) {
		files, errs := fetchFromGitHub(ctx, s, "owner/repo", "", []string{"path/to/file.go"})
		require.Empty(t, errs)
		require.Len(t, files, 1)
		assert.Equal(t, "path/to/file.go", files[0].FileName)
		assert.Equal(t, "package main", string(files[0].Content))
	})

	t.Run("error on invalid repo url", func(t *testing.T) {
		_, errs := fetchFromGitHub(ctx, s, "invalid-url", "", []string{"file.go"})
		assert.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "invalid github_repo format")
	})

	t.Run("error on file not found (404)", func(t *testing.T) {
		_, errs := fetchFromGitHub(ctx, s, "owner/repo", "", []string{"path/to/not-found.txt"})
		assert.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "status 404")
	})

	t.Run("error on server error (500)", func(t *testing.T) {
		_, errs := fetchFromGitHub(ctx, s, "owner/repo", "", []string{"path/to/server-error.txt"})
		assert.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "status 500")
	})

	t.Run("error on bad base64 content", func(t *testing.T) {
		_, errs := fetchFromGitHub(ctx, s, "owner/repo", "", []string{"path/to/bad-base64.txt"})
		assert.NotEmpty(t, errs)
		assert.Contains(t, errs[0].Error(), "illegal base64 data")
	})

	t.Run("partial success", func(t *testing.T) {
		files, errs := fetchFromGitHub(ctx, s, "owner/repo", "", []string{"path/to/file.go", "path/to/not-found.txt"})
		assert.Len(t, errs, 1, "should have one error for the not-found file")
		assert.Len(t, files, 1, "should have one successfully fetched file")
		assert.Equal(t, "path/to/file.go", files[0].FileName)
	})
}
