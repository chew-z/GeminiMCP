# GeminiMCP Refactoring Code Review

**Date:** 2026-04-09  
**Reviewers:** Claude Opus 4.6 (orchestrator), OpenAI Codex (2 rounds), Google Gemini 3.1 Pro (2 rounds)  
**Scope:** 20 recent commits covering 3 major refactoring areas  
**Method:** Multi-step pair review with follow-up investigation

---

## Refactoring Summary

The project underwent a significant refactoring across ~20 commits:

1. **Removed explicit caching and file storage** (`9da4931`): Deleted `cache.go` (200 lines) and `files.go` (232 lines). Removed `CacheStore`, `FileStore`, `CacheInfo`, `FileInfo` from structs. Simplified `gemini_ask_handler.go` to use inline text injection instead of Files API uploads for text files. Removed caching config fields. Net: -678 lines across 14 files.

2. **Removed legacy Gemini 2.5 thinking support** (`ef9d65c`): Eliminated thinking budget tokens (4096/16384/24576). Removed `thinkingBudgetLevel` config. Unified thinking config to use `ThinkingLevel` string (`minimal`/`low`/`medium`/`high`). Net: -177 lines across 11 files.

3. **Removed then re-added dynamic model fetching** (`c641cf4`, `1918d84`, `f29e223`, `b10328c`): First removed the old complex dynamic fetching, then re-added it in a cleaner form. New `fetch_models.go` queries Gemini API, classifies models into tiers (`pro`/`flash`/`flash-lite`), picks latest version per tier. Removed `fallback_models.go` static lists. Added symlink safety in local file reading.

---

## Findings

### Bug 1: MaxOutputTokens calculation is broken (CRITICAL)

**Severity:** Critical — likely breaks all default requests  
**Files:** `fetch_models.go:122`, `handlers_common.go:193-209`  
**Found by:** Both Codex and Gemini independently  

**Problem:** `fetch_models.go` stores `InputTokenLimit` (~1M tokens) into `ContextWindowSize`. Then `configureMaxTokensOutput` calculates:
```
int32(1,048,576 * 0.75) = 786,432  (for ask)
int32(1,048,576 * 0.50) = 524,288  (for search)
```
The Gemini API's actual **output** limit is ~65K tokens for Pro models. The API does not silently cap this — it rejects with `400 InvalidArgument`.

**Evidence:** Codex confirmed that `genai.Model` has an `OutputTokenLimit` field (at `types.go:4056` in `genai v1.52.1`) that is available but not used by the project. The field is separate from `InputTokenLimit`.

**Fix:** Add `MaxOutputTokens int` to `GeminiModelInfo`, populate from `c.model.OutputTokenLimit` in `fetch_models.go`, use it in `configureMaxTokensOutput`.

---

### Bug 2: Search handler rejects model aliases (HIGH)

**Severity:** High — aliases work in ask but fail in search  
**Files:** `gemini_search_handler.go:29-36`, `handlers_common.go:118-125`  
**Found by:** Codex (traced exact code path), confirmed by Gemini  

**Problem:** `GeminiSearchHandler` calls `ValidateModelID` BEFORE `ResolveModelID` (lines 29 vs 35). The ask handler uses `createModelConfig` which does the correct order (resolve first, then validate).

**Traced path for `model="gemini-pro-latest"`:**
1. `ValidateModelID("gemini-pro-latest")` called
2. Not in model store as family/version
3. Special allowlist only permits `preview`, `exp`, `-dev` — `latest` doesn't match
4. Returns error → request fails before `ResolveModelID` ever runs

**Fix:** Refactor search handler to use `createModelConfig` (with parameterized defaults), or at minimum swap the call order.

---

### Bug 3: ValidateModelID says "we'll try anyway" but blocks the request (HIGH)

**Severity:** High — contradicts its own documented intent  
**Files:** `model_functions.go:114-141`, `handlers_common.go:125`, `gemini_search_handler.go:29`  
**Found by:** Both Codex and Gemini independently  

