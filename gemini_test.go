//go:build ignore
// +build ignore

package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
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
	if len(resp) != 3 {
		t.Errorf("expected 3 tools, got %d", len(resp))
	}
	if resp[0].Name != "gemini_ask" {
		t.Errorf("expected tool name 'gemini_ask', got '%s'", resp[0].Name)
	}
	if resp[1].Name != "gemini_search" {
		t.Errorf("expected tool name 'gemini_search', got '%s'", resp[1].Name)
	}
	if resp[2].Name != "gemini_models" {
		t.Errorf("expected tool name 'gemini_models', got '%s'", resp[2].Name)
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
	if len(resp) != 3 {
		t.Errorf("expected 3 tools, got %d", len(resp))
	}
	if resp[0].Name != "gemini_ask" {
		t.Errorf("expected tool name 'gemini_ask', got '%s'", resp[0].Name)
	}
	if resp[1].Name != "gemini_search" {
		t.Errorf("expected tool name 'gemini_search', got '%s'", resp[1].Name)
	}
	if resp[2].Name != "gemini_models" {
		t.Errorf("expected tool name 'gemini_models', got '%s'", resp[2].Name)
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
	req := mcp.CallToolRequest{}
	req.Params.Name = "gemini_ask"
	req.Params.Arguments = map[string]interface{}{
		"query": "test query",
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
	content, ok := resp.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected content type TextContent, got %T", resp.Content[0])
	}
	if content.Text != errorMsg {
		t.Errorf("expected error message '%s', got '%s'", errorMsg, content.Text)
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

// TestFormatMCPResponse tests the formatMCPResponse method of GeminiServer
func TestFormatMCPResponse(t *testing.T) {
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

	// Test formatMCPResponse
	resp := server.formatMCPResponse(mockResp)

	// Verify response
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(resp.Content))
	}
	content, ok := resp.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected content type TextContent, got %T", resp.Content[0])
	}
	if content.Text != mockContent {
		t.Errorf("expected content '%s', got '%s'", mockContent, content.Text)
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
	req := mcp.CallToolRequest{}
	req.Params.Name = "invalid_tool"
	req.Params.Arguments = map[string]interface{}{
		"query": "test query",
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
	content, ok := resp.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected content type TextContent, got %T", resp.Content[0])
	}
	if content.Text != "unknown tool: invalid_tool" {
		t.Errorf("expected error message 'unknown tool: invalid_tool', got '%s'", content.Text)
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
	req := mcp.CallToolRequest{}
	req.Params.Name = "gemini_ask"
	req.Params.Arguments = map[string]interface{}{
		"query": 123, // Not a string
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
	content, ok := resp.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected content type TextContent, got %T", resp.Content[0])
	}
	if content.Text != "query must be a string" {
		t.Errorf("expected error message 'query must be a string', got '%s'", content.Text)
	}
}
