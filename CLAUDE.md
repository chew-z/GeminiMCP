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

# Run tests with verbose output
go test -v ./...

# Run specific test
go test -v -run TestConfigDefaults

# Run tests with coverage
go test -cover ./...
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

# Run server with HTTP transport
./bin/mcp-gemini --transport=http

# Run server with custom settings via command line
./bin/mcp-gemini --gemini-model=gemini-2.5-flash --enable-caching=true --transport=http

# Test MCP server locally (requires MCP client)
# Check server responds to MCP protocol messages

# Validate configuration and see available options
./bin/mcp-gemini --help
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

# HTTP transport settings (optional)
export GEMINI_ENABLE_HTTP="true"
export GEMINI_HTTP_ADDRESS=":8081"
export GEMINI_HTTP_PATH="/mcp"
export GEMINI_HTTP_STATELESS="false"
export GEMINI_HTTP_HEARTBEAT="30s"
export GEMINI_HTTP_CORS_ENABLED="true"
export GEMINI_HTTP_CORS_ORIGINS="*"
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

### Transport Modes

The server supports two transport modes, configurable via the `--transport` command-line flag:

1. **Stdio Transport** (default): Traditional stdin/stdout communication for command-line MCP clients
   ```bash
   ./bin/mcp-gemini --transport=stdio  # or just ./bin/mcp-gemini
   ```

2. **HTTP Transport**: RESTful HTTP endpoints with optional WebSocket upgrade for real-time communication
   ```bash
   ./bin/mcp-gemini --transport=http
   ```

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

## HTTP Transport Usage

### Starting with HTTP Transport

```bash
# Enable HTTP transport
export GEMINI_ENABLE_HTTP=true
export GEMINI_HTTP_ADDRESS=":8081"

# Start the server
./bin/mcp-gemini
```

### Authentication for HTTP Transport

The server supports JWT-based authentication for HTTP transport to secure API access.

#### Configuration

```bash
# Enable authentication
export GEMINI_AUTH_ENABLED=true
export GEMINI_AUTH_SECRET_KEY="your-secret-key-at-least-32-characters"

# Start server with authentication
./bin/mcp-gemini --transport=http --auth-enabled=true
```

#### Generate Authentication Tokens

```bash
# Generate a token for an admin user (31 days expiration by default)
export GEMINI_AUTH_SECRET_KEY="your-secret-key-at-least-32-characters"
./bin/mcp-gemini --generate-token --token-username=admin --token-role=admin

# Generate a token for a regular user (24-hour expiration)
./bin/mcp-gemini --generate-token --token-username=user1 --token-role=user --token-expiration=24
```

#### Using Authentication Tokens

Include the JWT token in HTTP requests using the Authorization header:

```bash
# Store the token (replace with actual token from generation command)
TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# Use token in API requests
curl -X POST http://localhost:8081/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```

**Security Notes:**
- The secret key should be at least 32 characters long for security
- Tokens are signed with HMAC-SHA256
- Authentication only applies to HTTP transport, not stdio transport
- Failed authentication attempts are logged with IP addresses

### HTTP Endpoints

When HTTP transport is enabled, the following endpoints are available:

- `GET /mcp` - Server-Sent Events (SSE) endpoint for real-time communication
- `POST /mcp` - Message endpoint for request/response communication

### Example HTTP Requests

```bash
# List available tools
curl -X POST http://localhost:8081/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'

# Call gemini_ask tool
curl -X POST http://localhost:8081/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "gemini_ask",
      "arguments": {
        "query": "Explain the MCP protocol"
      }
    }
  }'
```

### CORS Configuration

For web applications, configure CORS settings:

```bash
export GEMINI_HTTP_CORS_ENABLED=true
export GEMINI_HTTP_CORS_ORIGINS="https://myapp.com,https://localhost:3000"
```

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

### Running Tests

The project has comprehensive test coverage for core functionality:

1. **Configuration Tests** (`config_test.go`): Validates environment variable parsing, defaults, and overrides
2. **API Integration Tests** (`gemini_test.go`): Tests actual Gemini API interactions (requires API key)

When modifying configuration or API handling code, always run the relevant tests to ensure functionality remains intact.

### Key Dependencies

This project uses specific Go libraries that should be maintained:

- `github.com/mark3labs/mcp-go/mcp` - Core MCP protocol implementation
- `google.golang.org/genai` - Official Google Generative AI SDK
- `github.com/joho/godotenv` - Environment variable loading (auto-loaded via import)

The project targets Go 1.24+ and should maintain backward compatibility within the Go 1.x series.

### Command-Line Options

The server supports several command-line flags that override environment variables:

```bash
./bin/mcp-gemini [OPTIONS]

Available options:
  --gemini-model string          Gemini model name (overrides GEMINI_MODEL)
  --gemini-system-prompt string  System prompt (overrides GEMINI_SYSTEM_PROMPT)
  --gemini-temperature float     Temperature setting 0.0-1.0 (overrides GEMINI_TEMPERATURE)
  --enable-caching              Enable caching feature (overrides GEMINI_ENABLE_CACHING)
  --enable-thinking             Enable thinking mode (overrides GEMINI_ENABLE_THINKING)
  --transport string            Transport mode: 'stdio' (default) or 'http'
  --auth-enabled                Enable JWT authentication for HTTP transport (overrides GEMINI_AUTH_ENABLED)
  --generate-token              Generate a JWT token and exit
  --token-user-id string        User ID for token generation (default: "user1")
  --token-username string       Username for token generation (default: "admin")
  --token-role string           Role for token generation (default: "admin")
  --token-expiration int        Token expiration in hours (default: 744 = 31 days)
  --help                        Show help information
```

**Examples:**
```bash
# Start with HTTP transport and custom model
./bin/mcp-gemini --transport=http --gemini-model=gemini-2.5-flash

# Enable HTTP transport with authentication
./bin/mcp-gemini --transport=http --auth-enabled=true

# Disable caching and thinking mode
./bin/mcp-gemini --enable-caching=false --enable-thinking=false

# Set custom temperature and system prompt
./bin/mcp-gemini --gemini-temperature=0.8 --gemini-system-prompt="You are a helpful assistant"

# Generate a JWT token for authentication (31 days expiration by default)
export GEMINI_AUTH_SECRET_KEY="your-secret-key-at-least-32-characters"
./bin/mcp-gemini --generate-token --token-username=admin --token-role=admin
```

