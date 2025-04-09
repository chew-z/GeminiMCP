package main

import (
	"context"
	"encoding/json"

	"github.com/gomcpgo/mcp/pkg/protocol"
)

// GeminiServer implements the ToolHandler interface to provide research capabilities
// through Google's Gemini API.
// Defined in gemini.go

// ErrorGeminiServer implements the ToolHandler interface but returns error responses
// for all calls. Used when the Gemini server is in degraded mode due to initialization errors.
type ErrorGeminiServer struct {
	errorMessage string
	config       *Config // Added to check EnableCaching
}

// ListTools implements the ToolHandler interface for ErrorGeminiServer
// Returns the same tool signature as the normal Gemini server but in error mode
func (s *ErrorGeminiServer) ListTools(ctx context.Context) (*protocol.ListToolsResponse, error) {
	tools := []protocol.Tool{
		{
			Name:        "gemini_ask",
			Description: "Use Google's Gemini AI model to ask about complex coding problems",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "The coding problem that we are asking Gemini AI to work on [question + code]"
					},
					"model": {
						"type": "string",
						"description": "Optional: Specific Gemini model to use (overrides default configuration)"
					},
					"systemPrompt": {
						"type": "string",
						"description": "Optional: Custom system prompt to use for this request (overrides default configuration)"
					},
					"file_paths": {
						"type": "array",
						"items": {
							"type": "string"
						},
						"description": "Optional: Paths to files to include in the request context"
					},
					"use_cache": {
						"type": "boolean",
						"description": "Optional: Whether to try using a cache for this request (only works with compatible models)"
					},
					"cache_ttl": {
						"type": "string",
						"description": "Optional: TTL for cache if created (e.g., '10m', '1h'). Default is 10 minutes"
					}
				},
				"required": ["query"]
			}`),
		},
		{
			Name:        "gemini_search",
			Description: "Use Google's Gemini AI model with Google Search to answer questions with grounded information",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "The question to ask Gemini using Google Search for grounding"
					},
					"systemPrompt": {
						"type": "string",
						"description": "Optional: Custom system prompt to use for this request (overrides default configuration)"
					}
				},
				"required": ["query"]
			}`),
		},
		{
			Name:        "gemini_models",
			Description: "List available Gemini models with descriptions",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {},
				"required": []
			}`),
		},
	}

	return &protocol.ListToolsResponse{
		Tools: tools,
	}, nil
}

// CallTool implements the ToolHandler interface for ErrorGeminiServer
// Always returns the initialization error regardless of which tool is called
func (s *ErrorGeminiServer) CallTool(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	// Log which tool was attempted, if a logger is in the context
	if loggerValue := ctx.Value(loggerKey); loggerValue != nil {
		if logger, ok := loggerValue.(Logger); ok {
			logger.Info("Tool '%s' called in error mode", req.Name)
		}
	}

	// Return the same error message regardless of which tool is called
	return &protocol.CallToolResponse{
		IsError: true,
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: s.errorMessage,
			},
		},
	}, nil
}
