package main

import (
	"context"

	"google.golang.org/genai"
)

// These functions have been removed as they were unused after refactoring to use direct handlers with mcp-go types

// configureThinking configures thinking mode for the request if enabled and supported
func configureThinking(ctx context.Context, config *genai.GenerateContentConfig, args map[string]interface{}, modelInfo *GeminiModelInfo, enableThinking bool, defaultThinkingBudget int) {
	logger := getLoggerFromContext(ctx)

	if !enableThinking {
		return
	}

	if modelInfo == nil || !modelInfo.SupportsThinking {
		if modelInfo != nil {
			logger.Warn("Thinking mode requested but model %s doesn't support it", modelInfo.FamilyID)
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
	logger.Info("Thinking mode enabled with model %s", modelInfo.FamilyID)
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
