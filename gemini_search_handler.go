package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

// GeminiSearchHandler is a handler for the gemini_search tool that uses mcp-go types directly
func (s *GeminiServer) GeminiSearchHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling gemini_search request with direct handler")

	// Extract and validate query parameter (required)
	query, err := validateRequiredString(req, "query")
	if err != nil {
		return createErrorResult(err.Error()), nil
	}

	// Extract optional system prompt - use search-specific prompt as default
	systemPrompt := extractArgumentString(req, "systemPrompt", s.config.GeminiSearchSystemPrompt)

	// Extract optional model parameter - use search-specific model as default
	modelName := extractArgumentString(req, "model", s.config.GeminiSearchModel)
	if err := ValidateModelID(modelName); err != nil {
		logger.Error("Invalid model requested: %v", err)
		return createErrorResult(fmt.Sprintf("Invalid model specified: %v", err)), nil
	}

	// Resolve model ID to ensure we use a valid API-addressable version
	resolvedModelID := ResolveModelID(modelName)
	if resolvedModelID != modelName {
		logger.Info("Resolved model ID from '%s' to '%s'", modelName, resolvedModelID)
		modelName = resolvedModelID
	}
	logger.Info("Using %s model for Google Search integration", modelName)

	// Get model information for context window and thinking capability
	modelInfo := GetModelByID(modelName)
	if modelInfo == nil {
		logger.Warn("Model information not found for %s, using default parameters", modelName)
	}

	// Create the generate content configuration
	googleSearch := &genai.GoogleSearch{}

	// Extract and validate time range filter parameters
	startTimeStr := extractArgumentString(req, "start_time", "")
	endTimeStr := extractArgumentString(req, "end_time", "")

	startTime, endTime, err := validateTimeRange(startTimeStr, endTimeStr)
	if err != nil {
		return createErrorResult(err.Error()), nil
	}

	// If both time parameters are provided, create the time range filter
	if startTime != nil && endTime != nil {
		// Create the time range filter
		googleSearch.TimeRangeFilter = &genai.Interval{
			StartTime: *startTime,
			EndTime:   *endTime,
		}
		logger.Info("Applying time range filter from %s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
		Temperature:       genai.Ptr(float32(s.config.GeminiTemperature)),
		Tools: []*genai.Tool{
			{
				GoogleSearch: googleSearch,
			},
		},
	}

	// Configure thinking if supported
	// Gemini 3 uses thinking_level, Gemini 2.5 uses thinking_budget
	enableThinking := extractArgumentBool(req, "enable_thinking", s.config.EnableThinking)
	if enableThinking && modelInfo != nil && modelInfo.SupportsThinking {
		thinkingConfig := &genai.ThinkingConfig{
			IncludeThoughts: true,
		}

		args := req.GetArguments()

		// Check if this is a Gemini 3 model
		if IsGemini3Model(modelName) {
			// Gemini 3: Use thinking_level parameter
			thinkingLevel := s.config.ThinkingLevel

			// Check for thinking_level parameter in request
			if levelStr, ok := args["thinking_level"].(string); ok && levelStr != "" {
				if validateThinkingLevel(levelStr) {
					thinkingLevel = strings.ToLower(levelStr)
					logger.Info("Setting thinking level to: %s", thinkingLevel)
				} else {
					logger.Warn("Invalid thinking_level '%s' (valid: 'low', 'high'). Using config default: %s", levelStr, s.config.ThinkingLevel)
				}
			}

			// Set thinking level for Gemini 3
			thinkingConfig.ThinkingLevel = genai.ThinkingLevel(thinkingLevel)
			logger.Info("Thinking mode enabled for search with level '%s' and Gemini 3 model %s", thinkingLevel, modelName)
		} else {
			// Gemini 2.5: Use legacy thinking_budget parameter
			thinkingBudget := 0

			// Check for level first
			if levelStr, ok := args["thinking_budget_level"].(string); ok && levelStr != "" {
				thinkingBudget = getThinkingBudgetFromLevel(levelStr)
				logger.Info("Setting thinking budget to %d tokens from level: %s", thinkingBudget, levelStr)
			} else if budgetRaw, ok := args["thinking_budget"].(float64); ok && budgetRaw >= 0 {
				// If explicit budget was provided
				thinkingBudget = int(budgetRaw)
				logger.Info("Setting thinking budget to %d tokens from explicit value", thinkingBudget)
			} else if s.config.ThinkingBudget > 0 {
				// Fall back to config value
				thinkingBudget = s.config.ThinkingBudget
				logger.Info("Using default thinking budget of %d tokens", thinkingBudget)
			}

			// Set thinking budget if greater than 0
			if thinkingBudget > 0 {
				budget := int32(thinkingBudget)
				thinkingConfig.ThinkingBudget = &budget
			}
			logger.Info("Thinking mode enabled for search with budget %d and Gemini 2.5 model %s", thinkingBudget, modelName)
		}

		config.ThinkingConfig = thinkingConfig
	} else if enableThinking {
		if modelInfo != nil {
			logger.Warn("Thinking mode requested but model %s doesn't support it", modelName)
		} else {
			logger.Warn("Thinking mode requested but unknown if model supports it")
		}
	}

	// Configure max tokens (50% of context window by default for search)
	configureMaxTokensOutput(ctx, config, req, modelInfo, 0.5)

	// Create content with the query
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResult("Internal error: Gemini client not properly initialized"), nil
	}

	// Initialize response data
	var sources []SourceInfo
	var searchQueries []string

	// Track seen URLs to avoid duplicates
	seenURLs := make(map[string]bool)

	// Non-streaming search request with metadata extraction
	resp, err := withRetry(ctx, s.config, logger, "gemini.models.generate_content", func(ctx context.Context) (*genai.GenerateContentResponse, error) {
		return s.client.Models.GenerateContent(ctx, modelName, contents, config)
	})
	if err != nil {
		logger.Error("Gemini Search API error: %v", err)
		return createErrorResult(fmt.Sprintf("Error from Gemini Search API: %v", err)), nil
	}

	responseText := processSearchResponse(resp, &sources, &searchQueries, seenURLs)

	// Build and return the search response
	result, err := buildSearchResponse(responseText, sources, searchQueries)
	if err != nil {
		logger.Error("Failed to build search response: %v", err)
		return createErrorResult(err.Error()), nil
	}

	return result, nil
}
