# Project Context: GeminiMCP Server

## Overview

This project is a Go-based MCP (Model Control Protocol) server that acts as a bridge to Google's Gemini API. It's designed as a single, self-contained binary for easy deployment and use with MCP-compatible clients. The server supports the Gemini 3+ family of models.

## Architecture

- **Language**: Go (Golang)
- **Main Entrypoint**: `main.go`
- **Configuration**: `config.go` (environment variables with CLI overrides)
- **Core Logic**:
    - `gemini_server.go`: Gemini service initialization (client setup).
    - `gemini_ask_handler.go`: Handler for `gemini_ask` — file gathering, inline injection, binary upload.
    - `gemini_search_handler.go`: Handler for `gemini_search` — Google Search grounding.
    - `gemini_models_handler.go`: Handler for `gemini_models` — model documentation.
    - `prompt_handlers.go`: Handlers for MCP prompts.
    - `handlers_common.go`: Shared utilities — parameter extraction, model config, response formatting.
    - `tools.go`: MCP tool definitions (gemini_ask, gemini_search, gemini_models).
    - `file_handlers.go`: GitHub file fetching with concurrent downloads and retry.
- **Model Management**: `model_functions.go`, `fetch_models.go`, `completions.go`.
- **Transport**: Supports `stdio` and `http` (with JWT authentication via `auth.go`, `http_server.go`).
- **Caching**: Relies on Gemini's automatic implicit caching — files placed before query in request for optimal prefix matching.
- **Dependencies**:
    - `github.com/mark3labs/mcp-go/mcp`: MCP protocol implementation.
    - `github.com/mark3labs/mcp-go/server`: MCP server implementation.
    - `google.golang.org/genai`: Google Gemini API client.
    - `github.com/joho/godotenv`: for loading `.env` files.

## Development Guidelines

- **Build**: `go build -o ./bin/mcp-gemini .`
- **Testing**: `./run_test.sh`
- **Formatting**: `./run_format.sh`
- **Linting**: `./run_lint.sh`
- **Error Handling**: The server has a "degraded mode" to handle initialization errors gracefully.
- **Logging**: A custom logger is used throughout the application.

## AI Assistant Guidelines

- When adding a new tool, define it in `tools.go`, implement the handler in a dedicated `gemini_*_handler.go` file, and register it in `setupGeminiServer()` in `server_handlers.go`.
- When adding a new prompt, define it in `prompts.go`, implement the handler in `prompt_handlers.go` using the `server.PromptHandlerFunc` type, and register it in `setupGeminiServer()`.
- When modifying configuration, update `config.go` for defaults, `NewConfig()` for parsing, `structs.go` for the `Config` struct, and `main.go` for CLI flags.
- Always use `ResolveModelID()` before making API calls to convert model family IDs to specific version IDs.
- Text files are injected inline via `NewPartFromText()` — the MIME type map in `gemini_utils.go` determines what counts as text.
- Binary files (images, PDFs) are uploaded via the Files API in `processWithFiles()`.
- Parts ordering: files first (cacheable prefix), query last (variable suffix) — this maximizes implicit cache hits.
- Use the existing logging infrastructure for any new logging.
- Follow the existing code style and patterns.

## Other instructions

@./AGENTS.md

@./GOLANG.md
@./CODANNA.md
