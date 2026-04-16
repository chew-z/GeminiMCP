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

	registerPrompts(mcpServer, geminiSvc, logger)

	// Log thinking configuration if enabled
	model := GetModelByID(config.GeminiModel)
	if model != nil && model.SupportsThinking {
		logger.Info("Thinking enabled for model %s (context window: %d tokens)",
			config.GeminiModel, model.ContextWindowSize)
	}

	if config.Prequalify {
		logger.Info("System prompt selection: pre-qualification enabled (model %s)",
			config.PrequalifyModel)
	} else {
		logger.Info("System prompt selection: pre-qualification disabled — using systemPromptGeneral")
	}

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

	// Create MCP server in degraded mode
	mcpServer := server.NewMCPServer(
		"gemini",
		"1.0.0",
	)

	// Create error server
	errorServer := &ErrorGeminiServer{
		errorMessage: errorMsg,
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

// registerPrompts wires every PromptDefinition into the MCP server. Prompts
// with a HandlerFactory get a custom handler (used by the github-workflow
// prompts); all others fall back to the generic problem_statement handler.
func registerPrompts(mcpServer *server.MCPServer, geminiSvc *GeminiServer, logger Logger) {
	for _, p := range Prompts {
		var handler server.PromptHandlerFunc
		if p.HandlerFactory != nil {
			handler = server.PromptHandlerFunc(p.HandlerFactory(geminiSvc))
		} else {
			handler = geminiSvc.promptHandler(p)
		}
		mcpServer.AddPrompt(*p.Prompt, wrapPromptHandlerWithLogger(handler, p.Name, logger))
		logger.Info("Registered prompt: %s", p.Name)
	}
}

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

		switch {
		case err != nil:
			logger.Error("Tool '%s' failed: %v", toolName, err)
		case resp != nil && resp.IsError:
			logger.Warn("Tool '%s' returned an error result", toolName)
		default:
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

	logger.Info("Registered error handlers for all tools")
}
