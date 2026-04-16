# GeminiMCP — Architecture Overview

> Generated 2026-04-11 via Codanna semantic analysis (2 428 symbols, 44 files, 395 relationships).

---

## 1. Purpose

GeminiMCP is a **Model Context Protocol (MCP) server** that exposes Google Gemini
models to MCP clients (Claude Code, IDEs, custom tooling). It provides:

- A primary `gemini_ask` tool — multimodal, GitHub-context-aware question answering.
- A `gemini_search` tool — Google Search-grounded queries returning structured JSON.
- Three GitHub **workflow prompts** plus eight generic coding prompts that compose the tools.
- Server-side query pre-qualification that auto-selects a tailored system prompt.
- MCP `notifications/progress` + task-augmented execution for long-running calls.
- Two transport modes: **HTTP** (preferred, JWT-secured) and **stdio** (legacy/local).

---

## 2. High-Level Component Map

```
┌─────────────────────────────────────────────────────────────┐
│                        MCP Client                           │
│             (Claude Code, IDE extension, …)                 │
└────────────────────────┬────────────────────────────────────┘
                         │  MCP protocol
          ┌──────────────┴──────────────┐
          │  Transport Layer            │
          │  http_server.go  main.go    │
          │  ┌──────────┐ ┌─────────┐  │
          │  │  HTTP +  │ │ stdio   │  │
          │  │  JWT     │ │(fallback│  │
          │  └──────────┘ └─────────┘  │
          └──────────────┬──────────────┘
                         │
          ┌──────────────▼──────────────┐
          │  MCP Server (mcp-go)         │
          │  server_handlers.go          │
          │  tools.go  prompts.go        │
          └──────────────┬──────────────┘
                         │
          ┌──────────────▼──────────────┐
          │  GeminiServer               │
          │  (config + genai.Client)    │
          ├─────────────────────────────┤
          │  gemini_ask_handler.go      │ ← primary tool
          │  gemini_search_handler.go   │ ← search tool
          │  prompt_handlers.go         │ ← workflow prompts
          └──────────────┬──────────────┘
                         │
          ┌──────────────┴──────────────┐
          │  Cross-cutting Services      │
          ├──────────────────────────────┤
          │  fetch_models.go  (catalog)  │
          │  github_diff.go   (HTTP/GH)  │
          │  retry.go         (backoff)  │
          │  auth.go          (JWT)      │
          │  config.go        (env)      │
          └──────────────────────────────┘
```

---

## 3. Startup Sequence

`main.go:runMain()` orchestrates a deterministic startup pipeline:

```
parse CLI flags
  └─ NewConfig()              config.go:142   — read all env vars
  └─ NewLogger()              logger.go       — structured levelled logger
  └─ setupGeminiServer()      server_handlers.go:13
       ├─ genai.NewClient()                   — Gemini API client
       ├─ FetchGeminiModels()  fetch_models.go:53  — populate model catalog
       ├─ register tools       tools.go
       └─ registerPrompts()    server_handlers.go:109
  └─ resolveAndValidateModel() handlers_common.go — map tier names → real IDs
  └─ start transport
       ├─ HTTP → startHTTPServer()  http_server.go:18
       └─ stdio → server.ServeStdio()
```

If any step fails, `handleStartupError()` registers an error-only MCP server so
clients receive a descriptive fault instead of a silent crash.

---

## 4. Source File Reference