**Problem:** `ValidateModelID` returns an `error` with message ending "However, we will attempt to use this model anyway." But all callers use `if err != nil` and return early with `createErrorResult`. The function's intent (warn-and-continue) conflicts with Go's `error` return semantics.

**Fix:** Change return type to `string` (warning message), or have callers log instead of failing.

---

### Issue 4: Static defaults ignore dynamic model fetch (MEDIUM)

**Severity:** Medium — server won't auto-track newest models  
**Files:** `config.go:15-16`, `model_functions.go:20-27`, `fetch_models.go:49-78`  
**Found by:** Both Codex and Gemini  

**Problem:** `config.go` hardcodes `defaultGeminiModel = "gemini-3.1-pro-preview"` and `modelAliases` maps `"gemini-pro-latest"` to the same. After `FetchGeminiModels` discovers actual latest models, nothing updates these defaults or aliases.

If the API returns `gemini-4-pro-preview` as the latest Pro model, the server still defaults to the old one.

**Fix:** Have `FetchGeminiModels` call `AddDynamicAlias` for standard "latest" aliases based on tier winners.

---

### Issue 5: Thinking config is duplicated (LOW)

**Severity:** Low — maintenance burden, not a bug  
**Files:** `handlers_common.go:150-171`, `gemini_search_handler.go:82-107`  
**Found by:** Gemini (initial), Codex confirmed extractable  

**Problem:** Near-identical thinking configuration blocks in ask and search handlers. Only differences: default level source (`ThinkingLevel` vs `SearchThinkingLevel`) and warning message phrasing.

**Fix:** Extract into shared `configureThinking()` helper.

---

### Issue 6: FindFamilyReplacement is effectively dead (LOW)

**Severity:** Low — defensive code, not harmful  
**Files:** `model_functions.go:160-181`, `handlers_common.go:18-45`  
**Found by:** Codex (confirmed architecture prevents it from finding replacements)  

**Problem:** With 1-version-per-family from `fetch_models.go`, `FindFamilyReplacement` can never find a different version. `checkModelStatus` still logs deprecation warnings, but the auto-redirect via `AddDynamicAlias` is never reached.

**Status:** Acceptable as defensive code. Document the limitation.

---

### Issue 7: Stale comment in config.go (LOW)

**Severity:** Low — cosmetic  
**File:** `config.go:14`  

`// Note: if this value changes, make sure to update the models.go list` — references removed static model lists.

---

### Issue 8: gemini_models_handler.go missing bounds check (LOW)

**Severity:** Low — edge case  
**File:** `gemini_models_handler.go`  
**Found by:** Gemini  

**Problem:** Accesses `models[0]` without checking if the models slice is empty. Could panic if model fetch failed at startup.

---

## Clean Areas (No Issues Found)

- **Struct cleanup:** No leftover `CacheStore`/`FileStore`/`CacheInfo`/`FileInfo` fields
- **Removed fields:** `PreferredForThinking`, `PreferredForSearch`, `PreferredForCaching`, `SupportsCaching` — all fully removed
- **Tier classification** in `classifyModel`: Correctly checks `flash-lite` before `flash`
- **Version comparison** in `isNewerModel`: Sound logic with `-latest` alias handling
- **Implicit caching strategy:** Parts ordering (files first, query last) is correctly implemented
- **Argument extraction** in `extractArgumentStringArray`: Robust handling of JSON arrays, strings, and mixed inputs

---

## Recommendations (Priority Order)

1. **Fix MaxOutputTokens** — use `OutputTokenLimit` from SDK (Bug 1)
2. **Fix search handler model resolution order** — refactor to use `createModelConfig` or swap order (Bug 2)
3. **Fix ValidateModelID semantics** — change to warning, not error (Bug 3)
4. **Wire dynamic aliases** — update "latest" aliases after model fetch (Issue 4)
5. **Extract thinking config helper** — DRY up duplicated code (Issue 5)
6. **Add bounds check** in `gemini_models_handler.go` (Issue 8)
7. **Remove stale comment** in `config.go` (Issue 7)
