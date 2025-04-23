package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
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
func (s *GeminiServer) ListTools(ctx context.Context) ([]mcp.Tool, error) {

	tools := []mcp.Tool{
		mcp.NewTool(
			"gemini_ask",
			mcp.WithDescription("Use Google's Gemini AI model to ask about complex coding problems"),
			mcp.WithString("query", mcp.Required(), mcp.Description("The coding problem that we are asking Gemini AI to work on [question + code]")),
			mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
			mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
			mcp.WithArray("file_paths", mcp.Description("Optional: Paths to files to include in the request context")),
			mcp.WithBoolean("use_cache", mcp.Description("Optional: Whether to try using a cache for this request (only works with compatible models)")),
			mcp.WithString("cache_ttl", mcp.Description("Optional: TTL for cache if created (e.g., '10m', '1h'). Default is 10 minutes")),
			mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process (only works with Pro models)")),
			mcp.WithNumber("thinking_budget", mcp.Description("Optional: Maximum number of tokens to allocate for the model's thinking process (0-24576)")),
			mcp.WithString("thinking_budget_level", mcp.Description("Optional: Predefined thinking budget level (none, low, medium, high)")),
			mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
		),
		mcp.NewTool(
			"gemini_search",
			mcp.WithDescription("Use Google's Gemini AI model with Google Search to answer questions with grounded information"),
			mcp.WithString("query", mcp.Required(), mcp.Description("The question to ask Gemini using Google Search for grounding")),
			mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
			mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process (when supported)")),
			mcp.WithNumber("thinking_budget", mcp.Description("Optional: Maximum number of tokens to allocate for the model's thinking process (0-24576)")),
			mcp.WithString("thinking_budget_level", mcp.Description("Optional: Predefined thinking budget level (none, low, medium, high)")),
			mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
			mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
		),
		mcp.NewTool(
			"gemini_models",
			mcp.WithDescription("List available Gemini models with descriptions"),
		),
	}

	return tools, nil
}

// CallTool implements the ToolHandler interface for GeminiServer
func (s *GeminiServer) CallTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	switch req.Params.Name {
	case "gemini_ask":
		// Convert to internal request
		internalReq := &internalCallToolRequest{
			Name:      req.Params.Name,
			Arguments: req.Params.Arguments,
		}
		resp, err := s.handleAskGemini(ctx, internalReq)
		if err != nil {
			return nil, err
		}
		return convertToMCPResult(resp), nil
	case "gemini_search":
		// Convert to internal request
		internalReq := &internalCallToolRequest{
			Name:      req.Params.Name,
			Arguments: req.Params.Arguments,
		}
		resp, err := s.handleGeminiSearch(ctx, internalReq)
		if err != nil {
			return nil, err
		}
		return convertToMCPResult(resp), nil
	case "gemini_models":
		resp, err := s.handleGeminiModels(ctx)
		if err != nil {
			return nil, err
		}
		return convertToMCPResult(resp), nil
	default:
		return mcp.NewToolResultError(fmt.Sprintf("unknown tool: %s", req.Params.Name)), nil
	}
}
