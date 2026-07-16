# GeminiMCP — Quick Reference

> Cheat sheet. Full docs: [TOOLS.md](TOOLS.md) · [PROMPTS.md](PROMPTS.md) · [USAGE.md](USAGE.md) · [ARCHITECTURE OVERVIEW.md](ARCHITECTURE%20OVERVIEW.md)

## Start the server

```bash
PROVIDER=deepseek PROVIDER_API_KEY=... PROVIDER_MODEL=deepseek-v4-pro ./bin/mcp-gemini
PROVIDER=qwen PROVIDER_API_KEY=... PROVIDER_MODEL=qwen3.7-max \
  PROVIDER_BASE_URL=https://dashscope-intl.aliyuncs.com/compatible-mode/v1 ./bin/mcp-gemini
```

## `gemini_ask`

`query` is required. GitHub context parameters are optional and combinable:
`github_repo`, `github_ref`, `github_files`, `github_pr`, `github_commits`,
`github_diff_base`, and `github_diff_head`. The provider model and reasoning
policy are server configuration, not tool parameters.

## Provider configuration

| Variable | Description |
|---|---|
| `PROVIDER` | Required: `deepseek` or `qwen` |
| `PROVIDER_API_KEY` | Required provider credential |
| `PROVIDER_MODEL` | `deepseek-v4-pro`, `qwen3.7-max`, or `qwen3.7-plus` |
| `PROVIDER_BASE_URL` | Optional for DeepSeek; required for Qwen |
| `PROVIDER_MAX_TOKENS` | `0` uses the API default |

DeepSeek defaults to `https://api.deepseek.com`. A Qwen-compatible endpoint is
`https://dashscope-intl.aliyuncs.com/compatible-mode/v1`.

## Build / development

```bash
go build -o bin/mcp-gemini .
go test ./...
./run_lint.sh
```
