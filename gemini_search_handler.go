package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gomcpgo/mcp/pkg/protocol"
	"google.golang.org/genai"
)

// handleGeminiSearch handles requests to the gemini_search tool
func (s *GeminiServer) handleGeminiSearch(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)

	// Extract and validate query parameter (required)
	query, ok := req.Arguments["query"].(string)
	if !ok {
		return createErrorResponse("query must be a string"), nil
	}

	// Extract optional systemPrompt parameter
	systemPrompt := s.config.GeminiSearchSystemPrompt
	if customPrompt, ok := req.Arguments["systemPrompt"].(string); ok && customPrompt != "" {
		logger.Info("Using request-specific search system prompt")
		systemPrompt = customPrompt
	}

	// Extract optional model parameter - use the specific search model from config as default
	modelName := s.config.GeminiSearchModel
	if customModel, ok := req.Arguments["model"].(string); ok && customModel != "" {
		// Validate the custom model
		if err := ValidateModelID(customModel); err != nil {
			logger.Error("Invalid model requested: %v", err)
			return createErrorResponse(fmt.Sprintf("Invalid model specified: %v", err)), nil
		}
		logger.Info("Using request-specific model: %s", customModel)
		modelName = customModel
	}
	logger.Info("Using %s model for Google Search integration", modelName)

	// Check if thinking mode is requested
	enableThinking := s.config.EnableThinking
	if thinkingRaw, ok := req.Arguments["enable_thinking"].(bool); ok {
		enableThinking = thinkingRaw
	}

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

	// Check if max_tokens parameter was provided
	if maxTokensRaw, ok := req.Arguments["max_tokens"].(float64); ok && maxTokensRaw > 0 {
		maxTokens := int(maxTokensRaw)

		// Cap at model's maximum if available
		if modelInfo != nil && maxTokens > modelInfo.ContextWindowSize {
			logger.Warn("Requested max_tokens (%d) exceeds model's context window size (%d), capping at model limit",
				maxTokens, modelInfo.ContextWindowSize)
			maxTokens = modelInfo.ContextWindowSize
		}

		// Set the maximum output token limit
		config.MaxOutputTokens = int32(maxTokens)
		logger.Info("Setting max output tokens to %d", maxTokens)
	} else {
		// Set a safe default if not specified
		if modelInfo != nil {
			// For search queries, use a more conservative limit (50% of context)
			// as search results will take up context space too
			safeTokenLimit := int32(modelInfo.ContextWindowSize / 2)
			config.MaxOutputTokens = safeTokenLimit
			logger.Debug("Using default max output tokens: %d (50%% of context window)", safeTokenLimit)
		}
	}

	// Configure thinking mode if enabled and model supports it
	if enableThinking {
		if modelInfo != nil && modelInfo.SupportsThinking {
			thinkingConfig := &genai.ThinkingConfig{
				IncludeThoughts: true,
			}

			// Determine thinking budget - check for level first, then explicit value
			thinkingBudget := 0

			// Check if thinking_budget_level parameter was provided
			if levelStr, ok := req.Arguments["thinking_budget_level"].(string); ok && levelStr != "" {
				thinkingBudget = getThinkingBudgetFromLevel(levelStr)
				logger.Info("Setting thinking budget to %d tokens from level: %s for search request", thinkingBudget, levelStr)
			} else if budgetRaw, ok := req.Arguments["thinking_budget"].(float64); ok && budgetRaw >= 0 {
				// If explicit budget was provided, use that instead of level
				thinkingBudget = int(budgetRaw)
				logger.Info("Setting thinking budget to %d tokens from explicit value for search request", thinkingBudget)
			} else if s.config.ThinkingBudget > 0 {
				// Fall back to config value if neither level nor explicit budget provided
				thinkingBudget = s.config.ThinkingBudget
				logger.Info("Using default thinking budget of %d tokens from config for search request", thinkingBudget)
			}

			// Only set the thinking budget if it's greater than 0
			if thinkingBudget > 0 {
				budget := int32(thinkingBudget)
				thinkingConfig.ThinkingBudget = &budget
			}

			config.ThinkingConfig = thinkingConfig
			logger.Info("Thinking mode enabled for search request with model %s", modelName)
		} else {
			if modelInfo != nil {
				logger.Warn("Thinking mode requested but model %s doesn't support it", modelName)
			} else {
				logger.Warn("Thinking mode requested but unknown if model %s supports it", modelName)
			}
		}
	}

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
		return createErrorResponse("Internal error: Gemini client not properly initialized"), nil
	}

	// Stream the response
	for result, err := range s.client.Models.GenerateContentStream(ctx, modelName, contents, config) {
		if err != nil {
			logger.Error("Gemini Search API error: %v", err)
			return createErrorResponse(fmt.Sprintf("Error from Gemini Search API: %v", err)), nil
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
		return createErrorResponse(fmt.Sprintf("Failed to format search response: %v", err)), nil
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: string(responseJSON),
			},
		},
	}, nil
}
