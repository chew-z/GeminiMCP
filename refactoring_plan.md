### Proposed Refactoring Plan

Here is a detailed, step-by-step plan to implement the new, more efficient architecture.

---

#### **Phase 1: Redefine the Prompt Handler's Role**

The goal of this phase is to make prompt handlers act as "template generators," not file processors.

1.  **Modify `prompt_handlers.go` - Refactor `handlePrompt`:**
    *   The function will no longer call `readLocalFiles`. Its responsibility will end at expanding file paths.
    *   The `PromptBuilder` function signature will be changed to `func(req mcp.GetPromptRequest, language string) (string, string)`, removing the `codeContent` parameter.
    *   The function will now return a structured JSON object containing the `system_prompt`, `user_prompt_template`, and the list of `file_paths`. This JSON will be returned in the `Description` field of the `mcp.GetPromptResult`. The `Messages` field will be empty.

2.  **Modify `prompt_handlers.go` - Update All Prompt Handlers:**
    *   Each handler (e.g., `CodeReviewHandler`, `DocGenerateHandler`) will be updated to use the new `handlePrompt`.
    *   The `PromptBuilder` they provide will now generate a `user_prompt_template` with a placeholder (e.g., `Please review the following code:

{{file_content}}`) instead of the actual code.

---

#### **Phase 2: Clean Up Utilities and Configuration**

This phase removes the now-unnecessary code.

1.  **Modify `gemini_utils.go`:**
    *   Remove the `readLocalFiles`, `detectLanguageFromPath`, and `detectPrimaryLanguage` functions entirely.
    *   The `ProjectLanguage` setting from the config will now be passed directly to the `PromptBuilder` in `handlePrompt`.

2.  **Modify `config.go` and `structs.go`:**
    *   The `ProjectLanguage` configuration will be kept as it's still useful for the templates. No other changes are needed here.

---

#### **Phase 3: Empower the `gemini_ask` Tool**

This phase makes the `gemini_ask` tool flexible enough to handle the new workflow.

1.  **Modify `direct_handlers.go` - Update `GeminiAskHandler`:**
    *   I will adjust the logic for handling the `systemPrompt` parameter.
    *   The new logic will be:
        1.  Check if the `systemPrompt` argument exists in the request.
        2.  If it exists (even if it's an empty string), use the value provided by the client.
        3.  If the argument does **not** exist, then and only then fall back to the default `config.GeminiSystemPrompt`.
    *   This change is critical as it allows the client to suppress the default system prompt, preventing the "double prompt" issue.

---

#### **Phase 4: Update Tests**

The tests must be updated to reflect the new architecture.

1.  **Modify `prompts_test.go`:**
    *   The tests will be rewritten to validate the new JSON output from the prompt handlers.
    *   Instead of checking for file content in the result, the tests will parse the JSON from the `Description` field and assert that the `system_prompt`, `user_prompt_template`, and `file_paths` are correct.
    *   Tests for file I/O within the prompt handlers will be removed.
