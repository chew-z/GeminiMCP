package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// main is the entry point for the application.
// It sets up the MCP server with the appropriate handlers and starts it.
func main() {
	// Define command-line flags for configuration override
	geminiModelFlag := flag.String("gemini-model", "", "Gemini model name (overrides env var)")
	geminiSystemPromptFlag := flag.String("gemini-system-prompt", "", "System prompt (overrides env var)")
	geminiTemperatureFlag := flag.Float64("gemini-temperature", -1, "Temperature setting (0.0-1.0, overrides env var)")
	enableCachingFlag := flag.Bool("enable-caching", true, "Enable caching feature (overrides env var)")
	enableThinkingFlag := flag.Bool("enable-thinking", true, "Enable thinking mode for supported models (overrides env var)")
	flag.Parse()

	// Create application context with logger
	logger := NewLogger(LevelInfo)
	ctx := context.WithValue(context.Background(), loggerKey, logger)

	// Create configuration from environment variables
	config, err := NewConfig()
	if err != nil {
		handleStartupError(ctx, err)
		return
	}

	// Store config in context for error handler to access
	ctx = context.WithValue(ctx, configKey, config)

	// Fetch available Gemini models first if API key is available
	// This ensures we have the latest models before validation
	if config.GeminiAPIKey != "" {
		logger.Info("Attempting to fetch available Gemini models...")
		if err := FetchGeminiModels(ctx, config.GeminiAPIKey); err != nil {
			// Just log the error but continue with fallback models
			logger.Warn("Could not fetch Gemini models: %v. Using fallback model list.", err)
		}
	} else {
		logger.Warn("No Gemini API key available, using fallback model list")
	}

	// Override with command-line flags if provided
	if *geminiModelFlag != "" {
		// We'll use the model specified, even if it's not in our known list
		// This allows for new models and preview versions
		if err := ValidateModelID(*geminiModelFlag); err != nil {
			// Just log a warning, we'll still use the model
			logger.Info("Using custom model: %s (not in known list, but may be valid)", *geminiModelFlag)
		} else {
			logger.Info("Using known model: %s", *geminiModelFlag)
		}
		config.GeminiModel = *geminiModelFlag
	}
	if *geminiSystemPromptFlag != "" {
		logger.Info("Overriding Gemini system prompt with flag value")
		config.GeminiSystemPrompt = *geminiSystemPromptFlag
	}

	// Override temperature if provided and valid
	if *geminiTemperatureFlag >= 0 {
		// Validate temperature is within range
		if *geminiTemperatureFlag > 1.0 {
			logger.Error("Invalid temperature value: %v. Must be between 0.0 and 1.0", *geminiTemperatureFlag)
			handleStartupError(ctx, fmt.Errorf("invalid temperature: %v", *geminiTemperatureFlag))
			return
		}
		logger.Info("Overriding Gemini temperature with flag value: %v", *geminiTemperatureFlag)
		config.GeminiTemperature = *geminiTemperatureFlag
	}

	// Override enable caching if flag is provided
	config.EnableCaching = *enableCachingFlag
	logger.Info("Caching feature is %s", getCachingStatusStr(config.EnableCaching))

	// Override enable thinking if flag is provided
	config.EnableThinking = *enableThinkingFlag
	logger.Info("Thinking feature is %s", getCachingStatusStr(config.EnableThinking))

	// Store config in context for error handler to access (already done earlier)

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"gemini",
		"1.0.0",
	)

	// Create and register the Gemini server tools
	if err := setupGeminiServer(ctx, mcpServer, config); err != nil {
		handleStartupError(ctx, err)
		return
	}

	logger.Info("Starting Gemini MCP server")
	if err := server.ServeStdio(mcpServer); err != nil {
		logger.Error("Server error: %v", err)
		os.Exit(1)
	}
}

