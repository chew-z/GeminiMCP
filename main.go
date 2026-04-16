package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/mark3labs/mcp-go/server"
)

var (
	runMainFn = runMain
	osExitFn  = os.Exit
	osArgsFn  = func() []string { return os.Args[1:] }

	createTokenCommandFn = CreateTokenCommand
	newLoggerFn          = NewLogger
	newConfigFn          = NewConfig
	handleStartupErrorFn = handleStartupError
	setupGeminiServerFn  = setupGeminiServer
	resolveModelFn       = resolveAndValidateModel
	validateModelIDFn    = ValidateModelID
	startHTTPServerFn    = startHTTPServer
	serveStdioFn         = server.ServeStdio
	getEnvFn             = os.Getenv
	newMCPServerFn       = func(config *Config, logger Logger) *server.MCPServer {
		opts := []server.ServerOption{
			server.WithTitle("Gemini MCP"),
			server.WithDescription("Gemini LLM for analysis, reasoning and research"),
			server.WithInstructions(`gemini_ask: send a prompt to Gemini, optionally with GitHub repository context.
gemini_search: answer questions using web search with source citations.

Defaults are optimized per model tier. Override parameters exist but are rarely needed.
github_repo is required when using any github_* parameter. github_files requires github_ref.`),
			server.WithCompletions(),
			server.WithPromptCompletionProvider(&GeminiCompletionProvider{}),
			server.WithToolCapabilities(true),
		}
		if config.MaxConcurrentTasks > 0 {
			opts = append(opts,
				server.WithTaskCapabilities(true, true, true),
				server.WithMaxConcurrentTasks(config.MaxConcurrentTasks),
				server.WithTaskHooks(newTaskHooks(logger)),
			)
		}
		return server.NewMCPServer("gemini", "1.0.0", opts...)
	}
)

// main is the entry point for the application.
// It sets up the MCP server with the appropriate handlers and starts it.
func main() {
	exitCode := runMainFn(osArgsFn())
	if exitCode != 0 {
		osExitFn(exitCode)
	}
}

