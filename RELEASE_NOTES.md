# v1.1.0 — Qwen Responses API Migration

## Highlights

- **Qwen now uses the Responses API** (`/v1/responses`) instead of Chat Completions. This enables `reasoning.effort` for granular reasoning control (replacing the deprecated `enable_thinking`), server-side session caching via `x-dashscope-session-cache` header, and lays groundwork for `previous_response_id` multi-turn conversations.
- DeepSeek remains on Chat Completions — both vendors share the same `Provider` interface, transparent to callers.

## Changes

### New
- `responsesProvider` — universal Responses API provider with full status handling (completed, incomplete, failed, cancelled)
- `qwenResponsesDialect` — maps `ThinkingSpec` to `reasoning.effort` (max/none), injects session cache header
- Debug-level logging of reasoning item count and summary length for Responses API parity with Chat Completions

### Modified
- `NewProvider` routes `PROVIDER=qwen` to `responsesProvider` (was `openaiProvider`)
- Rate limit handling and retry mechanism improvements

### Removed
- `qwenDialect` and `qwenThinkingEnabled` (Chat Completions Qwen dialect, superseded)

## Upgrade Notes

No configuration changes required. Existing `.env` with `PROVIDER=qwen` works out of the box — the server automatically selects the Responses API based on the vendor setting.

## Verification

- Build, test, vet, format, and lint all pass
- Local HTTP smoke test confirmed: reasoning tokens reported, proper usage accounting
