### Architecture Overview

#### Overall Design
The project follows a **Modular Monolith** architecture centered around Google's Gemini AI models. It implements a **Server/Handler Pattern** where the core server (`GeminiServer`) processes requests through specialized handlers for different tools (`gemini_ask`, `gemini_search`, etc.). Key characteristics:
- **Transport-Agnostic**: Supports both stdio (default) and HTTP transports
- **MCP Protocol**: Uses Mark3labs Command Protocol (MCP) for tool/prompt definitions
- **Layered Architecture**: Clear separation between transport, business logic, and AI integration layers

#### Component Breakdown

1. **Core Components**:
   - `GeminiServer`: Central coordinator managing AI interactions, file/cache stores
   - `Config`: Centralized configuration management (env vars + flags)
   - `AuthMiddleware`: JWT-based authentication for HTTP transport
   - `FileStore`/`CacheStore`: Manage file uploads and content caching

2. **Handlers**:
   - `GeminiAskHandler`: Processes code analysis/review requests
   - `GeminiSearchHandler`: Handles search-grounded queries
   - `GeminiModelsHandler`: Provides model documentation
   - `PromptHandler`: Generates instructions for MCP clients

3. **Supporting Services**:
   - `Logger`: Unified logging with configurable levels
   - `ModelStore`: Manages Gemini model metadata and validation
   - `HTTP Server`: Configurable web transport implementation

#### Data Flow
1. **Request Initiation**:
   - MCP request received via stdio/HTTP
   - Authenticated via JWT (HTTP only)
   
2. **Processing**:
   - Request routed to appropriate handler
   - Handler extracts parameters → validates inputs → prepares Gemini config
   - File uploads processed → content cached (if enabled)
   - Gemini API called with generated content/config

3. **Response**:
   - Gemini response converted to MCP format
   - Error handling with standardized error format
   - Search responses enriched with metadata/sources

#### Key Dependencies
1. **`google.golang.org/genai`**: Official Gemini API client
2. **`github.com/mark3labs/mcp-go`**: MCP protocol implementation
3. **`github.com/joho/godotenv`**: Environment variable management
4. **Standard Library**: Extensive use of net/http, sync, context

#### Potential Issues
1. **Scalability**:
   - Monolithic design may limit horizontal scaling
   - In-memory caching (no persistence/eviction strategy)
   - File uploads handle in memory (risk of OOM with large files)

2. **Security**:
   - JWT secret key length not enforced (only warns <32 chars)
   - Stdio transport lacks authentication
   - No input sanitization for file paths

3. **Maintainability**:
   - Global state in model/file/cache stores
   - Large `structs.go` file (700+ LOC) could be split
   - Tight coupling between handlers and Gemini API

4. **Reliability**:
   - No retry mechanism for Gemini API calls
   - Degraded mode lacks health checks
   - HTTP shutdown timeout not configurable

5. **AI-Specific Concerns**:
   - Thinking budget validation missing
   - No fallback for model resolution failures
   - Hardcoded model list might become outdated

### Summary
This project provides a robust integration layer between MCP protocol and Google's Gemini AI, featuring:
- **Modular monolith** architecture with clear separation of concerns
- **Dual transport support** (stdio/HTTP) with auth for HTTP
- **Sophisticated AI features**: Thinking mode, content caching, search grounding
- **Enterprise-grade config** via env vars/flags with validation

**Recommendations**:
1. Implement persistent caching (Redis/Memcached)
2. Add input validation/sanitization layer
3. Introduce retry logic for Gemini API
4. Split `structs.go` into domain-specific files
5. Add model version auto-discovery
6. Implement health checks for degraded mode

The architecture effectively balances flexibility with functionality, though would benefit from enhanced reliability measures and security hardening for production deployments.