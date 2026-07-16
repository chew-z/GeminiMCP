# GeminiMCP — Prompt Reference

## Prompt registry

Workflow prompts are `review_pr`, `explain_commit`, and `compare_refs`. Generic
coding prompts include code review, explanation, debugging, refactoring,
architecture, tests, security, and research. They collect task arguments and
forward a provider-backed `gemini_ask` request.

## Server-side prompt selection

The server selects the system prompt from the request and available GitHub
context. Clients do not select a model or reasoning policy through prompt
arguments; those are fixed by the configured provider.

## Provider model configuration

The startup allowlist is:

| Provider | Models |
|---|---|
| DeepSeek | `deepseek-v4-pro` |
| Qwen | `qwen3.7-max`, `qwen3.7-plus` |

Configure `PROVIDER`, `PROVIDER_API_KEY`, and `PROVIDER_MODEL`. Qwen also
requires `PROVIDER_BASE_URL`; a compatible international endpoint is
`https://dashscope-intl.aliyuncs.com/compatible-mode/v1`.
