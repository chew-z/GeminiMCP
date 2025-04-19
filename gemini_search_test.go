package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gomcpgo/mcp/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

func TestHandleGeminiSearchWithModelParameter(t *testing.T) {
	// Create a mock client with tracking for the used model
	mockClient := &mockGeminiClient{
		lastUsedModel: "",
		mockResponse: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: genai.NewContentFromText("Test response", genai.RoleModel),
				},
			},
		},
	}

	// Create server with the mock client and config
	server := &GeminiServer{
		config: &Config{
			GeminiSearchModel: "gemini-2.0-flash",
			EnableThinking:    true,
		},
		client: mockClient,
	}

	// Test cases
	testCases := []struct {
		name          string
		args          map[string]interface{}
		expectedModel string
		expectError   bool
	}{
		{
			name: "Default model",
			args: map[string]interface{}{
				"query": "test query",
			},
			expectedModel: "gemini-2.0-flash",
			expectError:   false,
		},
		{
			name: "Custom model",
			args: map[string]interface{}{
				"query": "test query",
				"model": "gemini-2.5-pro-exp-03-25",
			},
			expectedModel: "gemini-2.5-pro-exp-03-25",
			expectError:   false,
		},
		{
			name: "Invalid model",
			args: map[string]interface{}{
				"query": "test query",
				"model": "invalid-model-name-that-will-fail-validation",
			},
			expectError: true,
		},
		{
			name: "With thinking enabled",
			args: map[string]interface{}{
				"query":           "test query",
				"model":           "gemini-2.5-pro-exp-03-25",
				"enable_thinking": true,
			},
			expectedModel: "gemini-2.5-pro-exp-03-25",
			expectError:   false,
		},
		{
			name: "With max tokens",
			args: map[string]interface{}{
				"query":      "test query",
				"model":      "gemini-2.0-flash",
				"max_tokens": float64(1000),
			},
			expectedModel: "gemini-2.0-flash",
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset the last used model
			mockClient.lastUsedModel = ""

			// Create the request
			req := &protocol.CallToolRequest{
				Name:      "gemini_search",
				Arguments: tc.args,
			}

			// Call the function
			resp, err := server.handleGeminiSearch(context.Background(), req)

			// Verify the result
			assert.NoError(t, err, "Function should not return an error")

			if tc.expectError {
				assert.True(t, resp.IsError, "Response should be an error")
			} else {
				assert.False(t, resp.IsError, "Response should not be an error")
				assert.Equal(t, tc.expectedModel, mockClient.lastUsedModel, "Should use the expected model")
			}
		})
	}
}

// Mock Gemini client for testing
type mockGeminiClient struct {
	lastUsedModel string
	mockResponse  *genai.GenerateContentResponse
}

// Mock implementation of client.Models.GenerateContentStream
func (m *mockGeminiClient) GenerateContentStream(
	ctx context.Context,
	model string,
	contents []*genai.Content,
	generateConfig *genai.GenerateContentConfig,
) <-chan genai.StreamingResponse {
	// Record the used model
	m.lastUsedModel = model

	// Create and return a channel with a single response
	resultChan := make(chan genai.StreamingResponse, 1)
	resultChan <- genai.StreamingResponse{
		Response: m.mockResponse,
		Err:      nil,
	}
	close(resultChan)
	return resultChan
}

// Ensure mockGeminiClient implements the necessary interface
var _ interface {
	GenerateContentStream(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) <-chan genai.StreamingResponse
} = (*mockGeminiClient)(nil)

// Sample response to parse
type SearchResponse struct {
	Answer        string       `json:"answer"`
	Sources       []SourceInfo `json:"sources,omitempty"`
	SearchQueries []string     `json:"search_queries,omitempty"`
}

// SourceInfo represents a source from search results
type SourceInfo struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"` // "web" or "retrieved_context"
}

// Helper function to parse the response
func parseSearchResponse(resp *protocol.CallToolResponse) (*SearchResponse, error) {
	if resp == nil || len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	content := resp.Content[0].Text
	var searchResp SearchResponse
	if err := json.Unmarshal([]byte(content), &searchResp); err != nil {
		return nil, err
	}

	return &searchResp, nil
}
