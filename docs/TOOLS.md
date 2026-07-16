# GeminiMCP — Tool Reference

## Tool: `gemini_ask`

`gemini_ask` sends a coding or analysis request to the configured DeepSeek or
Qwen provider. Provider choice, model, token limit, and reasoning policy are
configured at server startup and cannot be overridden per call.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `query` | string | Yes | The coding question or task |
| `github_repo` | string | No* | `owner/repo`; required when any GitHub context is used |
| `github_ref` | string | No | Ref for `github_files` |
| `github_files` | string[] | No | Repository paths to attach as text context |
| `github_pr` | number | No | Pull request context |
| `github_commits` | string[] | No | Commit context |
| `github_diff_base` | string | No | Compare base; pair with `github_diff_head` |
| `github_diff_head` | string | No | Compare head; pair with `github_diff_base` |

Example:

```json
{"github_repo":"owner/repo","github_pr":42,"query":"Review this change for races"}
```

## Provider setup

Use `PROVIDER=deepseek` with `PROVIDER_MODEL=deepseek-v4-pro`, or
`PROVIDER=qwen` with `PROVIDER_MODEL=qwen3.7-max` or `qwen3.7-plus`. Qwen
requires `PROVIDER_BASE_URL`; DeepSeek defaults to `https://api.deepseek.com`.
