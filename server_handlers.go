package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

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

	// Create handler for gemini_ask using direct handler
	// Register gemini_ask with logger wrapper using shared tool definition
	mcpServer.AddTool(GeminiAskTool, wrapHandlerWithLogger(geminiSvc.GeminiAskHandler, "gemini_ask", logger))
	logger.Info("Registered tool: gemini_ask")

	// Use shared tool definition for gemini_search

	// Create handler for gemini_search using direct handler
	// Register gemini_search with logger wrapper using shared tool definition
	mcpServer.AddTool(GeminiSearchTool, wrapHandlerWithLogger(geminiSvc.GeminiSearchHandler, "gemini_search", logger))
	logger.Info("Registered tool: gemini_search")

	// Use shared tool definition for gemini_models

	// Create handler for gemini_models using direct handler
	// Register gemini_models with logger wrapper using shared tool definition
	mcpServer.AddTool(GeminiModelsTool, wrapHandlerWithLogger(geminiSvc.GeminiModelsHandler, "gemini_models", logger))
	logger.Info("Registered tool: gemini_models")

	// Register all prompts from the definitions
	for _, p := range Prompts {
		handler := geminiSvc.promptHandler(p)
		mcpServer.AddPrompt(*p.Prompt, wrapPromptHandlerWithLogger(handler, p.Name, logger))
		logger.Info("Registered prompt: %s", p.Name)
	}

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
		if cfg, ok := configValue.(*Config); ok {
			config = cfg
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

// enforceHTTPAuth checks for authentication on HTTP requests and logs user info.
// It returns an error if authentication fails.
func enforceHTTPAuth(ctx context.Context, resourceType, resourceName string, logger Logger) error {
	// Check if this is an HTTP request
	if httpMethod, ok := ctx.Value(httpMethodKey).(string); !ok || httpMethod == "" {
		return nil // Not an HTTP request, so no auth check needed
	}

	// Check for authentication errors
	if authError := getAuthError(ctx); authError != "" {
		logger.Warn("Authentication failed for %s '%s': %s", resourceType, resourceName, authError)
		return fmt.Errorf("authentication required: %s", authError)
	}

	// Log successful authentication
	if isAuthenticated(ctx) {
		userID, username, role := getUserInfo(ctx)
		logger.Info("%s '%s' called by authenticated user %s (%s) with role %s",
			resourceType, resourceName, username, userID, role)
	}

	return nil
}

// wrapHandlerWithLogger creates a middleware wrapper for logging and authentication around a tool handler
func wrapHandlerWithLogger(handler server.ToolHandlerFunc, toolName string, logger Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logger.Info("Calling tool '%s'...", toolName)

		if err := enforceHTTPAuth(ctx, "tool", toolName, logger); err != nil {
			return createErrorResult(err.Error()), nil
		}

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

// wrapPromptHandlerWithLogger creates a middleware wrapper for logging and authentication around a prompt handler
func wrapPromptHandlerWithLogger(handler server.PromptHandlerFunc, promptName string, logger Logger) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		logger.Info("Calling prompt '%s'...", promptName)

		if err := enforceHTTPAuth(ctx, "prompt", promptName, logger); err != nil {
			return &mcp.GetPromptResult{
				Description: err.Error(),
				Messages:    []mcp.PromptMessage{},
			}, nil
		}

		// Call the actual handler
		resp, err := handler(ctx, req)

		if err != nil {
			logger.Error("Prompt '%s' failed: %v", promptName, err)
		} else {
			logger.Info("Prompt '%s' completed successfully", promptName)
		}

		// Return the original response and error
		return resp, err
	}
}

// Register error handlers for all tools
func registerErrorTools(mcpServer *server.MCPServer, errorServer *ErrorGeminiServer, logger Logger) {
	// Register error handlers for all tools using shared tool definitions
	mcpServer.AddTool(GeminiAskTool, wrapHandlerWithLogger(errorServer.handleErrorResponse, "gemini_ask", logger))
	mcpServer.AddTool(GeminiSearchTool, wrapHandlerWithLogger(errorServer.handleErrorResponse, "gemini_search", logger))
	mcpServer.AddTool(GeminiModelsTool, wrapHandlerWithLogger(errorServer.handleErrorResponse, "gemini_models", logger))

	logger.Info("Registered error handlers for all tools")
}