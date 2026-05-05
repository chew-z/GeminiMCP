package main

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

// buildSearchConfig assembles the Gemini search generation config from the
// request and model metadata.
func buildSearchConfig(
	ctx context.Context,
	s *GeminiServer,
	req mcp.CallToolRequest,
	modelName string,
	modelInfo *GeminiModelInfo,
) (*genai.GenerateContentConfig, error) {
	logger := getLoggerFromContext(ctx)
	googleSearch := &genai.GoogleSearch{}

	startTimeStr := extractArgumentString(req, "start_time", "")
	endTimeStr := extractArgumentString(req, "end_time", "")
	startTime, endTime, err := validateTimeRange(startTimeStr, endTimeStr)
	if err != nil {
		return nil, err
	}
	if startTime != nil && endTime != nil {
		googleSearch.TimeRangeFilter = &genai.Interval{
			StartTime: *startTime,
			EndTime:   *endTime,
		}
		logger.Info("Applying time range filter from %s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPromptSearch, ""),
		Temperature:       genai.Ptr(float32(s.config.GeminiTemperature)),
		Tools: []*genai.Tool{
			{GoogleSearch: googleSearch},
		},
	}
	config.ServiceTier = serviceTierFromString(s.config.ServiceTier)

	configureThinking(ctx, req, config, s.config.SearchThinkingLevel, modelInfo, modelName)
	configureMaxTokensOutput(ctx, config, req, modelInfo, 0.5)
	return config, nil
}

// GeminiSearchHandler is a handler for the gemini_search tool that uses mcp-go types directly
func (s *GeminiServer) GeminiSearchHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Debug("handling gemini_search request with direct handler")

	query, err := validateRequiredString(req, "query")
	if err != nil {
		return createErrorResult(err.Error()), nil
	}

	modelName := extractArgumentString(req, "model", s.config.GeminiSearchModel)
	modelName, err = resolveAndValidateModel(ctx, modelName)
	if err != nil {
		return createErrorResult(err.Error()), nil
	}
	logger.Info("Using %s model for web search", modelName)

	modelInfo := GetModelByID(modelName)
	if modelInfo == nil {
		logger.Warn("Model information not found for %s, using default parameters", modelName)
	}

	config, err := buildSearchConfig(ctx, s, req, modelName, modelInfo)
	if err != nil {
		return createErrorResult(err.Error()), nil
	}

	contents := []*genai.Content{
		genai.NewContentFromParts(
			wrapUserTurnQueryOnly(query, finalInstructionSearch),
			genai.RoleUser,
		),
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
	stop := startProgressReporter(ctx, req,
		s.config.ProgressInterval,
		s.config.HTTPTimeout.Seconds(),
		progressLabel(modelName, config),
		logger)
	defer stop()
	resp, err := withRetry(ctx, s.config, logger, "gemini.models.generate_content", func(ctx context.Context) (*genai.GenerateContentResponse, error) {
		return s.client.Models.GenerateContent(ctx, modelName, contents, config)
	})
	if err != nil {
		logGeminiAPIError(ctx, logger, "Gemini Search API error", err)
		return createErrorResult(fmt.Sprintf("Error from Gemini Search API: %v", err)), nil
	}

	checkModelStatus(ctx, resp, modelName)
	responseText := processSearchResponse(resp, &sources, &searchQueries, seenURLs)

	// Build and return the search response
	result, err := buildSearchResponse(responseText, sources, searchQueries)
	if err != nil {
		logger.Error("Failed to build search response: %v", err)
		return createErrorResult(err.Error()), nil
	}

	return result, nil
}
