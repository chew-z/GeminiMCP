# Model Auto-Redirect Refactoring Review — 2026-04-09

Review by Codex (o3) and Gemini 3.1 Pro after 22-commit refactoring.

## First Rules Alignment

| Rule | Status |
|------|--------|
| 1. Single binary, 3 tools | Aligned |
| 2. 3 tiers, latest/preview, dynamic fetch | Partially aligned — see issues #2, #3, #5 |
| 3. Never reject, always redirect | Aligned after today's changes |
| 4. Implicit prefix caching | Aligned — files-before-query ordering correct |
| 5. Env-var config, dual transport | Aligned |

## Issues

### 1. CRITICAL — Output Token Miscalculation

`ContextWindowSize` stores the **input** token limit (`c.model.InputTokenLimit`), but `configureMaxTokensOutput` in `handlers_common.go:210` uses 75% of it as the **output** limit. Gemini models have much smaller output caps (8K–65K). Setting 750K output tokens will cause API rejections.

**Files:** `fetch_models.go:97`, `handlers_common.go:210`

**Fix:** Store `OutputTokenLimit` separately in `GeminiModelInfo` and use it as the cap for `MaxOutputTokens`.

### 2. HIGH — Hardcoded Aliases Contradict Dynamic Fetch

Static `modelAliases` map in `model_functions.go:18-24` pins specific version strings (e.g., `"gemini-pro-latest" → "gemini-3.1-pro-preview"`), subverting the dynamic startup fetch. If API returns `gemini-4.0-pro` as winner, `"gemini-pro-latest"` still routes to 3.1.

**Fix:** Start `modelAliases` empty. Let unknown models fall through to `bestModelForTier()`. Only `checkModelStatus` and `addDynamicAlias` should populate aliases at runtime.

### 3. HIGH — Hardcoded Default Model in Config

`config.go:14` hardcodes `"gemini-3.1-pro-preview"` as `defaultGeminiModel`. Brittle when models are fetched dynamically.

**Fix:** Use a generic tier identifier (e.g., `"gemini-pro"`) that `ResolveModelID` → `ValidateModelID` → `bestModelForTier` will resolve to whatever the startup fetch selected.

### 4. MEDIUM — Cross-Tier Fallback in bestModelForTier()

When tier classification fails, `bestModelForTier()` falls back to `models[0]` (pro), silently upgrading flash-lite requests to pro tier.

**Fix:** Only fall back within the same tier. If no tier match, return the input model name unchanged (let it pass through).

### 5. MEDIUM — Startup Doesn't Filter by PREVIEW/STABLE

`selectBestModels` in `fetch_models.go` picks the latest model per tier by version number without checking `ModelStage`. A DEPRECATED model with the highest version could win.

**Fix:** Filter candidates by `ModelStage` during `selectBestModels` — only accept PREVIEW and STABLE.

### 6. LOW — CLI Flag Validated Before Catalog Fetch

`--gemini-model` validation in `main.go:56` runs before `FetchGeminiModels`, so the redirect has no catalog to work with.

**Fix:** Move validation after `setupGeminiServer` (which calls `FetchGeminiModels`), or defer validation entirely to request time.

## Reviewers

- **Codex (o3):** Findings #2, #4, #5, #6
- **Gemini 3.1 Pro:** Findings #1, #2, #3, #5
