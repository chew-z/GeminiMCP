package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// createTempCodeFile creates a temporary file with the given content for testing
func createTempCodeFile(content string, extension string) (string, func(), error) {
	tmpFile, err := os.CreateTemp("", "test_code_*"+extension)
	if err != nil {
		return "", nil, err
	}

	_, err = tmpFile.Write([]byte(content))
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", nil, err
	}

	tmpFile.Close()

	// Return cleanup function
	cleanup := func() {
		os.Remove(tmpFile.Name())
	}

	return tmpFile.Name(), cleanup, nil
}

// createTestConfig creates a complete test configuration with prompt defaults
func createTestConfig() *Config {
	return &Config{
		GeminiAPIKey:            "test-key",
		GeminiModel:             "gemini-2.5-pro",
		GeminiSystemPrompt:      "You are a helpful assistant.",
		MaxFileSize:             1024 * 1024, // 1MB for testing
		PromptDefaultAudience:   "intermediate",
		PromptDefaultFocus:      "general",
		PromptDefaultSeverity:   "warning",
		PromptDefaultDocFormat:  "markdown",
		PromptDefaultFramework:  "standard",
		PromptDefaultCoverage:   "comprehensive",
		PromptDefaultCompliance: "OWASP",
	}
}

// TestCodeReviewPrompt tests the code review prompt
func TestCodeReviewPrompt(t *testing.T) {
	// Create test file
	testCode := "function test() { return 'hello'; }"
	tempFile, cleanup, err := createTempCodeFile(testCode, ".js")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer cleanup()

	// Create GeminiServer with test config
	config := createTestConfig()
	ctx := context.Background()
	ctx = context.WithValue(ctx, loggerKey, NewLogger(LevelInfo))

	geminiSvc, err := NewGeminiServer(ctx, config)
	if err != nil {
		t.Skipf("Skipping test - could not create GeminiServer: %v", err)
	}

	// Test with required argument (using file path)
	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "code_review",
			Arguments: map[string]string{
				"files": tempFile,
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

	// Verify the result contains file content
	if len(result.Messages) > 0 {
		content := result.Messages[0].Content.(mcp.TextContent).Text
		if !strings.Contains(content, testCode) {
			t.Error("Expected result to contain file content")
		}
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

	if result.Description != "Error: files argument is required" {
		t.Errorf("Expected error message, got: %s", result.Description)
	}
}

// TestExplainCodePrompt tests the explain code prompt
func TestExplainCodePrompt(t *testing.T) {
	// Create test file with fibonacci code
	testCode := "function fibonacci(n) { return n <= 1 ? n : fibonacci(n-1) + fibonacci(n-2); }"
	tempFile, cleanup, err := createTempCodeFile(testCode, ".js")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer cleanup()

	config := createTestConfig()
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
				"files":    tempFile,
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
				if len(content.Text) > 0 && strings.Contains(content.Text, "beginner") && strings.Contains(content.Text, testCode) {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Error("Expected user message with content including audience info and file content")
	}
}

// TestDebugHelpPrompt tests the debug help prompt
func TestDebugHelpPrompt(t *testing.T) {
	// Create test file with problematic code
	testCode := "x = 1/0"
	tempFile, cleanup, err := createTempCodeFile(testCode, ".py")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer cleanup()

	config := createTestConfig()
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
				"files":             tempFile,
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

	// Verify the result contains file content and error information
	if len(result.Messages) > 0 {
		content := result.Messages[0].Content.(mcp.TextContent).Text
		if !strings.Contains(content, testCode) || !strings.Contains(content, "ZeroDivisionError") {
			t.Error("Expected result to contain file content and error message")
		}
	}
}

// TestSecurityAnalysisPrompt tests the security analysis prompt
func TestSecurityAnalysisPrompt(t *testing.T) {
	// Create test file with SQL injection vulnerability
	testCode := "SELECT * FROM users WHERE id = '" + "user_input" + "'"
	tempFile, cleanup, err := createTempCodeFile(testCode, ".sql")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer cleanup()

	config := createTestConfig()
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
				"files":         tempFile,
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

	// Verify the result contains file content and uses config defaults
	if len(result.Messages) > 0 {
		content := result.Messages[0].Content.(mcp.TextContent).Text
		if !strings.Contains(content, testCode) || !strings.Contains(content, "OWASP") {
			t.Error("Expected result to contain file content and config default compliance (OWASP)")
		}
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

// TestFileUtilities tests the file reading and expansion utilities
func TestFileUtilities(t *testing.T) {
	// Create a temporary directory with multiple files
	tempDir, err := os.MkdirTemp("", "test_dir_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	files := map[string]string{
		"main.go":     "package main\nfunc main() { println(\"Hello\") }",
		"utils.js":    "function helper() { return true; }",
		"README.md":   "# Test Project",
		"config.json": `{"debug": true}`,
	}

	for filename, content := range files {
		filepath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filepath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Test expandFilePaths with directory
	expandedPaths, err := expandFilePaths([]string{tempDir})
	if err != nil {
		t.Fatalf("expandFilePaths failed: %v", err)
	}

	if len(expandedPaths) == 0 {
		t.Error("Expected expanded paths, got none")
	}

	// Test readLocalFiles
	config := createTestConfig()
	content, lang, err := readLocalFiles(expandedPaths, config.MaxFileSize)
	if err != nil {
		t.Fatalf("readLocalFiles failed: %v", err)
	}

	if content == "" {
		t.Error("Expected file content, got empty string")
	}

	if lang == "" {
		t.Error("Expected detected language, got empty string")
	}

	// Verify content contains data from multiple files
	for _, expectedContent := range files {
		if !strings.Contains(content, expectedContent) {
			t.Errorf("Expected content to contain %s", expectedContent)
		}
	}
}

// TestDirectoryExpansionWithPrompt tests using a directory with a prompt
func TestDirectoryExpansionWithPrompt(t *testing.T) {
	// Create a temporary directory with code files
	tempDir, err := os.MkdirTemp("", "test_prompt_dir_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	testCode := "class User { constructor(name) { this.name = name; } }"
	jsFile := filepath.Join(tempDir, "user.js")
	err = os.WriteFile(jsFile, []byte(testCode), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := createTestConfig()
	ctx := context.Background()
	ctx = context.WithValue(ctx, loggerKey, NewLogger(LevelInfo))

	geminiSvc, err := NewGeminiServer(ctx, config)
	if err != nil {
		t.Skipf("Skipping test - could not create GeminiServer: %v", err)
	}

	// Test code review with directory path
	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: "code_review",
			Arguments: map[string]string{
				"files": tempDir, // Use directory path
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

	// Verify the result contains file content from directory
	if len(result.Messages) > 0 {
		content := result.Messages[0].Content.(mcp.TextContent).Text
		if !strings.Contains(content, testCode) {
			t.Error("Expected result to contain file content from directory")
		}
	}
}
