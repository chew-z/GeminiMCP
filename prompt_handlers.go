package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// PromptHandlerFunc defines the signature for prompt handlers
type PromptHandlerFunc func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error)

// CodeReviewHandler handles the code_review prompt
func (s *GeminiServer) CodeReviewHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling code_review prompt")

	// Extract required argument
	code, ok := req.Params.Arguments["code"]
	if !ok || code == "" {
		return &mcp.GetPromptResult{
			Description: "Error: code argument is required",
			Messages:    []mcp.PromptMessage{},
		}, nil
	}

	// Extract optional arguments
	language := extractPromptArgString(req, "language", "auto-detect")
	focus := extractPromptArgString(req, "focus", "general best practices")
	severity := extractPromptArgString(req, "severity", "warning")

	// Build system prompt for code review
	systemPrompt := fmt.Sprintf(`You are an expert code reviewer. Analyze the provided code for:
- Code quality and best practices
- Potential bugs and issues
- Security vulnerabilities
- Performance concerns
- Maintainability and readability

Focus areas: %s
Minimum severity level: %s
Language: %s

Provide specific, actionable feedback with line references where applicable.`, focus, severity, language)

	// Build user prompt
	userPrompt := fmt.Sprintf("Please review this code:\n\n```%s\n%s\n```", language, code)

	return &mcp.GetPromptResult{
		Description: "Code review analysis prompt",
		Messages: []mcp.PromptMessage{
			{
				Role: "system",
				Content: mcp.TextContent{
					Type: "text",
					Text: systemPrompt,
				},
			},
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: userPrompt,
				},
			},
		},
	}, nil
}

// ExplainCodeHandler handles the explain_code prompt
func (s *GeminiServer) ExplainCodeHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling explain_code prompt")

	// Extract required argument
	code, ok := req.Params.Arguments["code"]
	if !ok || code == "" {
		return &mcp.GetPromptResult{
			Description: "Error: code argument is required",
			Messages:    []mcp.PromptMessage{},
		}, nil
	}

	// Extract optional arguments
	audience := extractPromptArgString(req, "audience", "intermediate")
	includeExamples := extractPromptArgString(req, "include_examples", "true")
	focusAreas := extractPromptArgString(req, "focus_areas", "overall functionality")

	// Build system prompt
	systemPrompt := fmt.Sprintf(`You are an expert software engineer and educator. Explain the provided code in a clear, comprehensive manner.

Target audience: %s developers
Focus on: %s
Include examples: %s

Structure your explanation with:
1. Overview of what the code does
2. Step-by-step breakdown of logic
3. Key algorithms or patterns used
4. Important design decisions
5. Usage examples (if requested)

Make the explanation appropriate for the target audience level.`, audience, focusAreas, includeExamples)

	// Build user prompt
	userPrompt := fmt.Sprintf("Please explain how this code works:\n\n```\n%s\n```", code)

	return &mcp.GetPromptResult{
		Description: "Code explanation prompt",
		Messages: []mcp.PromptMessage{
			{
				Role: "system",
				Content: mcp.TextContent{
					Type: "text",
					Text: systemPrompt,
				},
			},
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: userPrompt,
				},
			},
		},
	}, nil
}

// DebugHelpHandler handles the debug_help prompt
func (s *GeminiServer) DebugHelpHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling debug_help prompt")

	// Extract required argument
	code, ok := req.Params.Arguments["code"]
	if !ok || code == "" {
		return &mcp.GetPromptResult{
			Description: "Error: code argument is required",
			Messages:    []mcp.PromptMessage{},
		}, nil
	}

	// Extract optional arguments
	errorMessage := extractPromptArgString(req, "error_message", "")
	expectedBehavior := extractPromptArgString(req, "expected_behavior", "")
	context := extractPromptArgString(req, "context", "")

	// Build system prompt
	systemPrompt := `You are an expert debugging assistant. Help identify and solve the issue in the provided code.

Follow this debugging approach:
1. Analyze the code for potential issues
2. Consider the error message and symptoms
3. Compare expected vs actual behavior
4. Identify root causes
5. Provide specific fix suggestions
6. Explain why the issue occurred

Be thorough and provide step-by-step guidance for resolving the problem.`

	// Build user prompt with available information
	var userPromptParts []string
	userPromptParts = append(userPromptParts, "Please help debug this issue:")
	userPromptParts = append(userPromptParts, fmt.Sprintf("\n**Code:**\n```\n%s\n```", code))

	if errorMessage != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("\n**Error Message:**\n%s", errorMessage))
	}

	if expectedBehavior != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("\n**Expected Behavior:**\n%s", expectedBehavior))
	}

	if context != "" {
		userPromptParts = append(userPromptParts, fmt.Sprintf("\n**Additional Context:**\n%s", context))
	}

	userPrompt := strings.Join(userPromptParts, "\n")

	return &mcp.GetPromptResult{
		Description: "Debug assistance prompt",
		Messages: []mcp.PromptMessage{
			{
				Role: "system",
				Content: mcp.TextContent{
					Type: "text",
					Text: systemPrompt,
				},
			},
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: userPrompt,
				},
			},
		},
	}, nil
}

