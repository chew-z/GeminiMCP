package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

const prequalifySystemPrompt = `Classify this request into exactly one category.

Categories:
- general: Non-programming question, general knowledge, research, non-code tasks
- analyze: Understanding, explaining, or documenting code; architecture analysis
- review: Code quality review, best practices, refactoring, performance
- security: Security vulnerabilities, authentication, authorization, OWASP
- debug: Bug fixing, error analysis, troubleshooting, test failures
- tests: Generating new unit/integration tests or test cases for existing code`

// prequalifyQuery classifies a user query into one of the five categories
// using a lightweight Flash model call with structured enum output.
// On any failure it returns an error; the caller decides the fallback.
func (s *GeminiServer) prequalifyQuery(ctx context.Context, query, contextSummary string) (queryCategory, error) {
	logger := getLoggerFromContext(ctx)
	modelName := resolvePrequalifyModel(s.config.PrequalifyModel)

	userMessage := query
	if contextSummary != "" {
		userMessage = query + "\n\n" + contextSummary
	}

	config := buildPrequalifyConfig(modelName, s.config.PrequalifyThinkingLevel)
	contents := []*genai.Content{
		genai.NewContentFromText(userMessage, genai.RoleUser),
	}

	resp, err := s.client.Models.GenerateContent(ctx, modelName, contents, config)
	if err != nil {
		return "", fmt.Errorf("prequalify API call failed: %w", err)
	}

	cat, err := parsePrequalifyResponse(resp)
	if err != nil {
		return "", err
	}
	logger.Info("Pre-qualified query as '%s'", cat)
	return cat, nil
}

// resolvePrequalifyModel converts a tier name to a concrete model ID.
func resolvePrequalifyModel(tierName string) string {
	modelName := bestModelForTier(tierName)
	if modelName == "" {
		modelName = tierName
	}
	return ResolveModelID(modelName)
}

// buildPrequalifyConfig creates the GenerateContentConfig for pre-qualification.
func buildPrequalifyConfig(modelName, thinkingLevel string) *genai.GenerateContentConfig {
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(prequalifySystemPrompt, ""),
		ResponseMIMEType:  "application/json",
		ResponseSchema: &genai.Schema{
			Type:   genai.TypeString,
			Format: "enum",
			Enum:   []string{"general", "analyze", "review", "security", "debug", "tests"},
		},
		ServiceTier: genai.ServiceTierPriority,
	}

	modelInfo := GetModelByID(modelName)
	if modelInfo != nil && modelInfo.SupportsThinking {
		config.ThinkingConfig = &genai.ThinkingConfig{
			ThinkingLevel: genai.ThinkingLevel(thinkingLevel),
		}
	}
	return config
}

// parsePrequalifyResponse extracts and validates the category from an API response.
func parsePrequalifyResponse(resp *genai.GenerateContentResponse) (queryCategory, error) {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "", fmt.Errorf("prequalify returned empty response")
	}

	raw := strings.TrimSpace(resp.Text())

	// Try JSON-unquoting (structured output wraps the value in quotes)
	var parsed string
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		raw = parsed
	}

	cat := queryCategory(strings.ToLower(strings.TrimSpace(raw)))
	switch cat {
	case categoryGeneral, categoryAnalyze, categoryReview, categorySecurity, categoryDebug, categoryTests:
		return cat, nil
	default:
		return "", fmt.Errorf("prequalify returned unknown category: %q", raw)
	}
}

// resolveSystemPromptAsync returns a buffered channel that yields the
// system-prompt string for this request. It is the SOLE assigner of system
// instructions for gemini_ask and runs concurrently with context gathering.
//
// Resolution precedence:
//  1. GEMINI_PREQUALIFY=false → systemPromptGeneral (synchronous, no API call)
//  2. Pre-qualification succeeds → systemPromptForCategory(cat)
//  3. Pre-qualification fails    → analyze if any github_* present, else general
func (s *GeminiServer) resolveSystemPromptAsync(ctx context.Context, req mcp.CallToolRequest, logger Logger) <-chan string {
	ch := make(chan string, 1)
	if !s.config.Prequalify || s.client == nil || s.client.Models == nil {
		ch <- systemPromptGeneral
		return ch
	}
	go func() {
		query, ok := req.GetArguments()["query"].(string)
		if !ok {
			ch <- systemPromptGeneral
			return
		}
		summary := buildContextSummary(req)
		cat, err := s.prequalifyQuery(ctx, query, summary)
		if err != nil {
			logger.Warn("Pre-qualification failed: %v, using fallback", err)
			if hasGitHubContext(req) {
				cat = categoryAnalyze
			} else {
				cat = categoryGeneral
			}
		}
		ch <- systemPromptForCategory(cat)
	}()
	return ch
}

// buildContextSummary produces a short one-line description of the GitHub
// context attached to a request, suitable for the pre-qualification classifier.
// It contains no actual content — just names, numbers, and counts.
func buildContextSummary(req mcp.CallToolRequest) string {
	var parts []string

	repo := extractArgumentString(req, "github_repo", "")

	if prNum, hasPR := extractArgumentInt(req, "github_pr"); hasPR {
		parts = append(parts, fmt.Sprintf("PR #%d", prNum))
	}

	commits := extractArgumentStringArray(req, "github_commits")
	if len(commits) > 0 {
		parts = append(parts, fmt.Sprintf("%d commit(s)", len(commits)))
	}

	diffBase := extractArgumentString(req, "github_diff_base", "")
	diffHead := extractArgumentString(req, "github_diff_head", "")
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
	if extractArgumentString(req, "github_repo", "") != "" {
		return true
	}
	if _, hasPR := extractArgumentInt(req, "github_pr"); hasPR {
		return true
	}
	if len(extractArgumentStringArray(req, "github_commits")) > 0 {
		return true
	}
	if len(extractArgumentStringArray(req, "github_files")) > 0 {
		return true
	}
	if extractArgumentString(req, "github_diff_base", "") != "" {
		return true
	}
	return false
}
