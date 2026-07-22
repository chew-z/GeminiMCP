# GeminiMCP — Usage Guide

## Prerequisites

Go 1.26+, an MCP client, and a configured provider are required. Set
`PROVIDER`, `PROVIDER_API_KEY`, and `PROVIDER_MODEL`; Qwen also requires
`PROVIDER_BASE_URL`.

## Build and run

```bash
go build -o bin/mcp-gemini .
PROVIDER=deepseek PROVIDER_API_KEY=... PROVIDER_MODEL=deepseek-v4-pro ./bin/mcp-gemini
```

For Qwen, configure a compatible endpoint:

```bash
PROVIDER=qwen PROVIDER_API_KEY=... PROVIDER_MODEL=qwen3.7-max \
  PROVIDER_BASE_URL=https://dashscope-intl.aliyuncs.com/compatible-mode/v1 ./bin/mcp-gemini
```

## Provider environment

| Variable | Requirement | Description |
| --- | --- | --- |
| `PROVIDER` | Required | `deepseek` or `qwen` |
| `PROVIDER_API_KEY` | Required | Provider credential |
| `PROVIDER_MODEL` | Required | `deepseek-v4-pro`, `qwen3.7-max`, `qwen3.7-plus`, or `qwen3.8-max-preview` (preview) |
| `PROVIDER_BASE_URL` | Qwen required | DeepSeek defaults to `https://api.deepseek.com` |
| `PROVIDER_MAX_TOKENS` | Optional | `0` uses the API default |

`GEMINI_TEMPERATURE`, timeout, retry, HTTP, authentication, logging, and GitHub
context settings remain available as documented in `.env.example`.

## Tools and prompts

`gemini_ask` accepts a required `query` plus optional GitHub context. The server
owns provider selection, model choice, and reasoning policy. Workflow and coding
prompts forward to the same provider-backed request path.

## Transport

stdio is the default. Use `--transport=http` for the HTTP MCP endpoint; JWT
authentication uses the existing `GEMINI_AUTH_*` settings.
