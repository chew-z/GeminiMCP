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

// cliFlags captures the parsed command-line flags for runMain.
type cliFlags struct {
	geminiModel       string
	geminiTemperature float64
	serviceTier       string
	transport         string

	authEnabled     bool
	generateToken   bool
	tokenUserID     string
	tokenUsername   string
	tokenRole       string
	tokenExpiration int
}

// applyCLIOverrides applies temperature, service-tier, and auth overrides from
// the command line to the loaded config. The returned error signals a
// recoverable startup failure (the caller should enter degraded mode).
func applyCLIOverrides(flags *cliFlags, config *Config, logger Logger) error {
	if flags.geminiTemperature >= 0 {
		if flags.geminiTemperature > 1.0 {
			logger.Error("Invalid temperature value: %v. Must be between 0.0 and 1.0", flags.geminiTemperature)
			return fmt.Errorf("invalid temperature: %v", flags.geminiTemperature)
		}
		logger.Info("Overriding Gemini temperature with flag value: %v", flags.geminiTemperature)
		config.GeminiTemperature = flags.geminiTemperature
	}

	if flags.serviceTier != "" {
		config.ServiceTier = flags.serviceTier
		logger.Info("Service tier overridden to: %s", config.ServiceTier)
	}
	logger.Info("Service tier: %s", config.ServiceTier)

	if flags.authEnabled {
		config.AuthEnabled = true
		logger.Info("Authentication feature enabled via command line flag")
	}
	return nil
}

// applyModelFlag validates and applies the --gemini-model flag. It must run
// after the model catalog is populated.
func applyModelFlag(modelFlag string, config *Config, logger Logger) error {
	if modelFlag == "" {
		return nil
	}
	validatedID, redirected, err := validateModelIDFn(modelFlag)
	if err != nil {
		logger.Error("Invalid --gemini-model: %v", err)
		return err
	}
	if redirected {
		logger.Warn("Custom model '%s' redirected to '%s'", modelFlag, validatedID)
	} else {
		logger.Info("Using known model: %s", validatedID)
	}
	config.GeminiModel = validatedID
	return nil
}

// resolveDefaultModels maps tier-level model aliases (gemini-pro, gemini-flash,
// …) to concrete IDs against the populated catalog.
func resolveDefaultModels(ctx context.Context, config *Config) error {
	var err error
	if config.GeminiModel, err = resolveModelFn(ctx, config.GeminiModel); err != nil {
		return err
	}
	if config.GeminiSearchModel, err = resolveModelFn(ctx, config.GeminiSearchModel); err != nil {
		return err
	}
	return nil
}

// runTransport starts the requested transport and returns the process exit code.
func runTransport(ctx context.Context, mcpServer *server.MCPServer, config *Config, logger Logger, transport string) int {
	if transport != "stdio" && transport != "http" {
		logger.Error("Invalid transport mode: %s. Must be 'stdio' or 'http'", transport)
		return 1
	}

	if transport == "http" || (config.EnableHTTP && transport == "stdio") {
		logger.Info("Starting Gemini MCP server with HTTP transport on %s%s", config.HTTPAddress, config.HTTPPath)
		if err := startHTTPServerFn(ctx, mcpServer, config, logger); err != nil {
			logger.Error("HTTP server error: %v", err)
			return 1
		}
		return 0
	}

	logger.Info("Starting Gemini MCP server with stdio transport")
	if err := serveStdioFn(mcpServer); err != nil {
		logger.Error("Server error: %v", err)
		return 1
	}
	return 0
}

func runMain(args []string) int {
	flagSet := flag.NewFlagSet("GeminiMCP", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	flags := &cliFlags{}
	flagSet.StringVar(&flags.geminiModel, "gemini-model", "", "Gemini model name (overrides env var)")
	flagSet.Float64Var(&flags.geminiTemperature, "gemini-temperature", -1, "Temperature setting (0.0-1.0, overrides env var)")
	flagSet.StringVar(&flags.serviceTier, "service-tier", "", "Service tier: flex, standard, priority (overrides env var)")
	flagSet.StringVar(&flags.transport, "transport", "stdio", "Transport mode: 'stdio' (default) or 'http'")
	flagSet.BoolVar(&flags.authEnabled, "auth-enabled", false, "Enable JWT authentication for HTTP transport (overrides env var)")
	flagSet.BoolVar(&flags.generateToken, "generate-token", false, "Generate a JWT token and exit")
	flagSet.StringVar(&flags.tokenUserID, "token-user-id", "user1", "User ID for token generation")
	flagSet.StringVar(&flags.tokenUsername, "token-username", "admin", "Username for token generation")
	flagSet.StringVar(&flags.tokenRole, "token-role", "admin", "Role for token generation")
	flagSet.IntVar(&flags.tokenExpiration, "token-expiration", 744, "Token expiration in hours (default: 744 = 31 days)")

	if err := flagSet.Parse(args); err != nil {
		return 1
	}

	if flags.generateToken {
		secretKey := getEnvFn("GEMINI_AUTH_SECRET_KEY")
		createTokenCommandFn(secretKey, flags.tokenUserID, flags.tokenUsername, flags.tokenRole, flags.tokenExpiration)
		return 0
	}

	logger := newLoggerFn(parseLogLevel(getEnvFn("GEMINI_LOG_LEVEL"), LevelInfo))
	config, err := newConfigFn(logger)
	if err != nil {
		ctx := context.WithValue(context.Background(), loggerKey, logger)
		handleStartupErrorFn(ctx, err)
		return 0
	}

	ctx := context.WithValue(context.Background(), loggerKey, logger)
	ctx = context.WithValue(ctx, configKey, config)

	if err := applyCLIOverrides(flags, config, logger); err != nil {
		handleStartupErrorFn(ctx, err)
		return 0
	}

	// Fatal post-override check: authentication demands a secret key; we must
	// never enter degraded mode while advertising auth.
	if config.AuthEnabled && config.AuthSecretKey == "" {
		logger.Error("CRITICAL: Authentication is enabled, but GEMINI_AUTH_SECRET_KEY is not set. Server is shutting down.")
		return 1
	}

	mcpServer := newMCPServerFn(config, logger)
	logServerCapabilities(logger, config)

	if err := setupGeminiServerFn(ctx, mcpServer, config); err != nil {
		handleStartupErrorFn(ctx, err)
		return 0
	}

	if err := resolveDefaultModels(ctx, config); err != nil {
		handleStartupErrorFn(ctx, err)
		return 0
	}

	if err := applyModelFlag(flags.geminiModel, config, logger); err != nil {
		handleStartupErrorFn(ctx, err)
		return 0
	}

	return runTransport(ctx, mcpServer, config, logger, flags.transport)
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
