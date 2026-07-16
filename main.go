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
	startHTTPServerFn    = startHTTPServer
	serveStdioFn         = server.ServeStdio
	getEnvFn             = os.Getenv
	newMCPServerFn       = func(config *Config, logger Logger) *server.MCPServer {
		return server.NewMCPServer("gemini", "1.0.0", buildMCPServerOptions(config, logger)...)
	}
)

// serverWebsiteURL is the canonical project URL advertised in serverInfo.
const serverWebsiteURL = "https://github.com/RobertJ-RM/GeminiMCP"

// buildMCPServerOptions assembles the server.ServerOption list used by both
// the normal startup path and the degraded-mode fallback in
// handleStartupError. Centralising the list guarantees that panic recovery,
// schema validation, and capability advertisements stay in lockstep across
// both servers.
func buildMCPServerOptions(config *Config, logger Logger) []server.ServerOption {
	opts := []server.ServerOption{
		server.WithTitle("Gemini MCP"),
		server.WithDescription("Gemini LLM for analysis, reasoning and research"),
		server.WithWebsiteURL(serverWebsiteURL),
		server.WithInstructions(`gemini_ask: send a prompt to the configured provider, optionally with GitHub repository context.
github_repo is required when using any github_* parameter. github_files requires github_ref.`),
		server.WithCompletions(),
		server.WithToolCapabilities(true),
		server.WithRecovery(),
		server.WithInputSchemaValidation(),
		server.WithStrictInputSchemaDefault(),
	}
	if config != nil && config.MaxConcurrentTasks > 0 {
		opts = append(opts,
			server.WithTaskCapabilities(
				true, // list:          tasks/list discoverable
				true, // cancel:        tasks/cancel honoured
				true, // toolCallTasks: tools may declare task support
			),
			server.WithMaxConcurrentTasks(config.MaxConcurrentTasks),
			server.WithTaskHooks(newTaskHooks(logger)),
		)
	}
	return opts
}

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
	geminiTemperature float64
	transport         string

	authEnabled     bool
	generateToken   bool
	tokenUserID     string
	tokenUsername   string
	tokenRole       string
	tokenExpiration int
}

// applyCLIOverrides applies temperature and auth overrides from
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

	if flags.authEnabled {
		config.AuthEnabled = true
		logger.Info("Authentication feature enabled via command line flag")
	}
	return nil
}

// validateAuthPostOverride enforces the post-CLI-override invariants that
// authentication requires. The canonical config loader rejects these cases
// earlier; this guard catches hand-rolled Config values (tests, future
// internal callers) plus the --auth-enabled flag flipping AuthEnabled after
// the loader ran.
func validateAuthPostOverride(config *Config, logger Logger) bool {
	if config.AuthEnabled && config.AuthSecretKey == "" {
		logger.Error("CRITICAL: Authentication is enabled, but GEMINI_AUTH_SECRET_KEY is not set. Server is shutting down.")
		return false
	}
	if config.AuthEnabled && config.HTTPPublicURL == "" {
		logger.Error("CRITICAL: Authentication is enabled, but GEMINI_HTTP_PUBLIC_URL is not set. " +
			"Set it to the externally-facing resource URL (e.g. https://mcp.example.com/mcp) so RFC 9728 metadata can be served.")
		return false
	}
	return true
}

// runTransport starts the requested transport and returns the process exit code.
func runTransport(ctx context.Context, mcpServer *server.MCPServer, config *Config, logger Logger, transport string) int {
	if transport != "stdio" && transport != "http" {
		logger.Error("Invalid transport mode: %s. Must be 'stdio' or 'http'", transport)
		return 1
	}

	if transport == "http" || (config.EnableHTTP && transport == "stdio") {
		if config.EnableHTTP && transport == "stdio" {
			logger.Warn("Transport 'stdio' requested but GEMINI_ENABLE_HTTP=true; starting HTTP transport instead")
		}
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
	flagSet.Float64Var(&flags.geminiTemperature, "gemini-temperature", -1, "Temperature setting (0.0-1.0, overrides env var)")
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

	// Fatal post-override checks: authentication demands a secret key and a
	// stable public URL; we must never enter degraded mode while advertising
	// auth.
	if !validateAuthPostOverride(config, logger) {
		return 1
	}

	mcpServer := newMCPServerFn(config, logger)
	logServerCapabilities(logger, config)

	if err := setupGeminiServerFn(ctx, mcpServer, config); err != nil {
		handleStartupErrorFn(ctx, err)
		return 0
	}

	return runTransport(ctx, mcpServer, config, logger, flags.transport)
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