| File | Responsibility |
|------|----------------|
| `main.go` | Entry point; flag parsing; startup orchestration; transport selection |
| `server_handlers.go` | MCP tool + prompt registration; logger-wrapping middleware |
| `structs.go` | Core types: `Config`, `GeminiServer`, `PromptDefinition`, `ModelVersion` |
| `config.go` | `NewConfig` — all env vars with typed parsers |
| `gemini_ask_handler.go` | `GeminiAskHandler` — context assembly, file uploads, prompt injection |
| `gemini_search_handler.go` | `GeminiSearchHandler` — Google Search grounded queries |
| `prompt_handlers.go` | Workflow prompt handlers: `review_pr`, `explain_commit`, `compare_refs` |
| `prompts.go` | `PromptDefinition` catalog; `problem_statement` generic prompts |
| `tools.go` | MCP tool schema definitions |
| `fetch_models.go` | Dynamic model catalog from live Gemini API; tier classification |
| `handlers_common.go` | `createModelConfig`; `extractArgumentString`; `SafeWriter`; grounding-source collection |
| `github_diff.go` | GitHub API HTTP client; rate-limit handling; `sanitizeUntrustedBlockContent` |
| `github_diff_handler.go` | `gatherCompareDiff` — base…head diff fetching |
| `github_pr_handler.go` | PR body + review-comments context |
| `github_commits_handler.go` | Commit message context |
| `file_handlers.go` | GitHub file upload to Gemini Files API; rate-limit + status-code handling |
| `prequalify.go` | Flash-based query classifier; runs in parallel with context fetching |
| `system_prompts.go` | XML-structured system prompts keyed by classification category |
| `progress.go` | Periodic MCP `notifications/progress` during long Gemini calls |
| `task_hooks.go` | Lifecycle hooks for task-augmented tool execution |
| `auth.go` | `AuthMiddleware`; JWT validation; `Claims` |
| `http_server.go` | `startHTTPServer`; CORS; `createHTTPMiddleware` |
| `retry.go` | `withRetry`; `computeBackoff` — exponential backoff with full jitter |
| `model_functions.go` | `ValidateModelID`; model-tier resolution helpers |
| `gemini_server.go` | Gemini client lifecycle helpers |
| `gemini_utils.go` | Shared Gemini API utilities |
| `completions.go` | MCP argument auto-complete provider |
| `context.go` | `context.Context` key constants and helper accessors |
| `logger.go` | `Logger` interface; levelled implementation |

---

## 5. Model Tier System

The server maps three **logical tiers** to the latest live Gemini model IDs.
The catalog is populated at startup by querying the Gemini API (`FetchGeminiModels`,
`fetch_models.go:53`):

| Tier | Alias | Typical model |
|------|-------|---------------|
| `pro` | `gemini-pro` | `gemini-2.5-pro-*` |
| `flash` | `gemini-flash` | `gemini-2.5-flash-*` |
| `flash-lite` | `gemini-flash-lite` | `gemini-2.0-flash-lite-*` |

`classifyModel()` (`fetch_models.go:177`) scores each API-listed model against
version floors and category rules. `selectBestModels()` picks the winner per tier.
Clients may also pass explicit model IDs which are validated by `ValidateModelID`.

---

## 6. `gemini_ask` — Request Lifecycle

`GeminiAskHandler` (`gemini_ask_handler.go:51`) is the largest component (~700 lines).

```
GeminiAskHandler
  └─ parseAskRequest()
  └─ resolveSystemPromptAsync()         — prequalify.go: Flash classifier in parallel
  └─ gatherAllContext()                 — runs concurrently with the classifier
       ├─ gatherGitHubContext()         — commits → diff → PR bundle
       │    └─ fetchGitHubContextSources() (parallel-safe)
       └─ gatherFileUploads()           — github_files
            └─ gatherGitHubFiles()
  └─ applyContextInventory()            — prepend inventory block to system prompt
  └─ processWithFiles()  |
     processWithoutFiles()              — call Gemini API with assembled parts
```

Pre-qualification (`prequalify.go`) classifies every request into one of six
categories (`general`, `analyze`, `review`, `security`, `debug`, `tests`) and
picks a tailored XML system prompt from `system_prompts.go`. Runs in parallel
with context fetching so it adds no visible latency. Falls back to `analyze`
when `github_*` context is present or `general` otherwise if the classifier
fails or is disabled via `GEMINI_PREQUALIFY=false`.

### GitHub Context Sources (composable)

All four sources are **independent, optional peers** in one request:

