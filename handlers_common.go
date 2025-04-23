package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

// extractArgumentString extracts a string argument from the request parameters
func extractArgumentString(args map[string]interface{}, name string, defaultValue string) string {
	if val, ok := args[name].(string); ok && val != "" {
		return val
	}
	return defaultValue
}

// extractArgumentBool extracts a boolean argument from the request parameters
func extractArgumentBool(args map[string]interface{}, name string, defaultValue bool) bool {
	if val, ok := args[name].(bool); ok {
		return val
	}
	return defaultValue
}

// extractArgumentFloat extracts a float64 argument from the request parameters
func extractArgumentFloat(args map[string]interface{}, name string, defaultValue float64) float64 {
	if val, ok := args[name].(float64); ok {
		return val
	}
	return defaultValue
}

// extractArgumentStringArray extracts a string array argument from the request parameters
func extractArgumentStringArray(args map[string]interface{}, name string) []string {
	var result []string
	if rawArray, ok := args[name].([]interface{}); ok {
		for _, item := range rawArray {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
	}
	return result
}

// createModelConfig creates a GenerateContentConfig for Gemini API based on request parameters
func createModelConfig(ctx context.Context, params map[string]interface{}, config *Config, defaultModel string) (*genai.GenerateContentConfig, string, error) {
	logger := getLoggerFromContext(ctx)
	
	// Extract model parameter - use defaultModel if not specified
	modelName := extractArgumentString(params, "model", defaultModel)
	
	// Validate the model
	if err := ValidateModelID(modelName); err != nil {
		logger.Error("Invalid model requested: %v", err)
		return nil, "", fmt.Errorf("invalid model specified: %v", err)
	}
	
	// Extract system prompt
	systemPrompt := extractArgumentString(params, "systemPrompt", config.GeminiSystemPrompt)
	
	// Get model information
	modelInfo := GetModelByID(modelName)
	if modelInfo == nil {
		logger.Warn("Model information not found for %s, using default parameters", modelName)
	}
	
	// Create the configuration
	contentConfig := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
		Temperature:       genai.Ptr(float32(config.GeminiTemperature)),
	}
	
	// Configure thinking if supported
	enableThinking := extractArgumentBool(params, "enable_thinking", config.EnableThinking)
	if enableThinking && modelInfo != nil && modelInfo.SupportsThinking {
		thinkingConfig := &genai.ThinkingConfig{
			IncludeThoughts: true,
		}
		
		// Determine thinking budget
		thinkingBudget := 0
		
		// Check for level first
		if levelStr, ok := params["thinking_budget_level"].(string); ok && levelStr != "" {
			thinkingBudget = getThinkingBudgetFromLevel(levelStr)
			logger.Info("Setting thinking budget to %d tokens from level: %s", thinkingBudget, levelStr)
		} else if budgetRaw, ok := params["thinking_budget"].(float64); ok && budgetRaw >= 0 {
			// If explicit budget was provided, use that instead of level
			thinkingBudget = int(budgetRaw)
			logger.Info("Setting thinking budget to %d tokens from explicit value", thinkingBudget)
		} else if config.ThinkingBudget > 0 {
			// Fall back to config value if neither level nor explicit budget provided
			thinkingBudget = config.ThinkingBudget
			logger.Info("Using default thinking budget of %d tokens", thinkingBudget)
		}
		
		// Set budget if greater than 0
		if thinkingBudget > 0 {
			budget := int32(thinkingBudget)
			thinkingConfig.ThinkingBudget = &budget
		}
		
		contentConfig.ThinkingConfig = thinkingConfig
		logger.Info("Thinking mode enabled with budget %d for model %s", thinkingBudget, modelName)
	} else if enableThinking && (modelInfo == nil || !modelInfo.SupportsThinking) {
		logger.Warn("Thinking mode was requested but model doesn't support it")
	}
	
	// Configure max tokens with default ratio of context window
	configureMaxTokensOutput(ctx, contentConfig, params, modelInfo, 0.75)
	
	return contentConfig, modelName, nil
}

// configureMaxTokensOutput configures the maximum output tokens for the request
func configureMaxTokensOutput(ctx context.Context, config *genai.GenerateContentConfig, args map[string]interface{}, modelInfo *GeminiModelInfo, defaultRatio float64) {
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

// createErrorResult creates a standardized error result for mcp.CallToolResult
func createErrorResult(message string) *mcp.CallToolResult {
	return mcp.NewToolResultError(message)
}

// convertGenaiResponseToMCPResult converts a Gemini API response to an MCP result
func convertGenaiResponseToMCPResult(resp *genai.GenerateContentResponse, withThinking bool) *mcp.CallToolResult {
	// Check for empty response
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return mcp.NewToolResultError("Gemini API returned an empty response")
	}
	
	// Get the text from the response
	text := resp.Text()
	if text == "" {
		text = "The Gemini model returned an empty response. This might indicate that the model couldn't generate an appropriate response for your query. Please try rephrasing your question or providing more context."
	}
	
	// If thinking was requested, try to extract thinking data
	if withThinking {
		// Try to extract thinking from the response
		thinking := extractThinkingFromResponse(resp)
		if thinking != "" {
			// Return JSON with both answer and thinking
			thinkingJSON := fmt.Sprintf(`{"answer": %q, "thinking": %q}`, text, thinking)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(thinkingJSON),
				},
			}
		}
	}
	
	// Return simple text response
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(text),
		},
	}
}

// extractThinkingFromResponse attempts to extract thinking text from a Gemini response
func extractThinkingFromResponse(resp *genai.GenerateContentResponse) string {
	// This is not directly available in the Go API, would need to parse raw JSON
	// For now, return empty string to indicate no thinking data 
	// A proper implementation would need to look at resp.Candidates[0] raw data
	return ""
}
