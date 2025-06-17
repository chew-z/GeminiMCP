# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GeminiMCP is a Model Control Protocol (MCP) server that integrates with Google's Gemini API. It serves as a bridge between MCP-compatible clients (like Claude Desktop) and Google's Gemini models, providing a standardized interface for model interactions.

**Note: This server exclusively supports Gemini 2.5 family models.** Only Gemini 2.5 models (Pro, Flash, Flash Lite) are supported as they provide optimal thinking mode and implicit caching capabilities.

## Build and Development Commands

### Building the Project

```bash
# Build the binary
go build -o ./bin/mcp-gemini .
```

### Testing

```bash
# Run all tests (sets PATH for Go binary)
./run_test.sh
```

### Code Formatting and Linting

```bash
# Format code with gofmt
./run_format.sh

# Run linter (golangci-lint)
./run_lint.sh

# Both scripts should be run before committing changes
```

### Debugging and Troubleshooting

```bash
# Run server with debug logging
GEMINI_LOG_LEVEL=debug ./bin/mcp-gemini

# Test MCP server locally (requires MCP client)
# Check server responds to MCP protocol messages

# Validate configuration
./mcp-gemini --help
```

### Development Environment Setup

```bash
# Essential environment variables for development
export GEMINI_API_KEY="your_api_key_here"
export GEMINI_MODEL="gemini-2.5-pro-06-05"
export GEMINI_SEARCH_MODEL="gemini-2.5-flash-preview-05-20"

# Optional development settings
export GEMINI_LOG_LEVEL="debug"
export GEMINI_ENABLE_CACHING="true"
export GEMINI_ENABLE_THINKING="true"
```

## Architecture Overview

The GeminiMCP server follows a clean, modular architecture:

1. **Server Initialization** (`main.go`): Entry point that configures and launches the MCP server.

2. **Configuration** (`config.go`): Handles environment variables, flags, and defaults for the server.

3. **Tool Definitions** (`tools.go`): Defines the MCP tools (gemini_ask, gemini_search, gemini_models) exposed by the server.

4. **Handlers** (`direct_handlers.go`): Implements the tool handlers that process requests and interact with the Gemini API.

5. **Model Management** (`fetch_models.go`, `fallback_models.go`): Fetches available models from the Gemini API and maintains model metadata.

6. **Context Caching** (`cache.go`): Implements caching for Gemini contexts to improve performance for repeated queries.

7. **File Handling** (`files.go`): Manages file uploads for providing context to Gemini.

8. **Utility Components**:
   - `logger.go`: Logger for consistent log output
   - `context.go`: Context key definitions
   - `middleware.go`: Request processing middleware
   - `structs.go`: Shared data structures
   - `gemini_utils.go`: Utility functions for interacting with Gemini API

## Key Concepts

### MCP Integration

The server implements the Model Control Protocol to provide a standardized interface for AI model interactions. The `github.com/mark3labs/mcp-go` library handles the protocol specifics.

### Tool Handlers

Three primary tools are exposed:

1. **gemini_ask**: For general queries, code analysis, and creative tasks
2. **gemini_search**: For grounded search queries using Google Search
3. **gemini_models**: For listing available Gemini models

### Model Management

The server dynamically fetches available Gemini models at startup and organizes them by preferences and capabilities. Models are categorized for specific tasks (thinking, caching, search).

### Caching System

A sophisticated caching system allows for efficient repeated queries, particularly useful for code analysis. Only models with specific version suffixes (e.g., `-001`) support caching.

### Thinking Mode

Certain Gemini models (primarily Pro models) support "thinking mode" which exposes the model's reasoning process. The server configures thinking with adjustable budget levels.

### Error Handling

The server implements graceful degradation with fallback models and a dedicated error server mode when initialization fails.

## Common Workflows

### Adding a New Tool

1. Define the tool specification in `tools.go` using the `mcp.NewTool` function with appropriate parameters
2. Create a handler function in `direct_handlers.go` following existing patterns
3. Register the tool in `setupGeminiServer` in `main.go`
4. Add error handling in `registerErrorTools` in `main.go`

### Modifying Configuration

1. Update default values or variable names in `config.go`
2. Add env variable parsing in `NewConfig` function
3. Update the `Config` struct in `structs.go` if needed
4. Add flag handling in `main.go` if the setting should be configurable via CLI

### Updating Model Handling

1. Modify model capabilities and preferences in `fetch_models.go`
2. If adding fallbacks, update `fallback_models.go`
3. Test with different model IDs to ensure proper resolution
4. **Important**: Always use `ResolveModelID()` when passing model names to the Gemini API to convert family IDs (like `gemini-2.5-flash`) to specific version IDs (like `gemini-2.5-flash-preview-05-20`)

@/Users/rrj/Projekty/CodeAssist/Prompts/EDIT-STEP-BY-STEP.md
