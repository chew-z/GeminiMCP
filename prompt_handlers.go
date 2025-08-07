package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// PromptHandlerFunc defines the signature for prompt handlers
type PromptHandlerFunc func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error)

// PromptTemplate represents the structured output of a prompt handler
type PromptTemplate struct {
	SystemPrompt       string   `json:"system_prompt"`
	UserPromptTemplate string   `json:"user_prompt_template"`
	FilePaths          []string `json:"file_paths"`
}

type PromptBuilder func(req mcp.GetPromptRequest, language string) (string, string)

func (s *GeminiServer) handlePrompt(
	ctx context.Context,
	req mcp.GetPromptRequest,
	description string,
	builder PromptBuilder,
) (*mcp.GetPromptResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info(fmt.Sprintf("Handling prompt: %s", description))

	filesArg, ok := req.Params.Arguments["files"]
	if !ok || filesArg == "" {
		return nil, fmt.Errorf("files argument is required")
	}

	filePaths := parseFilePaths(filesArg)
	expandedPaths, err := expandFilePaths(filePaths)
	if err != nil {
		return nil, fmt.Errorf("failed to expand file paths: %w", err)
	}

	language := s.config.ProjectLanguage
	if lang, ok := req.Params.Arguments["language"]; ok && lang != "" {
		language = lang
	}

	systemPrompt, userPromptTemplate := builder(req, language)

	template := PromptTemplate{
		SystemPrompt:       systemPrompt,
		UserPromptTemplate: userPromptTemplate,
		FilePaths:          expandedPaths,
	}

	jsonResult, err := json.Marshal(template)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal prompt template: %w", err)
	}

	return mcp.NewGetPromptResult(
		description,
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleAssistant, mcp.NewTextContent(string(jsonResult))),
		},
	), nil
}

// CodeReviewHandler handles the code_review prompt
func (s *GeminiServer) CodeReviewHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	result, err := s.handlePrompt(ctx, req, "Code review analysis prompt",
		func(req mcp.GetPromptRequest, language string) (string, string) {
			focus := extractPromptArgWithConfig(req, "focus", s.config.PromptDefaultFocus, "general best practices")
			severity := extractPromptArgWithConfig(req, "severity", s.config.PromptDefaultSeverity, "warning")

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

			userPromptTemplate := "Please review this code:\n\n```%s\n{{file_content}}\n```"
			return systemPrompt, fmt.Sprintf(userPromptTemplate, language)
		})

	if err != nil {
		return createPromptErrorResult(fmt.Sprintf("Error in code_review prompt: %v", err)), nil
	}
	return result, nil
}

// ExplainCodeHandler handles the explain_code prompt
func (s *GeminiServer) ExplainCodeHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	result, err := s.handlePrompt(ctx, req, "Code explanation prompt",
		func(req mcp.GetPromptRequest, language string) (string, string) {
			audience := extractPromptArgWithConfig(req, "audience", s.config.PromptDefaultAudience, "intermediate")
			includeExamples := extractPromptArgString(req, "include_examples", "true")
			focusAreas := extractPromptArgString(req, "focus_areas", "overall functionality")

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

			userPromptTemplate := "Please explain how this code works:\n\n```%s\n{{file_content}}\n```"
			return systemPrompt, fmt.Sprintf(userPromptTemplate, language)
		})

	if err != nil {
		return createPromptErrorResult(fmt.Sprintf("Error in explain_code prompt: %v", err)), nil
	}
	return result, nil
}

// DebugHelpHandler handles the debug_help prompt
func (s *GeminiServer) DebugHelpHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	result, err := s.handlePrompt(ctx, req, "Debug assistance prompt",
		func(req mcp.GetPromptRequest, language string) (string, string) {
			errorMessage := extractPromptArgString(req, "error_message", "")
			expectedBehavior := extractPromptArgString(req, "expected_behavior", "")
			context := extractPromptArgString(req, "context", "")

			systemPrompt := `You are an expert debugging assistant. Help identify and solve the issue in the provided code.

Follow this debugging approach:
1. Analyze the code for potential issues
2. Consider the error message and symptoms
3. Compare expected vs actual behavior
4. Identify root causes
5. Provide specific fix suggestions
6. Explain why the issue occurred

Be thorough and provide step-by-step guidance for resolving the problem.`

			var userPromptParts []string
			userPromptParts = append(userPromptParts, "Please help debug this issue:")
			userPromptParts = append(userPromptParts, fmt.Sprintf("\n**Code:**\n```%s\n{{file_content}}\n```", language))

			if errorMessage != "" {
				userPromptParts = append(userPromptParts, fmt.Sprintf("\n**Error Message:**\n%s", errorMessage))
			}

			if expectedBehavior != "" {
				userPromptParts = append(userPromptParts, fmt.Sprintf("\n**Expected Behavior:**\n%s", expectedBehavior))
			}

			if context != "" {
				userPromptParts = append(userPromptParts, fmt.Sprintf("\n**Additional Context:**\n%s", context))
			}

			userPromptTemplate := strings.Join(userPromptParts, "\n")
			return systemPrompt, userPromptTemplate
		})

	if err != nil {
		return createPromptErrorResult(fmt.Sprintf("Error in debug_help prompt: %v", err)), nil
	}
	return result, nil
}

