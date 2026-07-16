# GeminiMCP — Architecture Overview

## Overview

GeminiMCP is an MCP server whose `gemini_ask` tool combines a server-owned XML
context envelope with a configured OpenAI-compatible provider. DeepSeek and
Qwen use the same provider-neutral generation interface.

## Startup sequence

1. `NewConfig` validates `PROVIDER`, credentials, endpoint settings, and the
   static model allowlist.
2. `NewProvider` constructs the DeepSeek or Qwen dialect.
3. The server registers `gemini_ask`, prompts, and HTTP or stdio transport.

## Provider model allowlist

| Provider | Model | Endpoint |
|---|---|---|
| DeepSeek | `deepseek-v4-pro` | Defaults to `https://api.deepseek.com` |
| Qwen | `qwen3.7-max`, `qwen3.7-plus` | `PROVIDER_BASE_URL` required |

Qwen accepts a DashScope-compatible endpoint such as
`https://dashscope-intl.aliyuncs.com/compatible-mode/v1`.

## Request lifecycle

`gemini_ask` gathers optional GitHub files, PRs, commits, and diffs; renders the
user envelope; selects a system prompt; and calls `Provider.Generate`. The
provider maps the common generation request to vendor-specific chat-completion
JSON, while the handler owns retries and result conversion.

## Components

| Component | Responsibility |
|---|---|
| `config.go` | Provider and server configuration |
| `provider.go` | Provider-neutral types and provider selection |
| `openaicompat.go` | DeepSeek and Qwen request dialects |
| `gemini_ask_handler.go` | Context gathering and generation orchestration |
| `prequalify.go` | Server-side system-prompt selection |
| `http_server.go` | HTTP transport and authentication integration |
