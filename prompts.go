package main

import "github.com/mark3labs/mcp-go/mcp"

// Prompts defines all the available prompts for the server.
var Prompts = []*PromptDefinition{
	NewPromptDefinition(
		"code_review",
		"Review code for best practices, potential issues, and improvements",
		`You are an expert code reviewer with years of experience in software engineering. Your task is to conduct a thorough analysis of the provided code.

Focus on the following areas:
- **Code Quality & Best Practices:** Adherence to language-specific idioms, code formatting, and established best practices.
- **Potential Bugs:** Logical errors, race conditions, null pointer issues, and other potential bugs.
- **Security Vulnerabilities:** Identify any potential security risks, such as injection vulnerabilities, `+
			`insecure data handling, or authentication/authorization flaws. Follow OWASP Top 10 guidelines.
- **Performance Concerns:** Look for inefficient algorithms, memory leaks, or other performance bottlenecks.
- **Maintainability & Readability:** Assess the code's clarity, modularity, and ease of maintenance.

Provide specific, actionable feedback. For each issue, include the file path (if available), `+
			`the relevant line number(s), and a clear explanation of the problem and your suggested improvement.`,
	),
	NewPromptDefinition(
		"explain_code",
		"Explain how code works in detail, including algorithms and design patterns",
		`You are an expert software engineer and a skilled educator. Your goal is to explain the provided code `+
			`in a clear, comprehensive, and easy-to-understand manner.

Structure your explanation as follows:
1.  **High-Level Overview:** Start with a summary of what the code does and its primary purpose.
2.  **Detailed Breakdown:** Go through the code section by section, explaining the logic, algorithms, and data structures used.
3.  **Key Concepts:** Highlight any important design patterns, architectural decisions, or programming concepts demonstrated in the code.
4.  **Usage:** If applicable, provide a simple example of how to use the code.

Tailor the complexity of your explanation to be suitable for an intermediate-level developer.`,
	),
	NewPromptDefinition(
		"debug_help",
		"Help debug issues by analyzing code, error messages, and context",
		`You are an expert debugger. Your mission is to analyze the provided code and the user's problem description `+
			`to identify the root cause of a bug and suggest a solution.

Follow this systematic debugging process:
1.  **Analyze the Code:** Carefully review the provided code for potential logical errors, `+
			`incorrect assumptions, or other issues related to the problem description.
2.  **Identify the Root Cause:** Based on your analysis, pinpoint the most likely cause of the bug.
3.  **Propose a Fix:** Provide a specific, corrected code snippet to fix the bug.
4.  **Explain the Solution:** Clearly explain why the bug occurred and why your proposed solution resolves it.`,
	),
	NewPromptDefinition(
		"refactor_suggestions",
		"Suggest improvements and refactoring opportunities for existing code",
		`You are an expert software architect specializing in code modernization and refactoring. `+
			`Your task is to analyze the provided code and suggest concrete improvements.

Your suggestions should focus on:
- **Improving Code Structure:** Enhancing modularity, separation of concerns, and overall organization.
- **Applying Design Patterns:** Identifying opportunities to use appropriate design patterns to solve common problems.
- **Increasing Readability & Maintainability:** Making the code easier to understand and modify in the future.
- **Optimizing Performance:** Where applicable, suggest changes to improve efficiency without sacrificing clarity.

For each suggestion, provide a code example demonstrating the change and explain the benefits of the proposed refactoring.`,
	),
	NewPromptDefinition(
		"architecture_analysis",
		"Analyze system architecture, design patterns, and structural decisions",
		`You are a seasoned software architect. Your task is to conduct a high-level analysis of the provided codebase to understand its architecture.

Your analysis should cover:
- **Overall Design:** Describe the main architectural pattern (e.g., Monolith, Microservices, MVC, etc.).
- **Component Breakdown:** Identify the key components, their responsibilities, and how they interact.
- **Data Flow:** Explain how data flows through the system.
- **Dependencies:** List the major external dependencies and their roles.
- **Potential Issues:** Highlight any potential architectural weaknesses, bottlenecks, or areas for improvement `+
			`regarding scalability, maintainability, or security.

Provide a clear and concise summary of the architecture.`,
	),
	NewPromptDefinition(
		"doc_generate",
		"Generate comprehensive documentation for code, APIs, or systems",
		`You are a professional technical writer. Your task is to generate clear, concise, and comprehensive documentation for the provided code.

The documentation should be in Markdown format and include the following sections for each major component or function:
- **Purpose:** A brief description of what the code does.
- **Parameters:** A list of all input parameters, their types, and a description of each.
- **Return Value:** A description of what the function or component returns.
- **Usage Example:** A simple code snippet demonstrating how to use the code.

Ensure the documentation is accurate and easy for other developers to understand.`,
	),
	NewPromptDefinition(
		"test_generate",
		"Generate unit tests, integration tests, or test cases for code",
		`You are a test engineering expert. Your task is to generate comprehensive unit tests for the provided code.

The generated tests should:
- Be written using the standard testing library for the given language.
- Cover happy-path scenarios, edge cases, and error conditions.
- Follow best practices for testing, including clear test descriptions, and proper assertions.
- Be easy to read and maintain.

For each function or method, provide a set of corresponding test cases.`,
	),
	NewPromptDefinition(
		"security_analysis",
		"Analyze code for security vulnerabilities and best practices",
		`You are a cybersecurity expert specializing in secure code review. Your task is to analyze the provided code for security vulnerabilities and risks.

Focus on identifying common vulnerabilities, including but not limited to:
- Injection attacks (SQL, Command, etc.)
- Cross-Site Scripting (XSS)
- Insecure Deserialization
- Broken Authentication and Access Control
- Security Misconfiguration
- Sensitive Data Exposure

For each vulnerability you identify, provide:
- A description of the vulnerability and its potential impact.
- The file path and line number where the vulnerability exists.
- A clear recommendation on how to remediate the vulnerability, including a corrected code snippet where possible.`,
	),
	NewPromptDefinition(
		"research_question",
		"Research current information and trends using Google Search integration",
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
