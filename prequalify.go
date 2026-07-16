package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const prequalifySystemPrompt = `Classify this request into exactly one category.

Categories:
- general: Non-programming question, general knowledge, research, non-code tasks
- analyze: Understanding, explaining, or documenting code; architecture analysis
- review: Code quality review, best practices, refactoring, performance
- security: Security vulnerabilities, authentication, authorization, OWASP
- debug: Bug fixing, error analysis, troubleshooting, test failures
- tests: Generating new unit/integration tests or test cases for existing code

Respond with JSON in exactly this form: {"category": "<one category name>"}.`

// prequalifyQuery classifies a user query into one of the five categories
// using the configured provider model with thinking disabled and JSON mode.
// On any failure it returns an error; the caller decides the fallback.
func (s *GeminiServer) prequalifyQuery(ctx context.Context, query, contextSummary string) (queryCategory, error) {
	if s == nil || s.provider == nil {
		return "", fmt.Errorf("prequalify: provider not initialized")
	}
	if strings.TrimSpace(query) == "" && strings.TrimSpace(contextSummary) == "" {
		return "", fmt.Errorf("prequalify: empty query and context")
	}

	logger := getLoggerFromContext(ctx)

	userMessage := query
	if contextSummary != "" {
		userMessage = query + "\n\n" + contextSummary
	}

	resp, err := s.provider.Generate(ctx, GenerationRequest{
		SystemPrompt:   prequalifySystemPrompt,
		Parts:          []ContentPart{{Text: userMessage}},
		ResponseFormat: "json_object",
		Thinking:       ThinkingSpec{Enabled: false},
		Temperature:    0,
	})
	if err != nil {
		return "", fmt.Errorf("prequalify API call failed: %w", err)
	}

	cat, raw, err := parsePrequalifyResponse(resp)
	if err != nil {
		logger.Debug("prequalify: raw=%q parse error: %v", raw, err)
		return "", err
	}
	logger.Debug("prequalify: raw=%q parsed=%s model=%s", raw, cat, s.config.ActiveModel())
	logger.Info("Pre-qualified query as '%s'", cat)
	return cat, nil
}

// parsePrequalifyResponse extracts and validates the category from an API
// response. The second return is the raw classifier text so callers can
// surface it in DEBUG logs when parsing fails.
func parsePrequalifyResponse(resp *GenerationResponse) (queryCategory, string, error) {
	if resp == nil {
		return "", "", fmt.Errorf("prequalify returned empty response")
	}

	raw := strings.TrimSpace(resp.Text)
	if raw == "" {
		return "", raw, fmt.Errorf("prequalify returned unknown category: %q", raw)
	}

	if category, ok := decodePrequalifyCategory(raw); ok {
		return validatePrequalifyCategory(category, raw)
	}
	if strings.HasPrefix(raw, "{") {
		return "", raw, fmt.Errorf("prequalify returned JSON object without a string category")
	}
	return validatePrequalifyCategory(raw, raw)
}

// decodePrequalifyCategory extracts a category from supported JSON response
// shapes: a category object, a single-key string object, or a JSON string.
func decodePrequalifyCategory(raw string) (string, bool) {
	if strings.HasPrefix(raw, "{") {
		return decodePrequalifyObject(raw)
	}

	var category string
	if err := json.Unmarshal([]byte(raw), &category); err == nil {
		return category, true
	}
	return "", false
}

// decodePrequalifyObject extracts a string category from supported JSON object shapes.
func decodePrequalifyObject(raw string) (string, bool) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &object); err != nil {
		return "", false
	}
	if value, ok := object["category"]; ok {
		return decodeJSONString(value)
	}
	if len(object) != 1 {
		return "", false
	}
	for _, value := range object {
		return decodeJSONString(value)
	}
	return "", false
}

// decodeJSONString decodes one JSON value only when it is a string.
func decodeJSONString(value json.RawMessage) (string, bool) {
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return "", false
	}
	return decoded, true
}