| Parameter | Handler | Content |
|-----------|---------|---------|
| `github_pr` | `github_pr_handler.go` | PR body + review comments |
| `github_commits` | `github_commits_handler.go` | Commit messages (up to `GEMINI_MAX_GITHUB_COMMITS`) |
| `github_diff_base/head` | `github_diff_handler.go` | Compare diff (hunk-aware truncation) |
| `github_files` | `file_handlers.go` | File content as Gemini file uploads |

Stable merge order: `[commits] → [diff] → [PR bundle] → [files]`.
`github_repo` is **required** whenever any `github_*` parameter is used.

### Context Inventory

`applyContextInventory()` prepends a deterministic inventory block to the system
instruction, letting the model cite which sources are present without hallucinating.

---

## 7. GitHub API Layer (`github_diff.go`)

```
githubAPIGet()
  └─ withRetry()           retry.go — exponential backoff, full jitter
       └─ githubAPIGetOnce()
            └─ classifyGitHubStatus()
                 └─ waitForGitHubRateLimitReset()
                      └─ rateLimitWaitFromHeaders()
                           — Retry-After (delta-sec + HTTP-date)
                           — X-RateLimit-Reset fallback
                           — clamped to 3 600 s (overflow guard)
```

**Hunk-aware diff truncation**: large diffs are cut at logical `@@` boundaries
(never mid-hunk) up to `GEMINI_MAX_GITHUB_DIFF_BYTES` (default 500 KB).

---

## 8. Security

### Prompt Injection Prevention

`sanitizeUntrustedBlockContent()` (`github_diff.go:324`) runs on every piece of
attacker-controlled text (PR bodies, commit messages, review comments). It performs
a multi-pass scan per line:

1. Strip invisible Unicode runes (`Cf`, `Mn`, `Me` categories).
2. Normalize dash variants (`Pd` + U+2212 MINUS SIGN) to ASCII `-`.
3. Normalize alternative line separators (`VT`, `FF`, `NEL`, `LS`, `PS`) to LF.
4. Redact any line matching the server block-header anchor `---<whitespace>`.

### JWT Authentication (HTTP transport)

`AuthMiddleware` (`auth.go:16`) validates Bearer tokens on every HTTP request.
`Claims` carries `UserID`, `Username`, and `Role`. Enabled via `--auth-enabled` or
`GEMINI_AUTH_ENABLED=true`. The server **refuses to start** if auth is enabled but
`GEMINI_AUTH_SECRET_KEY` is absent.

---

## 9. Transport Modes

| Mode | Flag | Auth | Notes |
|------|------|------|-------|
| **stdio** | `--transport=stdio` (default) | None | Preferred for local MCP workflows |
| **HTTP** | `--transport=http` | Optional JWT | Preferred for remote/shared deployments |

HTTP server (`http_server.go`) uses `mcp-go`'s SSE transport, adds CORS via
`isOriginAllowed()`, and injects logger + config into every request's `context.Context`.

---

## 10. Long-Running Operations

Pro-tier calls with large context can run for 60–300 s, exceeding client-side
deadlines. Two independent, additive mechanisms keep long calls viable:

### Progress notifications (`progress.go`)

Clients that send `_meta.progressToken` on a tool call receive periodic
`notifications/progress` pings during `GenerateContent`. The session-scoped
reporter ticks every `GEMINI_PROGRESS_INTERVAL` (default `10s`; `0` disables)
and reports elapsed seconds against `GEMINI_TIMEOUT` along with a short
description of the active model and thinking level. Clients without a progress
token see no behaviour change.

### Task-augmented execution (`task_hooks.go`)

Both tools advertise `taskSupport: optional` (MCP spec 2025-11-25). Clients
that include a `task` field on `tools/call` receive a `CreateTaskResult` within
milliseconds while the actual Gemini call continues in a server goroutine; the
final answer is delivered via `notifications/tasks/status` and `tasks/result`.
This removes the client-side 60 s deadline entirely.
`GEMINI_MAX_CONCURRENT_TASKS` (default `10`) caps simultaneous task executions;
`0` disables task mode. Clients that don't send `task` stay on the synchronous
path transparently.

---

## 11. MCP Workflow Prompts

Three shortcuts registered by `registerPrompts()` / `prompt_handlers.go`:

