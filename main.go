package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

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
	transportFlag := flag.String("transport", "stdio", "Transport mode: 'stdio' (default) or 'http'")

	// Authentication flags
	authEnabledFlag := flag.Bool("auth-enabled", false, "Enable JWT authentication for HTTP transport (overrides env var)")
	generateTokenFlag := flag.Bool("generate-token", false, "Generate a JWT token and exit")
	tokenUserIDFlag := flag.String("token-user-id", "user1", "User ID for token generation")
	tokenUsernameFlag := flag.String("token-username", "admin", "Username for token generation")
	tokenRoleFlag := flag.String("token-role", "admin", "Role for token generation")
	tokenExpirationFlag := flag.Int("token-expiration", 744, "Token expiration in hours (default: 744 = 31 days)")

	flag.Parse()

	// Handle token generation if requested
	if *generateTokenFlag {
		secretKey := os.Getenv("GEMINI_AUTH_SECRET_KEY")
		CreateTokenCommand(secretKey, *tokenUserIDFlag, *tokenUsernameFlag, *tokenRoleFlag, *tokenExpirationFlag)
		return
	}

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

	// Override authentication if flag is provided
	if *authEnabledFlag {
		config.AuthEnabled = true
		logger.Info("Authentication feature enabled via command line flag")
	}

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

	// Validate transport flag
	if *transportFlag != "stdio" && *transportFlag != "http" {
		logger.Error("Invalid transport mode: %s. Must be 'stdio' or 'http'", *transportFlag)
		os.Exit(1)
	}

	// Start the appropriate transport based on command-line flag
	if *transportFlag == "http" {
		logger.Info("Starting Gemini MCP server with HTTP transport on %s%s", config.HTTPAddress, config.HTTPPath)
		if err := startHTTPServer(ctx, mcpServer, config, logger); err != nil {
			logger.Error("HTTP server error: %v", err)
			os.Exit(1)
		}
	} else {
		logger.Info("Starting Gemini MCP server with stdio transport")
		if err := server.ServeStdio(mcpServer); err != nil {
			logger.Error("Server error: %v", err)
			os.Exit(1)
		}
	}
}

// startHTTPServer starts the HTTP transport server
func startHTTPServer(ctx context.Context, mcpServer *server.MCPServer, config *Config, logger Logger) error {
	// Create HTTP server options
	var opts []server.StreamableHTTPOption

	// Configure heartbeat if enabled
	if config.HTTPHeartbeat > 0 {
		opts = append(opts, server.WithHeartbeatInterval(config.HTTPHeartbeat))
	}

	// Configure stateless mode
	if config.HTTPStateless {
		opts = append(opts, server.WithStateLess(true))
	}

	// Configure endpoint path
	opts = append(opts, server.WithEndpointPath(config.HTTPPath))

	// Add HTTP context function for CORS, logging, and authentication
	if config.HTTPCORSEnabled || config.AuthEnabled {
		opts = append(opts, server.WithHTTPContextFunc(createHTTPMiddleware(config, logger)))
	}

	// Create streamable HTTP server
	httpServer := server.NewStreamableHTTPServer(mcpServer, opts...)

	// Create custom HTTP server with OAuth well-known endpoint
	customServer := &http.Server{
		Addr:    config.HTTPAddress,
		Handler: createCustomHTTPHandler(httpServer, config, logger),
	}

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	wg.Add(1)

	// Start server in goroutine
	go func() {
		defer wg.Done()
		if err := customServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed to start: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		logger.Info("Received signal %v, shutting down HTTP server...", sig)
	case <-ctx.Done():
		logger.Info("Context cancelled, shutting down HTTP server...")
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), config.HTTPTimeout)
	defer shutdownCancel()

	if err := customServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error: %v", err)
		return err
	}

	wg.Wait()
	logger.Info("HTTP server stopped")
	return nil
}

