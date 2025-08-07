package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestCodeReviewPrompt tests the code review prompt
func TestCodeReviewPrompt(t *testing.T) {
	// Create a mock GeminiServer for testing
	config := &Config{
		GeminiAPIKey:       "test-key",
		GeminiModel:        "gemini-2.5-pro",
		GeminiSystemPrompt: "You are a helpful assistant.",
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, loggerKey, NewLogger(LevelInfo))

	geminiSvc, err := NewGeminiServer(ctx, config)
	if err != nil {
		t.Skipf("Skipping test - could not create GeminiServer: %v", err)
	}

	// Test with required argument
	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "code_review",
			Arguments: map[string]string{
				"code": "function test() { return 'hello'; }",
			},
		},
	}

	result, err := geminiSvc.CodeReviewHandler(ctx, req)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if len(result.Messages) == 0 {
		t.Error("Expected messages in result")
	}

	// Test with missing required argument
	reqMissing := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name:      "code_review",
			Arguments: map[string]string{},
		},
	}

	result, err = geminiSvc.CodeReviewHandler(ctx, reqMissing)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if result.Description != "Error: code argument is required" {
		t.Errorf("Expected error message, got: %s", result.Description)
	}
}

// TestExplainCodePrompt tests the explain code prompt
func TestExplainCodePrompt(t *testing.T) {
	config := &Config{
		GeminiAPIKey:       "test-key",
		GeminiModel:        "gemini-2.5-pro",
		GeminiSystemPrompt: "You are a helpful assistant.",
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, loggerKey, NewLogger(LevelInfo))

	geminiSvc, err := NewGeminiServer(ctx, config)
	if err != nil {
		t.Skipf("Skipping test - could not create GeminiServer: %v", err)
	}

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "explain_code",
			Arguments: map[string]string{
				"code":     "function fibonacci(n) { return n <= 1 ? n : fibonacci(n-1) + fibonacci(n-2); }",
				"audience": "beginner",
			},
		},
	}

	result, err := geminiSvc.ExplainCodeHandler(ctx, req)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if len(result.Messages) == 0 {
		t.Error("Expected messages in result")
	}

	// Check that message exists and contains content (now combined as user message)
	found := false
	for _, msg := range result.Messages {
		if msg.Role == mcp.RoleUser {
			if content, ok := msg.Content.(mcp.TextContent); ok {
				if len(content.Text) > 0 && strings.Contains(content.Text, "beginner") {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Error("Expected user message with content including audience info")
	}
}

// TestDebugHelpPrompt tests the debug help prompt
func TestDebugHelpPrompt(t *testing.T) {
	config := &Config{
		GeminiAPIKey:       "test-key",
		GeminiModel:        "gemini-2.5-pro",
		GeminiSystemPrompt: "You are a helpful assistant.",
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, loggerKey, NewLogger(LevelInfo))

	geminiSvc, err := NewGeminiServer(ctx, config)
	if err != nil {
		t.Skipf("Skipping test - could not create GeminiServer: %v", err)
	}

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "debug_help",
			Arguments: map[string]string{
				"code":              "x = 1/0",
				"error_message":     "ZeroDivisionError: division by zero",
				"expected_behavior": "Should handle division by zero gracefully",
			},
		},
	}

	result, err := geminiSvc.DebugHelpHandler(ctx, req)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if len(result.Messages) == 0 {
		t.Error("Expected messages in result")
	}
}

// TestSecurityAnalysisPrompt tests the security analysis prompt
func TestSecurityAnalysisPrompt(t *testing.T) {
	config := &Config{
		GeminiAPIKey:       "test-key",
		GeminiModel:        "gemini-2.5-pro",
		GeminiSystemPrompt: "You are a helpful assistant.",
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, loggerKey, NewLogger(LevelInfo))

	geminiSvc, err := NewGeminiServer(ctx, config)
	if err != nil {
		t.Skipf("Skipping test - could not create GeminiServer: %v", err)
	}

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "security_analysis",
			Arguments: map[string]string{
				"code":          "SELECT * FROM users WHERE id = '" + "user_input" + "'",
				"scope":         "SQL injection vulnerabilities",
				"include_fixes": "true",
			},
		},
	}

	result, err := geminiSvc.SecurityAnalysisHandler(ctx, req)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if len(result.Messages) == 0 {
		t.Error("Expected messages in result")
	}
}

// TestPromptArgumentExtraction tests the helper function for extracting prompt arguments
func TestPromptArgumentExtraction(t *testing.T) {
	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "test_prompt",
			Arguments: map[string]string{
				"string_arg": "test_value",
				"empty_arg":  "",
			},
		},
	}

	// Test existing string argument
	result := extractPromptArgString(req, "string_arg", "default")
	if result != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", result)
	}

	// Test empty string argument (should return default)
	result = extractPromptArgString(req, "empty_arg", "default")
	if result != "default" {
		t.Errorf("Expected 'default', got '%s'", result)
	}

	// Test missing argument (should return default)
	result = extractPromptArgString(req, "missing_arg", "default")
	if result != "default" {
		t.Errorf("Expected 'default', got '%s'", result)
	}

}

// TestAllPromptDefinitions tests that all prompt definitions are valid
func TestAllPromptDefinitions(t *testing.T) {
	prompts := []struct {
		name   string
		prompt mcp.Prompt
	}{
		{"CodeReviewPrompt", CodeReviewPrompt},
		{"ExplainCodePrompt", ExplainCodePrompt},
		{"DebugHelpPrompt", DebugHelpPrompt},
		{"RefactorSuggestionsPrompt", RefactorSuggestionsPrompt},
		{"ArchitectureAnalysisPrompt", ArchitectureAnalysisPrompt},
		{"DocGeneratePrompt", DocGeneratePrompt},
		{"TestGeneratePrompt", TestGeneratePrompt},
		{"SecurityAnalysisPrompt", SecurityAnalysisPrompt},
	}

	for _, p := range prompts {
		t.Run(p.name, func(t *testing.T) {
			if p.prompt.Name == "" {
				t.Errorf("Prompt %s has empty name", p.name)
			}

			if p.prompt.Description == "" {
				t.Errorf("Prompt %s has empty description", p.name)
			}
		})
	}
}
