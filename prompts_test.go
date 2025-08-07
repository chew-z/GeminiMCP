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

	// Define the test cases for each prompt handler.
	testCases := []struct {
		name                 string
		handler              func(context.Context, mcp.GetPromptRequest) (*mcp.GetPromptResult, error)
		expectedSubstrings []string // We now check for substrings instead of a full match.
	}{
		{
			name:    "code_review",
			handler: geminiSvc.CodeReviewHandler,
			expectedSubstrings: []string{
				"You are an expert code reviewer",
				"Follow OWASP Top 10 guidelines",
			},
		},
		{
			name:    "explain_code",
			handler: geminiSvc.ExplainCodeHandler,
			expectedSubstrings: []string{
				"You are an expert software engineer and a skilled educator",
				"Tailor the complexity of your explanation",
			},
		},
		{
			name:    "debug_help",
			handler: geminiSvc.DebugHelpHandler,
			expectedSubstrings: []string{
				"You are an expert debugger",
				"Provide a specific, corrected code snippet to fix the bug",
			},
		},
		{
			name:    "refactor_suggestions",
			handler: geminiSvc.RefactorSuggestionsHandler,
			expectedSubstrings: []string{
				"You are an expert software architect specializing in code modernization",
				"explain the benefits of the proposed refactoring",
			},
		},
		{
			name:    "architecture_analysis",
			handler: geminiSvc.ArchitectureAnalysisHandler,
			expectedSubstrings: []string{
				"You are a seasoned software architect",
				"Provide a clear and concise summary of the architecture",
			},
		},
		{
			name:    "doc_generate",
			handler: geminiSvc.DocGenerateHandler,
			expectedSubstrings: []string{
				"You are a professional technical writer",
				"documentation should be in Markdown format",
			},
		},
		{
			name:    "test_generate",
			handler: geminiSvc.TestGenerateHandler,
			expectedSubstrings: []string{
				"You are a test engineering expert",
				"Cover happy-path scenarios, edge cases, and error conditions",
				"For each function or method, provide a set of corresponding test cases",
			},
		},
		{
			name:    "security_analysis",
			handler: geminiSvc.SecurityAnalysisHandler,
			expectedSubstrings: []string{
				"You are a cybersecurity expert",
				"Cross-Site Scripting (XSS)",
			},
		},
	}

	problemStatement := "Please check my code for potential issues."

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.GetPromptRequest{
				Params: mcp.GetPromptParams{
					Name: tc.name,
					Arguments: map[string]string{
						"problem_statement": problemStatement,
					},
				},
			}

			result, err := tc.handler(ctx, req)
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

			// Verify that the instructions contain all expected substrings.
			for _, sub := range tc.expectedSubstrings {
				if !strings.Contains(instructions, sub) {
					t.Errorf("Expected instructions to contain the substring '%s', but it was missing", sub)
				}
			}
		})
	}
}
