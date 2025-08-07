package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestPromptHandlers is a table-driven test that covers all prompt handlers.
func TestPromptHandlers(t *testing.T) {
	// Create a test config and server instance.
	config := &Config{
		GeminiAPIKey: "test-key",
		GeminiModel:  "gemini-2.5-pro",
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, loggerKey, NewLogger(LevelInfo))
	geminiSvc, err := NewGeminiServer(ctx, config)
	if err != nil {
		t.Fatalf("Skipping tests: could not create GeminiServer instance: %v", err)
	}

	problemStatement := "Please check my code for potential issues."

	for _, p := range Prompts {
		t.Run(p.Name, func(t *testing.T) {
			req := mcp.GetPromptRequest{
				Params: mcp.GetPromptParams{
					Name: p.Name,
					Arguments: map[string]string{
						"problem_statement": problemStatement,
					},
				},
			}

			handler := geminiSvc.promptHandler(p)
			result, err := handler(ctx, req)
			if err != nil {
				t.Fatalf("Handler returned an unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("Handler returned a nil result")
			}

			textContent, ok := result.Messages[0].Content.(mcp.TextContent)
			if !ok {
				t.Fatal("Expected message content to be TextContent")
			}
			instructions := textContent.Text

			if !strings.Contains(instructions, problemStatement) {
				t.Errorf("Expected instructions to contain the problem statement, but it was missing")
			}

			// Verify that the instructions contain the system prompt.
			if !strings.Contains(instructions, p.SystemPrompt.GetSystemPrompt()) {
				t.Errorf("Expected instructions to contain the system prompt, but it was missing")
			}
		})
	}
}
