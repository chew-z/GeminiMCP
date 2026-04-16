# GeminiMCP — Prompts Reference

> Canonical reference for the server's prompt system: how MCP prompts are
> registered, how the server selects a system prompt for every request, and
> how to extend either layer.

For tool / parameter usage see [TOOLS.md](TOOLS.md). For one-screen cheat
sheet see [QUICKREF.md](QUICKREF.md).

---

## 1. Two Layers

The server has two distinct, non-overlapping prompt concepts:

| Layer | Lives in | Visible to | Picked by | Purpose |
|-------|----------|------------|-----------|---------|
| **MCP prompts** | `prompts.go` | MCP clients | The user (slash menu) | Discoverable entry points that emit a pre-filled `gemini_ask` (or `gemini_search`) invocation. |
| **System prompts** | `system_prompts.go` | Gemini (model only) | The server | XML-structured instructions injected as `SystemInstruction` on every Gemini call. Drive the model's role, output format, and constraints. |

The two layers are independent. An MCP prompt does **not** map to a system
prompt. Both `gemini_ask` calls — whether triggered by an MCP prompt, by a
direct tool invocation, or anywhere in between — flow through the same
server-side selection logic.

Per CLAUDE.md principle #1: **the server is the sole authority on system
prompt selection**. The `gemini_ask` and `gemini_search` tools have no
`systemPrompt` parameter and clients cannot inject one.

---

## 2. MCP Prompt Registry

Eleven prompts are registered by `registerPrompts` (`server_handlers.go`)
from the `Prompts` slice in `prompts.go`.

### 2.1 GitHub-workflow prompts (3)

Custom handlers in `prompt_handlers.go` emit a pre-filled `gemini_ask`
invocation with the appropriate `github_*` parameters.

| Prompt | Required args | Optional args | Wraps |
|--------|---------------|---------------|-------|
| `review_pr` | `owner`, `repo`, `pr_number` | `focus` | `gemini_ask` + `github_pr` |
| `explain_commit` | `owner`, `repo`, `sha` | `question` | `gemini_ask` + `github_commits` |
| `compare_refs` | `owner`, `repo`, `base`, `head` | `question` | `gemini_ask` + `github_diff_base` / `github_diff_head` |

For inspecting files, clients call `gemini_ask` directly with `github_repo`,
`github_files`, and `github_ref`. There is no wrapper prompt — the parameters
are already self-explanatory.

### 2.2 Generic coding prompts (8)

All eight share the same argument schema:

| Argument | Required | Description |
|----------|----------|-------------|
| `problem_statement` | Yes | Free-form description of the problem or code |
| `model` | No | Override the default model (tier alias or explicit ID) |
| `thinking_level` | No | `minimal`, `low`, `medium`, `high` |

| Prompt | Best for |
|--------|----------|
| `code_review` | Quality, bugs, security, performance, maintainability |
| `explain_code` | Algorithms, design patterns, data structures |
| `debug_help` | Root-cause analysis with fix proposals |
| `refactor_suggestions` | Modernization and structural improvements |
| `architecture_analysis` | High-level design, component breakdown, data flow |
| `test_generate` | Unit / integration tests with happy path + edge cases |
| `security_analysis` | OWASP Top 10 vulnerability scan with remediations |
| `research_question` | Web-search-grounded research (routes to `gemini_search`) |

Each emits an instruction message asking the MCP client to invoke
`gemini_ask` (or `gemini_search` for `research_question`) with the
`problem_statement` wrapped in a sanitized `<problem_statement>` block. The
generation of these instructions lives in `createTaskInstructions` and
`createSearchInstructions` in `prompt_handlers.go`.

The generic prompts do **not** carry per-prompt system prompt strings.
The server picks the system prompt server-side from the request itself.

---

## 3. System Prompts — Categories

Defined as XML-structured `const` strings in `system_prompts.go`. Each
follows the Gemini 3 prompting guide: `<role>` / `<instructions>` /
`<constraints>` / `<output_format>`.

### 3.1 The six `gemini_ask` categories

| Category | Constant | Role | Output format |
|----------|----------|------|---------------|
| `general` | `systemPromptGeneral` | Knowledgeable assistant | Direct Markdown answer |
| `analyze` | `systemPromptAnalyze` | Senior engineer explaining code | Overview / Detailed Breakdown / Key Design Decisions |
| `review` | `systemPromptReview` | Senior reviewer | Summary / Critical Issues / Improvements / Positive Notes |
| `security` | `systemPromptSecurity` | OWASP-trained cybersecurity expert | Summary / Findings (severity-tagged) / Recommendations |
| `debug` | `systemPromptDebug` | Debugger and systems engineer | Problem Analysis / Root Cause / Fix / Verification |
| `tests` | `systemPromptTests` | Test engineering expert | Tests for X (code block) / Coverage Summary |

