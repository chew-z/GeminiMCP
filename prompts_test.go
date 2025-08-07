package main

import (
	"context"
	"encoding/json"
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
		ProjectLanguage:         "go",
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
	tempFile, cleanup, err := createTempCodeFile("function test() { return 'hello'; }", ".js")
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

	// Verify the result is a valid JSON template
	var template PromptTemplate
	err = json.Unmarshal([]byte(result.Description), &template)
	if err != nil {
		t.Fatalf("Failed to unmarshal prompt template: %v", err)
	}

	if template.SystemPrompt == "" {
		t.Error("Expected system prompt in template")
	}

	if !strings.Contains(template.UserPromptTemplate, "{{file_content}}") {
		t.Error("Expected user prompt template to contain placeholder")
	}

	if len(template.FilePaths) != 1 || template.FilePaths[0] != tempFile {
		t.Errorf("Expected file path %s in template, got %v", tempFile, template.FilePaths)
	}
}

// TestExplainCodePrompt tests the explain code prompt
func TestExplainCodePrompt(t *testing.T) {
	// Create test file with fibonacci code
	tempFile, cleanup, err := createTempCodeFile("function fibonacci(n) { return n <= 1 ? n : fibonacci(n-1) + fibonacci(n-2); }", ".js")
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

	// Verify the result is a valid JSON template
	var template PromptTemplate
	err = json.Unmarshal([]byte(result.Description), &template)
	if err != nil {
		t.Fatalf("Failed to unmarshal prompt template: %v", err)
	}

	if !strings.Contains(template.SystemPrompt, "beginner") {
		t.Error("Expected system prompt to contain audience info")
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
		"config.json": `{ \"debug\": true }`,
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

	if len(expandedPaths) != len(files) {
		t.Errorf("Expected %d expanded paths, got %d", len(files), len(expandedPaths))
	}
}