// createHTTPMiddleware creates an HTTP context function with CORS, logging, and authentication
func createHTTPMiddleware(config *Config, logger Logger) server.HTTPContextFunc {
	// Create authentication middleware
	var authMiddleware *AuthMiddleware
	if config.AuthEnabled {
		authMiddleware = NewAuthMiddleware(config.AuthSecretKey, config.AuthEnabled, logger)
		logger.Info("HTTP authentication enabled")
	}

	return func(ctx context.Context, r *http.Request) context.Context {
		// Log HTTP request
		logger.Info("HTTP %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Apply authentication middleware if enabled
		if authMiddleware != nil {
			// Create a wrapper function for the next middleware step
			nextFunc := func(ctx context.Context, r *http.Request) context.Context {
				return ctx
			}
			// Apply authentication middleware
			ctx = authMiddleware.HTTPContextFunc(nextFunc)(ctx, r)
		}

		// Add CORS headers if enabled
		if config.HTTPCORSEnabled {
			// Check if request origin is allowed
			origin := r.Header.Get("Origin")
			if origin != "" && isOriginAllowed(origin, config.HTTPCORSOrigins) {
				// Note: We can't set response headers directly here as this is a context function
				// CORS headers would need to be handled at the HTTP server level
				logger.Info("CORS: Origin %s is allowed", origin)
			}
		}

		// Add request info to context
		ctx = context.WithValue(ctx, "http_method", r.Method)
		ctx = context.WithValue(ctx, "http_path", r.URL.Path)
		ctx = context.WithValue(ctx, "http_remote_addr", r.RemoteAddr)

		return ctx
	}
}

// isOriginAllowed checks if the origin is in the allowed list
func isOriginAllowed(origin string, allowedOrigins []string) bool {
	for _, allowed := range allowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
		// Support wildcard subdomains (e.g., "*.example.com")
		if strings.HasPrefix(allowed, "*.") {
			domain := strings.TrimPrefix(allowed, "*.")
			if strings.HasSuffix(origin, domain) {
				return true
			}
		}
	}
	return false
}

// createCustomHTTPHandler creates a custom HTTP handler that includes OAuth well-known endpoint
func createCustomHTTPHandler(mcpHandler http.Handler, config *Config, logger Logger) http.Handler {
	mux := http.NewServeMux()

	// Add OAuth well-known endpoint
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("OAuth well-known endpoint accessed from %s", r.RemoteAddr)

		// Create OAuth authorization server metadata
		metadata := map[string]interface{}{
			"issuer":                           fmt.Sprintf("http://%s", r.Host),
			"authorization_endpoint":           fmt.Sprintf("http://%s/oauth/authorize", r.Host),
			"token_endpoint":                   fmt.Sprintf("http://%s/oauth/token", r.Host),
			"response_types_supported":         []string{"code"},
			"grant_types_supported":            []string{"authorization_code"},
			"code_challenge_methods_supported": []string{"S256"},
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")

		// Add CORS headers if enabled
		if config.HTTPCORSEnabled {
			origin := r.Header.Get("Origin")
			if origin != "" && isOriginAllowed(origin, config.HTTPCORSOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
		}

		if err := json.NewEncoder(w).Encode(metadata); err != nil {
			logger.Error("Failed to encode OAuth metadata: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Handle all other requests with the MCP handler
	mux.Handle("/", mcpHandler)

	return mux
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

// wrapHandlerWithLogger creates a middleware wrapper for logging and authentication around a tool handler
func wrapHandlerWithLogger(handler server.ToolHandlerFunc, toolName string, logger Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logger.Info("Calling tool '%s'...", toolName)

		// Check authentication for HTTP requests if enabled
		// Note: We need to check if this is an HTTP request and if auth is enabled
		if httpMethod, ok := ctx.Value("http_method").(string); ok && httpMethod != "" {
			// This is an HTTP request, check if auth is required
			// Get config from the context (we'll need to pass it through)
			if authError := getAuthError(ctx); authError != "" {
				logger.Warn("Authentication failed for tool '%s': %s", toolName, authError)
				return createErrorResult(fmt.Sprintf("Authentication required: %s", authError)), nil
			}

			// Log successful authentication if present
			if isAuthenticated(ctx) {
				userID, username, role := getUserInfo(ctx)
				logger.Info("Tool '%s' called by authenticated user %s (%s) with role %s", toolName, username, userID, role)
			}
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

// Register error handlers for all tools
func registerErrorTools(mcpServer *server.MCPServer, errorServer *ErrorGeminiServer, logger Logger) {
	// Register error handlers for all tools using shared tool definitions
	mcpServer.AddTool(GeminiAskTool, wrapHandlerWithLogger(errorServer.handleErrorResponse, "gemini_ask", logger))
	mcpServer.AddTool(GeminiSearchTool, wrapHandlerWithLogger(errorServer.handleErrorResponse, "gemini_search", logger))
	mcpServer.AddTool(GeminiModelsTool, wrapHandlerWithLogger(errorServer.handleErrorResponse, "gemini_models", logger))

	logger.Info("Registered error handlers for all tools")
}

// Note: The adapter functions to communicate with the old code have been removed
// as they are no longer needed with direct handlers using mcp-go types
