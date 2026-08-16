# GeminiMCP — Architecture Overview

## Overview

GeminiMCP is an MCP server whose `gemini_ask` tool combines a server-owned XML
context envelope with a configured LLM provider. Both vendors route through
the Responses API — DeepSeek natively (`POST /responses` on
`api.deepseek.com`), Qwen via the DashScope-compatible endpoint — and share
the same provider-neutral `Provider` interface.

## Startup sequence

1. `NewConfig` validates `PROVIDER`, credentials, endpoint settings, and the
   static model allowlist.
2. `NewProvider` constructs the appropriate provider implementation:
   - `deepseek` → `responsesProvider` with `deepseekResponsesDialect`
   - `qwen` → `responsesProvider` with `qwenResponsesDialect`
3. The server registers `gemini_ask`, prompts, and HTTP or stdio transport.

## Provider model allowlist

| Provider | API | Model | Endpoint |
| --- | --- | --- | --- |
| DeepSeek | Responses | `deepseek-v4-pro`, `deepseek-v4-flash` | Defaults to `https://api.deepseek.com` |
| Qwen | Responses | `qwen3.7-max`, `qwen3.7-plus`, `qwen3.8-max-preview` (preview) | `PROVIDER_BASE_URL` required |

Qwen accepts a DashScope-compatible endpoint such as
`https://dashscope-intl.aliyuncs.com/compatible-mode/v1`.

`qwen3.8-max-preview` is thinking-only: it rejects `enable_thinking=false`
(reasoning effort `none`) with a 400. Per Alibaba's deep-thinking guide it
accepts only `low`, `high`, and `xhigh` (aliases `minimal`/`medium`/`max`),
defaults to `xhigh` with a 131072-token thinking budget, and long generations
at top effort are expected behavior. The Responses API exposes no
thinking-budget cap — effort is the only lever — so the dialect serves this
model at `low` effort on every request (measured: review-class work in ~45s
vs `high` overrunning multi-minute budgets on the same task); add future
thinking-forced models to `thinkingForcedQwenModels` in `provider.go`.

Prequalification runs on a dedicated cheap-model provider, not the main one
(`prequalifyModelForVendor` in `provider.go`: `qwen3.7-plus` for Qwen,
`deepseek-v4-flash` for DeepSeek — same credentials and endpoint). This is
load-bearing, not just cost hygiene: a prequalify call immediately followed
by a generation on `qwen3.8-max-preview` reliably wedged the generation in
production (>300s until gateway 504) while either call alone was fine.

## Request lifecycle

`gemini_ask` gathers optional GitHub files, PRs, commits, and diffs; renders the
user envelope; selects a system prompt; and calls `Provider.Generate`. The
single provider implementation maps the common generation request to the
Responses API shape (`input` / `instructions`), while each vendor's dialect
owns reasoning-effort mapping and caching headers. The handler owns retries
and result conversion.

## Provider implementations

### Responses API (`responsesProvider`)

Used by both vendors. Calls `client.Responses.New(...)`. Vendor differences
are isolated in a `responsesDialect` interface:

- `qwenResponsesDialect` maps `ThinkingSpec` to `reasoning.effort`
  (replacing the deprecated `enable_thinking`) and enables server-side
  session caching via the `x-dashscope-session-cache` header.
- `deepseekResponsesDialect` maps thinking-enabled requests to
  `reasoning.effort=max` and thinking-disabled (prequalify) requests to
  `none`. DeepSeek's Responses endpoint is stateless, so no caching headers
  are sent.

Both calls are single-turn: the full context envelope is sent in `input` on
every request, and no conversation state is kept server-side.

## Components

| Component | Responsibility |
| --- | --- |
| `config.go` | Provider and server configuration |
| `provider.go` | Provider-neutral types and provider selection |
| `responses_provider.go` | Responses API provider and response conversion |
| `deepseek_responses_dialect.go` | DeepSeek Responses API dialect (reasoning effort) |
| `qwen_responses_dialect.go` | Qwen Responses API dialect (reasoning effort, session cache) |
| `gemini_ask_handler.go` | Context gathering and generation orchestration |
| `prequalify.go` | Server-side system-prompt selection |
| `http_server.go` | HTTP transport and authentication integration |
