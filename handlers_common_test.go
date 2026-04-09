package main

import (
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestExtractArgumentStringArray(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		key      string
		expected []string
	}{
		{
			name: "array from client",
			args: map[string]interface{}{
				"file_paths": []any{"a.go", "b.go", 42},
			},
			key:      "file_paths",
			expected: []string{"a.go", "b.go"},
		},
		{
			name: "json string array",
			args: map[string]interface{}{
				"file_paths": `["x.go","y.go"]`,
			},
			key:      "file_paths",
			expected: []string{"x.go", "y.go"},
		},
		{
			name: "plain string value",
			args: map[string]interface{}{
				"file_paths": "single.go",
			},
			key:      "file_paths",
			expected: []string{"single.go"},
		},
		{
			name: "malformed json falls back to plain string",
			args: map[string]interface{}{
				"file_paths": "[bad-json",
			},
			key:      "file_paths",
			expected: []string{"[bad-json"},
		},
		{
			name: "empty string returns empty slice",
			args: map[string]interface{}{
				"file_paths": "",
			},
			key:      "file_paths",
			expected: nil,
		},
		{
			name:     "missing key returns empty slice",
			args:     map[string]interface{}{},
			key:      "file_paths",
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
		require.NoError(t, validateFilePathArray([]string{"src/main.go", "README.md"}, true))
		assert.Error(t, validateFilePathArray([]string{"../secret.txt"}, true))
		assert.Error(t, validateFilePathArray([]string{"/etc/passwd"}, true))
	})

	t.Run("local paths are not validated here", func(t *testing.T) {
		require.NoError(t, validateFilePathArray([]string{"../allowed-later.txt"}, false))
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
