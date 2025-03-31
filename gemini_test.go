package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gomcpgo/mcp/pkg/protocol"
	"github.com/google/generative-ai-go/genai"
)

// TestNewGeminiServer tests the creation of a new GeminiServer
func TestNewGeminiServer(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "empty API key",
			config: &Config{
				GeminiAPIKey: "",
				GeminiModel:  "gemini-pro",
			},
			expectError: true,
		},
		// Note: We can't easily test successful creation without mocking the genai.NewClient function
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewGeminiServer(context.Background(), tc.config)
			if tc.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("did not expect error but got: %v", err)
			}
		})
	}
}

// TestGeminiServerListTools tests the ListTools method of GeminiServer
func TestGeminiServerListTools(t *testing.T) {
	// Create a mock GeminiServer
	server := &GeminiServer{
		config: &Config{
			GeminiAPIKey: "test-key",
			GeminiModel:  "gemini-pro",
		},
		// We don't need a real client for this test
	}

	// Test ListTools
	resp, err := server.ListTools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify response
	if len(resp.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(resp.Tools))
	}
	if resp.Tools[0].Name != "research" {
		t.Errorf("expected tool name 'research', got '%s'", resp.Tools[0].Name)
	}
}

// TestErrorGeminiServerListTools tests the ListTools method of ErrorGeminiServer
func TestErrorGeminiServerListTools(t *testing.T) {
	// Create an ErrorGeminiServer
	server := &ErrorGeminiServer{
		errorMessage: "test error",
	}

	// Test ListTools
	resp, err := server.ListTools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify response
	if len(resp.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(resp.Tools))
	}
	if resp.Tools[0].Name != "research" {
		t.Errorf("expected tool name 'research', got '%s'", resp.Tools[0].Name)
	}
}

// TestErrorGeminiServerCallTool tests the CallTool method of ErrorGeminiServer
func TestErrorGeminiServerCallTool(t *testing.T) {
	// Create an ErrorGeminiServer
	errorMsg := "initialization failed"
	server := &ErrorGeminiServer{
		errorMessage: errorMsg,
	}

	// Test CallTool
	req := &protocol.CallToolRequest{
		Name: "research",
		Arguments: map[string]interface{}{
			"query": "test query",
		},
	}

	resp, err := server.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify response
	if !resp.IsError {
		t.Error("expected IsError to be true")
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(resp.Content))
	}
	if resp.Content[0].Text != errorMsg {
		t.Errorf("expected error message '%s', got '%s'", errorMsg, resp.Content[0].Text)
	}
}

// MockGenerateContentResponse creates a mock Gemini API response for testing
func MockGenerateContentResponse(content string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text(content),
					},
					Role: "model",
				},
			},
		},
	}
}

// TestFormatResponse tests the formatResponse method of GeminiServer
func TestFormatResponse(t *testing.T) {
	// Create a GeminiServer
	server := &GeminiServer{
		config: &Config{
			GeminiAPIKey: "test-key",
			GeminiModel:  "gemini-pro",
		},
	}

	// Create a mock response
	mockContent := "This is a test response from Gemini."
	mockResp := MockGenerateContentResponse(mockContent)

	// Test formatResponse
	resp := server.formatResponse(mockResp)

	// Verify response
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("expected content type 'text', got '%s'", resp.Content[0].Type)
	}
	if resp.Content[0].Text != mockContent {
		t.Errorf("expected content '%s', got '%s'", mockContent, resp.Content[0].Text)
	}
}

// TestGeminiServerCallTool_InvalidTool tests CallTool with an invalid tool name
func TestGeminiServerCallTool_InvalidTool(t *testing.T) {
	// Create a GeminiServer
	server := &GeminiServer{
		config: &Config{
			GeminiAPIKey: "test-key",
			GeminiModel:  "gemini-pro",
		},
	}

	// Test with invalid tool name
	req := &protocol.CallToolRequest{
		Name: "invalid_tool",
		Arguments: map[string]interface{}{
			"query": "test query",
		},
	}

	resp, err := server.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify response
	if !resp.IsError {
		t.Error("expected IsError to be true")
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(resp.Content))
	}
	if resp.Content[0].Text != "unknown tool: invalid_tool" {
		t.Errorf("expected error message 'unknown tool: invalid_tool', got '%s'", resp.Content[0].Text)
	}
}

// TestGeminiServerCallTool_InvalidArgument tests CallTool with an invalid argument
func TestGeminiServerCallTool_InvalidArgument(t *testing.T) {
	// Create a GeminiServer
	server := &GeminiServer{
		config: &Config{
			GeminiAPIKey: "test-key",
			GeminiModel:  "gemini-pro",
		},
	}

	// Test with invalid argument
	req := &protocol.CallToolRequest{
		Name: "research",
		Arguments: map[string]interface{}{
			"query": 123, // Not a string
		},
	}

	resp, err := server.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify response
	if !resp.IsError {
		t.Error("expected IsError to be true")
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(resp.Content))
	}
	if resp.Content[0].Text != "query must be a string" {
		t.Errorf("expected error message 'query must be a string', got '%s'", resp.Content[0].Text)
	}
}

// TestRetryWithBackoff tests the retry logic
func TestRetryWithBackoff(t *testing.T) {
	ctx := context.Background()
	logger := NewLogger(LevelDebug)

	tests := []struct {
		name          string
		maxRetries    int
		shouldSucceed bool
		operation     func() error
	}{
		{
			name:          "succeeds first time",
			maxRetries:    2,
			shouldSucceed: true,
			operation: func() error {
				return nil
			},
		},
		{
			name:          "succeeds after retries",
			maxRetries:    2,
			shouldSucceed: true,
			operation: func() func() error {
				attempts := 0
				return func() error {
					attempts++
					if attempts <= 1 {
						return errors.New("temporary error")
					}
					return nil
				}
			}(),
		},
		{
			name:          "never succeeds",
			maxRetries:    2,
			shouldSucceed: false,
			operation: func() error {
				return errors.New("permanent error")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := RetryWithBackoff(
				ctx,
				tc.maxRetries,
				10*time.Millisecond, // Small values for quick tests
				50*time.Millisecond,
				tc.operation,
				func(err error) bool { return true }, // All errors are retriable in this test
				logger,
			)

			if tc.shouldSucceed && err != nil {
				t.Errorf("expected success but got error: %v", err)
			}
			if !tc.shouldSucceed && err == nil {
				t.Error("expected error but got success")
			}
		})
	}
}
