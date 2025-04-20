package main

import (
	"context"
	"encoding/json"

	"github.com/gomcpgo/mcp/pkg/protocol"
	"google.golang.org/genai"
)

// getLoggerFromContext safely extracts a logger from the context or creates a new one
func getLoggerFromContext(ctx context.Context) Logger {
	loggerValue := ctx.Value(loggerKey)
	if loggerValue != nil {
		if l, ok := loggerValue.(Logger); ok {
			return l
		}
	}
	// Create a new logger if one isn't in the context or type assertion fails
	return NewLogger(LevelInfo)
}

// createErrorResponse creates a standardized error response
func createErrorResponse(message string) *protocol.CallToolResponse {
	return &protocol.CallToolResponse{
		IsError: true,
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: message,
			},
		},
	}
}

// formatResponse formats the Gemini API response
func (s *GeminiServer) formatResponse(resp *genai.GenerateContentResponse) *protocol.CallToolResponse {
	var content string
	thinking := ""

	// Extract text from the response
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		content = resp.Text()

		// Try to extract thinking output from candidate
		candidate := resp.Candidates[0]
		// The ThinkingOutput field is not directly exposed in the Go API
		// We'll need to check the raw JSON if available
		if data, err := json.Marshal(candidate); err == nil {
			var candidateMap map[string]interface{}
			if err := json.Unmarshal(data, &candidateMap); err == nil {
				if thinkingOutput, ok := candidateMap["thinkingOutput"].(map[string]interface{}); ok {
					if thinkingText, ok := thinkingOutput["thinking"].(string); ok {
						thinking = thinkingText
					}
				}
			}
		}
	}

	// Check for empty content and provide a fallback message
	if content == "" {
		content = "The Gemini model returned an empty response. This might indicate that the model couldn't generate an appropriate response for your query. Please try rephrasing your question or providing more context."
	}

	// If thinking output was found, include it in the response
	if thinking != "" {
		// Create a JSON response with thinking included
		thinkingResp := map[string]string{
			"answer":   content,
			"thinking": thinking,
		}

		// Convert to JSON
		thinkingJSON, err := json.Marshal(thinkingResp)
		if err == nil {
			return &protocol.CallToolResponse{
				Content: []protocol.ToolContent{
					{
						Type: "text",
						Text: string(thinkingJSON),
					},
				},
			}
		}
		// Fall back to just content if JSON conversion fails
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: content,
			},
		},
	}
}
