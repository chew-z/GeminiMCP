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
