# GeminiMCP Server

## First principles

1. MCP clients provide task intent and context; the server decides model tier, thinking, token limits, retries, and caching.

2. Repository context is the only input surface: use `github_repo` + `github_files` / `github_pr` / `github_commits` / `github_diff_*`.

3. The model contract is three logical tiers (pro, flash, flash-lite). The server maps tiers to latest models.

4. Preferred mode is HTTP with JWT auth. stdio is a compatibility fallback for local MCP workflows.

## Commands

- Build: `go build -o ./bin/mcp-gemini .`
- Test: `./run_test.sh`
- Format: `./run_format.sh`
- Lint: `./run_lint.sh`
- Release: `./run_release.sh` (requires `RELEASE_NOTES.md` in project root)

## Environment

Required:
- `GEMINI_API_KEY` — Google AI Studio or Vertex AI key
- `GEMINI_GITHUB_TOKEN` — needed for all `github_*` parameters; optional for public repos

Copy `.env.example` to `.env` and fill in required values.

## Deployment and diagnostics

- `docs/reports/` is gitignored — do not use `git diff` to verify edits there.
- `docs/nginx/` is documentation only. The live nginx config is on the remote server (`karma.rrj.pl`).
- After any deploy, curl the production endpoint directly before closing the task.
- When diagnosing runtime behavior, read `logs/Gemini.log` before making assumptions.

## Known architectural constraints

- `WriteTimeout` in `http_server.go` is `GEMINI_TIMEOUT + 60s` (overridable via `GEMINI_HTTP_WRITE_TIMEOUT`). Long thinking-mode pro calls can exceed the outbound budget and produce `context.Canceled` — check this coupling first when diagnosing cancellations.
- `http.TimeoutHandler` is contraindicated: it buffers writes and breaks MCP streaming heartbeats. Do not propose it for timeout handling.
- `GEMINI_HTTP_CORS_ENABLED` stays `false` in production — the server has no browser clients.

## Other instructions

@./GOLANG.md
@./CODANNA.md
