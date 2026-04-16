# GeminiMCP — Tools & Prompts Reference

> Generated 2026-04-11 via Codanna semantic analysis.

---

## Overview

The server exposes **2 MCP tools** and **11 MCP prompts** (3 GitHub workflow
shortcuts + 8 generic coding prompts).

| Name | Type | Purpose |
|------|------|---------|
| `gemini_ask` | Tool | Multimodal question answering with optional GitHub context |
| `gemini_search` | Tool | Google Search-grounded queries |
| `review_pr` | Prompt | Review a GitHub pull request |
| `explain_commit` | Prompt | Explain what a single commit does |
| `compare_refs` | Prompt | Summarize a diff between two refs |
| `code_review` | Prompt | Review code for quality and best practices |
| `explain_code` | Prompt | Explain how code works |
| `debug_help` | Prompt | Debug an issue |
| `refactor_suggestions` | Prompt | Suggest refactoring improvements |
| `architecture_analysis` | Prompt | Analyze system architecture |
| `test_generate` | Prompt | Generate unit tests |
| `security_analysis` | Prompt | Analyze code for security vulnerabilities |
| `research_question` | Prompt | Research with Google Search |

---

## Tool: `gemini_ask`

**Handler:** `GeminiAskHandler` (`gemini_ask_handler.go:51`)

The primary tool. Sends a query to Gemini, optionally enriched with GitHub
repository context (files, PR, commits, compare diffs). All GitHub context
parameters are **independent, optional peers** — mix and match freely in one call.

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | **Yes** | The coding question or task |
| `model` | string | No | Override default model (tier alias or explicit model ID) |
| `thinking_level` | string | No | `low`, `medium`, `high`. Default is tier-aware: `high` for pro, `medium` for flash and flash-lite. |
| `github_repo` | string | No* | Repository in `owner/repo` format — **required when any `github_*` param is used** |
| `github_ref` | string | No | Git branch, tag, or SHA — applies to `github_files` only |
| `github_files` | string[] | No | File paths in the repo to attach as inline context |
| `github_pr` | number | No | PR number — attaches description, unified diff, and review comments |
| `github_commits` | string[] | No | Commit SHAs (short or full) — attaches patch + subject for each |
| `github_diff_base` | string | No | Base ref for a compare diff — must be paired with `github_diff_head` |
| `github_diff_head` | string | No | Head ref for a compare diff — must be paired with `github_diff_base` |

### GitHub Context Behavior

When GitHub parameters are present, the server fetches each source **independently
and in parallel-safe order**, then merges them into the request in this stable sequence:

```
[commits] → [diff (compare)] → [PR bundle (diff + description + review comments)] → [files]
```

A **context inventory** block is prepended to the system instruction so the model
can cite which sources are actually present.

**Limits (configurable via env vars):**

| Source | Default limit |
|--------|--------------|
| `github_files` | 20 files, 1 MB each |
| `github_commits` | 10 commits |
| `github_pr` unified diff | 500 KB (hunk-aware truncation) |
| `github_diff_*` compare diff | 500 KB (hunk-aware truncation) |
| `github_pr` review comments | 50 comments |

Hunk-aware truncation cuts large diffs at logical `@@` boundaries so Gemini always
receives syntactically valid diff fragments.

**Security:** All attacker-controlled text (PR bodies, commit messages, review
comments) passes through a multi-pass Unicode sanitizer that neutralizes
prompt-injection attempts before being sent to the model.

### Mutual Exclusions

- `github_diff_base` and `github_diff_head` must be provided **together**.
- `github_repo` is **required** whenever any `github_*` parameter is present.

### Model Selection

Pass a tier alias or an explicit model ID:

| Alias | Resolves to |
|-------|------------|
| `gemini-pro` | Latest Gemini Pro |
| `gemini-flash` | Latest Gemini Flash |
| `gemini-flash-lite` | Latest Gemini Flash Lite |

The catalog is populated from the live Gemini API at server startup.

### Examples

**Simple question:**
```json
{
  "query": "What is the difference between context.WithCancel and context.WithTimeout in Go?"
}
```

