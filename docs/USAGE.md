# GeminiMCP — Usage Guide

> Generated 2026-04-11 via Codanna semantic analysis.

---

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| Go 1.25+ | `go build` or pre-built binary in `bin/` |
| `GEMINI_API_KEY` | Google AI Studio or Vertex AI key |
| `GEMINI_GITHUB_TOKEN` | Required only for `github_*` context parameters |
| MCP client | Claude Code, a compatible IDE extension, or any MCP-capable host |

---

## Build

```bash
go build -o bin/mcp-gemini .
# or use the helper scripts:
./run_format.sh   # goimports + gofmt
./run_lint.sh     # golangci-lint
./run_test.sh     # full test suite
```

---

## Transport Modes

The server supports two mutually exclusive transports, selected at startup.

### stdio (default — local MCP workflows)

```bash
./bin/mcp-gemini
# or explicitly:
./bin/mcp-gemini --transport=stdio
```

Claude Code `.mcp.json` / `mcpServers` entry:

```json
{
  "gemini": {
    "command": "/path/to/bin/mcp-gemini",
    "env": {
      "GEMINI_API_KEY": "your-key",
      "GEMINI_GITHUB_TOKEN": "ghp_..."
    }
  }
}
```

### HTTP (preferred for shared / remote deployments)

```bash
./bin/mcp-gemini --transport=http
```

Defaults to `http://localhost:8080/mcp`. Override with env vars:

```bash
GEMINI_HTTP_ADDRESS=":9090" GEMINI_HTTP_PATH="/api/mcp" ./bin/mcp-gemini --transport=http
```

Claude Code `.mcp.json` for HTTP transport:

```json
{
  "gemini": {
    "url": "http://localhost:8080/mcp"
  }
}
```

---

## Authentication (HTTP transport only)

JWT authentication is **disabled by default**. Enable it to restrict access.

### Enable

```bash
./bin/mcp-gemini --transport=http --auth-enabled \
  # GEMINI_AUTH_SECRET_KEY must be set in the environment
```

Or via env var:

```bash
GEMINI_AUTH_ENABLED=true GEMINI_AUTH_SECRET_KEY=<secret> ./bin/mcp-gemini --transport=http
```

> **Note:** The server **refuses to start** if `--auth-enabled` is set but `GEMINI_AUTH_SECRET_KEY` is absent.

### Generate a token

```bash
./bin/mcp-gemini --generate-token \
  --token-user-id=alice \
  --token-username=alice \
  --token-role=admin \
  --token-expiration=744   # hours (default: 744 = 31 days)
```

Pass the token as a Bearer header in the MCP client configuration.

---

## Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--transport` | `stdio` | `stdio` or `http` |
| `--gemini-model` | _(config)_ | Override default model (tier alias or full ID) |
| `--gemini-temperature` | _(config)_ | Float 0.0–1.0 |
| `--service-tier` | _(config)_ | `flex`, `standard`, or `priority` |
| `--auth-enabled` | `false` | Enable JWT auth (HTTP only) |
| `--generate-token` | — | Print a JWT and exit |
| `--token-user-id` | `user1` | User ID embedded in token |
| `--token-username` | `admin` | Username embedded in token |
| `--token-role` | `admin` | Role embedded in token |
| `--token-expiration` | `744` | Token TTL in hours |

---

## Environment Variables

### Required

| Variable | Description |
|----------|-------------|
| `GEMINI_API_KEY` | Google Gemini API key |

### Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error`. Set to `debug` when troubleshooting request assembly, context merging, or GitHub fetches. |

### Model

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_MODEL` | `gemini-pro` | Default model for `gemini_ask` (tier alias or explicit ID) |
| `GEMINI_SEARCH_MODEL` | `gemini-flash-lite` | Default model for `gemini_search` |

Tier aliases resolve to the latest live API model at startup:

| Alias | Tier |
|-------|------|
| `gemini-pro` / `pro` | Pro (highest capability) |
| `gemini-flash` / `flash` | Flash (balanced) |
| `gemini-flash-lite` / `flash-lite` | Flash Lite (fastest) |

### Inference

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_TEMPERATURE` | `1.0` | Sampling temperature (0.0–1.0) |
| `GEMINI_THINKING_LEVEL` | tier-aware (`high` pro / `medium` flash / flash-lite) | Override thinking level for `gemini_ask`: `minimal`, `low`, `medium`, `high`. Thinking is always on for models that support it. |
| `GEMINI_SEARCH_THINKING_LEVEL` | `low` | Override thinking level for `gemini_search` |
| `GEMINI_SERVICE_TIER` | `standard` | Service tier: `flex`, `standard`, `priority` |
| `GEMINI_TIMEOUT` | `300s` | HTTP timeout for Gemini API calls |

### Query pre-qualification

