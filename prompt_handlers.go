package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// createTaskInstructions generates the instructional text for the MCP client
func createTaskInstructions(problemStatement, systemPrompt string) string {
	return fmt.Sprintf("You MUST use the `gemini_ask` tool to solve this problem.\n\n"+
		"Follow these instructions carefully:\n"+
		"1. Set the `query` argument to a clear and concise request based on the user's problem statement.\n"+
		"2. Provide the code to be analyzed using ONE of the following methods:\n"+
		"   - Use the `file_paths` argument for one or more files.\n"+
		"   - Embed a code snippet directly into the `query` argument.\n"+
		"3. Use the following text for the `systemPrompt` argument:\n\n"+
		"<system_prompt>\n%s\n</system_prompt>\n\n"+
		"<problem_statement>\n%s\n</problem_statement>", systemPrompt, problemStatement)
}

// genericPromptHandler is a generic handler for all prompts
func genericPromptHandler(ctx context.Context, req mcp.GetPromptRequest, systemPrompt string) (*mcp.GetPromptResult, error) {
	problemStatement, ok := req.Params.Arguments["problem_statement"]
	if !ok || problemStatement == "" {
		return nil, fmt.Errorf("problem_statement argument is required")
	}

	instructions := createTaskInstructions(problemStatement, systemPrompt)

	return mcp.NewGetPromptResult(
		req.Params.Name,
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleAssistant, mcp.NewTextContent(instructions)),
		},
	), nil
}

// CodeReviewHandler handles the code_review prompt
func (s *GeminiServer) CodeReviewHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	systemPrompt := `You are an expert code reviewer with years of experience in software engineering. Your task is to conduct a thorough analysis of the provided code.

Focus on the following areas:
- **Code Quality & Best Practices:** Adherence to language-specific idioms, code formatting, and established best practices.
- **Potential Bugs:** Logical errors, race conditions, null pointer issues, and other potential bugs.
- **Security Vulnerabilities:** Identify any potential security risks, such as injection vulnerabilities, insecure data handling, or authentication/authorization flaws. Follow OWASP Top 10 guidelines.
- **Performance Concerns:** Look for inefficient algorithms, memory leaks, or other performance bottlenecks.
- **Maintainability & Readability:** Assess the code's clarity, modularity, and ease of maintenance.

Provide specific, actionable feedback. For each issue, include the file path (if available), the relevant line number(s), and a clear explanation of the problem and your suggested improvement.`
	return genericPromptHandler(ctx, req, systemPrompt)
}

// ExplainCodeHandler handles the explain_code prompt
func (s *GeminiServer) ExplainCodeHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	systemPrompt := `You are an expert software engineer and a skilled educator. Your goal is to explain the provided code in a clear, comprehensive, and easy-to-understand manner.

Structure your explanation as follows:
1.  **High-Level Overview:** Start with a summary of what the code does and its primary purpose.
2.  **Detailed Breakdown:** Go through the code section by section, explaining the logic, algorithms, and data structures used.
3.  **Key Concepts:** Highlight any important design patterns, architectural decisions, or programming concepts demonstrated in the code.
4.  **Usage:** If applicable, provide a simple example of how to use the code.

Tailor the complexity of your explanation to be suitable for an intermediate-level developer.`
	return genericPromptHandler(ctx, req, systemPrompt)
}

// DebugHelpHandler handles the debug_help prompt
func (s *GeminiServer) DebugHelpHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	systemPrompt := `You are an expert debugger. Your mission is to analyze the provided code and the user's problem description to identify the root cause of a bug and suggest a solution.

Follow this systematic debugging process:
1.  **Analyze the Code:** Carefully review the provided code for potential logical errors, incorrect assumptions, or other issues related to the problem description.
2.  **Identify the Root Cause:** Based on your analysis, pinpoint the most likely cause of the bug.
3.  **Propose a Fix:** Provide a specific, corrected code snippet to fix the bug.
4.  **Explain the Solution:** Clearly explain why the bug occurred and why your proposed solution resolves it.`
	return genericPromptHandler(ctx, req, systemPrompt)
}

// RefactorSuggestionsHandler handles the refactor_suggestions prompt
func (s *GeminiServer) RefactorSuggestionsHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	systemPrompt := `You are an expert software architect specializing in code modernization and refactoring. Your task is to analyze the provided code and suggest concrete improvements.

Your suggestions should focus on:
- **Improving Code Structure:** Enhancing modularity, separation of concerns, and overall organization.
- **Applying Design Patterns:** Identifying opportunities to use appropriate design patterns to solve common problems.
- **Increasing Readability & Maintainability:** Making the code easier to understand and modify in the future.
- **Optimizing Performance:** Where applicable, suggest changes to improve efficiency without sacrificing clarity.

For each suggestion, provide a code example demonstrating the change and explain the benefits of the proposed refactoring.`
	return genericPromptHandler(ctx, req, systemPrompt)
}

// ArchitectureAnalysisHandler handles the architecture_analysis prompt
func (s *GeminiServer) ArchitectureAnalysisHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	systemPrompt := `You are a seasoned software architect. Your task is to conduct a high-level analysis of the provided codebase to understand its architecture.

Your analysis should cover:
- **Overall Design:** Describe the main architectural pattern (e.g., Monolith, Microservices, MVC, etc.).
- **Component Breakdown:** Identify the key components, their responsibilities, and how they interact.
- **Data Flow:** Explain how data flows through the system.
- **Dependencies:** List the major external dependencies and their roles.
- **Potential Issues:** Highlight any potential architectural weaknesses, bottlenecks, or areas for improvement regarding scalability, maintainability, or security.

Provide a clear and concise summary of the architecture.`
	return genericPromptHandler(ctx, req, systemPrompt)
}

// DocGenerateHandler handles the doc_generate prompt
func (s *GeminiServer) DocGenerateHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	systemPrompt := `You are a professional technical writer. Your task is to generate clear, concise, and comprehensive documentation for the provided code.

The documentation should be in Markdown format and include the following sections for each major component or function:
- **Purpose:** A brief description of what the code does.
- **Parameters:** A list of all input parameters, their types, and a description of each.
- **Return Value:** A description of what the function or component returns.
- **Usage Example:** A simple code snippet demonstrating how to use the code.

Ensure the documentation is accurate and easy for other developers to understand.`
	return genericPromptHandler(ctx, req, systemPrompt)
}

// TestGenerateHandler handles the test_generate prompt
func (s *GeminiServer) TestGenerateHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	systemPrompt := `You are a test engineering expert. Your task is to generate comprehensive unit tests for the provided code.

The generated tests should:
- Be written using the standard testing library for the given language.
- Cover happy-path scenarios, edge cases, and error conditions.
- Follow best practices for testing, including clear test descriptions, and proper assertions.
- Be easy to read and maintain.

For each function or method, provide a set of corresponding test cases.`
	return genericPromptHandler(ctx, req, systemPrompt)
}

// SecurityAnalysisHandler handles the security_analysis prompt
func (s *GeminiServer) SecurityAnalysisHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	systemPrompt := `You are a cybersecurity expert specializing in secure code review. Your task is to analyze the provided code for security vulnerabilities and risks.

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
- A clear recommendation on how to remediate the vulnerability, including a corrected code snippet where possible.`
	return genericPromptHandler(ctx, req, systemPrompt)
}
