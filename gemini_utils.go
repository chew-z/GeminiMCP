package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

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

// Helper function to get MIME type from file path
func getMimeTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	case ".wav":
		return "audio/wav"
	case ".doc", ".docx":
		return "application/msword"
	case ".xls", ".xlsx":
		return "application/vnd.ms-excel"
	case ".ppt", ".pptx":
		return "application/vnd.ms-powerpoint"
	case ".zip":
		return "application/zip"
	case ".csv":
		return "text/csv"
	case ".go":
		return "text/plain" // Changed from "text/x-go" to "text/plain"
	case ".py":
		return "text/plain" // Changed from "text/x-python" to "text/plain"
	case ".java":
		return "text/plain" // Changed from "text/x-java" to "text/plain"
	case ".c", ".cpp", ".h", ".hpp":
		return "text/plain" // Changed from "text/x-c" to "text/plain"
	case ".rb":
		return "text/plain"
	case ".php":
		return "text/plain"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
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