The mapping `category → constant` lives in `systemPromptForCategory`
(`system_prompts.go`). Default arm returns `systemPromptGeneral`.

### 3.2 The `gemini_search` system prompt

`systemPromptSearch` is hard-wired in `gemini_search_handler.go`. It is not
selected by pre-qualification — search always uses the same prompt because
the task shape (web-grounded research, cited sources) is fixed.

### 3.3 Context inventory addendum

When any `github_*` context is attached, `applyContextInventory`
(`gemini_ask_handler.go`) appends a deterministic, descriptive trailer to
the chosen system prompt that names every block actually present (file
count, commit shas, PR number + title, diff base..head). This is purely
descriptive — it adds no new instructions, only labels the model can cite
by their XML tag name (`<file>`, `<commit>`, `<diff>`, `<pull_request>`).

### 3.4 User-turn envelope + final instruction

Independently of the system prompt, every `gemini_ask` and `gemini_search`
user turn is wrapped by `envelope.go` into a Gemini 3-style XML envelope:

- `<context repo="…">` (present only when at least one `github_*` source
  succeeds) holds every `<commit>`, `<diff>`, `<pull_request>`, and `<file>`
  fragment in stable merge order.
- An anchor line `USING THE CONTEXT PROVIDED ABOVE, YOUR TASK IS:` bridges
  the context and the task.
- `<task>` wraps `<query>` and, when any source failed, a sibling
  `<unloaded_context>` listing the failed items.
- `<final_instruction>` carries a short scenario-specific body picked from
  `final_instructions.go` by the **same category** the pre-qualifier
  produced: `general`, `analyze`, `review`, `security`, `debug`, `tests`.
  `gemini_search` uses a hard-coded `finalInstructionSearch`.

