package main

import (
	"context"
	"fmt"
	"os"
	"time"

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

	// Create MCP server in degraded mode using the same option list as the
	// normal path so panic recovery, schema validation, and capability
	// advertisements stay consistent across both servers.
	mcpServer := server.NewMCPServer(
		"gemini",
		"1.0.0",
		buildMCPServerOptions(nil, logger)...,
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

// enforceHTTPAuth checks for authentication on HTTP requests. Successful
// authentication is logged at the tool-entry level by wrapHandlerWithLogger,
// so this function is silent on the happy path and only warns on failure.
func enforceHTTPAuth(ctx context.Context, resourceType, resourceName string, logger Logger) error {
	// Check if this is an HTTP request
	if httpMethod, ok := ctx.Value(httpMethodKey).(string); !ok || httpMethod == "" {
		return nil // Not an HTTP request, so no auth check needed
	}

	if authError := getAuthError(ctx); authError != "" {
		logger.Warn("Authentication failed for %s '%s': %s", resourceType, resourceName, authError)
		return fmt.Errorf("authentication required: %s", authError)
	}

	if isAuthenticated(ctx) {
		userID, username, role := getUserInfo(ctx)
		logger.Debug("%s '%s' accessed by user=%s id=%s role=%s",
			resourceType, resourceName, username, userID, role)
	}

	return nil
}

// wrapHandlerWithLogger creates a middleware wrapper for logging and
// authentication around a tool handler. It mints a per-call request ID,
// scopes the context logger with a "[req-id]" prefix, and emits one
// structured INFO line at start and one at completion.
func wrapHandlerWithLogger(handler server.ToolHandlerFunc, toolName string, logger Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		reqID := newRequestID()
		ctx, rlog := withRequestLogger(ctx, logger, reqID)
		start := time.Now()

		user, role := "-", "-"
		if isAuthenticated(ctx) {
			_, u, r := getUserInfo(ctx)
			if u != "" {
				user = u
			}
			if r != "" {
				role = r
			}
		}
		rlog.Info("tool=%s start user=%s role=%s", toolName, user, role)

		if err := enforceHTTPAuth(ctx, "tool", toolName, rlog); err != nil {
			rlog.Info("tool=%s done status=auth_error duration=%s", toolName, time.Since(start).Round(time.Millisecond))
			return createErrorResult(err.Error()), nil
		}

		resp, err := handler(ctx, req)
		duration := time.Since(start).Round(time.Millisecond)

		switch {
		case err != nil:
			rlog.Error("tool=%s done status=error duration=%s err=%v", toolName, duration, err)
		case resp != nil && resp.IsError:
			rlog.Warn("tool=%s done status=tool_error duration=%s", toolName, duration)
		default:
			rlog.Info("tool=%s done status=ok duration=%s", toolName, duration)
		}

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
