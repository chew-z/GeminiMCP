package main

import "github.com/mark3labs/mcp-go/mcp"

// CodeReviewPrompt defines a prompt for code review tasks
var CodeReviewPrompt = mcp.NewPrompt(
	"code_review",
	mcp.WithPromptDescription("Review code for best practices, potential issues, and improvements"),
	mcp.WithArgument("files", mcp.RequiredArgument(), mcp.ArgumentDescription("File path(s) to analyze (e.g., './src/auth.js' or './src/')")),
	mcp.WithArgument("language", mcp.ArgumentDescription("Programming language (auto-detected if not specified)")),
	mcp.WithArgument("focus", mcp.ArgumentDescription("Specific areas to focus on (security, performance, style, etc.)")),
	mcp.WithArgument("severity", mcp.ArgumentDescription("Minimum severity level for issues (info, warning, error)")),
)

// ExplainCodePrompt defines a prompt for explaining code functionality
var ExplainCodePrompt = mcp.NewPrompt(
	"explain_code",
	mcp.WithPromptDescription("Explain how code works in detail, including algorithms and design patterns"),
	mcp.WithArgument("files", mcp.RequiredArgument(), mcp.ArgumentDescription("File path(s) to explain (e.g., './algorithm.py' or './src/')")),
	mcp.WithArgument("audience", mcp.ArgumentDescription("Target audience level (beginner, intermediate, expert)")),
	mcp.WithArgument("include_examples", mcp.ArgumentDescription("Whether to include usage examples (true/false)")),
	mcp.WithArgument("focus_areas", mcp.ArgumentDescription("Specific aspects to focus on (algorithm, architecture, patterns, etc.)")),
)

// DebugHelpPrompt defines a prompt for debugging assistance
var DebugHelpPrompt = mcp.NewPrompt(
	"debug_help",
	mcp.WithPromptDescription("Help debug issues by analyzing code, error messages, and context"),
	mcp.WithArgument("files", mcp.RequiredArgument(), mcp.ArgumentDescription("File path(s) with problematic code (e.g., './buggy.js')")),
	mcp.WithArgument("error_message", mcp.ArgumentDescription("Error message or description of the issue")),
	mcp.WithArgument("expected_behavior", mcp.ArgumentDescription("What the code should do")),
	mcp.WithArgument("context", mcp.ArgumentDescription("Additional context about the environment or setup")),
)

// RefactorSuggestionsPrompt defines a prompt for code refactoring suggestions
var RefactorSuggestionsPrompt = mcp.NewPrompt(
	"refactor_suggestions",
	mcp.WithPromptDescription("Suggest improvements and refactoring opportunities for existing code"),
	mcp.WithArgument("files", mcp.RequiredArgument(), mcp.ArgumentDescription("File path(s) to analyze for refactoring (e.g., './legacy.js')")),
	mcp.WithArgument("goals", mcp.ArgumentDescription("Refactoring goals (readability, performance, maintainability, etc.)")),
	mcp.WithArgument("constraints", mcp.ArgumentDescription("Any constraints or limitations to consider")),
	mcp.WithArgument("include_examples", mcp.ArgumentDescription("Whether to provide refactored code examples (true/false)")),
)

// ArchitectureAnalysisPrompt defines a prompt for system architecture analysis
var ArchitectureAnalysisPrompt = mcp.NewPrompt(
	"architecture_analysis",
	mcp.WithPromptDescription("Analyze system architecture, design patterns, and structural decisions"),
	mcp.WithArgument("files", mcp.RequiredArgument(), mcp.ArgumentDescription("File path(s) or directories for architectural analysis (e.g., './src/')")),
	mcp.WithArgument("scope", mcp.ArgumentDescription("Analysis scope (component, service, system, etc.)")),
	mcp.WithArgument("focus", mcp.ArgumentDescription("Focus areas (scalability, security, maintainability, etc.)")),
	mcp.WithArgument("include_recommendations", mcp.ArgumentDescription("Whether to include improvement recommendations (true/false)")),
)

// DocGeneratePrompt defines a prompt for documentation generation
var DocGeneratePrompt = mcp.NewPrompt(
	"doc_generate",
	mcp.WithPromptDescription("Generate comprehensive documentation for code, APIs, or systems"),
	mcp.WithArgument("files", mcp.RequiredArgument(), mcp.ArgumentDescription("File path(s) to document (e.g., './api.js' or './src/')")),
	mcp.WithArgument("doc_type", mcp.ArgumentDescription("Type of documentation (api, user_guide, technical, readme)")),
	mcp.WithArgument("format", mcp.ArgumentDescription("Documentation format (markdown, rst, plain_text)")),
	mcp.WithArgument("include_examples", mcp.ArgumentDescription("Whether to include usage examples (true/false)")),
)

// TestGeneratePrompt defines a prompt for test case generation
var TestGeneratePrompt = mcp.NewPrompt(
	"test_generate",
	mcp.WithPromptDescription("Generate unit tests, integration tests, or test cases for code"),
	mcp.WithArgument("files", mcp.RequiredArgument(), mcp.ArgumentDescription("File path(s) to generate tests for (e.g., './utils.js')")),
	mcp.WithArgument("test_type", mcp.ArgumentDescription("Type of tests (unit, integration, e2e)")),
	mcp.WithArgument("framework", mcp.ArgumentDescription("Testing framework to use")),
	mcp.WithArgument("coverage", mcp.ArgumentDescription("Test coverage level (basic, comprehensive)")),
)

// SecurityAnalysisPrompt defines a prompt for security analysis
var SecurityAnalysisPrompt = mcp.NewPrompt(
	"security_analysis",
	mcp.WithPromptDescription("Analyze code for security vulnerabilities and best practices"),
	mcp.WithArgument("files", mcp.RequiredArgument(), mcp.ArgumentDescription("File path(s) to analyze for security issues (e.g., './auth/' or './api.js')")),
	mcp.WithArgument("scope", mcp.ArgumentDescription("Security analysis scope (input_validation, authentication, authorization, etc.)")),
	mcp.WithArgument("compliance", mcp.ArgumentDescription("Compliance standards to check against (OWASP, NIST, etc.)")),
	mcp.WithArgument("include_fixes", mcp.ArgumentDescription("Whether to include fix suggestions (true/false)")),
)
