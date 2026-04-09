package main

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// GeminiModelsHandler is a handler for the gemini_models tool that uses mcp-go types directly
func (s *GeminiServer) GeminiModelsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling gemini_models request with direct handler")

	// Create error-safe writer
	writer := NewSafeWriter(logger)

	// Write the header
	writer.Write("# Available Gemini Models\n\n")

	writer.Write("This server supports Gemini 3.x models, providing 2 main tools:\n")
	writer.Write("- `gemini_ask`: For general queries, coding problems (default: code review system prompt)\n")
	writer.Write("- `gemini_search`: For search-grounded queries (default: search assistant system prompt)\n\n")

	// Gemini 3.1 Pro
	writer.Write("## Gemini 3.1 Pro\n")
	writer.Write("- **Model ID**: `gemini-3.1-pro-preview` (default for gemini_ask)\n")
	writer.Write("- **Description**: Advanced intelligence, complex problem-solving, and powerful agentic capabilities\n")
	writer.Write("- **Context Window**: 1M tokens\n")
	writer.Write("- **Best for**: Complex reasoning, detailed analysis, comprehensive code review, advanced problem-solving\n")
	writer.Write("- **Thinking Mode**: Yes (uses `thinking_level`: minimal, low, medium, high [default])\n")
	writer.Write("- **Caching**: Automatic implicit caching (no configuration needed)\n")
	writer.Write("- **Temperature**: Default 1.0\n\n")

	// Gemini 3 Flash
	writer.Write("## Gemini 3 Flash\n")
	writer.Write("- **Model ID**: `gemini-3-flash-preview`\n")
	writer.Write("- **Alias**: `gemini-flash-latest`\n")
	writer.Write("- **Description**: Frontier-class performance rivaling larger models at a fraction of the cost\n")
	writer.Write("- **Context Window**: 1M tokens\n")
	writer.Write("- **Best for**: General programming tasks, standard code review, balanced price-performance\n")
	writer.Write("- **Thinking Mode**: Yes\n")
	writer.Write("- **Caching**: Automatic implicit caching (no configuration needed)\n\n")

	// Gemini 3.1 Flash Lite
	writer.Write("## Gemini 3.1 Flash Lite\n")
	writer.Write("- **Model ID**: `gemini-3.1-flash-lite-preview` (default for gemini_search)\n")
	writer.Write("- **Alias**: `gemini-flash-lite-latest`\n")
	writer.Write("- **Description**: Fastest and most cost-efficient model for high-volume, lightweight tasks\n")
	writer.Write("- **Context Window**: 1M tokens\n")
	writer.Write("- **Best for**: Search queries, classification, data extraction, lightweight agentic tasks\n")
	writer.Write("- **Thinking Mode**: Yes (minimal by default for speed/cost, can be increased)\n")
	writer.Write("- **Caching**: Automatic implicit caching (no configuration needed)\n\n")

	// Tool Usage Examples
	writer.Write("## Tool Usage Examples\n\n")

	// gemini_ask examples
	writer.Write("### gemini_ask Examples\n\n")

	writer.Write("**General Problem (non-coding):**\n")
	writer.Write("```json\n")
	writer.Write("{\n")
	writer.Write("  \"query\": \"Explain quantum computing in simple terms\",\n")
	writer.Write("  \"model\": \"gemini-3-flash-preview\",\n")
	writer.Write("  \"systemPrompt\": \"You are an expert science communicator. Explain complex topics clearly for a general audience.\"\n")
	writer.Write("}\n")
	writer.Write("```\n\n")

	writer.Write("**Coding Problem with Files (Gemini 3 Pro):**\n")
	writer.Write("```json\n")
	writer.Write("{\n")
	writer.Write("  \"query\": \"Review this code for security vulnerabilities and performance issues\",\n")
	writer.Write("  \"model\": \"gemini-3.1-pro-preview\",\n")
	writer.Write("  \"github_files\": [\"gemini_ask_handler.go\", \"gemini_server.go\"],\n")
	writer.Write("  \"github_repo\": \"chew-z/GeminiMCP\",\n")
	writer.Write("  \"github_ref\": \"main\",\n")
	writer.Write("  \"enable_thinking\": true,\n")
	writer.Write("  \"thinking_level\": \"high\"\n")
	writer.Write("}\n")
	writer.Write("```\n")
	writer.Write("*Note: Default system prompt optimized for code review will be used*\n\n")

	writer.Write("**Custom System Prompt Override (Gemini 3 Pro):**\n")
	writer.Write("```json\n")
	writer.Write("{\n")
	writer.Write("  \"query\": \"Analyze this code architecture\",\n")
	writer.Write("  \"model\": \"gemini-3.1-pro-preview\",\n")
	writer.Write("  \"systemPrompt\": \"You are a senior software architect. Focus on design patterns, scalability, and maintainability.\",\n")
	writer.Write("  \"github_files\": [\"main.go\"],\n")
	writer.Write("  \"github_repo\": \"chew-z/GeminiMCP\",\n")
	writer.Write("  \"github_ref\": \"main\"\n")
	writer.Write("}\n")
	writer.Write("```\n\n")

	// gemini_search examples
	writer.Write("### gemini_search Examples\n\n")

	// Basic Search example
	var b strings.Builder
	b.WriteString("**Basic Search:**\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"query\": \"What are the latest developments in Go programming language?\",\n")
	b.WriteString("  \"model\": \"gemini-flash-lite-latest\"\n")
	b.WriteString("}\n")
	b.WriteString("```\n\n")
	writer.Write("%s", b.String())

	// Search with Time Filtering example
	var b2 strings.Builder
	b2.WriteString("**Search with Time Filtering:**\n")
	b2.WriteString("```json\n")
	b2.WriteString("{\n")
	b2.WriteString("  \"query\": \"Recent security vulnerabilities in JavaScript frameworks\",\n")
	b2.WriteString("  \"model\": \"gemini-3-flash-preview\",\n")
	b2.WriteString("  \"start_time\": \"2024-01-01T00:00:00Z\",\n")
	b2.WriteString("  \"end_time\": \"2024-12-31T23:59:59Z\"\n")
	b2.WriteString("}\n")
	b2.WriteString("```\n\n")
	writer.Write("%s", b2.String())

	// Search with Thinking Mode example
	var b3 strings.Builder
	b3.WriteString("**Search with Thinking Mode (Gemini 3 Pro):**\n")
	b3.WriteString("```json\n")
	b3.WriteString("{\n")
	b3.WriteString("  \"query\": \"Compare the pros and cons of different cloud deployment strategies\",\n")
	b3.WriteString("  \"model\": \"gemini-3.1-pro-preview\",\n")
	b3.WriteString("  \"enable_thinking\": true,\n")
	b3.WriteString("  \"thinking_level\": \"high\"\n")
	b3.WriteString("}\n")
	b3.WriteString("```\n\n")
	writer.Write("%s", b3.String())

	// System Prompt Details
	writer.Write("## System Prompt Details\n\n")
	writer.Write("**Default System Prompts:**\n")
	writer.Write("- **gemini_ask**: Optimized for thorough code review (senior developer perspective)\n")
	writer.Write("- **gemini_search**: Helpful search assistant for accurate, up-to-date information\n\n")
	writer.Write("**Override via:**\n")
	writer.Write("- `systemPrompt` parameter in requests\n")
	writer.Write("- `GEMINI_SYSTEM_PROMPT` env variable (for gemini_ask)\n")
	writer.Write("- `GEMINI_SEARCH_SYSTEM_PROMPT` env variable (for gemini_search)\n")
	writer.Write("- Command line flags: `--gemini-system-prompt`\n\n")

	// File Attachments
	writer.Write("## File Attachments (gemini_ask only)\n\n")
	writer.Write("Attach files to provide context for your queries. This is particularly useful for code review, debugging, and analysis:\n\n")

	// File attachments JSON example using strings.Builder
	{
		var b strings.Builder
		b.WriteString("```json\n")
		b.WriteString("// Code review with multiple files\n")
		b.WriteString("{\n")
		b.WriteString("  \"query\": \"Review this code for potential issues and suggest improvements\",\n")
		b.WriteString("  \"model\": \"gemini-2.5-pro\",\n")
		b.WriteString("  \"github_files\": [\n")
		b.WriteString("    \"main.go\",\n")
		b.WriteString("    \"config.go\",\n")
		b.WriteString("    \"tools.go\"\n")
		b.WriteString("  ],\n")
		b.WriteString("  \"github_repo\": \"chew-z/GeminiMCP\",\n")
		b.WriteString("  \"github_ref\": \"main\"\n")
		b.WriteString("}\n\n")
		b.WriteString("// Documentation analysis\n")
		b.WriteString("{\n")
		b.WriteString("  \"query\": \"Explain how these components interact and suggest documentation improvements\",\n")
		b.WriteString("  \"model\": \"gemini-3-flash-preview\",\n")
		b.WriteString("  \"github_files\": [\n")
		b.WriteString("    \"README.md\",\n")
		b.WriteString("    \"tools.go\"\n")
		b.WriteString("  ],\n")
		b.WriteString("  \"github_repo\": \"chew-z/GeminiMCP\",\n")
		b.WriteString("  \"github_ref\": \"main\"\n")
		b.WriteString("}\n")
		b.WriteString("```\n\n")
		writer.Write("%s", b.String())
	}

	// Caching
	writer.Write("## Caching (gemini_ask only)\n\n")
	writer.Write("**Implicit Caching (Automatic):**\n")
	writer.Write("- ~90%% token discount for requests with common prefixes\n")
	writer.Write("- Pro: 4096+ tokens minimum\n")
	writer.Write("- Flash: 1024+ tokens minimum\n")
	writer.Write("- Files are placed at the beginning of requests automatically to maximize cache hits\n")
	writer.Write("- No configuration needed — savings are applied automatically\n\n")

	// Thinking Mode
	writer.Write("## Thinking Mode (both tools)\n\n")
	writer.Write("All supported models use the `thinking_level` parameter for controlling reasoning depth:\n\n")
	writer.Write("**Thinking Levels:**\n")
	writer.Write("- `minimal`: Matches 'no thinking' for most queries. The model may still think minimally for complex coding tasks\n")
	writer.Write("- `low`: Minimizes latency and cost. Best for simple instruction following, chat, or high-throughput applications\n")
	writer.Write("- `medium`: Balanced thinking for most tasks\n")
	writer.Write("- `high` (default): Maximizes reasoning depth. The model may take longer but output will be more carefully reasoned\n\n")

	// Thinking mode JSON examples using strings.Builder
	{
		var b strings.Builder
		b.WriteString("```json\n")
		b.WriteString("// Gemini 3 Pro with thinking (default is high)\n")
		b.WriteString("{\n")
		b.WriteString("  \"query\": \"Solve this complex algorithm problem step by step\",\n")
		b.WriteString("  \"model\": \"gemini-3.1-pro-preview\",\n")
		b.WriteString("  \"enable_thinking\": true,\n")
		b.WriteString("  \"thinking_level\": \"high\"\n")
		b.WriteString("}\n\n")
		b.WriteString("// Low thinking level for faster responses\n")
		b.WriteString("{\n")
		b.WriteString("  \"query\": \"Quick code syntax check\",\n")
		b.WriteString("  \"model\": \"gemini-3.1-pro-preview\",\n")
		b.WriteString("  \"enable_thinking\": true,\n")
		b.WriteString("  \"thinking_level\": \"low\"\n")
		b.WriteString("}\n")
		b.WriteString("```\n\n")
		writer.Write("%s", b.String())
	}

	// Time Filtering
	writer.Write("## Time Filtering (gemini_search only)\n\n")
	writer.Write("Filter search results by publication date using RFC3339 format:\n\n")
	writer.Write("- Use `start_time` and `end_time` together (both required)\n")
	writer.Write("- Format: `YYYY-MM-DDTHH:MM:SSZ`\n\n")

	// Advanced Examples
	writer.Write("## Advanced Examples\n\n")
	// Comprehensive code review with thinking and caching (gemini_ask with Gemini 3 Pro)
	{
		var b strings.Builder
		b.Grow(1000) // Pre-allocate reasonable size

		// First JSON example
		b.WriteString("```json\n// Comprehensive code review with thinking (gemini_ask with Gemini 3 Pro)\n")
		b.WriteString("{\n")
		b.WriteString("  \"query\": \"Perform a thorough security and performance review of this codebase\",\n")
		b.WriteString("  \"model\": \"gemini-3.1-pro-preview\",\n")
		b.WriteString("  \"github_files\": [\n")
		b.WriteString("    \"main.go\",\n")
		b.WriteString("    \"gemini_ask_handler.go\",\n")
		b.WriteString("    \"file_handlers.go\"\n")
		b.WriteString("  ],\n")
		b.WriteString("  \"github_repo\": \"chew-z/GeminiMCP\",\n")
		b.WriteString("  \"github_ref\": \"main\",\n")
		b.WriteString("  \"enable_thinking\": true,\n")
		b.WriteString("  \"thinking_level\": \"high\"\n")
		b.WriteString("}\n\n")

		// Second JSON example
		b.WriteString("// Custom system prompt with file context (gemini_ask with Gemini 3 Pro)\n")
		b.WriteString("{\n")
		b.WriteString("  \"query\": \"Suggest architectural improvements for better scalability\",\n")
		b.WriteString("  \"model\": \"gemini-3.1-pro-preview\",\n")
		b.WriteString("  \"systemPrompt\": \"You are a senior software architect. Focus on scalability, maintainability, and best practices.\",\n")
		b.WriteString("  \"github_files\": [\"README.md\"],\n")
		b.WriteString("  \"github_repo\": \"chew-z/GeminiMCP\",\n")
		b.WriteString("  \"github_ref\": \"main\",\n")
		b.WriteString("  \"enable_thinking\": true,\n")
		b.WriteString("  \"thinking_level\": \"high\"\n")
		b.WriteString("}\n```\n")

		writer.Write("%s", b.String())
	}

	// Check for write failures and return error if any occurred
	if writer.Failed() {
		return createErrorResult("Error generating model list"), nil
	}

	// Return the formatted content
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(writer.String()),
		},
	}, nil
}
