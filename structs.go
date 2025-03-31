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
		{
			Name:        "gemini_upload_file",
			Description: "Upload a file to Gemini for processing",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"filename": {"type": "string", "description": "Name of the file"},
					"mime_type": {"type": "string", "description": "MIME type of the file"},
					"content": {"type": "string", "description": "Base64-encoded file content"},
					"display_name": {"type": "string", "description": "Optional human-readable name for the file"}
				},
				"required": ["filename", "mime_type", "content"]
			}`),
		},
		{
			Name:        "gemini_list_files",
			Description: "List all uploaded files",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {},
				"required": []
			}`),
		},
		{
			Name:        "gemini_delete_file",
			Description: "Delete an uploaded file",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_id": {"type": "string", "description": "ID of the file to delete"}
				},
				"required": ["file_id"]
			}`),
		},
	}
	
	// Add cache tools if caching is enabled
	if s.config != nil && s.config.EnableCaching {
		tools = append(tools, []protocol.Tool{
			{
				Name:        "gemini_create_cache",
				Description: "Create a cached context for repeated queries",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"model": {"type": "string", "description": "Gemini model to use"},
						"system_prompt": {"type": "string", "description": "Optional system prompt for the context"},
						"file_ids": {"type": "array", "items": {"type": "string"}, "description": "Optional IDs of files to include in the context"},
						"content": {"type": "string", "description": "Optional text content to include in the context"},
						"ttl": {"type": "string", "description": "Optional time-to-live for the cache (e.g. '1h', '24h')"},
						"display_name": {"type": "string", "description": "Optional human-readable name for the cache"}
					},
					"required": ["model"]
				}`),
			},
			{
				Name:        "gemini_query_with_cache",
				Description: "Query Gemini using a cached context",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"cache_id": {"type": "string", "description": "ID of the cache to use"},
						"query": {"type": "string", "description": "Query to send to Gemini"}
					},
					"required": ["cache_id", "query"]
				}`),
			},
			{
				Name:        "gemini_list_caches",
				Description: "List all cached contexts",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {},
					"required": []
				}`),
			},
			{
				Name:        "gemini_delete_cache",
				Description: "Delete a cached context",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"cache_id": {"type": "string", "description": "ID of the cache to delete"}
					},
					"required": ["cache_id"]
				}`),
			},
		}...)
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