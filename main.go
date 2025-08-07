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

// Helper function to get caching status as a string
func getCachingStatusStr(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
