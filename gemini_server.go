package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gomcpgo/mcp/pkg/protocol"
	"google.golang.org/genai"
)

// NewGeminiServer creates a new GeminiServer with the provided configuration
func NewGeminiServer(ctx context.Context, config *Config) (*GeminiServer, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	if config.GeminiAPIKey == "" {
		return nil, errors.New("Gemini API key is required")
	}

	// Initialize the Gemini client
	clientConfig := &genai.ClientConfig{
		APIKey: config.GeminiAPIKey,
	}
	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	// Create the file and cache stores
	fileStore := NewFileStore(client, config)
	cacheStore := NewCacheStore(client, config, fileStore)

	return &GeminiServer{
		config:     config,
		client:     client,
		fileStore:  fileStore,
		cacheStore: cacheStore,
	}, nil
}

// Close closes the Gemini client connection (client doesn't need to be closed in the new API)
func (s *GeminiServer) Close() {
	// No need to close the client in the new API
}

// ListTools implements the ToolHandler interface for GeminiServer
func (s *GeminiServer) ListTools(ctx context.Context) (*protocol.ListToolsResponse, error) {
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
					},
					"enable_thinking": {
						"type": "boolean",
						"description": "Optional: Enable thinking mode to see model's reasoning process (only works with Pro models)"
					},
					"thinking_budget": {
						"type": "integer",
						"description": "Optional: Maximum number of tokens to allocate for the model's thinking process (0-24576)"
					},
					"thinking_budget_level": {
						"type": "string",
						"enum": ["none", "low", "medium", "high"],
						"description": "Optional: Predefined thinking budget level (none: 0, low: 4096, medium: 16384, high: 24576)"
					},
					"max_tokens": {
						"type": "integer",
						"description": "Optional: Maximum token limit for the response. Default is determined by the model"
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
					},
					"enable_thinking": {
						"type": "boolean",
						"description": "Optional: Enable thinking mode to see model's reasoning process (when supported)"
					},
					"thinking_budget": {
						"type": "integer",
						"description": "Optional: Maximum number of tokens to allocate for the model's thinking process (0-24576)"
					},
					"thinking_budget_level": {
						"type": "string",
						"enum": ["none", "low", "medium", "high"],
						"description": "Optional: Predefined thinking budget level (none: 0, low: 4096, medium: 16384, high: 24576)"
					},
					"max_tokens": {
						"type": "integer",
						"description": "Optional: Maximum token limit for the response. Default is determined by the model"
					},
					"model": {
						"type": "string",
						"description": "Optional: Specific Gemini model to use (overrides default configuration)"
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

// CallTool implements the ToolHandler interface for GeminiServer
func (s *GeminiServer) CallTool(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	switch req.Name {
	case "gemini_ask":
		return s.handleAskGemini(ctx, req)
	case "gemini_search":
		return s.handleGeminiSearch(ctx, req)
	case "gemini_models":
		return s.handleGeminiModels(ctx)
	default:
		return createErrorResponse(fmt.Sprintf("unknown tool: %s", req.Name)), nil
	}
}