// validatePrequalifyCategory normalizes and validates one classifier category.
func validatePrequalifyCategory(value, raw string) (queryCategory, string, error) {
	cat := queryCategory(strings.ToLower(strings.TrimSpace(value)))
	switch cat {
	case categoryGeneral, categoryAnalyze, categoryReview, categorySecurity, categoryDebug, categoryTests:
		return cat, raw, nil
	default:
		return "", raw, fmt.Errorf("prequalify returned unknown category: %q", value)
	}
}

// resolvedPrompt carries both the system-prompt string and the category it was
// resolved from. Callers need the category to select the matching
// <final_instruction> body for the user-turn envelope.
type resolvedPrompt struct {
	SystemPrompt string
	Category     queryCategory
}

// resolveSystemPromptAsync returns a buffered channel that yields the resolved
// system prompt and category for this request. It is the SOLE assigner of
// system instructions for gemini_ask and runs concurrently with context
// gathering.
//
// Resolution precedence:
//  1. GEMINI_PREQUALIFY=false → systemPromptGeneral + categoryGeneral (synchronous, no API call)
//  2. Pre-qualification succeeds → systemPromptForCategory(cat) + cat
//  3. Pre-qualification fails    → analyze if any github_* present, else general
func (s *GeminiServer) resolveSystemPromptAsync(ctx context.Context, req mcp.CallToolRequest, query string, logger Logger) <-chan resolvedPrompt {
	ch := make(chan resolvedPrompt, 1)
	if !s.config.Prequalify || s.provider == nil {
		ch <- resolvedPrompt{SystemPrompt: systemPromptGeneral, Category: categoryGeneral}
		return ch
	}
	go func() {
		summary := buildContextSummary(req)
		cat, err := s.prequalifyQuery(ctx, query, summary)
		fallback := err != nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				logger.Warn("Pre-qualification failed: %v, using fallback", err)
			}
			if hasGitHubContext(req) {
				cat = categoryAnalyze
			} else {
				cat = categoryGeneral
			}
		}
		logger.Debug("prequalify resolved: category=%s fallback=%v has_github_context=%v",
			cat, fallback, hasGitHubContext(req))
		ch <- resolvedPrompt{SystemPrompt: systemPromptForCategory(cat), Category: cat}
	}()
	return ch
}

// buildContextSummary produces a short one-line description of the GitHub
// context attached to a request, suitable for the pre-qualification classifier.
// It contains no actual content — just names, numbers, and counts.
func buildContextSummary(req mcp.CallToolRequest) string {
	var parts []string

	repo := extractArgumentString(req, "github_repo")

	if prNum, hasPR := extractGitHubPRNumber(req); hasPR {
		parts = append(parts, fmt.Sprintf("PR #%d", prNum))
	}

	commits := extractArgumentStringArray(req, "github_commits")
	if len(commits) > 0 {
		parts = append(parts, fmt.Sprintf("%d commit(s)", len(commits)))
	}

	diffBase := extractArgumentString(req, "github_diff_base")
	diffHead := extractArgumentString(req, "github_diff_head")
	if diffBase != "" && diffHead != "" {
		parts = append(parts, fmt.Sprintf("diff %s..%s", diffBase, diffHead))
	}

	files := extractArgumentStringArray(req, "github_files")
	if len(files) > 0 {
		parts = append(parts, fmt.Sprintf("%d file(s)", len(files)))
	}

	if len(parts) == 0 {
		return ""
	}

	summary := "Context: " + strings.Join(parts, ", ")
	if repo != "" {
		summary += " in " + repo
	}
	return summary
}

// hasGitHubContext returns true if the request includes any github_* parameters.
func hasGitHubContext(req mcp.CallToolRequest) bool {
	if extractArgumentString(req, "github_repo") != "" {
		return true
	}
	if _, hasPR := extractGitHubPRNumber(req); hasPR {
		return true
	}
	if len(extractArgumentStringArray(req, "github_commits")) > 0 {
		return true
	}
	if len(extractArgumentStringArray(req, "github_files")) > 0 {
		return true
	}
	if extractArgumentString(req, "github_diff_base") != "" {
		return true
	}
	return false
}
