package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

// checkModelStatus inspects the ModelStatus returned by the Gemini API
// and auto-redirects any model that is not PREVIEW or STABLE to the best
// replacement in the same tier for all future requests.
func checkModelStatus(ctx context.Context, resp *genai.GenerateContentResponse, modelName string) {
	if resp == nil || resp.ModelStatus == nil {
		return
	}

	logger := getLoggerFromContext(ctx)
	status := resp.ModelStatus

	// Only PREVIEW and STABLE are acceptable stages; redirect everything else.
	switch status.ModelStage {
	case genai.ModelStagePreview, genai.ModelStageStable:
		return // acceptable, no action needed
	}

	replacement := bestModelForTier(modelName)
	if replacement != modelName {
		addDynamicAlias(modelName, replacement)
		logger.Warn("Model %s is %s — auto-redirected to %s for future requests",
			modelName, status.ModelStage, replacement)
	} else {
		logger.Warn("Model %s is %s but no better replacement found in tier",
			modelName, status.ModelStage)
	}
}

// extractArgumentString extracts a string argument from the request parameters
func extractArgumentString(req mcp.CallToolRequest, name string, defaultValue string) string {
	args := req.GetArguments()
	if val, ok := args[name].(string); ok && val != "" {
		return val
	}
	return defaultValue
}

// extractArgumentInt extracts an integer argument from the request parameters.
// MCP clients typically send numeric fields as JSON numbers (float64) or strings
// depending on the transport; we accept both forms. Returns (value, ok) where
// ok is false if the parameter is missing, empty, or not parseable.
func extractArgumentInt(req mcp.CallToolRequest, name string) (int, bool) {
	args := req.GetArguments()
	switch v := args[name].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case string:
		if v == "" {
			return 0, false
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// extractArgumentBool extracts a boolean argument from the request parameters
func extractArgumentBool(req mcp.CallToolRequest, name string, defaultValue bool) bool {
	args := req.GetArguments()
	if val, ok := args[name].(bool); ok {
		return val
	}
	return defaultValue
}

// extractArgumentStringArray extracts a string array argument from the request parameters.
// It handles three input forms:
//   - []interface{} (JSON array from proper MCP clients)
//   - string containing a JSON array (e.g., '["file1.go", "file2.go"]' from some clients)
//   - plain string (single value, e.g., "config.py")
func extractArgumentStringArray(req mcp.CallToolRequest, name string) []string {
	var result []string
	args := req.GetArguments()
	switch v := args[name].(type) {
	case []any:
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
	case string:
		if v == "" {
			return result
		}
		// Some MCP clients pass arrays as JSON strings — try to parse
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "[") {
			var parsed []string
			if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
				return parsed
			}
		}
		result = append(result, v)
	}
	return result
}

// serviceTierFromString converts a string to a genai.ServiceTier
func serviceTierFromString(tier string) genai.ServiceTier {
	switch tier {
	case "flex":
		return genai.ServiceTierFlex
	case "priority":
		return genai.ServiceTierPriority
	default: // "standard" or anything else
		return genai.ServiceTierStandard
	}
}

// resolveAndValidateModel resolves aliases and validates the model ID.
// Returns an error for non-Gemini model names; old Gemini model names are
// redirected to the current best model in the same tier.
func resolveAndValidateModel(ctx context.Context, modelName string) (string, error) {
	logger := getLoggerFromContext(ctx)

	resolvedModelID := ResolveModelID(modelName)
	if resolvedModelID != modelName {
		logger.Info("Resolved model ID from '%s' to '%s'", modelName, resolvedModelID)
		modelName = resolvedModelID
	}

	validatedID, redirected, err := ValidateModelID(modelName)
	if err != nil {
		return "", err
	}
	if redirected {
		logger.Warn("Unknown Gemini model '%s' redirected to '%s'", modelName, validatedID)
		modelName = validatedID
	}
	return modelName, nil
}

// configureThinking sets up thinking configuration on a GenerateContentConfig
// if the model supports it and thinking is requested.
func configureThinking(ctx context.Context, req mcp.CallToolRequest, config *genai.GenerateContentConfig,
	enableByDefault bool, defaultLevel string, modelInfo *GeminiModelInfo, modelName string) {

	logger := getLoggerFromContext(ctx)
	enableThinking := extractArgumentBool(req, "enable_thinking", enableByDefault)

	if enableThinking && modelInfo != nil && modelInfo.SupportsThinking {
		thinkingLevel := defaultLevel

		// Check for thinking_level parameter in request
		if levelStr, ok := req.GetArguments()["thinking_level"].(string); ok && levelStr != "" {
			if validateThinkingLevel(levelStr) {
				thinkingLevel = strings.ToLower(levelStr)
				logger.Info("Setting thinking level to: %s", thinkingLevel)
			} else {
				logger.Warn("Invalid thinking_level '%s' (valid: minimal, low, medium, high). Using default: %s", levelStr, defaultLevel)
			}
		}

		config.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: true,
			ThinkingLevel:   genai.ThinkingLevel(thinkingLevel),
		}
		logger.Info("Thinking mode enabled with level '%s' for model %s", thinkingLevel, modelName)
	} else if enableThinking {
		if modelInfo != nil {
			logger.Warn("Thinking mode requested but model %s doesn't support it", modelName)
		} else {
			logger.Warn("Thinking mode requested but unknown if model supports it")
		}
	}
}

