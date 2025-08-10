package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptHandlers(t *testing.T) {
	// Setup a minimal server instance for testing handlers in isolation.
	// We don't need a full NewGeminiServer() initialization for this unit test.
	geminiSvc := &GeminiServer{
		config: &Config{
			GeminiModel: "test-model",
		},
	}
	ctx := context.Background()
	problemStatement := "Please check my code for potential issues."

	// This is a hypothetical prompt definition for the test.
	// In the real code, this would come from the `Prompts` slice.
	testPrompt := NewPromptDefinition(
		"test-prompt",
		"A test prompt",
		"You are a helpful assistant.",
	)

	t.Run("happy path", func(t *testing.T) {
		req := mcp.GetPromptRequest{
			Params: mcp.GetPromptParams{
				Name: testPrompt.Name,
				Arguments: map[string]string{
					"problem_statement": problemStatement,
				},
			},
		}

		handler := geminiSvc.promptHandler(testPrompt)
		result, err := handler(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Messages, 1)

		textContent, ok := result.Messages[0].Content.(mcp.TextContent)
		require.True(t, ok, "Expected message content to be TextContent")

		instructions := textContent.Text
		assert.Contains(t, instructions, problemStatement, "Instructions should contain the problem statement")
		assert.Contains(t, instructions, testPrompt.SystemPrompt, "Instructions should contain the system prompt")
	})

	t.Run("error on missing problem_statement argument", func(t *testing.T) {
		req := mcp.GetPromptRequest{
			Params: mcp.GetPromptParams{
				Name:      testPrompt.Name,
				Arguments: map[string]string{}, // Missing problem_statement
			},
		}

		handler := geminiSvc.promptHandler(testPrompt)
		result, err := handler(ctx, req)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required argument: problem_statement")
		assert.Nil(t, result)
	})
}