// RefactorSuggestionsHandler handles the refactor_suggestions prompt
func (s *GeminiServer) RefactorSuggestionsHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling refactor_suggestions prompt")

	// Extract required argument
	code, ok := req.Params.Arguments["code"]
	if !ok || code == "" {
		return &mcp.GetPromptResult{
			Description: "Error: code argument is required",
			Messages:    []mcp.PromptMessage{},
		}, nil
	}

	// Extract optional arguments
	goals := extractPromptArgString(req, "goals", "improve code quality and maintainability")
	constraints := extractPromptArgString(req, "constraints", "maintain existing functionality")
	includeExamples := extractPromptArgString(req, "include_examples", "true")

	// Build system prompt
	systemPrompt := fmt.Sprintf(`You are an expert software architect and refactoring specialist. Analyze the provided code and suggest improvements.

Refactoring goals: %s
Constraints: %s
Include examples: %s

Focus on:
- Code structure and organization
- Design patterns and principles
- Performance optimizations
- Readability and maintainability
- Error handling improvements
- Naming and clarity

Provide prioritized suggestions with clear explanations of benefits and trade-offs.`, goals, constraints, includeExamples)

	// Build user prompt
	userPrompt := fmt.Sprintf("Please analyze this code and suggest refactoring improvements:\n\n```\n%s\n```", code)

	return &mcp.GetPromptResult{
		Description: "Refactoring suggestions prompt",
		Messages: []mcp.PromptMessage{
			{
				Role: "system",
				Content: mcp.TextContent{
					Type: "text",
					Text: systemPrompt,
				},
			},
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: userPrompt,
				},
			},
		},
	}, nil
}

// ArchitectureAnalysisHandler handles the architecture_analysis prompt
func (s *GeminiServer) ArchitectureAnalysisHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling architecture_analysis prompt")

	// Extract required argument
	codeFiles, ok := req.Params.Arguments["code_files"]
	if !ok || codeFiles == "" {
		return &mcp.GetPromptResult{
			Description: "Error: code_files argument is required",
			Messages:    []mcp.PromptMessage{},
		}, nil
	}

	// Extract optional arguments
	scope := extractPromptArgString(req, "scope", "system")
	focus := extractPromptArgString(req, "focus", "overall architecture")
	includeRecommendations := extractPromptArgString(req, "include_recommendations", "true")

	// Build system prompt
	systemPrompt := fmt.Sprintf(`You are an expert software architect. Analyze the provided code structure and architecture.

Analysis scope: %s level
Focus areas: %s
Include recommendations: %s

Examine:
- Overall system design and structure
- Component relationships and dependencies
- Design patterns and architectural principles
- Scalability and performance considerations
- Security architecture
- Maintainability and extensibility
- Potential architectural issues or improvements

Provide a comprehensive architectural assessment with insights and actionable recommendations.`, scope, focus, includeRecommendations)

	// Build user prompt
	userPrompt := fmt.Sprintf("Please analyze the architecture of this code:\n\n%s", codeFiles)

	return &mcp.GetPromptResult{
		Description: "Architecture analysis prompt",
		Messages: []mcp.PromptMessage{
			{
				Role: "system",
				Content: mcp.TextContent{
					Type: "text",
					Text: systemPrompt,
				},
			},
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: userPrompt,
				},
			},
		},
	}, nil
}