The server runs a lightweight Flash classifier in parallel with GitHub context
fetching and picks a tailored system prompt server-side (see
[PROMPTS.md](PROMPTS.md)). Clients cannot inject system prompts.

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_PREQUALIFY` | `true` | Enable/disable pre-qualification |
| `GEMINI_PREQUALIFY_MODEL` | `gemini-flash` | Tier alias or model ID used for classification |
| `GEMINI_PREQUALIFY_THINKING` | `medium` | Thinking level for the classifier |

### Long-running operations

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_PROGRESS_INTERVAL` | `10s` | Cadence for MCP `notifications/progress` during long Gemini calls. Clients opt in via `_meta.progressToken`. Set to `0` to disable. |
| `GEMINI_MAX_CONCURRENT_TASKS` | `10` | Cap on concurrent task-augmented tool executions. Set to `0` to disable task mode entirely. |

### HTTP Transport

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_ENABLE_HTTP` | `false` | Enable HTTP transport without `--transport=http` flag |
| `GEMINI_HTTP_ADDRESS` | `:8080` | Bind address |
| `GEMINI_HTTP_PATH` | `/mcp` | MCP endpoint path |
| `GEMINI_HTTP_STATELESS` | `false` | Stateless SSE mode |
| `GEMINI_HTTP_HEARTBEAT` | — | Heartbeat interval (e.g. `30s`) |
| `GEMINI_HTTP_CORS_ENABLED` | `false` | Enable CORS |
| `GEMINI_HTTP_CORS_ORIGINS` | — | Comma-separated allowed origins. Supports exact origins (e.g. `https://app.example.com`) and wildcard subdomains (e.g. `*.example.com`), with exact host boundary checks |
| `GEMINI_HTTP_PUBLIC_URL` | — | Externally-facing resource identifier per RFC 9728 (e.g. `https://your.host/path-prefix`). Validated at startup: scheme must be `https` for any host, or `http` only when host is loopback (`localhost`, `127.0.0.1`, `[::1]`). Required behind a reverse proxy that strips a path prefix; otherwise the request-derived URL is used. |
| `GEMINI_HTTP_TRUST_FORWARDED_PROTO` | `false` | Honour `X-Forwarded-Proto` for scheme detection in the RFC 9728 metadata fallback. Set to `true` only when behind a trusted reverse proxy (e.g. nginx). Without this, the metadata document is refused (503) on non-loopback HTTP requests. |

### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_AUTH_ENABLED` | `false` | Enable JWT authentication (HTTP only) |
| `GEMINI_AUTH_SECRET_KEY` | — | JWT signing secret — **required when auth is enabled** |

### GitHub

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_GITHUB_TOKEN` | — | GitHub personal access token (for private repos) |
| `GEMINI_GITHUB_API_BASE_URL` | — | Override for GitHub Enterprise |
| `GEMINI_MAX_GITHUB_FILES` | `20` | Max files per `github_files` call |
| `GEMINI_MAX_GITHUB_FILE_SIZE` | `1048576` | Max bytes per file (1 MB) |
| `GEMINI_MAX_GITHUB_DIFF_BYTES` | `512000` | Max bytes for a unified diff (500 KB) |
| `GEMINI_MAX_GITHUB_COMMITS` | `10` | Max commits per `github_commits` call |
| `GEMINI_MAX_GITHUB_PR_REVIEW_COMMENTS` | `50` | Max PR review comments fetched |

### Retry

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_MAX_RETRIES` | `2` | Max retry attempts for GitHub API calls |
| `GEMINI_INITIAL_BACKOFF` | `1s` | Initial backoff delay |
| `GEMINI_MAX_BACKOFF` | `10s` | Maximum backoff delay (exponential with full jitter) |

---

## Startup Sequence

The server performs these steps in order at launch:

1. Parse CLI flags and env vars (`NewConfig`)
2. Connect to Gemini API and fetch the live model catalog (`FetchGeminiModels`)
3. Register MCP tools (`gemini_ask`, `gemini_search`) and prompts
4. Resolve tier alias defaults to actual model IDs
5. Validate transport + auth configuration
6. Start stdio or HTTP transport

If any step fails, the server enters **degraded mode** — it starts a minimal MCP
server that returns descriptive errors for every tool call, so clients receive
useful feedback instead of a silent connection drop.

---

## GitHub Token Scopes

For public repos no token is required. For private repos, `GEMINI_GITHUB_TOKEN` needs:

- `repo` — full read access to private repositories
- Or more granular: `contents:read`, `pull_requests:read`

---

## Quickstart Examples

### Ask a coding question (no GitHub context)

```
gemini_ask(
  query="Explain the difference between sync.Mutex and sync.RWMutex in Go"
)
```

### Review a pull request

```
gemini_ask(
  github_repo="owner/repo",
  github_pr=42,
  query="Review this PR for security issues and race conditions"
)
```

### Mix GitHub context sources

```
gemini_ask(
  github_repo="owner/repo",
  github_pr=42,
  github_commits=["abc1234", "def5678"],
  github_files=["internal/auth/middleware.go"],
  query="Does the new middleware correctly handle the edge cases fixed in these commits?"
)
```

### Grounded web search

```
gemini_search(
  query="Go 1.25 new features",
  start_time="2025-01-01T00:00:00Z",
  end_time="2025-12-31T23:59:59Z"
)
```

### Use a workflow prompt (Claude Code)

In Claude Code, type `/review_pr` and supply `owner`, `repo`, `pr_number` — the
prompt emits a pre-filled `gemini_ask` call automatically.
