package main

import "github.com/mark3labs/mcp-go/mcp"

// Prompts defines all the available prompts for the server.
//
// Generic coding prompts are thin pass-throughs whose query text flows through
// pre-qualification like any other gemini_ask call. The server is the sole
// authority on system prompt selection — clients cannot inject one. The
// per-prompt names below remain as discoverable entry points for MCP clients.
var Prompts = []*PromptDefinition{
	NewPromptDefinition(
		"code_review",
		"Review code for best practices, potential issues, and improvements",
	),
	NewPromptDefinition(
		"explain_code",
		"Explain how code works in detail, including algorithms and design patterns",
	),
	NewPromptDefinition(
		"debug_help",
		"Help debug issues by analyzing code, error messages, and context",
	),
	NewPromptDefinition(
		"refactor_suggestions",
		"Suggest improvements and refactoring opportunities for existing code",
	),
	NewPromptDefinition(
		"architecture_analysis",
		"Analyze system architecture, design patterns, and structural decisions",
	),
	NewPromptDefinition(
		"test_generate",
		"Generate unit tests, integration tests, or test cases for code",
	),
	NewPromptDefinition(
		"security_analysis",
		"Analyze code for security vulnerabilities and best practices",
	),
	NewPromptDefinition(
		"research_question",
		"Research current information and trends using web search",
	),
	// --- GitHub-workflow shortcut prompts ---
	//
	// These are discoverable entry points that emit a pre-filled `gemini_ask`
	// invocation using the new github_* parameters. They are equal-status
	// shortcuts, NOT a hierarchy — smart clients can ignore them entirely and
	// call `gemini_ask` directly with any mix of parameters.
	newGitHubPromptDefinition(
		"review_pr",
		"Review a GitHub pull request (diff + description + review comments) using gemini_ask",
		[]mcp.PromptArgument{
			{Name: "owner", Description: "GitHub repository owner.", Required: true},
			{Name: "repo", Description: "GitHub repository name.", Required: true},
			{Name: "pr_number", Description: "Pull request number.", Required: true},
			{Name: "focus", Description: "Optional: aspect the review should focus on (e.g. security, tests)."},
		},
		buildReviewPRHandler,
	),
	newGitHubPromptDefinition(
		"explain_commit",
		"Explain what a single commit does and why, using gemini_ask with github_commits",
		[]mcp.PromptArgument{
			{Name: "owner", Description: "GitHub repository owner.", Required: true},
			{Name: "repo", Description: "GitHub repository name.", Required: true},
			{Name: "sha", Description: "Commit SHA (short or full).", Required: true},
			{Name: "question", Description: "Optional: follow-up question about the commit."},
		},
		buildExplainCommitHandler,
	),
	newGitHubPromptDefinition(
		"compare_refs",
		"Summarize the diff between two GitHub refs using gemini_ask with github_diff_*",
		[]mcp.PromptArgument{
			{Name: "owner", Description: "GitHub repository owner.", Required: true},
			{Name: "repo", Description: "GitHub repository name.", Required: true},
			{Name: "base", Description: "Base ref (branch, tag, or SHA).", Required: true},
			{Name: "head", Description: "Head ref (branch, tag, or SHA).", Required: true},
			{Name: "question", Description: "Optional: follow-up question about the changes."},
		},
		buildCompareRefsHandler,
	),
}

// newGitHubPromptDefinition is the constructor used for the bespoke GitHub
// workflow prompts. It wires in a per-prompt argument schema and a custom
// handler factory instead of relying on the default problem_statement flow.
func newGitHubPromptDefinition(
	name, description string,
	args []mcp.PromptArgument,
	factory func(s *GeminiServer) mcpPromptHandlerFunc,
) *PromptDefinition {
	return &PromptDefinition{
		Prompt: &mcp.Prompt{
			Name:        name,
			Description: description,
			Arguments:   args,
		},
		HandlerFactory: factory,
	}
}
