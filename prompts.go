package main

import "github.com/mark3labs/mcp-go/mcp"

// Single-argument prompt for all tasks
var problemStatementArgument = mcp.WithArgument("problem_statement", mcp.RequiredArgument(), mcp.ArgumentDescription("A clear and concise description of the programming problem or task."))

// CodeReviewPrompt defines a prompt for code review tasks
var CodeReviewPrompt = mcp.NewPrompt(
	"code_review",
	mcp.WithPromptDescription("Review code for best practices, potential issues, and improvements"),
	problemStatementArgument,
)

// ExplainCodePrompt defines a prompt for explaining code functionality
var ExplainCodePrompt = mcp.NewPrompt(
	"explain_code",
	mcp.WithPromptDescription("Explain how code works in detail, including algorithms and design patterns"),
	problemStatementArgument,
)

// DebugHelpPrompt defines a prompt for debugging assistance
var DebugHelpPrompt = mcp.NewPrompt(
	"debug_help",
	mcp.WithPromptDescription("Help debug issues by analyzing code, error messages, and context"),
	problemStatementArgument,
)

// RefactorSuggestionsPrompt defines a prompt for code refactoring suggestions
var RefactorSuggestionsPrompt = mcp.NewPrompt(
	"refactor_suggestions",
	mcp.WithPromptDescription("Suggest improvements and refactoring opportunities for existing code"),
	problemStatementArgument,
)

// ArchitectureAnalysisPrompt defines a prompt for system architecture analysis
var ArchitectureAnalysisPrompt = mcp.NewPrompt(
	"architecture_analysis",
	mcp.WithPromptDescription("Analyze system architecture, design patterns, and structural decisions"),
	problemStatementArgument,
)

// DocGeneratePrompt defines a prompt for documentation generation
var DocGeneratePrompt = mcp.NewPrompt(
	"doc_generate",
	mcp.WithPromptDescription("Generate comprehensive documentation for code, APIs, or systems"),
	problemStatementArgument,
)

// TestGeneratePrompt defines a prompt for test case generation
var TestGeneratePrompt = mcp.NewPrompt(
	"test_generate",
	mcp.WithPromptDescription("Generate unit tests, integration tests, or test cases for code"),
	problemStatementArgument,
)

// SecurityAnalysisPrompt defines a prompt for security analysis
var SecurityAnalysisPrompt = mcp.NewPrompt(
	"security_analysis",
	mcp.WithPromptDescription("Analyze code for security vulnerabilities and best practices"),
	problemStatementArgument,
)