func runMain(args []string) int {
	flagSet := flag.NewFlagSet("GeminiMCP", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	// Define command-line flags for configuration override
	geminiModelFlag := flagSet.String("gemini-model", "", "Gemini model name (overrides env var)")
	geminiTemperatureFlag := flagSet.Float64("gemini-temperature", -1, "Temperature setting (0.0-1.0, overrides env var)")
	serviceTierFlag := flagSet.String("service-tier", "", "Service tier: flex, standard, priority (overrides env var)")
	transportFlag := flagSet.String("transport", "stdio", "Transport mode: 'stdio' (default) or 'http'")

	// Authentication flags
	authEnabledFlag := flagSet.Bool("auth-enabled", false, "Enable JWT authentication for HTTP transport (overrides env var)")
	generateTokenFlag := flagSet.Bool("generate-token", false, "Generate a JWT token and exit")
	tokenUserIDFlag := flagSet.String("token-user-id", "user1", "User ID for token generation")
	tokenUsernameFlag := flagSet.String("token-username", "admin", "Username for token generation")
	tokenRoleFlag := flagSet.String("token-role", "admin", "Role for token generation")
	tokenExpirationFlag := flagSet.Int("token-expiration", 744, "Token expiration in hours (default: 744 = 31 days)")

	if err := flagSet.Parse(args); err != nil {
		return 1
	}

	// Handle token generation if requested
	if *generateTokenFlag {
		secretKey := getEnvFn("GEMINI_AUTH_SECRET_KEY")
		createTokenCommandFn(secretKey, *tokenUserIDFlag, *tokenUsernameFlag, *tokenRoleFlag, *tokenExpirationFlag)
		return 0
	}

	// Create application context with logger
	logger := newLoggerFn(LevelInfo)
	config, err := newConfigFn(logger)
	if err != nil {
		// Create a temporary context just for this error
		ctx := context.WithValue(context.Background(), loggerKey, logger)
		handleStartupErrorFn(ctx, err)
		return 0
	}

	// Now create the main context with everything it needs
	ctx := context.WithValue(context.Background(), loggerKey, logger)
	ctx = context.WithValue(ctx, configKey, config)

	// Override temperature if provided and valid
	if *geminiTemperatureFlag >= 0 {
		// Validate temperature is within range
		if *geminiTemperatureFlag > 1.0 {
			logger.Error("Invalid temperature value: %v. Must be between 0.0 and 1.0", *geminiTemperatureFlag)
			handleStartupErrorFn(ctx, fmt.Errorf("invalid temperature: %v", *geminiTemperatureFlag))
			return 0
		}
		logger.Info("Overriding Gemini temperature with flag value: %v", *geminiTemperatureFlag)
		config.GeminiTemperature = *geminiTemperatureFlag
	}

	// Override service tier if flag is provided
	if *serviceTierFlag != "" {
		config.ServiceTier = *serviceTierFlag
		logger.Info("Service tier overridden to: %s", config.ServiceTier)
	}
	logger.Info("Service tier: %s", config.ServiceTier)

	// Override authentication if flag is provided
	if *authEnabledFlag {
		config.AuthEnabled = true
		logger.Info("Authentication feature enabled via command line flag")
	}

	// Final validation of authentication configuration after all overrides
	if config.AuthEnabled && config.AuthSecretKey == "" {
		// This is a critical security failure.
		// We must exit immediately and not enter degraded mode.
		logger.Error("CRITICAL: Authentication is enabled, but GEMINI_AUTH_SECRET_KEY is not set. Server is shutting down.")
		return 1
	}

	// Create MCP server
	mcpServer := newMCPServerFn(config, logger)
	logServerCapabilities(logger, config)

	// Create and register the Gemini server tools (populates model catalog)
	if err := setupGeminiServerFn(ctx, mcpServer, config); err != nil {
		handleStartupErrorFn(ctx, err)
		return 0
	}

	// Resolve default model names against the now-populated catalog.
	// Tier-level defaults like "gemini-pro" get mapped to actual model IDs.
	var resolveErr error
	config.GeminiModel, resolveErr = resolveModelFn(ctx, config.GeminiModel)
	if resolveErr != nil {
		handleStartupErrorFn(ctx, resolveErr)
		return 0
	}
	config.GeminiSearchModel, resolveErr = resolveModelFn(ctx, config.GeminiSearchModel)
	if resolveErr != nil {
		handleStartupErrorFn(ctx, resolveErr)
		return 0
	}

	// Override model AFTER catalog is populated so ValidateModelID works correctly
	if *geminiModelFlag != "" {
		validatedID, redirected, flagErr := validateModelIDFn(*geminiModelFlag)
		if flagErr != nil {
			logger.Error("Invalid --gemini-model: %v", flagErr)
			handleStartupErrorFn(ctx, flagErr)
			return 0
		}
		if redirected {
			logger.Warn("Custom model '%s' redirected to '%s'", *geminiModelFlag, validatedID)
		} else {
			logger.Info("Using known model: %s", validatedID)
		}
		config.GeminiModel = validatedID
	}

	// Validate transport flag
	if *transportFlag != "stdio" && *transportFlag != "http" {
		logger.Error("Invalid transport mode: %s. Must be 'stdio' or 'http'", *transportFlag)
		return 1
	}

	// Start the appropriate transport based on command-line flag or config
	if *transportFlag == "http" || (config.EnableHTTP && *transportFlag == "stdio") {
		logger.Info("Starting Gemini MCP server with HTTP transport on %s%s", config.HTTPAddress, config.HTTPPath)
		if err := startHTTPServerFn(ctx, mcpServer, config, logger); err != nil {
			logger.Error("HTTP server error: %v", err)
			return 1
		}
	} else {
		logger.Info("Starting Gemini MCP server with stdio transport")
		if err := serveStdioFn(mcpServer); err != nil {
			logger.Error("Server error: %v", err)
			return 1
		}
	}

	return 0
}

// Helper function to get feature status as a string
func getFeatureStatusStr(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// logServerCapabilities summarises advertised MCP capabilities at startup so
// operators can see at a glance what the server will offer clients.
func logServerCapabilities(logger Logger, config *Config) {
	logger.Info("MCP capabilities: tools=true completions=true prompt_completion=true")

	if config.MaxConcurrentTasks > 0 {
		logger.Info(
			"MCP capabilities: tasks=enabled max_concurrent=%d progress=supported heartbeat=supported cancel=supported",
			config.MaxConcurrentTasks,
		)
	} else {
		logger.Info("MCP capabilities: tasks=disabled (GEMINI_MAX_CONCURRENT_TASKS<=0)")
	}

	if config.ProgressInterval > 0 {
		logger.Info("MCP capabilities: progress_notifications=enabled interval=%s", config.ProgressInterval)
	} else {
		logger.Info("MCP capabilities: progress_notifications=disabled (GEMINI_PROGRESS_INTERVAL<=0)")
	}
}
