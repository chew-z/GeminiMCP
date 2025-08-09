# Repository Guidelines

## Project Structure & Module Organization
- Root package: Go sources in `./` (entrypoint `main.go`).
- Handlers and core: `server_handlers.go`, `direct_handlers.go`, `prompt_handlers.go`, `handlers_common.go`.
- HTTP and auth: `http_server.go`, `gemini_server.go`, `auth.go`.
- Config and utilities: `config.go`, `gemini_utils.go`, `cache.go`, `files.go`, `logger.go`.
- Tests: `*_test.go` in root (e.g., `config_test.go`, `gemini_test.go`).
- Binaries: `bin/` (ignored in git; built locally).
- CI and tooling: `.github/workflows/`, `.gemini/` (release helper), `.vscode/`, `.codex/`.

## Build, Test, and Development Commands
- Build: `go build -o ./bin/mcp-gemini .` (produces single static binary).
- Run (stdio): `./bin/mcp-gemini`.
- Run (HTTP): `./bin/mcp-gemini --transport=http`.
- Tests: `./run_test.sh`.
- Format: `./run_format.sh`.
- Lint: `./run_lint.sh` (auto-fixes where safe).

## Coding Style & Naming Conventions
- Formatting: gofmt-required; CI expects formatted code.
- Indentation: tabs (default Go style); 100–120 col soft wrap.
- Names: exported identifiers `CamelCase`; unexported `camelCase`; packages lower-case, short, no underscores.
- Files: `snake_case.go` mirroring feature area (e.g., `prompt_handlers.go`).
- Errors: return `error` values; wrap with context; prefer sentinel/`errors.Is` over string matching.

## Testing Guidelines
- Framework: standard `testing` package.
- Location: keep tests next to code as `*_test.go` with `TestXxx(t *testing.T)`.
- Running: `./run_test.sh`; coverage (optional): `go test -cover ./...`.
- Add table-driven tests for handlers and config parsing; include edge cases and auth/HTTP flags where relevant.

## Commit & Pull Request Guidelines
- Conventional Commits: `feat(scope): …`, `fix(scope): …`, `docs: …`, `refactor: …`, `test: …`, `chore: …`, `ci: …` (matches current history).
- PRs: include a clear description, linked issues, and usage notes for new flags/endpoints; update `README.md` when CLI or env vars change. Attach logs or screenshots for HTTP responses when helpful.
- Pre-push: run format, lint, and tests locally; keep PRs focused and small.

## Security & Configuration Tips
- Do not commit secrets; use env vars like `GEMINI_API_KEY`. `.env` is git-ignored.
- For HTTP mode, configure CORS origins and JWT secret; avoid permissive defaults in production.
