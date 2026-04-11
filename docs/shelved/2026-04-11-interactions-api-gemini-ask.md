# Shelf note: Interactions API for `gemini_ask`

**Status:** Shelved — revisit when `google.golang.org/genai` exposes an Interactions client.
**Date:** 2026-04-11

## Context

Exploratory question: *"If we use `gemini_ask` in project (GitHub repo) context, would it make sense to use Gemini's Interactions API for long-term persistence, so each conversation doesn't start from zero?"*

User intent (in their own words):
- *"We don't start each conversation from zero point. We can start in a different moment of the repo history but LLM will be already aware what this repo is and what we are working on."*
- *"We might provide smaller, more narrow context."*
- *"When we ask for review of implementation Gemini suggested there is continuity."*

Hard constraint: **Go SDK only** — no bypassing `google.golang.org/genai` with direct REST.

## What we found

### Current state of `gemini_ask` in this codebase

- Tool definition: `tools.go:11-44` (`GeminiAskTool`)
- Handler: `gemini_ask_handler.go:49` (`GeminiAskHandler`)
- Call site: `s.client.Models.GenerateContent(...)` at `gemini_ask_handler.go:542` (with files) and `:566` (without files)
- Context pipeline: `parseAskRequest` → `gatherAllContext` (`:58`) → `applyContextInventory` (`:72`) → `processWithFiles` / `processWithoutFiles`
- **Fully stateless.** No session/thread field in the tool schema. No persistence layer. No in-memory cache keyed by repo.
- Implicit prefix caching by Gemini is noted at `gemini_ask_handler.go:473`, but not managed by the server.
- Adjacent tools: `gemini_search` (`tools.go:47-64`) — also stateless.
- `go.mod:11`: `google.golang.org/genai v1.53.0`

### Interactions API (from https://ai.google.dev/gemini-api/docs/interactions.md.txt)

- **Beta** — docs explicitly warn "features and schemas are subject to breaking changes"
- SDKs documented: **Python** (`google-genai`), **JavaScript** (`@google/genai`), **REST** (cURL). **Go SDK is not mentioned.**
- REST endpoint: `POST https://generativelanguage.googleapis.com/v1beta/interactions` (streaming via `?alt=sse`)
- Thread continuation: pass prior interaction `id` as `previous_interaction_id`
- Storage default: `store=true`. `tools` and `generation_config` must be re-specified every turn even with server-side state.

### `google.golang.org/genai` v1.53.0 (latest, released 2026-04-08)

Top-level services on `Client`:
`Models`, `Caches`, `Chats`, `Files`, `FileSearchStores`, `Batches`, `Tunings`, `Live`, `Tokens`, `Operations`, `Documents`

- **No `Interactions` service. No `Interaction` type. No `previous_interaction_id`.**
- Zero mentions of "interaction" in the public API docs on pkg.go.dev.
- Release notes v1.45.0 → v1.53.0 contain nothing about Interactions.

## Why shelved

Given the user's Go-SDK-only constraint, Interactions API is currently unreachable without violating it. Combined with the API's Beta status (breaking-change risk), the cost of early adoption via direct REST outweighs the benefit today.

## Signal to revisit

Watch `google.golang.org/genai` release notes for any of:
- A new `Interactions` field on `Client`
- Types matching `Interaction`, `InteractionConfig`, `CreateInteractionConfig`
- A `previous_interaction_id` parameter anywhere

Check periodically at: https://github.com/googleapis/go-genai/releases

When that lands (and Interactions exits Beta, or stability is acceptable), re-open this exploration.

## Alternatives noted for the goal (not pursued now)

The user's underlying goal — "narrower per-call context + conversation continuity across `gemini_ask` calls" — is achievable with building blocks already in the Go SDK, but was **not chosen** in this session. Recorded here only so we don't re-derive them next time:

- `client.Chats` — multi-turn chat, client-side history management
- `client.Caches` — Gemini Context Caching, server-side with TTL, referenced by cache name; a natural fit for cached repo/PR/diff payloads keyed on a content hash
- A Postgres-backed session store inside GeminiMCP, with a `session_id` added to `gemini_ask`'s schema — orthogonal to whichever SDK surface is used

If the user later wants to pursue continuity without waiting for Interactions, start a new exploration scoped to one of these three options (or a hybrid of `Caches` + session store).

## Outcome

No code changes. No schema changes. No new plan. Bookmark this file.
