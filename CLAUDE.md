# CLAUDE.md

GeminiMCP is an Model Control Protocol (MCP) server bridging MCP clients with Google's Gemini 2.5 family models (Pro, Flash, Flash-Lite).

## Build Commands

```bash
# Build
go build -o ./bin/mcp-gemini .

# Test & Format (run before commits)
./run_test.sh
./run_format.sh
./run_lint.sh
```

## Architecture

**Core Files:**
- `main.go` - Server initialization, transport handling
- `config.go` - Environment variables, CLI flags, defaults
- `tools.go` - MCP tool definitions (gemini_ask, gemini_search, gemini_models)
- `direct_handlers.go` - Tool request handlers, Gemini API interaction
- `fetch_models.go`, `fallback_models.go` - Dynamic model management
- `cache.go` - Context caching for repeated queries
- `auth.go` - JWT authentication for HTTP transport

**Key Functions:**
- `ResolveModelID()` - Convert family IDs to specific version IDs (ALWAYS use before API calls)
- `ValidateModelID()` - Check model ID validity
- `NewConfig()` - Parse environment variables and flags

## Supported Models (As of GA Release)

- `gemini-2.5-pro` (production) - Advanced thinking, caching, 1M context
- `gemini-2.5-flash` (production) - Balanced performance, caching, 32K context  
- `gemini-2.5-flash-lite` (production) - Cost-optimized, no caching, 32K context

## Configuration

**Required:**
- `GEMINI_API_KEY` - Google AI Studio API key

**Key Environment Variables:**
```bash
GEMINI_MODEL="gemini-2.5-pro"                  # Default ask model
GEMINI_SEARCH_MODEL="gemini-2.5-flash-lite"   # Default search model
GEMINI_ENABLE_CACHING="true"                   # Context caching
GEMINI_ENABLE_THINKING="true"                  # Thinking mode
GEMINI_LOG_LEVEL="info"                        # debug|info|warn|error
```

**HTTP Transport:**
```bash
GEMINI_ENABLE_HTTP="true"
GEMINI_HTTP_ADDRESS=":8081"
GEMINI_HTTP_PATH="/mcp"
```

**Authentication (HTTP only):**
```bash
GEMINI_AUTH_ENABLED="true"
GEMINI_AUTH_SECRET_KEY="32char-minimum-secret"
```

## CLI Flags (Override Environment)

```bash
./bin/mcp-gemini --transport=http --gemini-model=gemini-2.5-flash --enable-caching=false
./bin/mcp-gemini --generate-token --token-username=admin --token-role=admin
```

## Transport Modes

**Stdio (default):** `./bin/mcp-gemini`
**HTTP:** `./bin/mcp-gemini --transport=http`

## HTTP Endpoints

- `POST /mcp` - MCP protocol messages  
- `GET /mcp` - Server-Sent Events
- `GET /.well-known/oauth-authorization-server` - OAuth metadata

## Development Workflows

**Add Tool:**
1. Define in `tools.go` using `mcp.NewTool()`
2. Implement handler in `direct_handlers.go`
3. Register in `setupGeminiServer()` in `main.go`
4. Add error handler in `registerErrorTools()`

**Modify Config:**
1. Update defaults in `config.go`
2. Add parsing in `NewConfig()`
3. Update `Config` struct in `structs.go`
4. Add CLI flag in `main.go` if needed

**Update Models:**
1. Modify `fetch_models.go` or `fallback_models.go`
2. Use `ResolveModelID()` for API calls
3. Test model resolution

## Recent Changes

- **Commit 9158646:** Updated Gemini 2.5 Flash Lite to GA model (`gemini-2.5-flash-lite`)
- **Commit 01b642f:** Added OAuth well-known endpoint (`/.well-known/oauth-authorization-server`)
- **Commit 48098fa, f3d47d8:** Improved authentication error handling and stderr logging
- **Commit 9f6ab9a:** Implemented JWT authentication for HTTP transport
- **Commit f78ae6c:** Added HTTP transport option

## Key Dependencies

- `github.com/mark3labs/mcp-go/mcp` - MCP protocol
- `google.golang.org/genai` - Gemini API SDK
- `github.com/joho/godotenv` - Environment loading

## Testing

```bash
# Configuration tests
go test -v -run TestConfigDefaults

# API integration tests (requires GEMINI_API_KEY)
go test -v -run TestGeminiServer
```

## Debug Commands

```bash
# Debug logging
GEMINI_LOG_LEVEL=debug ./bin/mcp-gemini

# HTTP with auth
./bin/mcp-gemini --transport=http --auth-enabled=true

# Generate auth token
export GEMINI_AUTH_SECRET_KEY="your-32char-secret"
./bin/mcp-gemini --generate-token --token-username=admin

# Test HTTP endpoint
curl -X POST http://localhost:8081/mcp -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```

**Security Notes:**
- Auth only applies to HTTP transport
- Secret key must be 32+ characters
- Tokens use HMAC-SHA256 signing
- Failed auth attempts logged with IP