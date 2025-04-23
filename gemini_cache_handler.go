// DEPRECATED: This file contains the legacy cache handler implementation using internal types.
// New code should use the direct handlers in direct_handlers.go instead.
// This implementation will be removed in a future version once all references
// have been updated to use the direct handlers.
package main

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// handleQueryWithCache handles internal requests to query with a cached context
// DEPRECATED: Use new direct handlers instead which use mcp-go types directly
func (s *GeminiServer) handleQueryWithCache(ctx context.Context, req *internalCallToolRequest) (*internalCallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling query with cache request")

	// Check if caching is enabled
	if !s.config.EnableCaching {
		return createErrorResponseWithMessage("caching is disabled"), nil
	}

	// Extract and validate required parameters
	cacheID, ok := req.Arguments["cache_id"].(string)
	if !ok || cacheID == "" {
		return createErrorResponseWithMessage("cache_id must be a non-empty string"), nil
	}

	query, ok := req.Arguments["query"].(string)
	if !ok || query == "" {
		return createErrorResponseWithMessage("query must be a non-empty string"), nil
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