// Helper function to get caching status as a string
func getCachingStatusStr(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// setupGeminiServer creates and registers Gemini server tools
func setupGeminiServer(ctx context.Context, mcpServer *server.MCPServer, config *Config) error {
	loggerValue := ctx.Value(loggerKey)
	logger, ok := loggerValue.(Logger)
	if !ok {
		return fmt.Errorf("logger not found in context")
	}

	// Create the Gemini service with configuration
	geminiSvc, err := NewGeminiServer(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create Gemini service: %w", err)
	}

	// Define and register gemini_ask tool
	geminiAskTool := mcp.NewTool(
		"gemini_ask",
		mcp.WithDescription("Use Google's Gemini AI model to ask about complex coding problems"),
		mcp.WithString("query", mcp.Required(), mcp.Description("The coding problem that we are asking Gemini AI to work on [question + code]")),
		mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
		mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
		mcp.WithArray("file_paths", mcp.Description("Optional: Paths to files to include in the request context")),
		mcp.WithBoolean("use_cache", mcp.Description("Optional: Whether to try using a cache for this request (only works with compatible models)")),
		mcp.WithString("cache_ttl", mcp.Description("Optional: TTL for cache if created (e.g., '10m', '1h'). Default is 10 minutes")),
		mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process (only works with Pro models)")),
		mcp.WithNumber("thinking_budget", mcp.Description("Optional: Maximum number of tokens to allocate for the model's thinking process (0-24576)")),
		mcp.WithString("thinking_budget_level", mcp.Description("Optional: Predefined thinking budget level (none, low, medium, high)")),
		mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
	)

	// Create handler for gemini_ask
	geminiAskHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Convert from mark3labs/mcp-go types to internal types
		internalReq := convertToInternalRequest(&req)
		resp, err := geminiSvc.handleAskGemini(ctx, internalReq)
		if err != nil {
			return nil, err
		}
		return convertToMCPResult(resp), nil
	}

	// Register gemini_ask with logger wrapper
	mcpServer.AddTool(geminiAskTool, wrapHandlerWithLogger(geminiAskHandler, "gemini_ask", logger))
	logger.Info("Registered tool: gemini_ask")

	// Define and register gemini_search tool
	geminiSearchTool := mcp.NewTool(
		"gemini_search",
		mcp.WithDescription("Use Google's Gemini AI model with Google Search to answer questions with grounded information"),
		mcp.WithString("query", mcp.Required(), mcp.Description("The question to ask Gemini using Google Search for grounding")),
		mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
		mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process (when supported)")),
		mcp.WithNumber("thinking_budget", mcp.Description("Optional: Maximum number of tokens to allocate for the model's thinking process (0-24576)")),
		mcp.WithString("thinking_budget_level", mcp.Description("Optional: Predefined thinking budget level (none, low, medium, high)")),
		mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
		mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
	)

	// Create handler for gemini_search
	geminiSearchHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Convert from mark3labs/mcp-go types to internal types
		internalReq := convertToInternalRequest(&req)
		resp, err := geminiSvc.handleGeminiSearch(ctx, internalReq)
		if err != nil {
			return nil, err
		}
		return convertToMCPResult(resp), nil
	}

	// Register gemini_search with logger wrapper
	mcpServer.AddTool(geminiSearchTool, wrapHandlerWithLogger(geminiSearchHandler, "gemini_search", logger))
	logger.Info("Registered tool: gemini_search")

	// Define and register gemini_models tool
	geminiModelsTool := mcp.NewTool(
		"gemini_models",
		mcp.WithDescription("List available Gemini models with descriptions"),
	)

	// Create handler for gemini_models
	geminiModelsHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// This handler doesn't need request parameters, so we can call directly
		resp, err := geminiSvc.handleGeminiModels(ctx)
		if err != nil {
			return nil, err
		}
		return convertToMCPResult(resp), nil
	}

	// Register gemini_models with logger wrapper
	mcpServer.AddTool(geminiModelsTool, wrapHandlerWithLogger(geminiModelsHandler, "gemini_models", logger))
	logger.Info("Registered tool: gemini_models")

	// Log file handling configuration
	logger.Info("File handling: max size %s, allowed types: %v",
		humanReadableSize(config.MaxFileSize),
		config.AllowedFileTypes)

	// Log cache configuration if enabled
	if config.EnableCaching {
		logger.Info("Cache settings: default TTL %v", config.DefaultCacheTTL)
	}

	// Log thinking configuration if enabled
	model := GetModelByID(config.GeminiModel)
	if config.EnableThinking && model != nil && model.SupportsThinking {
		logger.Info("Thinking mode enabled for model %s with context window size %d tokens",
			config.GeminiModel, model.ContextWindowSize)
	}

	// Log a truncated version of the system prompt for security/brevity
	promptPreview := config.GeminiSystemPrompt
	if len(promptPreview) > 50 {
		// Use proper UTF-8 safe truncation
		runeCount := 0
		for i := range promptPreview {
			runeCount++
			if runeCount > 50 {
				promptPreview = promptPreview[:i] + "..."
				break
			}
		}
	}
	logger.Info("Using system prompt: %s", promptPreview)

	return nil
}

// handleStartupError handles initialization errors by setting up an error server
func handleStartupError(ctx context.Context, err error) {
	// Safely extract logger from context
	loggerValue := ctx.Value(loggerKey)
	logger, ok := loggerValue.(Logger)
	if !ok {
		// Fallback to a new logger if type assertion fails
		logger = NewLogger(LevelError)
	}
	errorMsg := err.Error()

	logger.Error("Initialization error: %v", err)

	// Get config for EnableCaching status (if available)
	var config *Config
	configValue := ctx.Value(configKey)
	if configValue != nil {
		if cfg, ok := configValue.(Config); ok {
			config = &cfg
		}
	}

	// Create MCP server in degraded mode
	mcpServer := server.NewMCPServer(
		"gemini",
		"1.0.0",
	)

	// Create error server
	errorServer := &ErrorGeminiServer{
		errorMessage: errorMsg,
		config:       config,
	}

	// Register error handling for tools
	registerErrorTools(mcpServer, errorServer, logger)

	// Start server in degraded mode
	logger.Info("Starting Gemini MCP server in degraded mode")
	if err := server.ServeStdio(mcpServer); err != nil {
		logger.Error("Server error in degraded mode: %v", err)
		os.Exit(1)
	}
}