// RefactorSuggestionsHandler handles the refactor_suggestions prompt
func (s *GeminiServer) RefactorSuggestionsHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	result, err := s.handlePrompt(ctx, req, "Refactoring suggestions prompt",
		func(req mcp.GetPromptRequest, language string) (string, string) {
			goals := extractPromptArgString(req, "goals", "improve code quality and maintainability")
			constraints := extractPromptArgString(req, "constraints", "maintain existing functionality")
			includeExamples := extractPromptArgString(req, "include_examples", "true")

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

			userPromptTemplate := "Please analyze this code and suggest refactoring improvements:\n\n```%s\n{{file_content}}\n```"
			return systemPrompt, fmt.Sprintf(userPromptTemplate, language)
		})

	if err != nil {
		return createPromptErrorResult(fmt.Sprintf("Error in refactor_suggestions prompt: %v", err)), nil
	}
	return result, nil
}

// ArchitectureAnalysisHandler handles the architecture_analysis prompt
func (s *GeminiServer) ArchitectureAnalysisHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	result, err := s.handlePrompt(ctx, req, "Architecture analysis prompt",
		func(req mcp.GetPromptRequest, language string) (string, string) {
			scope := extractPromptArgString(req, "scope", "system")
			focus := extractPromptArgString(req, "focus", "overall architecture")
			includeRecommendations := extractPromptArgString(req, "include_recommendations", "true")

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

			userPromptTemplate := "Please analyze the architecture of this code:\n\n```%s\n{{file_content}}\n```"
			return systemPrompt, fmt.Sprintf(userPromptTemplate, language)
		})

	if err != nil {
		return createPromptErrorResult(fmt.Sprintf("Error in architecture_analysis prompt: %v", err)), nil
	}
	return result, nil
}

// DocGenerateHandler handles the doc_generate prompt
func (s *GeminiServer) DocGenerateHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	result, err := s.handlePrompt(ctx, req, "Documentation generation prompt",
		func(req mcp.GetPromptRequest, language string) (string, string) {
			docType := extractPromptArgString(req, "doc_type", "technical")
			format := extractPromptArgWithConfig(req, "format", s.config.PromptDefaultDocFormat, "markdown")
			includeExamples := extractPromptArgString(req, "include_examples", "true")

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

			userPromptTemplate := "Please generate documentation for this code:\n\n`%s\n{{file_content}}`"
			return systemPrompt, fmt.Sprintf(userPromptTemplate, language)
		})

	if err != nil {
		return createPromptErrorResult(fmt.Sprintf("Error in doc_generate prompt: %v", err)), nil
	}
	return result, nil
}

// TestGenerateHandler handles the test_generate prompt
func (s *GeminiServer) TestGenerateHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	result, err := s.handlePrompt(ctx, req, "Test generation prompt",
		func(req mcp.GetPromptRequest, language string) (string, string) {
			testType := extractPromptArgString(req, "test_type", "unit")
			framework := extractPromptArgWithConfig(req, "framework", s.config.PromptDefaultFramework, "standard library")
			coverage := extractPromptArgWithConfig(req, "coverage", s.config.PromptDefaultCoverage, "comprehensive")

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

			userPromptTemplate := "Please generate test cases for this code:\n\n```%s\n{{file_content}}\n```"
			return systemPrompt, fmt.Sprintf(userPromptTemplate, language)
		})

	if err != nil {
		return createPromptErrorResult(fmt.Sprintf("Error in test_generate prompt: %v", err)), nil
	}
	return result, nil
}

// SecurityAnalysisHandler handles the security_analysis prompt
func (s *GeminiServer) SecurityAnalysisHandler(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	result, err := s.handlePrompt(ctx, req, "Security analysis prompt",
		func(req mcp.GetPromptRequest, language string) (string, string) {
			scope := extractPromptArgString(req, "scope", "general security analysis")
			compliance := extractPromptArgWithConfig(req, "compliance", s.config.PromptDefaultCompliance, "OWASP guidelines")
			includeFixes := extractPromptArgString(req, "include_fixes", "true")

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

			userPromptTemplate := "Please perform a security analysis of this code:\n\n```%s\n{{file_content}}\n```"
			return systemPrompt, fmt.Sprintf(userPromptTemplate, language)
		})

	if err != nil {
		return createPromptErrorResult(fmt.Sprintf("Error in security_analysis prompt: %v", err)), nil
	}
	return result, nil
}

// Helper function to extract string arguments from prompt requests
func extractPromptArgString(req mcp.GetPromptRequest, key, defaultValue string) string {
	if val, ok := req.Params.Arguments[key]; ok && val != "" {
		return val
	}
	return defaultValue
}

// extractPromptArgWithConfig extracts argument with config-based defaults
func extractPromptArgWithConfig(req mcp.GetPromptRequest, key string, configDefault string, fallback string) string {
	// First check if argument is explicitly provided
	if val, ok := req.Params.Arguments[key]; ok && val != "" {
		return val
	}

	// Use config default if available
	if configDefault != "" {
		return configDefault
	}

	// Fall back to hardcoded default
	return fallback
}

// createPromptErrorResult creates a prompt result with an error message
func createPromptErrorResult(errorMsg string) *mcp.GetPromptResult {
	return mcp.NewGetPromptResult(
		"Error processing prompt",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleAssistant, mcp.NewTextContent(errorMsg)),
		},
	)
}
