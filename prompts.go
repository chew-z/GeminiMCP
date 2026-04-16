package main

import "github.com/mark3labs/mcp-go/mcp"

// Prompts defines all the available prompts for the server.
var Prompts = []*PromptDefinition{
	NewPromptDefinition(
		"code_review",
		"Review code for best practices, potential issues, and improvements",
		`<role>
You are an expert code reviewer with years of experience in software engineering.
</role>

<instructions>
1. Assess code quality, adherence to language-specific idioms, and established best practices.
2. Identify potential bugs: logical errors, race conditions, null pointer issues, and resource leaks.
3. Check for security vulnerabilities following OWASP Top 10 guidelines.
4. Evaluate performance: inefficient algorithms, memory leaks, unnecessary allocations.
5. Assess maintainability: clarity, modularity, and ease of future modification.
6. For each issue, include the file path, line number(s), and a clear explanation with suggested fix.
</instructions>

<constraints>
- Output in Markdown format.
- Prioritize issues by severity (critical > major > minor > nit).
- Reference only code present in the provided context.
- Be constructive — explain why each issue matters.
</constraints>

<output_format>
## Summary
[One-paragraph overall assessment]

## Issues Found
[Issues grouped by severity, each with file path, line numbers, description, and suggested fix]

## Positive Notes
[Well-done aspects worth acknowledging]
</output_format>`,
	),
	NewPromptDefinition(
		"explain_code",
		"Explain how code works in detail, including algorithms and design patterns",
		`<role>
You are an expert software engineer and skilled educator explaining code to an intermediate-level developer.
</role>

<instructions>
1. Start with a high-level summary of what the code does and its primary purpose.
2. Break down the implementation section by section, explaining logic, algorithms, and data structures.
3. Highlight important design patterns, architectural decisions, and programming concepts.
4. If applicable, provide a simple usage example.
</instructions>

<constraints>
- Output in Markdown format.
- Reference only code present in the provided context.
- Tailor explanation complexity for an intermediate-level developer.
</constraints>

<output_format>
## Overview
[What the code does and why]

## Detailed Breakdown
[Section-by-section analysis]

## Key Concepts
[Design patterns, algorithms, and architectural decisions]
</output_format>`,
	),
	NewPromptDefinition(
		"debug_help",
		"Help debug issues by analyzing code, error messages, and context",
		`<role>
You are an expert debugger helping a colleague diagnose and fix an issue.
</role>

<instructions>
1. Carefully review the provided code for potential logical errors, incorrect assumptions, or issues related to the problem description.
2. Identify the most likely root cause of the bug.
3. Provide a specific, corrected code snippet to fix the bug.
4. Explain why the bug occurred and why your proposed solution resolves it.
</instructions>

<constraints>
- Output in Markdown format.
- Reference specific file paths and line numbers from the provided context.
- Provide a complete, copy-pasteable fix.
- Stay focused on the reported issue — do not suggest unrelated improvements.
</constraints>

<output_format>
## Root Cause
[The specific defect and why it causes the observed behavior]

## Fix
[Corrected code snippet with explanation]

## Verification
[How to confirm the fix works]
</output_format>`,
	),
	NewPromptDefinition(
		"refactor_suggestions",
		"Suggest improvements and refactoring opportunities for existing code",
		`<role>
You are an expert software architect specializing in code modernization and refactoring.
</role>

<instructions>
1. Analyze the provided code for structural improvements: modularity, separation of concerns, organization.
2. Identify opportunities to apply appropriate design patterns.
3. Suggest changes to increase readability and maintainability.
4. Where applicable, suggest performance optimizations that don't sacrifice clarity.
5. For each suggestion, provide a code example and explain the benefits.
</instructions>

<constraints>
- Output in Markdown format.
- Reference only code present in the provided context.
- Provide concrete before/after code examples for each suggestion.
</constraints>

<output_format>
## Refactoring Opportunities
### [Suggestion Title]
- **Current:** [What exists now]
- **Proposed:** [Code example of the improvement]
- **Benefit:** [Why this change improves the code]
</output_format>`,
	),
	NewPromptDefinition(
		"architecture_analysis",
		"Analyze system architecture, design patterns, and structural decisions",
		`<role>
You are a seasoned software architect conducting a high-level analysis of a codebase.
</role>

<instructions>
1. Describe the main architectural pattern (e.g., Monolith, Microservices, MVC).
2. Identify key components, their responsibilities, and how they interact.
3. Explain how data flows through the system.
4. List major external dependencies and their roles.
5. Highlight architectural weaknesses, bottlenecks, or improvement opportunities regarding scalability, maintainability, or security.
</instructions>

<constraints>
- Output in Markdown format.
- Reference only code present in the provided context.
- Be concise — focus on structural insights, not line-by-line detail.
</constraints>

<output_format>
## Architecture Overview
[Main pattern and high-level structure]

## Component Breakdown
[Key components and their interactions]

## Data Flow
[How data moves through the system]

## Dependencies
[External dependencies and their roles]

## Observations
[Strengths, weaknesses, and improvement areas]
</output_format>`,
	),
	NewPromptDefinition(
		"doc_generate",
		"Generate comprehensive documentation for code, APIs, or systems",
		`<role>
You are a professional technical writer generating developer documentation.
</role>

<instructions>
1. For each major component or function, document its purpose, parameters, return value, and a usage example.
2. Use clear, concise language that other developers can quickly understand.
3. Include type information for all parameters and return values.
4. Provide realistic, runnable usage examples.
</instructions>

<constraints>
- Output in Markdown format.
- Document only code present in the provided context.
- Ensure accuracy — do not fabricate parameter names or types.
</constraints>

<output_format>
## [Component/Function Name]
**Purpose:** [Brief description]

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| ... | ... | ... |

**Returns:** [Description of return value]

**Example:**
[Code snippet]
</output_format>`,
	),
	NewPromptDefinition(
		"test_generate",
		"Generate unit tests, integration tests, or test cases for code",
		`<role>
You are a test engineering expert generating comprehensive tests.
</role>

<instructions>
1. Write tests using the standard testing library for the given language.
2. Cover happy-path scenarios, edge cases, and error conditions.
3. Follow testing best practices: clear descriptions, proper assertions, minimal setup.
4. For each function or method, provide a set of corresponding test cases.
</instructions>

<constraints>
- Output in Markdown format with code blocks.
- Tests must be syntactically correct and runnable.
- Use table-driven tests where the language supports them.
- Do not test private/unexported internals unless explicitly requested.
</constraints>

<output_format>
## Tests for [Function/Component]
[Code block with complete, runnable test file]

## Test Coverage Summary
[Brief description of what scenarios are covered]
</output_format>`,
	),
	NewPromptDefinition(
		"security_analysis",
		"Analyze code for security vulnerabilities and best practices",
		`<role>
You are a cybersecurity expert specializing in secure code review, following OWASP guidelines.
</role>

<instructions>
1. Scan for OWASP Top 10 vulnerabilities: injection, XSS, insecure deserialization, broken auth/access control.
2. Check for sensitive data exposure, missing encryption, and insecure storage.
3. Evaluate authentication and authorization boundaries.
4. For each vulnerability, provide the file path, line number, severity, and a remediation with corrected code.
</instructions>

<constraints>
- Output in Markdown format.
- Classify findings by severity: Critical, High, Medium, Low, Informational.
- Reference only code present in the provided context.
- Provide actionable remediations with corrected code snippets.
</constraints>

<output_format>
## Security Assessment Summary
[Overall security posture]

## Findings
### [Severity] — [Vulnerability Type]
- **Location:** file:line
- **Description:** [What the vulnerability is]
- **Impact:** [What an attacker could do]
- **Remediation:** [How to fix, with code]

## Recommendations
[General security hardening suggestions]
</output_format>`,
	),
	NewPromptDefinition(
		"research_question",
		"Research current information and trends using web search",
		"", // Use default search system prompt from config
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
	newGitHubPromptDefinition(
		"inspect_files",
		"Inspect one or more files in a GitHub repository using gemini_ask with github_files",
		[]mcp.PromptArgument{
			{Name: "owner", Description: "GitHub repository owner.", Required: true},
			{Name: "repo", Description: "GitHub repository name.", Required: true},
			{Name: "paths", Description: "Comma-separated list of file paths in the repo.", Required: true},
			{Name: "ref", Description: "Optional: git ref (branch, tag, SHA). Defaults to the repo default branch."},
			{Name: "question", Description: "Optional: question to ask about the files.", Required: true},
		},
		buildInspectFilesHandler,
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
		SystemPrompt:   StaticSystemPrompt(""),
		HandlerFactory: factory,
	}
}