**Attach repository files:**
```json
{
  "github_repo": "owner/repo",
  "github_files": ["internal/auth/middleware.go", "internal/auth/claims.go"],
  "github_ref": "main",
  "query": "Explain how JWT validation works in this codebase"
}
```

**Review a pull request:**
```json
{
  "github_repo": "owner/repo",
  "github_pr": 42,
  "query": "Review this PR for correctness and potential race conditions",
  "thinking_level": "high"
}
```

**Explain specific commits:**
```json
{
  "github_repo": "owner/repo",
  "github_commits": ["abc1234", "def5678"],
  "query": "What do these commits change and why?"
}
```

**Compare two branches:**
```json
{
  "github_repo": "owner/repo",
  "github_diff_base": "v1.2.0",
  "github_diff_head": "main",
  "query": "Summarize the breaking changes since v1.2.0"
}
```

**Mix all GitHub context sources:**
```json
{
  "github_repo": "owner/repo",
  "github_pr": 99,
  "github_commits": ["aaabbb"],
  "github_files": ["go.mod"],
  "query": "Does the PR correctly handle the dependency change introduced in the commit?"
}
```

---

## Tool: `gemini_search`

**Handler:** `GeminiSearchHandler` (`gemini_search_handler.go:13`)

Sends a query to Gemini with the **Google Search grounding tool** enabled. Returns
a structured JSON response containing the answer and cited sources.

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | **Yes** | The question to research |
| `model` | string | No | Override model (default: `GEMINI_SEARCH_MODEL`, typically `gemini-flash-lite`) |
| `thinking_level` | string | No | `minimal`, `low`, `medium`, `high` (default: `low`) |
| `start_time` | string | No | RFC3339 lower bound for search result dates (must pair with `end_time`) |
| `end_time` | string | No | RFC3339 upper bound for search result dates (must pair with `start_time`) |

### Response Format

```json
{
  "answer": "...",
  "sources": [
    { "title": "...", "type": "web" }
  ],
  "search_queries": ["..."]
}
```

### Time Range Filtering

Both `start_time` and `end_time` must be provided together. If only one is given,
the call returns a validation error.

```json
{
  "query": "Go 1.25 release notes",
  "start_time": "2025-01-01T00:00:00Z",
  "end_time": "2025-12-31T23:59:59Z"
}
```

### Examples

**Simple factual question:**
```json
{
  "query": "What are the memory model guarantees for Go channels?"
}
```

**Date-bounded research:**
```json
{
  "query": "PostgreSQL 18 new features",
  "start_time": "2025-01-01T00:00:00Z",
  "end_time": "2025-12-31T23:59:59Z"
}
```

---

## GitHub Workflow Prompts

These three prompts are **discoverable shortcuts** that emit a pre-filled
`gemini_ask` call. Smart clients can skip prompts entirely and call `gemini_ask`
directly with any parameter combination.

---

### Prompt: `review_pr`

Review a GitHub pull request — fetches PR description, unified diff, and review comments.

| Argument | Required | Description |
|----------|----------|-------------|
| `owner` | Yes | Repository owner |
| `repo` | Yes | Repository name |
| `pr_number` | Yes | Pull request number |
| `focus` | No | Aspect to focus on (e.g. `security`, `tests`, `performance`) |

**Emitted `gemini_ask` call:**
```
github_repo = "<owner>/<repo>"
github_pr   = <pr_number>
query       = "Review pull request #<pr_number> in <owner>/<repo>. [Focus on: <focus>.]"
```

**Example invocation in Claude Code:** `/review_pr`

---

### Prompt: `explain_commit`

Explain what a single commit does and why.

| Argument | Required | Description |
|----------|----------|-------------|
| `owner` | Yes | Repository owner |
| `repo` | Yes | Repository name |
| `sha` | Yes | Commit SHA (short or full) |
| `question` | No | Optional follow-up question |

**Emitted `gemini_ask` call:**
```
github_repo    = "<owner>/<repo>"
github_commits = ["<sha>"]
query          = "Explain what commit <sha> in <owner>/<repo> does and why. [<question>]"
```

---

### Prompt: `compare_refs`

Summarize the diff between two Git refs.

