# GeminiMCP Server

## First principles

1. **GeminiMCP is a single Go binary that bridges MCP clients to Google's Gemini API**, exposing three tools: `gemini_ask`, `gemini_search`, and `gemini_models`.

2. **Exactly 3 models are used — pro, flash, and flash-lite** — always in their latest/preview versions, fetched dynamically from the API at startup.

3. **The server sets reasonable defaults and minimizes LLM decision-making** — older or deprecated model requests are quietly redirected, not rejected. Token limits, thinking levels, and caching are handled automatically.

4. We use implicite Gemini API caching. **File context is injected inline before the query** to maximize automatic caching benefits.

5. Two transport modes: stdio (MCP clients) and HTTP (JWT auth)

## Commands

- Build: `go build -o ./bin/mcp-gemini .`
- Test: `./run_test.sh`
- Format: `./run_format.sh`
- Lint: `./run_lint.sh`

## Other instructions

@./GOLANG.md
@./CODANNA.md