Attacker-controlled metadata goes through `xmlAttr` (escape `& < > "`);
attacker-controlled bodies go through `cdataWrap` (CDATA-wrap with
`]]>` splitting). See [ARCHITECTURE OVERVIEW § 8](ARCHITECTURE%20OVERVIEW.md#8-security).

---

## 4. Pre-Qualification — Selection Algorithm

Implemented in `prequalify.go`. Runs concurrently with GitHub context
fetching so it adds zero visible latency on requests that already pay an
HTTP round-trip.

### 4.1 Flow

```
gemini_ask request
      │
      ├─ resolveSystemPromptAsync(ctx, req)         ── prequalify.go ──┐
      │       │                                                       │
      │       ├─ s.config.Prequalify == false?                        │
      │       │     └─ ch <- {SystemPrompt: general, Category: general}
      │       │                                                       │
      │       └─ goroutine                                             │
      │             ├─ buildContextSummary(req)    (one-line summary)  │
      │             ├─ prequalifyQuery(query, summary)                 │
      │             │     └─ Flash call w/ JSON-enum schema            │
      │             ├─ on error → analyze if hasGitHubContext(req)     │
      │             │             else general                         │
      │             └─ ch <- resolvedPrompt{                           │
      │                        SystemPrompt: systemPromptForCategory(cat),
      │                        Category:     cat,                      │
      │                     }                                          │
      │                                                                │
      ├─ gatherAllContext(ctx, req)  ── parallel ──                   │
      │                                                                │
      └─ <-promptCh   ── system instruction + final-instruction key ───┘
```

The channel yields a `resolvedPrompt{SystemPrompt, Category}`; the system
prompt becomes `SystemInstruction`, and the category selects the
`<final_instruction>` body via `finalInstructionFor(category)` in
`final_instructions.go`. `GeminiAskHandler` blocks on the channel only
after all context-fetching work is complete.

### 4.2 The classifier

| Property | Value |
|----------|-------|
| Model | `s.config.PrequalifyModel` resolved to a concrete tier (default `gemini-flash`) |
| Service tier | `priority` (always — pre-qualification is on the latency hot path) |
| Max output tokens | unset — model default (thinking tokens would otherwise starve the enum output) |
| Response schema | `enum` of the six category names |
| Thinking | Enabled at `s.config.PrequalifyThinkingLevel` if the model supports it |
| System prompt | `prequalifySystemPrompt` — an enumerated category list |

The user message is the original `query` plus a one-line context summary
produced by `buildContextSummary`: only counts and identifiers, never file
contents. Example:

```
What does this commit do?

Context: 1 commit(s) in owner/repo
```

### 4.3 Validation

`parsePrequalifyResponse` accepts a JSON-quoted enum value, lowercases and
trims it, and validates against the closed set `{general, analyze, review,
security, debug, tests}`. Any other return value is treated as an error and
falls through to the heuristic.

### 4.4 Final resolution precedence

For `gemini_ask`:

1. `GEMINI_PREQUALIFY=false` → `systemPromptGeneral` (no API call)
2. Pre-qualification succeeds → `systemPromptForCategory(cat)`
3. Pre-qualification fails → heuristic fallback:
   - `analyze` if any `github_*` parameter is present
   - `general` otherwise

For `gemini_search`: always `systemPromptSearch`.

---

## 5. Configuration

| Env var | Default | Description |
|---------|---------|-------------|
| `GEMINI_PREQUALIFY` | `true` | Enable pre-qualification. Set `false` to force `systemPromptGeneral` for every `gemini_ask` call. |
| `GEMINI_PREQUALIFY_MODEL` | `gemini-flash` | Tier name resolved at runtime to a concrete model ID. |
| `GEMINI_PREQUALIFY_THINKING` | `medium` | Thinking level for the classifier call. |

There is no `GEMINI_SYSTEM_PROMPT` or `GEMINI_SEARCH_SYSTEM_PROMPT`
operator override. The XML-structured prompts are the single source of
truth; if the operator wants different behavior they edit
`system_prompts.go` and rebuild.

---

## 6. Extending the System

### 6.1 Add a new category

1. Define a `categoryFoo queryCategory = "foo"` constant in
   `system_prompts.go`.
2. Add a `systemPromptFoo` const with the four XML sections (`role`,
   `instructions`, `constraints`, `output_format`).
3. Add a `case categoryFoo: return systemPromptFoo` arm to
   `systemPromptForCategory`.
4. Add `"foo"` to `ResponseSchema.Enum` and to the validation switch in
   `parsePrequalifyResponse` in `prequalify.go`.
5. Update the category list in `prequalifySystemPrompt` so the classifier
   knows the new option exists.
6. Add a `finalInstructionFoo` constant and a `categoryFoo: finalInstructionFoo`
   entry to `finalInstructionByCategory` in `final_instructions.go`.
7. Add a row to the category table in this file and in
   [TOOLS.md](TOOLS.md).

### 6.2 Add a new MCP prompt

For a generic problem-statement prompt:

```go
NewPromptDefinition(
    "my_prompt",
    "Description shown in the slash menu",
)
```

Append to the `Prompts` slice in `prompts.go`. The generic handler
(`promptHandler` in `prompt_handlers.go`) will emit a `gemini_ask`
instruction with the user's `problem_statement`. The system prompt will be
picked server-side via pre-qualification — do not bake one in.

For a GitHub-workflow prompt with custom arguments and a pre-filled
`gemini_ask` invocation, follow `buildReviewPRHandler` in
`prompt_handlers.go` as the reference implementation and register via
`newGitHubPromptDefinition`.

### 6.3 Modify an existing category prompt

Edit the relevant `systemPromptX` const in `system_prompts.go`. Keep the
four XML sections, keep `Output in Markdown format` in `<constraints>`,
and keep the output_format consistent so downstream MCP clients can rely
on a stable response shape.

---

## 7. Reference: Source Files

| File | Responsibility |
|------|----------------|
| `system_prompts.go` | `queryCategory` enum, `systemPromptForCategory`, all XML system-prompt constants. |
| `prequalify.go` | `resolveSystemPromptAsync` (yields `resolvedPrompt{SystemPrompt, Category}`), `prequalifyQuery`, `buildContextSummary`, `hasGitHubContext`, classifier config and validation. |
| `envelope.go` | `xmlAttr`, `cdataWrap`, `wrapUserTurnWithContext`, `wrapUserTurnQueryOnly`, `renderUnloadedContext` — the user-turn XML envelope. |
| `final_instructions.go` | `finalInstructionFor(category)` and the category → instruction table driving `<final_instruction>`. |
| `prompts.go` | `Prompts` slice — the MCP prompt registry. |
| `prompt_handlers.go` | `promptHandler` (generic), `buildReviewPRHandler` / `buildExplainCommitHandler` / `buildCompareRefsHandler` (custom), `createTaskInstructions`, `createSearchInstructions`. |
| `gemini_ask_handler.go` | `GeminiAskHandler` — the **sole** assigner of `SystemInstruction` for `gemini_ask`; `applyContextInventory` for the descriptive addendum; calls `wrapUserTurnWithContext` / `wrapUserTurnQueryOnly`. |
| `gemini_search_handler.go` | `GeminiSearchHandler` — assigns `systemPromptSearch` directly; wraps query via `wrapUserTurnQueryOnly` + `finalInstructionSearch`. |
| `handlers_common.go` | `createModelConfig` — does **not** touch `SystemInstruction`; the caller owns it. |
