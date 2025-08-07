# Project Context: GeminiMCP Server

## Overview
This project is a Go-based MCP (Model Control Protocol) server that acts as a bridge to Google's Gemini API. It's designed as a single, self-contained binary for easy deployment and use with MCP-compatible clients. The server exclusively supports the Gemini 2.5 family of models.

## Architecture
- **Language**: Go (Golang)
- **Main Entrypoint**: `main.go`
- **Configuration**: `config.go` (environment variables with CLI overrides)
- **Core Logic**:
    - `gemini_server.go`: Gemini service implementation.
    - `direct_handlers.go`: Handlers for the MCP tools.
    - `tools.go`: Definitions of the MCP tools.
- **Transport**: Supports `stdio` and `http` (with JWT authentication).
- **Dependencies**:
    - `github.com/mark3labs/mcp-go/mcp`: MCP protocol implementation.
    - `google.golang.org/genai`: Google Gemini API client.
    - `github.com/joho/godotenv`: for loading `.env` files.

## Development Guidelines
- **Build**: `go build -o ./bin/mcp-gemini .`
- **Testing**: `./run_test.sh`
- **Formatting**: `./run_format.sh`
- **Linting**: `./run_lint.sh`
- **Error Handling**: The server has a "degraded mode" to handle initialization errors gracefully.
- **Logging**: A custom logger is used throughout the application.

## Key Dependencies & Integrations
- **Google Gemini API**: The core integration. Requires a `GEMINI_API_KEY`.
- **MCP**: The server implements the Model Control Protocol.

## Configuration
The server is configured via environment variables, which can be overridden by CLI flags.
**Required:**
- `GEMINI_API_KEY`: Your Google AI Studio API key.

**Key Environment Variables:**
- `GEMINI_MODEL`: Default model for `gemini_ask` (e.g., `gemini-2.5-pro`).
- `GEMINI_SEARCH_MODEL`: Default model for `gemini_search` (e.g., `gemini-2.5-flash-lite`).
- `GEMINI_ENABLE_CACHING`: Enable/disable context caching (`true`/`false`).
- `GEMINI_ENABLE_THINKING`: Enable/disable thinking mode (`true`/`false`).
- `GEMINI_LOG_LEVEL`: Logging level (`debug`, `info`, `warn`, `error`).
- `GEMINI_ENABLE_HTTP`: Enable the HTTP transport (`true`/`false`).
- `GEMINI_HTTP_ADDRESS`: HTTP server address (e.g., `:8081`).
- `GEMINI_AUTH_ENABLED`: Enable JWT authentication for HTTP (`true`/`false`).
- `GEMINI_AUTH_SECRET_KEY`: Secret key for JWT.

## AI Assistant Guidelines
- When adding a new tool, define it in `tools.go`, implement the handler in `direct_handlers.go`, and register it in `setupGeminiServer()` in `main.go`.
- When modifying configuration, update `config.go` for defaults, `NewConfig()` for parsing, `structs.go` for the `Config` struct, and `main.go` for CLI flags.
- Always use `ResolveModelID()` before making API calls to convert model family IDs to specific version IDs.
- Use the existing logging infrastructure for any new logging.
- Follow the existing code style and patterns.