// createModelConfig creates a GenerateContentConfig for Gemini API based on request parameters
func createModelConfig(ctx context.Context, req mcp.CallToolRequest, config *Config, defaultModel string) (*genai.GenerateContentConfig, string, error) {
	// Extract model parameter - use defaultModel if not specified
	modelName := extractArgumentString(req, "model", defaultModel)

	modelName, err := resolveAndValidateModel(ctx, modelName)
	if err != nil {
		return nil, "", err
	}

	// Extract system prompt
	systemPrompt := config.GeminiSystemPrompt
	if sp, ok := req.GetArguments()["systemPrompt"].(string); ok {
		systemPrompt = sp
	}

	// Get model information
	logger := getLoggerFromContext(ctx)
	modelInfo := GetModelByID(modelName)
	if modelInfo == nil {
		logger.Warn("Model information not found for %s, using default parameters", modelName)
	}

	// Create the configuration
	contentConfig := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
		Temperature:       genai.Ptr(float32(config.GeminiTemperature)),
	}
	contentConfig.ServiceTier = serviceTierFromString(config.ServiceTier)

	// Configure thinking if supported
	configureThinking(ctx, req, contentConfig, config.EnableThinking, config.ThinkingLevel, modelInfo, modelName)

	// Configure max tokens with default ratio of context window
	configureMaxTokensOutput(ctx, contentConfig, req, modelInfo, 0.75)

	return contentConfig, modelName, nil
}

// configureMaxTokensOutput configures the maximum output tokens for the request
func configureMaxTokensOutput(
	ctx context.Context,
	config *genai.GenerateContentConfig,
	req mcp.CallToolRequest,
	modelInfo *GeminiModelInfo,
	defaultRatio float64,
) {
	logger := getLoggerFromContext(ctx)

	// Check if max_tokens parameter was provided
	args := req.GetArguments()
	if maxTokensRaw, ok := args["max_tokens"].(float64); ok && maxTokensRaw > 0 {
		maxTokens := int(maxTokensRaw)

		// Warn if tokens exceed the model's context window
		if modelInfo != nil && maxTokens > modelInfo.ContextWindowSize {
			logger.Warn("Requested max_tokens (%d) exceeds model's context window size (%d)",
				maxTokens, modelInfo.ContextWindowSize)
		}

		// Set the maximum output token limit
		config.MaxOutputTokens = int32(maxTokens)
		logger.Info("Setting max output tokens to %d", maxTokens)
	} else if modelInfo != nil {
		// Prefer the model's actual output token limit from the API
		if modelInfo.MaxOutputTokens > 0 {
			config.MaxOutputTokens = int32(modelInfo.MaxOutputTokens)
			logger.Debug("Using model's max output tokens: %d", modelInfo.MaxOutputTokens)
		} else {
			// Fallback: ratio of context window if output limit unknown
			safeTokenLimit := int32(float64(modelInfo.ContextWindowSize) * defaultRatio)
			config.MaxOutputTokens = safeTokenLimit
			logger.Debug("Using default max output tokens: %d (%.0f%% of context window)",
				safeTokenLimit, defaultRatio*100)
		}
	}
}

// createErrorResult creates a standardized error result for mcp.CallToolResult
func createErrorResult(message string) *mcp.CallToolResult {
	return mcp.NewToolResultError(message)
}

// convertGenaiResponseToMCPResult converts a Gemini API response to an MCP result
func convertGenaiResponseToMCPResult(resp *genai.GenerateContentResponse) *mcp.CallToolResult {
	// Check for empty response
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return mcp.NewToolResultError("Gemini API returned an empty response")
	}

	// Get the text from the response
	text := resp.Text()
	if text == "" {
		text = "The Gemini model returned an empty response. This might indicate that the model " +
			"couldn't generate an appropriate response for your query. Please try rephrasing your question or providing more context."
	}

	// The 'thinking' field is not directly available in the Go client library.
	// The response will be plain text. If thinking was enabled, the model's
	// reasoning might be part of the main text response, but it cannot be
	// separated into a distinct field.

	// Return simple text response
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(text),
		},
	}
}