| Prompt | Arguments | GitHub context used |
|--------|-----------|---------------------|
| `review_pr` | `repo`, `pr_number`, `focus` | `github_pr` + `github_diff_*` |
| `explain_commit` | `repo`, `sha` | `github_commits` |
| `compare_refs` | `repo`, `base`, `head` | `github_diff_*` |

Eight generic coding prompts (`code_review`, `explain_code`, `debug_help`,
`refactor_suggestions`, `architecture_analysis`, `test_generate`,
`security_analysis`, `research_question`) share a `problem_statement` argument
and forward to `gemini_ask` — system-prompt selection remains server-side.

---

## 12. Configuration Reference (key env vars)

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_API_KEY` | — | **Required** — Gemini API key |
| `GEMINI_MODEL` | `gemini-pro` | Default model (tier alias or explicit ID) |
| `GEMINI_SEARCH_MODEL` | `gemini-flash-lite` | Model for `gemini_search` |
| `GEMINI_TEMPERATURE` | `1.0` | Sampling temperature (0.0–1.0) |
| `GEMINI_THINKING_LEVEL` | tier-aware | Override thinking level for `gemini_ask` (`minimal` / `low` / `medium` / `high`) |
| `GEMINI_SEARCH_THINKING_LEVEL` | `low` | Override thinking level for `gemini_search` |
| `GEMINI_SERVICE_TIER` | `standard` | `flex` / `standard` / `priority` |
| `GEMINI_TIMEOUT` | `300s` | HTTP timeout for Gemini API calls |
| `GEMINI_PREQUALIFY` | `true` | Enable server-side query classification |
| `GEMINI_PREQUALIFY_MODEL` | `gemini-flash` | Model used by the classifier |
| `GEMINI_PREQUALIFY_THINKING` | `medium` | Thinking level for the classifier |
| `GEMINI_PROGRESS_INTERVAL` | `10s` | Cadence for `notifications/progress`; `0` disables |
| `GEMINI_MAX_CONCURRENT_TASKS` | `10` | Task-mode concurrency cap; `0` disables task mode |
| `GEMINI_AUTH_ENABLED` | `false` | Enable JWT auth for HTTP transport |
| `GEMINI_AUTH_SECRET_KEY` | — | JWT signing secret (required if auth enabled) |
| `GEMINI_HTTP_ADDRESS` | `:8080` | HTTP bind address |
| `GEMINI_HTTP_PATH` | `/mcp` | HTTP MCP endpoint path |
| `GEMINI_GITHUB_TOKEN` | — | GitHub PAT for private repo access |
| `GEMINI_MAX_GITHUB_DIFF_BYTES` | `512000` | Max diff size (500 KB) |
| `GEMINI_MAX_GITHUB_COMMITS` | `10` | Max commits fetched per request |
| `GEMINI_MAX_GITHUB_PR_REVIEW_COMMENTS` | `50` | Max PR review comments |

---

## 13. Testing Strategy

Tests live alongside sources (`*_test.go`). Notable test files:

| File | Coverage |
|------|----------|
| `github_context_test.go` | Full merge-path integration; rate-limit header parsing; prompt injection regression |
| `gemini_ask_handler_test.go` | Handler unit tests; mutual exclusion; context assembly |
| `gemini_search_handler_test.go` | Search handler; grounding metadata collection |
| `handlers_common_test.go` | Argument extraction; grounding-source dedup; `SafeWriter` |
| `progress_test.go` | Progress reporter lifecycle; notification cadence |
| `task_hooks_test.go` | Task hook events and session-ID tagging |
| `file_handlers_unit_test.go` | Rate-limit handling; status-code mapping; retry outcomes |
| `fetch_models_test.go` | `classifyModel` tier logic |
| `config_test.go` | Env-var parsing; defaults |
| `retry_test.go` | Backoff math; jitter bounds |
| `auth_test.go` | JWT validation; claims extraction |
| `http_server_test.go` | Middleware; CORS |
| `main_test.go` | Flag parsing; startup error handling; transport selection |

Run with: `./run_test.sh`
