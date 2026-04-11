### Architecture Overview

#### Overall Design
The project follows a **Modular Monolith** architecture centered around Google's Gemini AI models. It implements a **Server/Handler Pattern** where the core server (`GeminiServer`) processes requests through specialized handlers for different tools (`gemini_ask`, `gemini_search`, etc.). Key characteristics:
- **Transport-Agnostic**: Supports both stdio (default) and HTTP transports
- **MCP Protocol**: Uses Mark3labs Command Protocol (MCP) for tool/prompt definitions
- **Layered Architecture**: Clear separation between transport, business logic, and AI integration layers

#### Component Breakdown

1. **Core Components**:
   - `GeminiServer`: Central coordinator managing AI interactions
   - `Config`: Centralized configuration management (env vars + flags)
   - `AuthMiddleware`: JWT-based authentication for HTTP transport

2. **Handlers**:
   - `GeminiAskHandler`: Processes code analysis/review requests with file context
   - `GeminiSearchHandler`: Handles search-grounded queries
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
   - Files fetched (GitHub/local) → text files injected inline, binary via Files API
   - Parts ordered: files first (cacheable prefix), query last (variable suffix)
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
   - File uploads handled in memory (OOM guard via pre-read size check)

2. **Security**:
   - JWT secret key length not enforced (only warns <32 chars)
   - Stdio transport lacks authentication
   - Symlink chain traversal needs hardening

3. **Maintainability**:
   - Global state in model store
   - Tight coupling between handlers and Gemini API

4. **Reliability**:
   - Degraded mode lacks health checks
   - HTTP shutdown timeout not configurable

5. **AI-Specific Concerns**:
   - No fallback for model resolution failures
   - Hardcoded fallback model list might become outdated (mitigated by dynamic fetch at startup)

### Summary
This project provides a robust integration layer between MCP protocol and Google's Gemini AI, featuring:
- **Modular monolith** architecture with clear separation of concerns
- **Dual transport support** (stdio/HTTP) with auth for HTTP
- **Sophisticated AI features**: Thinking mode, implicit caching optimization, search grounding
- **Enterprise-grade config** via env vars/flags with validation

**Recommendations**:
1. Add input validation/sanitization layer
2. Harden symlink chain validation in local file reading
3. Add model version auto-discovery improvements
4. Implement health checks for degraded mode

The architecture effectively balances flexibility with functionality, though would benefit from enhanced reliability measures and security hardening for production deployments.
