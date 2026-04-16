# GeminiMCP — Quick Reference

> Cheat sheet. Full docs: [TOOLS.md](TOOLS.md) · [PROMPTS.md](PROMPTS.md) · [USAGE.md](USAGE.md) · [ARCHITECTURE OVERVIEW.md](ARCHITECTURE%20OVERVIEW.md)

---

## Start the server

```bash
# stdio (local, default)
./bin/mcp-gemini

# HTTP
./bin/mcp-gemini --transport=http

# HTTP + JWT auth
GEMINI_AUTH_SECRET_KEY=<secret> ./bin/mcp-gemini --transport=http --auth-enabled

# Generate a JWT
./bin/mcp-gemini --generate-token --token-user-id=alice --token-role=admin
```

---

## `gemini_ask` — parameters at a glance

```
query*          string    The question or task (required)
model           string    Tier alias or explicit model ID
thinking_level  string    low | medium | high  (default: tier-aware — high for pro, medium for flash/flash-lite)
```

### GitHub context (all optional, all combinable)

```
github_repo*        string    owner/repo  ← required when any github_* is used
github_ref          string    branch / tag / SHA  (for github_files only)
github_files        string[]  File paths to attach inline
github_pr           number    PR → description + diff + review comments
github_commits      string[]  Commit SHAs → patch + subject per commit
github_diff_base    string    Base ref for compare diff  ← pair with head
github_diff_head    string    Head ref for compare diff  ← pair with base
```

**Merge order:** `commits → compare diff → PR bundle → files`

---

## `gemini_search` — parameters at a glance

```
query*        string    The research question (required)
model         string    Override model
thinking_level  string  minimal | low | medium | high  (default: low)
start_time    string    RFC3339 — both or neither
end_time      string    RFC3339 — both or neither
```

---

## Model aliases

| Alias | Tier |
|-------|------|
| `gemini-pro` | Latest Gemini Pro |
| `gemini-flash` | Latest Gemini Flash |
| `gemini-flash-lite` | Latest Gemini Flash Lite |

---

## Workflow prompts

| Prompt | Required args | Optional args |
|--------|--------------|---------------|
| `review_pr` | `owner` `repo` `pr_number` | `focus` |
| `explain_commit` | `owner` `repo` `sha` | `question` |
| `compare_refs` | `owner` `repo` `base` `head` | `question` |

Generic prompts (`code_review`, `explain_code`, `debug_help`, `refactor_suggestions`,
`architecture_analysis`, `test_generate`, `security_analysis`,
`research_question`) all take: `problem_statement*` `model` `thinking_level`

---

## Common call patterns

```json
// Basic question
{ "query": "..." }

// Files from GitHub
{ "github_repo": "owner/repo", "github_files": ["path/to/file.go"], "query": "..." }

// PR review
{ "github_repo": "owner/repo", "github_pr": 42, "query": "Review for security issues" }

// Explain commits
{ "github_repo": "owner/repo", "github_commits": ["abc1234"], "query": "What does this change?" }

// Compare branches
{ "github_repo": "owner/repo", "github_diff_base": "v1.0.0", "github_diff_head": "main", "query": "Breaking changes?" }

// Mix everything
{ "github_repo": "owner/repo", "github_pr": 99, "github_commits": ["abc"], "github_files": ["go.mod"], "query": "..." }

// Grounded search with date range
{ "query": "Go 1.25 features", "start_time": "2025-01-01T00:00:00Z", "end_time": "2025-12-31T23:59:59Z" }
```

---

## Key environment variables

```bash
# Required
GEMINI_API_KEY=...

# Models
GEMINI_MODEL=gemini-pro          # default for gemini_ask
GEMINI_SEARCH_MODEL=gemini-flash-lite # default for gemini_search

# GitHub
GEMINI_GITHUB_TOKEN=ghp_...
GEMINI_MAX_GITHUB_DIFF_BYTES=512000   # 500 KB
GEMINI_MAX_GITHUB_COMMITS=10
GEMINI_MAX_GITHUB_PR_REVIEW_COMMENTS=50

# HTTP transport
GEMINI_HTTP_ADDRESS=:8080
GEMINI_HTTP_PATH=/mcp
GEMINI_AUTH_ENABLED=false
GEMINI_AUTH_SECRET_KEY=...

# Inference
GEMINI_TEMPERATURE=1.0
GEMINI_THINKING_LEVEL=high
GEMINI_SERVICE_TIER=standard     # flex | standard | priority

# Pre-qualification (auto system prompt selection)
GEMINI_PREQUALIFY=true           # disable with false
GEMINI_PREQUALIFY_MODEL=gemini-flash
GEMINI_PREQUALIFY_THINKING=medium

# Retry
GEMINI_MAX_RETRIES=2
GEMINI_INITIAL_BACKOFF=1s
GEMINI_MAX_BACKOFF=10s
```

---

## CLI flags

```
--transport          stdio* | http
--gemini-model       tier alias or model ID
--gemini-temperature 0.0-1.0
--service-tier       flex | standard | priority
--auth-enabled
--generate-token
--token-user-id      (default: user1)
--token-username     (default: admin)
--token-role         (default: admin)
--token-expiration   hours (default: 744)
```

---

## Build / dev

```bash
go build -o bin/mcp-gemini .
./run_test.sh
./run_lint.sh
./run_format.sh
```

---

## Limits & safety

| Rule | Detail |
|------|--------|
| `github_diff_base` ↔ `github_diff_head` | Must be paired |
| `github_repo` | Required when any `github_*` is used |
| Retry-After cap | 3 600 s max wait honoured |
| Auth secret missing | Server refuses to start |
| GitHub 422 on compare | Use `github_commits` with explicit SHAs instead |
