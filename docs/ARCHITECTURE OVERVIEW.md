# GeminiMCP — Architecture Overview

## Overview

GeminiMCP is an MCP server whose `gemini_ask` tool combines a server-owned XML
context envelope with a configured LLM provider. DeepSeek routes through the
OpenAI-compatible Chat Completions API; Qwen routes through the Responses API.
Both share the same provider-neutral `Provider` interface.

## Startup sequence

1. `NewConfig` validates `PROVIDER`, credentials, endpoint settings, and the
   static model allowlist.
2. `NewProvider` constructs the appropriate provider implementation:
   - `deepseek` → `openaiProvider` with `deepseekDialect` (Chat Completions)
   - `qwen` → `responsesProvider` with `qwenResponsesDialect` (Responses API)
3. The server registers `gemini_ask`, prompts, and HTTP or stdio transport.

## Provider model allowlist

| Provider | API | Model | Endpoint |
|---|---|---|---|
| DeepSeek | Chat Completions | `deepseek-v4-pro` | Defaults to `https://api.deepseek.com` |
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
user envelope; selects a system prompt; and calls `Provider.Generate`. Each
provider implementation maps the common generation request to its vendor-specific
API shape — Chat Completions (`messages`) for DeepSeek, Responses (`input` /
`instructions`) for Qwen — while the handler owns retries and result conversion.

## Provider implementations

### Chat Completions (`openaiProvider`)

Used by DeepSeek. Calls `client.Chat.Completions.New(...)`. Vendor differences
(thinking mode, reasoning effort) are isolated in a `vendorDialect` interface.

### Responses API (`responsesProvider`)

Used by Qwen. Calls `client.Responses.New(...)`. Maps `ThinkingSpec` to
`reasoning.effort` (replacing the deprecated `enable_thinking`), enables
server-side session caching via `x-dashscope-session-cache` header, and supports
`previous_response_id` for future multi-turn context management. Vendor
differences are isolated in a `responsesDialect` interface.

## Components

| Component | Responsibility |
|---|---|
| `config.go` | Provider and server configuration |
| `provider.go` | Provider-neutral types and provider selection |
| `openaicompat.go` | Chat Completions provider and DeepSeek dialect |
| `responses_provider.go` | Responses API provider and response conversion |
| `qwen_responses_dialect.go` | Qwen Responses API dialect (reasoning effort, session cache) |
| `gemini_ask_handler.go` | Context gathering and generation orchestration |
| `prequalify.go` | Server-side system-prompt selection |
| `http_server.go` | HTTP transport and authentication integration |