// Define the expected handler signature for tools
type MCPToolHandlerFunc = server.ToolHandlerFunc

// wrapHandlerWithLogger creates a middleware wrapper for logging around a tool handler
func wrapHandlerWithLogger(handler server.ToolHandlerFunc, toolName string, logger Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logger.Info("Calling tool '%s'...", toolName)

		// Call the actual handler
		resp, err := handler(ctx, req)

		if err != nil {
			logger.Error("Tool '%s' failed: %v", toolName, err)
		} else {
			logger.Info("Tool '%s' completed successfully", toolName)
		}

		// Return the original response and error
		return resp, err
	}
}

// registerErrorTools registers error handlers for all supported tools
func registerErrorTools(mcpServer *server.MCPServer, errorServer *ErrorGeminiServer, logger Logger) {
	// Register gemini_ask tool
	geminiAskTool := mcp.NewTool(
		"gemini_ask",
		mcp.WithDescription("Use Google's Gemini AI model to ask about complex coding problems"),
		mcp.WithString("query", mcp.Required(), mcp.Description("The coding problem that we are asking Gemini AI to work on [question + code]")),
		mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
		mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
		mcp.WithArray("file_paths", mcp.Description("Optional: Paths to files to include in the request context")),
		mcp.WithBoolean("use_cache", mcp.Description("Optional: Whether to try using a cache for this request (only works with compatible models)")),
		mcp.WithString("cache_ttl", mcp.Description("Optional: TTL for cache if created (e.g., '10m', '1h'). Default is 10 minutes")),
		mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process (only works with Pro models)")),
		mcp.WithNumber("thinking_budget", mcp.Description("Optional: Maximum number of tokens to allocate for the model's thinking process (0-24576)")),
		mcp.WithString("thinking_budget_level", mcp.Description("Optional: Predefined thinking budget level (none, low, medium, high)")),
		mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
	)
	mcpServer.AddTool(geminiAskTool, wrapHandlerWithLogger(errorServer.handleErrorResponse, "gemini_ask", logger))

	// Register gemini_search tool
	geminiSearchTool := mcp.NewTool(
		"gemini_search",
		mcp.WithDescription("Use Google's Gemini AI model with Google Search to answer questions with grounded information"),
		mcp.WithString("query", mcp.Required(), mcp.Description("The question to ask Gemini using Google Search for grounding")),
		mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
		mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process (when supported)")),
		mcp.WithNumber("thinking_budget", mcp.Description("Optional: Maximum number of tokens to allocate for the model's thinking process (0-24576)")),
		mcp.WithString("thinking_budget_level", mcp.Description("Optional: Predefined thinking budget level (none, low, medium, high)")),
		mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
		mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
	)
	mcpServer.AddTool(geminiSearchTool, wrapHandlerWithLogger(errorServer.handleErrorResponse, "gemini_search", logger))

	// Register gemini_models tool
	geminiModelsTool := mcp.NewTool(
		"gemini_models",
		mcp.WithDescription("List available Gemini models with descriptions"),
	)
	mcpServer.AddTool(geminiModelsTool, wrapHandlerWithLogger(errorServer.handleErrorResponse, "gemini_models", logger))

	logger.Info("Registered error handlers for all tools")
}

// During migration, we need these adapter functions to communicate with the old code

// For now, we'll define our own internal types to match the old protocol types
type internalCallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type internalCallToolResponse struct {
	IsError bool                  `json:"isError"`
	Content []internalToolContent `json:"content,omitempty"`
}

type internalToolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// convertToInternalRequest converts an mcp.CallToolRequest to our internal request type
func convertToInternalRequest(mcpReq *mcp.CallToolRequest) *internalCallToolRequest {
	return &internalCallToolRequest{
		Name:      mcpReq.Params.Name,
		Arguments: mcpReq.Params.Arguments,
	}
}

// convertToMCPResult converts our internal response to mcp.CallToolResult
func convertToMCPResult(protoResp *internalCallToolResponse) *mcp.CallToolResult {
	result := &mcp.CallToolResult{
		IsError: protoResp.IsError,
	}

	// Convert content items
	for _, content := range protoResp.Content {
		switch content.Type {
		case "text":
			result.Content = append(result.Content, mcp.NewTextContent(content.Text))
			// Note: We only handle text content for now during the migration
		}
	}

	return result
}
