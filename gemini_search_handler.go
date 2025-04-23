package main

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"
)

// handleGeminiSearch handles requests to the gemini_search tool
func (s *GeminiServer) handleGeminiSearch(ctx context.Context, req *internalCallToolRequest) (*internalCallToolResponse, error) {
	logger := getLoggerFromContext(ctx)

	// Extract and validate query parameter (required)
	query, ok := req.Arguments["query"].(string)
	if !ok {
		return createErrorResponseWithMessage("query must be a string"), nil
	}

	// Extract optional systemPrompt parameter for search
	systemPrompt := extractSystemPrompt(ctx, req.Arguments, s.config.GeminiSearchSystemPrompt)

	// Extract optional model parameter - use the specific search model from config as default
	modelName := extractModelParam(ctx, req.Arguments, s.config.GeminiSearchModel)
	logger.Info("Using %s model for Google Search integration", modelName)

	// Check if thinking mode is requested
	enableThinking := extractBoolParam(req.Arguments, "enable_thinking", s.config.EnableThinking)

	// Get model information for capabilities and context window
	modelInfo := GetModelByID(modelName)
	if modelInfo == nil {
		logger.Warn("Model information not found for %s, using default parameters", modelName)
	}

	// Create the generate content configuration
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
		Temperature:       genai.Ptr(float32(s.config.GeminiTemperature)),
		Tools: []*genai.Tool{
			{
				GoogleSearch: &genai.GoogleSearch{},
			},
		},
	}

	// Configure thinking
	configureThinking(ctx, config, req.Arguments, modelInfo, enableThinking, s.config.ThinkingBudget)

	// Configure max tokens (50% of context window by default for search queries)
	configureMaxTokens(ctx, config, req.Arguments, modelInfo, 0.5)

	// Create query content
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	// Initialize response data
	responseText := ""
	var sources []SourceInfo
	var searchQueries []string

	// Track seen URLs to avoid duplicates
	seenURLs := make(map[string]bool)

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResponseWithMessage("Internal error: Gemini client not properly initialized"), nil
	}

	// Stream the response
	for result, err := range s.client.Models.GenerateContentStream(ctx, modelName, contents, config) {
		if err != nil {
			logger.Error("Gemini Search API error: %v", err)
			return createErrorResponseWithMessage(fmt.Sprintf("Error from Gemini Search API: %v", err)), nil
		}

		// Extract text from each candidate
		if len(result.Candidates) > 0 && result.Candidates[0].Content != nil {
			responseText += result.Text()

			// Extract metadata if available
			if metadata := result.Candidates[0].GroundingMetadata; metadata != nil {
				// Collect search queries
				if len(metadata.WebSearchQueries) > 0 && len(searchQueries) == 0 {
					searchQueries = metadata.WebSearchQueries
				}

				// Collect sources from grounding chunks
				for _, chunk := range metadata.GroundingChunks {
					var source SourceInfo

					if web := chunk.Web; web != nil {
						// Skip if we've seen this URL already
						if seenURLs[web.URI] {
							continue
						}

						source = SourceInfo{
							Title: web.Title,
							URL:   web.URI,
							Type:  "web",
						}
						seenURLs[web.URI] = true
					} else if ctx := chunk.RetrievedContext; ctx != nil {
						// Skip if we've seen this URL already
						if seenURLs[ctx.URI] {
							continue
						}

						source = SourceInfo{
							Title: ctx.Title,
							URL:   ctx.URI,
							Type:  "retrieved_context",
						}
						seenURLs[ctx.URI] = true
					}

					if source.URL != "" {
						sources = append(sources, source)
					}
				}
			}
		}
	}

	// Check for empty content and provide a fallback message
	if responseText == "" {
		responseText = `The Gemini Search model returned an empty response.
			This might indicate an issue with the search functionality or that
			no relevant information was found. Please try rephrasing your question
			or providing more specific details.`
	}

	// Create the response JSON
	response := SearchResponse{
		Answer:        responseText,
		Sources:       sources,
		SearchQueries: searchQueries,
	}

	// Convert to JSON
	responseJSON, err := json.Marshal(response)
	if err != nil {
		logger.Error("Failed to marshal search response: %v", err)
		return createErrorResponseWithMessage(fmt.Sprintf("Failed to format search response: %v", err)), nil
	}

	return &internalCallToolResponse{
		Content: []internalToolContent{
			{
				Type: "text",
				Text: string(responseJSON),
			},
		},
	}, nil
}
