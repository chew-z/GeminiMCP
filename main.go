package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/gomcpgo/mcp/pkg/handler"
	"github.com/gomcpgo/mcp/pkg/server"
	_ "github.com/joho/godotenv/autoload"
)

// main is the entry point for the application.
// It sets up the MCP server with the appropriate handlers and starts it.
func main() {
	// Define command-line flags for configuration override
	geminiModelFlag := flag.String("gemini-model", "", "Gemini model name (overrides env var)")
	geminiSystemPromptFlag := flag.String("gemini-system-prompt", "", "System prompt (overrides env var)")
	geminiTemperatureFlag := flag.Float64("gemini-temperature", -1, "Temperature setting (0.0-1.0, overrides env var)")
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

	// Override with command-line flags if provided
	if *geminiModelFlag != "" {
		// Validate the model ID before setting it
		if err := ValidateModelID(*geminiModelFlag); err != nil {
			logger.Error("Invalid model specified: %v", err)
			handleStartupError(ctx, fmt.Errorf("invalid model specified: %w", err))
			return
		}
		logger.Info("Overriding Gemini model with flag value: %s", *geminiModelFlag)
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

	// Set up handler registry
	// NewHandlerRegistry is a constructor that doesn't return an error
	registry := handler.NewHandlerRegistry()

	// Create and register the Gemini server
	if err := setupGeminiServer(ctx, registry, config); err != nil {
		handleStartupError(ctx, err)
		return
	}

	// Start the MCP server
	srv := server.New(server.Options{
		Name:     "gemini",
		Version:  "1.0.0",
		Registry: registry,
	})

	logger.Info("Starting Gemini MCP server")
	if err := srv.Run(); err != nil {
		logger.Error("Server error: %v", err)
		os.Exit(1)
	}
}

// setupGeminiServer creates and registers a Gemini server
func setupGeminiServer(ctx context.Context, registry *handler.HandlerRegistry, config *Config) error {
	loggerValue := ctx.Value(loggerKey)
	logger, ok := loggerValue.(Logger)
	if !ok {
		return fmt.Errorf("logger not found in context")
	}

	// Create the Gemini server with configuration
	geminiServer, err := NewGeminiServer(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create Gemini server: %w", err)
	}

	// Wrap the server with logger middleware
	handlerWithLogger := NewLoggerMiddleware(geminiServer, logger)

	// Register the wrapped server
	registry.RegisterToolHandler(handlerWithLogger)
	logger.Info("Registered Gemini server in normal mode with model: %s", config.GeminiModel)
	
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

	// Create error server
	errorServer := &ErrorGeminiServer{
		errorMessage: errorMsg,
	}

	// Set up registry with error server
	// NewHandlerRegistry is a constructor that doesn't return an error
	registry := handler.NewHandlerRegistry()
	errorServerWithLogger := NewLoggerMiddleware(errorServer, logger)
	registry.RegisterToolHandler(errorServerWithLogger)

	// Start server in degraded mode
	logger.Info("Starting Gemini MCP server in degraded mode")
	srv := server.New(server.Options{
		Name:     "gemini",
		Version:  "1.0.0",
		Registry: registry,
	})

	if err := srv.Run(); err != nil {
		logger.Error("Server error in degraded mode: %v", err)
		os.Exit(1)
	}
}