| Argument | Required | Description |
|----------|----------|-------------|
| `owner` | Yes | Repository owner |
| `repo` | Yes | Repository name |
| `base` | Yes | Base ref (branch, tag, or SHA) |
| `head` | Yes | Head ref (branch, tag, or SHA) |
| `question` | No | Optional follow-up question |

**Emitted `gemini_ask` call:**
```
github_repo       = "<owner>/<repo>"
github_diff_base  = "<base>"
github_diff_head  = "<head>"
query             = "Summarize the changes between <base> and <head> in <owner>/<repo>."
```

> **Tip:** If the compare range exceeds GitHub's 300-file / ~10k-line limit
> (HTTP 422), use `github_commits` with explicit SHAs instead.

---

## Generic Coding Prompts

All eight generic prompts share the same argument schema:

| Argument | Required | Description |
|----------|----------|-------------|
| `problem_statement` | Yes | Description of the problem or code to analyse |
| `model` | No | Override model for this call |
| `thinking_level` | No | `minimal`, `low`, `medium`, `high` |

Each prompt emits instructions for the MCP client to call `gemini_ask`.

| Prompt | Best for |
|--------|----------|
| `code_review` | Quality, bugs, security, performance review |
| `explain_code` | Explaining algorithms, design patterns, data structures |
| `debug_help` | Root-cause analysis and fix suggestions |
| `refactor_suggestions` | Modernization and structural improvements |
| `architecture_analysis` | High-level design and component breakdown |
| `test_generate` | Writing unit/integration tests |
| `security_analysis` | OWASP Top 10 vulnerability scan |
| `research_question` | Routes to `gemini_search` instead of `gemini_ask` |

---

## Retry & Rate-Limit Behaviour

GitHub API calls use exponential backoff with full jitter:

- Initial backoff: `GEMINI_INITIAL_BACKOFF` (default `1s`)
- Maximum backoff: `GEMINI_MAX_BACKOFF` (default `10s`)
- Maximum attempts: `GEMINI_MAX_RETRIES` (default `2`)

When GitHub returns a `429` or `403` with a `Retry-After` / `X-RateLimit-Reset`
header, the server honours the requested wait (capped at **3600 s**) before
retrying. If the wait exceeds 2 minutes, the server falls back to the standard
jittered backoff rather than blocking.

---

## Query Pre-Qualification

`gemini_ask` automatically classifies every request into one of six categories
and selects a tailored XML-structured system prompt server-side.

| Category | Selected when |
|----------|--------------|
| `general` | Non-programming question, general knowledge, no code context |
| `analyze` | Understanding, explaining, or documenting code; architecture analysis |
| `review` | Code quality review, best practices, refactoring, performance |
| `security` | Security vulnerabilities, authentication, authorization, OWASP |
| `debug` | Bug fixing, error analysis, troubleshooting, test failures |
| `tests` | Generating new unit/integration tests for existing code |

Classification runs as a lightweight Flash model call **in parallel** with GitHub
context fetching, so it adds zero visible latency.

### Skip conditions (no pre-qualification)

1. `GEMINI_PREQUALIFY=false` — pre-qualification disabled; server falls back to the
   default general system prompt.
2. Classification call fails — fallback: `analyze` if any `github_*` context is
   attached, otherwise `general`.

### Configuration

| Env var | Default | Description |
|---------|---------|-------------|
| `GEMINI_PREQUALIFY` | `true` | Enable/disable pre-qualification |
| `GEMINI_PREQUALIFY_MODEL` | `gemini-flash` | Model tier for classification |
| `GEMINI_PREQUALIFY_THINKING` | `medium` | Thinking level for classification |

---

## Error Responses

All tools return errors as MCP tool results (not protocol-level errors), so
clients always receive a human-readable message.

| Situation | Behaviour |
|-----------|-----------|
| Missing required parameter | Error result with parameter name |
| Invalid model ID | Error result with validation details |
| GitHub API unreachable after retries | Error result with last HTTP status |
| Auth enabled but secret missing | Server refuses to start |
| Startup failure (API key missing etc.) | Degraded server returns errors for all calls |
