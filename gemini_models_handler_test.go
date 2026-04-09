package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiModelsHandler(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())

	s := &GeminiServer{
		config: &Config{
			GeminiModel:       "gemini-3.1-pro-preview",
			GeminiSearchModel: "gemini-3-flash-preview",
		},
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "gemini_models",
			Arguments: map[string]interface{}{},
		},
	}

	result, err := s.GeminiModelsHandler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, toolResultText(t, result))

	text := toolResultText(t, result)
	assert.Contains(t, text, "# Available Gemini Models")
	assert.Contains(t, text, "## Gemini 3.1 Pro")
	assert.Contains(t, text, "## Gemini 3 Flash")
	assert.Contains(t, text, "## Gemini 3.1 Flash Lite")
	assert.Contains(t, text, "- **Model ID**: `gemini-3.1-pro-preview` (default for gemini_ask)")
	assert.Contains(t, text, "- **Model ID**: `gemini-3-flash-preview` (default for gemini_search)")
	assert.Contains(t, text, "- **Alias**: `gemini-3.1-pro-preview-0401`")
	assert.Contains(t, text, "## Thinking Mode")
	assert.Contains(t, text, "## Implicit Caching")
	assert.Contains(t, text, "## File Attachments (gemini_ask)")
	assert.Contains(t, text, "## Time Filtering (gemini_search)")
}

func TestWriteFeatureDocs(t *testing.T) {
	t.Run("empty model list writes nothing", func(t *testing.T) {
		w := NewSafeWriter(NewLogger(LevelError))
		writeFeatureDocs(w, nil)
		assert.Equal(t, "", w.String())
		assert.False(t, w.Failed())
	})

	t.Run("single model is reused for both examples", func(t *testing.T) {
		w := NewSafeWriter(NewLogger(LevelError))
		writeFeatureDocs(w, []GeminiModelInfo{
			{
				FamilyID: "gemini-only-one",
			},
		})

		text := w.String()
		assert.Contains(t, text, "## File Attachments (gemini_ask)")
		assert.Contains(t, text, "## Time Filtering (gemini_search)")
		assert.Equal(t, 2, strings.Count(text, "\"model\": \"gemini-only-one\""))
	})
}

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		name   string
		input  int
		expect string
	}{
		{name: "million", input: 1_000_000, expect: "1M"},
		{name: "thousand", input: 2_000, expect: "2K"},
		{name: "non rounded value", input: 1_048_576, expect: "1048576"},
		{name: "small value", input: 512, expect: "512"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expect, formatTokenCount(tc.input))
		})
	}
}
