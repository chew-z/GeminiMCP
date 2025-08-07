package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

// createSearchInstructions generates instructions for gemini_search tool
func createSearchInstructions(problemStatement string) string {
	return fmt.Sprintf(`Use the 'gemini_search' tool for this research task.

Parameter selection guide:
1. query (always required): Create a focused search query from the problem statement
2. start_time + end_time (conditional): Set both when problem implies time-sensitive information
   - Format: "YYYY-MM-DDTHH:MM:SSZ"
3. Other parameters: Use defaults unless specifically needed

Example:
INPUT: "Who won Wimbledon this year?"
OUTPUT: gemini_search(
  query="Wimbledon winner 2025",
  start_time="2025-01-01T00:00:00Z",
  end_time="2025-12-31T23:59:59Z"
)

Problem statement:
%s`, problemStatement)
}

// promptHandler is the generic handler for all prompts
func (s *GeminiServer) promptHandler(p *PromptDefinition) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		problemStatement, ok := req.Params.Arguments["problem_statement"]
		if !ok || problemStatement == "" {
			return nil, fmt.Errorf("problem_statement argument is required")
		}

		var instructions string
		if p.Name == "research_question" {
			instructions = createSearchInstructions(problemStatement)
		} else {
			systemPrompt := p.SystemPrompt.GetSystemPrompt()
			instructions = createTaskInstructions(problemStatement, systemPrompt)
		}

		return mcp.NewGetPromptResult(
			req.Params.Name,
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.RoleAssistant, mcp.NewTextContent(instructions)),
			},
		), nil
	}
}
