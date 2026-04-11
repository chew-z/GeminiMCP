package main

import "github.com/mark3labs/mcp-go/mcp"

// GeminiAskTool defines the gemini_ask tool specification.
//
// The GitHub-sourced context parameters (github_files, github_pr, github_commits,
// github_diff_base/github_diff_head) are all independent, optional peers. They can
// be mixed freely in a single call; the server fetches each present source and
// merges them into a stable order before handing off to Gemini.
var GeminiAskTool = mcp.NewTool(
	"gemini_ask",
	mcp.WithDescription("Use Google's Gemini AI model to ask about complex coding problems. "+
		"Attach GitHub context via any combination of github_files, github_pr, github_commits, "+
		"and github_diff_base/github_diff_head — all are optional and can be mixed in one call."),
	mcp.WithString("query", mcp.Required(), mcp.Description("The coding problem that we are asking Gemini AI to work on [question + code]")),
	mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
	mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
	mcp.WithString("github_repo", mcp.Description(
		"Required. Must be always provided when any github_* context parameter is used!")),
	mcp.WithString("github_ref", mcp.Description(
		"Optional: Git branch, tag, or commit SHA. Applies only to 'github_files'.")),
	mcp.WithArray("github_files", mcp.Description(
		"Optional: array of file paths in github_repo at github_ref to attach as inline file context. "+
			"Independent of and combinable with github_pr / github_commits / github_diff_*.",
	), mcp.WithStringItems()),
	mcp.WithNumber("github_pr", mcp.Description(
		"Optional: pull request number in github_repo. Attaches the PR description, unified diff, "+
			"and review comments. Independent of and combinable with the other github_* parameters.")),
	mcp.WithArray("github_commits", mcp.Description(
		"Optional: array of commit SHAs (short or full) in github_repo. "+
			"Each commit's patch and subject are attached in the user-supplied order. "+
			"Independent of and combinable with the other github_* parameters.",
	), mcp.WithStringItems()),
	mcp.WithString("github_diff_base", mcp.Description(
		"Optional: base ref for a GitHub compare diff (branch, tag, or SHA). "+
			"Must be paired with github_diff_head.")),
	mcp.WithString("github_diff_head", mcp.Description(
		"Optional: head ref for a GitHub compare diff (branch, tag, or SHA). "+
			"Must be paired with github_diff_base.")),
	mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process")),
	mcp.WithString("thinking_level", mcp.Description("Optional: Thinking level (low, medium, high). Default is 'high'")),
	mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
)

// GeminiSearchTool defines the gemini_search tool specification
var GeminiSearchTool = mcp.NewTool(
	"gemini_search",
	mcp.WithDescription("Use Google's Gemini AI model with Google Search to answer questions with grounded information"),
	mcp.WithString("query", mcp.Required(), mcp.Description("The question to ask Gemini using Google Search for grounding")),
	mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
	mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process (when supported)")),
	mcp.WithString("thinking_level", mcp.Description("Optional: Thinking level (minimal, low, medium, high). Default is 'low' for search")),
	mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
	mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
	mcp.WithString("start_time", mcp.Description(
		"Optional: Filter search results to those published after this time "+
			"(RFC3339 format, e.g. '2024-01-01T00:00:00Z'). If provided, end_time must also be provided.",
	)),
	mcp.WithString("end_time", mcp.Description(
		"Optional: Filter search results to those published before this time "+
			"(RFC3339 format, e.g. '2024-12-31T23:59:59Z'). If provided, start_time must also be provided.",
	)),
)
