package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/mark3labs/mcp-go/server"
)

// main is the entry point for the application.
// It sets up the MCP server with the appropriate handlers and starts it.
func main() {
	// Define command-line flags for configuration override
	geminiModelFlag := flag.String("gemini-model", "", "Gemini model name (overrides env var)")
	geminiSystemPromptFlag := flag.String("gemini-system-prompt", "", "System prompt (overrides env var)")
	geminiTemperatureFlag := flag.Float64("gemini-temperature", -1, "Temperature setting (0.0-1.0, overrides env var)")
	enableThinkingFlag := flag.Bool("enable-thinking", true, "Enable thinking mode for supported models (overrides env var)")
	serviceTierFlag := flag.String("service-tier", "", "Service tier: flex, standard, priority (overrides env var)")
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
	config, err := NewConfig(logger)
	if err != nil {
		// Create a temporary context just for this error
		ctx := context.WithValue(context.Background(), loggerKey, logger)
		handleStartupError(ctx, err)
		return
	}

	// Now create the main context with everything it needs
	ctx := context.WithValue(context.Background(), loggerKey, logger)
	ctx = context.WithValue(ctx, configKey, config)

	// Override non-model flags before catalog fetch
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

	// Override enable thinking if flag is provided
	config.EnableThinking = *enableThinkingFlag
	logger.Info("Thinking feature is %s", getFeatureStatusStr(config.EnableThinking))

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
		os.Exit(1)
	}

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"gemini",
		"1.0.0",
		server.WithCompletions(),
		server.WithPromptCompletionProvider(&GeminiCompletionProvider{}),
	)

	// Create and register the Gemini server tools (populates model catalog)
	if err := setupGeminiServer(ctx, mcpServer, config); err != nil {
		handleStartupError(ctx, err)
		return
	}

	// Resolve default model names against the now-populated catalog.
	// Tier-level defaults like "gemini-pro" get mapped to actual model IDs.
	config.GeminiModel = resolveAndValidateModel(ctx, config.GeminiModel)
	config.GeminiSearchModel = resolveAndValidateModel(ctx, config.GeminiSearchModel)

	// Override model AFTER catalog is populated so ValidateModelID works correctly
	if *geminiModelFlag != "" {
		validatedID, redirected := ValidateModelID(*geminiModelFlag)
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
		os.Exit(1)
	}

	// Start the appropriate transport based on command-line flag or config
	if *transportFlag == "http" || (config.EnableHTTP && *transportFlag == "stdio") {
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

// Helper function to get feature status as a string
func getFeatureStatusStr(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
