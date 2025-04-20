package main

import (
	"context"
	"fmt"

	"github.com/gomcpgo/mcp/pkg/protocol"
	"google.golang.org/genai"
)

// handleQueryWithCache handles internal requests to query with a cached context
func (s *GeminiServer) handleQueryWithCache(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling query with cache request")

	// Check if caching is enabled
	if !s.config.EnableCaching {
		return createErrorResponse("caching is disabled"), nil
	}

	// Extract and validate required parameters
	cacheID, ok := req.Arguments["cache_id"].(string)
	if !ok || cacheID == "" {
		return createErrorResponse("cache_id must be a non-empty string"), nil
	}

	query, ok := req.Arguments["query"].(string)
	if !ok || query == "" {
		return createErrorResponse("query must be a non-empty string"), nil
	}

	// Get cache info
	cacheInfo, err := s.cacheStore.GetCache(ctx, cacheID)
	if err != nil {
		logger.Error("Failed to get cache info: %v", err)
		return createErrorResponse(fmt.Sprintf("failed to get cache: %v", err)), nil
	}

	// Send the query with cached content
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	config := &genai.GenerateContentConfig{
		CachedContent: cacheInfo.Name,
		Temperature:   genai.Ptr(float32(s.config.GeminiTemperature)),
	}

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResponse("Internal error: Gemini client not properly initialized"), nil
	}

	response, err := s.client.Models.GenerateContent(ctx, cacheInfo.Model, contents, config)
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		return createErrorResponse(fmt.Sprintf("error from Gemini API: %v", err)), nil
	}

	return s.formatResponse(response), nil
}
