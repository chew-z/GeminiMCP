package main

import (
	"context"
	"testing"

	"github.com/gomcpgo/mcp/pkg/protocol"
	"google.golang.org/genai"
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
		// Note: We can't easily test successful creation without mocking the genai.NewClient constructor
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
	if len(resp.Tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(resp.Tools))
	}
	if resp.Tools[0].Name != "gemini_ask" {
		t.Errorf("expected tool name 'gemini_ask', got '%s'", resp.Tools[0].Name)
	}
	if resp.Tools[1].Name != "gemini_search" {
		t.Errorf("expected tool name 'gemini_search', got '%s'", resp.Tools[1].Name)
	}
	if resp.Tools[2].Name != "gemini_models" {
		t.Errorf("expected tool name 'gemini_models', got '%s'", resp.Tools[2].Name)
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
	if len(resp.Tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(resp.Tools))
	}
	if resp.Tools[0].Name != "gemini_ask" {
		t.Errorf("expected tool name 'gemini_ask', got '%s'", resp.Tools[0].Name)
	}
	if resp.Tools[1].Name != "gemini_search" {
		t.Errorf("expected tool name 'gemini_search', got '%s'", resp.Tools[1].Name)
	}
	if resp.Tools[2].Name != "gemini_models" {
		t.Errorf("expected tool name 'gemini_models', got '%s'", resp.Tools[2].Name)
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
		Name: "gemini_ask",
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
					Parts: []*genai.Part{
						{Text: content},
					},
					Role: genai.RoleModel,
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
		Name: "gemini_ask",
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
