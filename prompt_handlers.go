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
	return fmt.Sprintf(`You are an AI assistant. Your task is to answer the user's question by generating a call to the 'gemini_search' tool.

Read the user's question below and then create a 'gemini_search' tool call.

**User's Question:**
"%s"

**Instructions for the 'gemini_search' tool:**

*   **'query' parameter (required):** Create a search query from the user's question.
*   **'start_time' and 'end_time' parameters (optional):**
    *   Use these only if the question is about a specific time period (e.g., "this year", "last month", "in 2023").
    *   If you use them, you must provide both 'start_time' and 'end_time'.
    *   The format is "YYYY-MM-DDTHH:MM:SSZ".

**Example:**

If the user's question is: "What were the most popular movies of 2023?"

Your response should be the following tool call:
'gemini_search(query="most popular movies of 2023", start_time="2023-01-01T00:00:00Z", end_time="2023-12-31T23:59:59Z")'

Now, generate the 'gemini_search' tool call for the user's question.`, problemStatement)
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
