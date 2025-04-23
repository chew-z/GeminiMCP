package main

import (
	"context"

	"google.golang.org/genai"
)

// extractModelParam extracts and validates the model parameter from the request
// Returns the specified model if valid, otherwise the default model
func extractModelParam(ctx context.Context, args map[string]interface{}, defaultModel string) string {
	logger := getLoggerFromContext(ctx)
	modelName := defaultModel

	if customModel, ok := args["model"].(string); ok && customModel != "" {
		// Validate the custom model
		if err := ValidateModelID(customModel); err != nil {
			logger.Error("Invalid model requested: %v", err)
			logger.Warn("Falling back to default model: %s", defaultModel)
			// Just log but use default if invalid
		} else {
			logger.Info("Using request-specific model: %s", customModel)
			modelName = customModel
		}
	}

	return modelName
}

// extractSystemPrompt extracts the system prompt from the request
func extractSystemPrompt(ctx context.Context, args map[string]interface{}, defaultPrompt string) string {
	logger := getLoggerFromContext(ctx)
	systemPrompt := defaultPrompt

	if customPrompt, ok := args["systemPrompt"].(string); ok && customPrompt != "" {
		logger.Info("Using request-specific system prompt")
		systemPrompt = customPrompt
	}

	return systemPrompt
}

// extractBoolParam extracts a boolean parameter from the request with a default fallback
func extractBoolParam(args map[string]interface{}, paramName string, defaultValue bool) bool {
	if paramValue, ok := args[paramName].(bool); ok {
		return paramValue
	}
	return defaultValue
}

// extractStringParam extracts a string parameter from the request with a default fallback
func extractStringParam(args map[string]interface{}, paramName string, defaultValue string) string {
	if paramValue, ok := args[paramName].(string); ok && paramValue != "" {
		return paramValue
	}
	return defaultValue
}

// extractNumberParam extracts a number parameter from the request with a default fallback
func extractNumberParam(args map[string]interface{}, paramName string, defaultValue float64) float64 {
	if paramValue, ok := args[paramName].(float64); ok {
		return paramValue
	}
	return defaultValue
}

// configureThinking configures thinking mode for the request if enabled and supported
func configureThinking(ctx context.Context, config *genai.GenerateContentConfig, args map[string]interface{}, modelInfo *GeminiModelInfo, enableThinking bool, defaultThinkingBudget int) {
	logger := getLoggerFromContext(ctx)

	if !enableThinking {
		return
	}

	if modelInfo == nil || !modelInfo.SupportsThinking {
		if modelInfo != nil {
			logger.Warn("Thinking mode requested but model %s doesn't support it", modelInfo.ID)
		} else {
			logger.Warn("Thinking mode requested but unknown if model supports it")
		}
		return
	}

	thinkingConfig := &genai.ThinkingConfig{
		IncludeThoughts: true,
	}

	// Determine thinking budget from params or config
	thinkingBudget := 0

	// First check for level
	if levelStr, ok := args["thinking_budget_level"].(string); ok && levelStr != "" {
		thinkingBudget = getThinkingBudgetFromLevel(levelStr)
		logger.Info("Setting thinking budget to %d tokens from level: %s", thinkingBudget, levelStr)
	} else if budgetRaw, ok := args["thinking_budget"].(float64); ok && budgetRaw >= 0 {
		// If explicit budget was provided, use that instead of level
		thinkingBudget = int(budgetRaw)
		logger.Info("Setting thinking budget to %d tokens from explicit value", thinkingBudget)
	} else {
		// Fall back to default config
		thinkingBudget = defaultThinkingBudget
		logger.Info("Using default thinking budget of %d tokens", thinkingBudget)
	}

	// Only set the thinking budget if it's greater than 0
	if thinkingBudget > 0 {
		budget := int32(thinkingBudget)
		thinkingConfig.ThinkingBudget = &budget
	}

	config.ThinkingConfig = thinkingConfig
	logger.Info("Thinking mode enabled with model %s", modelInfo.ID)
}

// configureMaxTokens configures the maximum output tokens for the request
func configureMaxTokens(ctx context.Context, config *genai.GenerateContentConfig, args map[string]interface{}, modelInfo *GeminiModelInfo, defaultRatio float64) {
	logger := getLoggerFromContext(ctx)

	// Check if max_tokens parameter was provided
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
		// Set a safe default if not specified using the provided ratio
		safeTokenLimit := int32(float64(modelInfo.ContextWindowSize) * defaultRatio)
		config.MaxOutputTokens = safeTokenLimit
		logger.Debug("Using default max output tokens: %d (%.0f%% of context window)",
			safeTokenLimit, defaultRatio*100)
	}
}

// createGenaiContentConfig creates and configures a GenerateContentConfig for Gemini API requests
func createGenaiContentConfig(ctx context.Context, args map[string]interface{}, config *Config, modelName string) *genai.GenerateContentConfig {
	logger := getLoggerFromContext(ctx)
	modelInfo := GetModelByID(modelName)

	// Create the initial config with system prompt
	systemPrompt := extractSystemPrompt(ctx, args, config.GeminiSystemPrompt)
	contentConfig := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
		Temperature:       genai.Ptr(float32(config.GeminiTemperature)),
	}

	// Configure thinking mode if enabled and supported
	enableThinking := extractBoolParam(args, "enable_thinking", config.EnableThinking)
	configureThinking(ctx, contentConfig, args, modelInfo, enableThinking, config.ThinkingBudget)

	// Configure max tokens (75% of context window by default for general queries)
	configureMaxTokens(ctx, contentConfig, args, modelInfo, 0.75)

	// Log the temperature setting
	logger.Debug("Using temperature: %v for model %s", config.GeminiTemperature, modelName)

	return contentConfig
}

// createErrorResponse creates a standardized error response
func createErrorResponseWithMessage(message string) *internalCallToolResponse {
	return &internalCallToolResponse{
		IsError: true,
		Content: []internalToolContent{
			{
				Type: "text",
				Text: message,
			},
		},
	}
}
