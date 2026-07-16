# GeminiMCP

An MCP server that exposes configured DeepSeek or Qwen models to MCP clients (Claude Code, IDE extensions, custom tooling).

## What it does

- **`gemini_ask`** — coding/analysis question answering with composable GitHub context (PRs, commits, diffs, files)
- **3 workflow prompts** — `review_pr`, `explain_commit`, `compare_refs`
- **7 coding prompts** — code review, explain, debug, refactor, architecture, tests, security
- **Structured XML envelope** — every user turn is rendered as `<context>` / `<task>` / `<final_instruction>` with attacker-controllable bodies emitted as raw text
- **Server-side system-prompt selection** — a classifier picks a category-specific instruction and matching `<final_instruction>` body
- Two transports: **HTTP** (JWT-secured, preferred) and **stdio** (local fallback)

## Quick start

```bash
# Build
go build -o bin/mcp-gemini .

# Configure
cp .env.example .env
# Edit .env — set PROVIDER, PROVIDER_API_KEY, and PROVIDER_MODEL

# Run (stdio, for Claude Code)
./bin/mcp-gemini

# Run (HTTP)
./bin/mcp-gemini --transport=http
```

Claude Code `mcpServers` entry (stdio):

```json
{
  "gemini": {
    "command": "/path/to/bin/mcp-gemini",
    "env": { "PROVIDER": "deepseek", "PROVIDER_API_KEY": "your-key", "PROVIDER_MODEL": "deepseek-v4-pro" }
  }
}
```

## Documentation

| Doc | Contents |
|-----|---------|
| [docs/QUICKREF.md](docs/QUICKREF.md) | One-page cheat sheet — parameters, env vars, CLI flags, examples |
| [docs/TOOLS.md](docs/TOOLS.md) | Full tool and prompt reference with all parameters and examples |
| [docs/PROMPTS.md](docs/PROMPTS.md) | MCP-prompt registry and server-side system-prompt selection |
| [docs/USAGE.md](docs/USAGE.md) | Installation, transport modes, authentication, all env vars |
| [docs/ARCHITECTURE OVERVIEW.md](docs/ARCHITECTURE%20OVERVIEW.md) | Codebase map, component diagram, request lifecycle |
| [.env.example](.env.example) | Annotated configuration template |

## Development

```bash
./run_test.sh     # tests
./run_lint.sh     # golangci-lint
./run_format.sh   # goimports + gofmt
task check        # all gates via go-task (fmt+vet+lint+test)
```

## Requirements

- Go 1.26+
- `PROVIDER`, `PROVIDER_API_KEY`, and `PROVIDER_MODEL` (DeepSeek or Qwen)
- `GEMINI_GITHUB_TOKEN` (GitHub PAT — only for `github_*` parameters)