// DocGenerateHandler handles the doc_generate prompt
func (s *GeminiServer) DocGenerateHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling doc_generate prompt")

	// Extract required argument
	code, ok := req.Params.Arguments["code"]
	if !ok || code == "" {
		return &mcp.GetPromptResult{
			Description: "Error: code argument is required",
			Messages:    []mcp.PromptMessage{},
		}, nil
	}

	// Extract optional arguments
	docType := extractPromptArgString(req, "doc_type", "technical")
	format := extractPromptArgString(req, "format", "markdown")
	includeExamples := extractPromptArgString(req, "include_examples", "true")

	// Build system prompt
	systemPrompt := fmt.Sprintf(`You are a technical documentation specialist. Generate comprehensive documentation for the provided code.

Documentation type: %s
Format: %s
Include examples: %s

Create documentation that includes:
- Overview and purpose
- Installation/setup instructions (if applicable)
- API reference or usage guide
- Parameters and return values
- Examples and use cases
- Error handling information
- Best practices and tips

Make the documentation clear, accurate, and user-friendly.`, docType, format, includeExamples)

	// Build user prompt
	userPrompt := fmt.Sprintf("Please generate documentation for this code:\n\n```\n%s\n```", code)

	return &mcp.GetPromptResult{
		Description: "Documentation generation prompt",
		Messages: []mcp.PromptMessage{
			{
				Role: "system",
				Content: mcp.TextContent{
					Type: "text",
					Text: systemPrompt,
				},
			},
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: userPrompt,
				},
			},
		},
	}, nil
}

// TestGenerateHandler handles the test_generate prompt
func (s *GeminiServer) TestGenerateHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling test_generate prompt")

	// Extract required argument
	code, ok := req.Params.Arguments["code"]
	if !ok || code == "" {
		return &mcp.GetPromptResult{
			Description: "Error: code argument is required",
			Messages:    []mcp.PromptMessage{},
		}, nil
	}

	// Extract optional arguments
	testType := extractPromptArgString(req, "test_type", "unit")
	framework := extractPromptArgString(req, "framework", "standard library")
	coverage := extractPromptArgString(req, "coverage", "comprehensive")

	// Build system prompt
	systemPrompt := fmt.Sprintf(`You are a test engineering expert. Generate comprehensive test cases for the provided code.

Test type: %s tests
Framework: %s
Coverage level: %s

Generate tests that cover:
- Happy path scenarios
- Edge cases and boundary conditions
- Error handling and invalid inputs
- Performance considerations (if applicable)
- Integration points (if applicable)

Follow testing best practices:
- Clear test names and descriptions
- Proper setup and teardown
- Assertions that verify expected behavior
- Good test data and mocking strategies

Make tests maintainable and easy to understand.`, testType, framework, coverage)

	// Build user prompt
	userPrompt := fmt.Sprintf("Please generate test cases for this code:\n\n```\n%s\n```", code)

	return &mcp.GetPromptResult{
		Description: "Test generation prompt",
		Messages: []mcp.PromptMessage{
			{
				Role: "system",
				Content: mcp.TextContent{
					Type: "text",
					Text: systemPrompt,
				},
			},
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: userPrompt,
				},
			},
		},
	}, nil
}

// SecurityAnalysisHandler handles the security_analysis prompt
func (s *GeminiServer) SecurityAnalysisHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling security_analysis prompt")

	// Extract required argument
	code, ok := req.Params.Arguments["code"]
	if !ok || code == "" {
		return &mcp.GetPromptResult{
			Description: "Error: code argument is required",
			Messages:    []mcp.PromptMessage{},
		}, nil
	}

	// Extract optional arguments
	scope := extractPromptArgString(req, "scope", "general security analysis")
	compliance := extractPromptArgString(req, "compliance", "OWASP guidelines")
	includeFixes := extractPromptArgString(req, "include_fixes", "true")

	// Build system prompt
	systemPrompt := fmt.Sprintf(`You are a cybersecurity expert specializing in secure code review. Analyze the provided code for security vulnerabilities and risks.

Analysis scope: %s
Compliance standards: %s
Include fixes: %s

Focus on identifying:
- Input validation vulnerabilities
- Authentication and authorization issues
- Data handling and privacy concerns
- Injection attacks (SQL, XSS, etc.)
- Insecure cryptographic practices
- Error handling that leaks information
- Access control problems
- Configuration security issues

Provide detailed analysis with:
- Vulnerability descriptions and impact
- Risk severity levels
- Specific remediation steps
- Best practice recommendations

Prioritize findings by risk level and exploitability.`, scope, compliance, includeFixes)

	// Build user prompt
	userPrompt := fmt.Sprintf("Please perform a security analysis of this code:\n\n```\n%s\n```", code)

	return &mcp.GetPromptResult{
		Description: "Security analysis prompt",
		Messages: []mcp.PromptMessage{
			{
				Role: "system",
				Content: mcp.TextContent{
					Type: "text",
					Text: systemPrompt,
				},
			},
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: userPrompt,
				},
			},
		},
	}, nil
}

// Helper function to extract string arguments from prompt requests
func extractPromptArgString(req mcp.GetPromptRequest, key, defaultValue string) string {
	if val, ok := req.Params.Arguments[key]; ok && val != "" {
		return val
	}
	return defaultValue
}