// SafeWriter provides error-safe writing to strings.Builder for handlers
type SafeWriter struct {
	builder *strings.Builder
	logger  Logger
	failed  bool
}

// NewSafeWriter creates a new SafeWriter instance
func NewSafeWriter(logger Logger) *SafeWriter {
	return &SafeWriter{
		builder: &strings.Builder{},
		logger:  logger,
		failed:  false,
	}
}

// Write adds formatted content to the builder, logging errors but continuing
func (sw *SafeWriter) Write(format string, args ...interface{}) {
	if sw.failed {
		return // Don't write if we've already failed
	}
	_, err := sw.builder.WriteString(fmt.Sprintf(format, args...))
	if err != nil {
		sw.logger.Error("Error writing to response: %v", err)
		sw.failed = true
	}
}

// Failed returns true if any write operations have failed
func (sw *SafeWriter) Failed() bool {
	return sw.failed
}

// String returns the built string
func (sw *SafeWriter) String() string {
	return sw.builder.String()
}

// Validation helper functions

// validateRequiredString validates that a required string parameter is not empty
func validateRequiredString(req mcp.CallToolRequest, paramName string) (string, error) {
	value, ok := req.GetArguments()[paramName].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s must be a string and cannot be empty", paramName)
	}
	return value, nil
}

// validateFilePathArray validates an array of file paths
func validateFilePathArray(filePaths []string, isGitHub bool) error {
	for _, filePath := range filePaths {
		if isGitHub {
			// GitHub file paths validation
			if strings.Contains(filePath, "..") || strings.HasPrefix(filePath, "/") {
				return fmt.Errorf("invalid file path: %s. Path must be relative and within the repository", filePath)
			}
		} else {
			// Local file paths are validated later in readLocalFiles
		}
	}
	return nil
}

// validateTimeRange validates RFC3339 time range parameters
func validateTimeRange(startTimeStr, endTimeStr string) (*time.Time, *time.Time, error) {
	// Both must be provided if either is provided
	if (startTimeStr != "" && endTimeStr == "") || (startTimeStr == "" && endTimeStr != "") {
		return nil, nil, fmt.Errorf("both start_time and end_time must be provided for time range filtering")
	}

	if startTimeStr == "" && endTimeStr == "" {
		return nil, nil, nil // No time range specified
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid start_time format: %v. Must be RFC3339 format (e.g. '2024-01-01T00:00:00Z')", err)
	}

	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid end_time format: %v. Must be RFC3339 format (e.g. '2024-12-31T23:59:59Z')", err)
	}

	// Ensure start time is before or equal to end time
	if startTime.After(endTime) {
		return nil, nil, fmt.Errorf("start_time must be before or equal to end_time")
	}

	return &startTime, &endTime, nil
}

// Response builder functions

// buildSearchResponse creates a formatted search response with sources and queries
func buildSearchResponse(responseText string, sources []SourceInfo, searchQueries []string) (*mcp.CallToolResult, error) {
	// Check for empty content and provide a fallback message
	if responseText == "" {
		responseText = `The Gemini Search model returned an empty response.
			This might indicate an issue with the search functionality or that
			no relevant information was found. Please try rephrasing your question
			or providing more specific details.`
	}

	// Create the response JSON
	searchResp := SearchResponse{
		Answer:        responseText,
		Sources:       sources,
		SearchQueries: searchQueries,
	}

	// Convert to JSON and return as text content
	responseJSON, err := json.Marshal(searchResp)
	if err != nil {
		return nil, fmt.Errorf("failed to format search response: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(string(responseJSON)),
		},
	}, nil
}

// processSearchResponse processes grounding metadata from a search response
func processSearchResponse(resp *genai.GenerateContentResponse, sources *[]SourceInfo, searchQueries *[]string, seenURLs map[string]bool) string {
	responseText := ""

	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		responseText = resp.Text()
		if metadata := resp.Candidates[0].GroundingMetadata; metadata != nil {
			if len(metadata.WebSearchQueries) > 0 && len(*searchQueries) == 0 {
				*searchQueries = metadata.WebSearchQueries
			}
			for _, chunk := range metadata.GroundingChunks {
				var source SourceInfo
				if web := chunk.Web; web != nil {
					if seenURLs[web.URI] {
						continue
					}
					source = SourceInfo{Title: web.Title, Type: "web"}
					seenURLs[web.URI] = true
				} else if retrievedCtx := chunk.RetrievedContext; retrievedCtx != nil {
					if seenURLs[retrievedCtx.URI] {
						continue
					}
					source = SourceInfo{Title: retrievedCtx.Title, Type: "retrieved_context"}
					seenURLs[retrievedCtx.URI] = true
				}
				if source.Title != "" {
					*sources = append(*sources, source)
				}
			}
		}
	}

	return responseText
}
