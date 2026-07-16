package main

import "github.com/mark3labs/mcp-go/mcp"

var GeminiAskTool = mcp.NewTool(
	"gemini_ask",
	mcp.WithDescription(
		"Send a prompt to Gemini, optionally enriched with GitHub repository context "+
			"(files, PRs, commits, diffs). All github_* parameters are independent and combinable."),
	mcp.WithTitleAnnotation("Ask Gemini a Question"),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithOpenWorldHintAnnotation(true),
	mcp.WithString("query", mcp.Required(), mcp.Description("The coding question or task for Gemini")),
	mcp.WithString("github_repo", mcp.Description("Required. Must be always provided when any github_* context parameter is used!")),
	mcp.WithString("github_ref", mcp.Description("Optional: Git branch, tag, or commit SHA. Applies only to 'github_files'.")),
	mcp.WithArray(
		"github_files",
		mcp.Description(
			"Optional: array of file paths in github_repo at github_ref to attach as inline file context. "+
				"Independent of and combinable with github_pr / github_commits / github_diff_*.",
		),
		mcp.WithStringItems(),
	),
	mcp.WithNumber("github_pr", mcp.Description("Optional: pull request number in github_repo.")),
	mcp.WithArray("github_commits", mcp.Description("Optional: array of commit SHAs (short or full)."), mcp.WithStringItems()),
	mcp.WithString("github_diff_base", mcp.Description("Optional: base ref for a GitHub compare diff; must be paired with github_diff_head.")),
	mcp.WithString("github_diff_head", mcp.Description("Optional: head ref for a GitHub compare diff; must be paired with github_diff_base.")),
	// Keep removed model controls backwards-compatible: the handler logs and
	// ignores unknown legacy arguments while all other tools remain strict.
	mcp.WithSchemaAdditionalProperties(true),
	mcp.WithTaskSupport(mcp.TaskSupportOptional),
)
