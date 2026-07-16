package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogAPIError(t *testing.T) {
	logger := &captureLogger{}
	logAPIError(context.Background(), logger, "provider", errors.New("boom"))
	entries := logger.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "ERROR", entries[0].level)
}

func TestExtractArgumentStringArray(t *testing.T) {
	for _, tt := range []struct {
		value any
		want  []string
	}{{[]any{"a", 1}, []string{"a"}}, {`["a","b"]`, []string{"a", "b"}}, {"a", []string{"a"}}} {
		req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"files": tt.value}}}
		assert.Equal(t, tt.want, extractArgumentStringArray(req, "files"))
	}
}

func TestRetainedHelpers(t *testing.T) {
	assert.NoError(t, validateFilePathArray([]string{"a.go"}))
	assert.Error(t, validateFilePathArray([]string{"../a.go"}))
	writer := NewSafeWriter(NewLogger(LevelError))
	writer.Write("%s", "ok")
	assert.Equal(t, "ok", writer.String())
}
