# GeminiMCP Server

## First principles

1. MCP clients provide task intent and context; the server decides model tier, thinking, token limits, retries, and caching.

2. Repository context is primary: use `github_repo` + `github_files` by default. Local `file_paths` is a discouraged stdio fallback.

3. The model contract is three logical tiers (pro, flash, flash-lite). The server maps tiers to latest models.

4. Preferred mode is HTTP with JWT auth. stdio is a compatibility fallback for local MCP workflows.

## Commands

- Build: `go build -o ./bin/mcp-gemini .`
- Test: `./run_test.sh`
- Format: `./run_format.sh`
- Lint: `./run_lint.sh`

## Other instructions

@./GOLANG.md
@./CODANNA.md
