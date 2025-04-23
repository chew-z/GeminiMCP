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
// Note: This method is no longer needed since tools are now registered directly
// in main.go using the shared definitions from tools.go
func (s *GeminiServer) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	tools := []mcp.Tool{
		GeminiAskTool,
		GeminiSearchTool,
		GeminiModelsTool,
	}
	return tools, nil
}

// CallTool implements the ToolHandler interface for GeminiServer
// Note: This method is no longer needed since handlers are now registered directly
// in main.go. It's kept for potential backwards compatibility.
func (s *GeminiServer) CallTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// We now register handlers directly, so this shouldn't be called anymore
	return mcp.NewToolResultError("This method is deprecated. Tool handlers are now registered directly."), nil
}
