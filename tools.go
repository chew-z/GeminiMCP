package main

import "github.com/mark3labs/mcp-go/mcp"

// GeminiAskTool defines the gemini_ask tool specification
var GeminiAskTool = mcp.NewTool(
	"gemini_ask",
	mcp.WithDescription("Use Google's Gemini AI model to ask about complex coding problems"),
	mcp.WithString("query", mcp.Required(), mcp.Description("The coding problem that we are asking Gemini AI to work on [question + code]")),
	mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
	mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
	mcp.WithArray("github_files", mcp.Description(
		"An array of file paths from a GitHub repository to provide context. "+
			"This is the preferred method. Requires 'github_repo' and 'github_ref' to be set.",
	)),
	mcp.WithString("github_repo", mcp.Description("GitHub repository (owner/repo) to fetch files from. Required when using 'github_files'.")),
	mcp.WithString("github_ref", mcp.Description("Git branch, tag, or commit SHA. Required when using 'github_files'.")),
	mcp.WithArray("file_paths", mcp.Description(
		"An array of local file paths. This should only be used when specifically instructed, "+
			"as it's only supported in 'stdio' transport mode.",
	)),
	mcp.WithBoolean("use_cache", mcp.Description("Optional: Whether to try using a cache for this request (only works with compatible models)")),
	mcp.WithString("cache_ttl", mcp.Description("Optional: TTL for cache if created (e.g., '10m', '1h'). Default is 10 minutes")),
	mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process")),
	mcp.WithString("thinking_level", mcp.Description("Optional: Thinking level for Gemini 3 (low, high). Default is 'high'")),
	mcp.WithNumber("thinking_budget", mcp.Description("Optional (Legacy): Maximum number of tokens for thinking process on Gemini 2.5 models (0-24576)")),
	mcp.WithString("thinking_budget_level", mcp.Description(
		"Optional (Legacy): Predefined thinking budget level for Gemini 2.5 models "+
			"(none, low, medium, high)",
	)),
	mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
)

// GeminiSearchTool defines the gemini_search tool specification
var GeminiSearchTool = mcp.NewTool(
	"gemini_search",
	mcp.WithDescription("Use Google's Gemini AI model with Google Search to answer questions with grounded information"),
	mcp.WithString("query", mcp.Required(), mcp.Description("The question to ask Gemini using Google Search for grounding")),
	mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
	mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process (when supported)")),
	mcp.WithString("thinking_level", mcp.Description("Optional: Thinking level for Gemini 3 (low, high). Default is 'high'")),
	mcp.WithNumber("thinking_budget", mcp.Description("Optional (Legacy): Maximum number of tokens for thinking process on Gemini 2.5 models (0-24576)")),
	mcp.WithString("thinking_budget_level", mcp.Description(
		"Optional (Legacy): Predefined thinking budget level for Gemini 2.5 models "+
			"(none, low, medium, high)",
	)),
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

// GeminiModelsTool defines the gemini_models tool specification
var GeminiModelsTool = mcp.NewTool(
	"gemini_models",
	mcp.WithDescription("List available Gemini models with descriptions"),
)
