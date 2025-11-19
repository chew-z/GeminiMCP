package main

import (
	"context"

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

	writer.Write("This server supports Gemini 3 Pro (latest) and Gemini 2.5 models, providing 2 main tools:\n")
	writer.Write("- `gemini_ask`: For general queries, coding problems (default: code review system prompt)\n")
	writer.Write("- `gemini_search`: For search-grounded queries (default: search assistant system prompt)\n\n")

	// Gemini 3 Pro - Latest model
	writer.Write("## Gemini 3 Pro (Latest)\n")
	writer.Write("- **Model ID**: `gemini-3-pro-preview` (default)\n")
	writer.Write("- **Description**: First model in the Gemini 3 series. Best for complex tasks requiring broad world knowledge and advanced reasoning across modalities\n")
	writer.Write("- **Context Window**: 1M tokens\n")
	writer.Write("- **Best for**: Complex reasoning, detailed analysis, comprehensive code review, advanced problem-solving\n")
	writer.Write("- **Thinking Mode**: Yes (uses `thinking_level` parameter: low, high [default]; medium coming soon)\n")
	writer.Write("- **Implicit Caching**: Yes (automatic optimization)\n")
	writer.Write("- **Explicit Caching**: Yes (user-controlled via `use_cache`)\n")
	writer.Write("- **Temperature**: Default 1.0 (recommended to avoid looping issues on complex tasks)\n\n")

	// Gemini 2.5 Pro
	writer.Write("## Gemini 2.5 Pro (Previous Generation)\n")
	writer.Write("- **Model ID**: `gemini-2.5-pro`\n")
	writer.Write("- **Description**: Previous generation model with strong performance\n")
	writer.Write("- **Context Window**: 1M tokens\n")
	writer.Write("- **Best for**: Complex reasoning, detailed analysis, code review\n")
	writer.Write("- **Thinking Mode**: Yes (legacy thinking_budget parameter)\n")
	writer.Write("- **Implicit Caching**: Yes (automatic optimization, 2048+ token minimum)\n")
	writer.Write("- **Explicit Caching**: Yes (user-controlled via `use_cache`)\n\n")

	// Gemini 2.5 Flash
	writer.Write("## Gemini 2.5 Flash\n")
	writer.Write("- **Model ID**: `gemini-flash-latest` (production)\n")
	writer.Write("- **Description**: Balanced price-performance with fast responses\n")
	writer.Write("- **Context Window**: 32K tokens\n")
	writer.Write("- **Best for**: General programming tasks, standard code review\n")
	writer.Write("- **Thinking Mode**: Yes\n")
	writer.Write("- **Implicit Caching**: Yes (automatic optimization, 1024+ token minimum)\n")
	writer.Write("- **Explicit Caching**: Yes (user-controlled via `use_cache`)\n\n")

	// Gemini 2.5 Flash Lite
	writer.Write("## Gemini 2.5 Flash Lite\n")
	writer.Write("- **Model ID**: `gemini-flash-lite-latest`\n")
	writer.Write("- **Description**: Optimized for cost efficiency and low latency\n")
	writer.Write("- **Context Window**: 32K tokens\n")
	writer.Write("- **Best for**: Search queries, lightweight tasks, quick responses\n")
	writer.Write("- **Thinking Mode**: Yes (off by default for speed/cost, can be enabled)\n")
	writer.Write("- **Implicit Caching**: No\n")
	writer.Write("- **Explicit Caching**: No (preview limitation)\n\n")

	// Tool Usage Examples
	writer.Write("## Tool Usage Examples\n\n")

	// gemini_ask examples
	writer.Write("### gemini_ask Examples\n\n")

	writer.Write("**General Problem (non-coding):**\n```json\n{\n  \"query\": \"Explain quantum computing in simple terms\",\n  \"model\": \"gemini-flash-latest\",\n  \"systemPrompt\": \"You are an expert science communicator. Explain complex topics clearly for a general audience.\"\n}\n```\n\n")

	writer.Write("**Coding Problem with Files and Cache (Gemini 3 Pro):**\n```json\n{\n  \"query\": \"Review this code for security vulnerabilities and performance issues\",\n  \"model\": \"gemini-3-pro-preview\",\n  \"file_paths\": [\"/path/to/auth.go\", \"/path/to/database.go\"],\n  \"use_cache\": true,\n  \"cache_ttl\": \"30m\",\n  \"enable_thinking\": true,\n  \"thinking_level\": \"high\"\n}\n```\n*Note: Default system prompt optimized for code review will be used*\n\n")

	writer.Write("**Custom System Prompt Override (Gemini 3 Pro):**\n```json\n{\n  \"query\": \"Analyze this code architecture\",\n  \"model\": \"gemini-3-pro-preview\",\n  \"systemPrompt\": \"You are a senior software architect. Focus on design patterns, scalability, and maintainability.\",\n  \"file_paths\": [\"/path/to/main.go\"]\n}\n```\n\n")

	// gemini_search examples
	writer.Write("### gemini_search Examples\n\n")

	writer.Write("**Basic Search:**\n```json\n{\n  \"query\": \"What are the latest developments in Go programming language?\",\n  \"model\": \"gemini-flash-lite-latest\"\n}\n```\n\n")

	writer.Write("**Search with Time Filtering:**\n```json\n{\n  \"query\": \"Recent security vulnerabilities in JavaScript frameworks\",\n  \"model\": \"gemini-flash-latest\",\n  \"start_time\": \"2024-01-01T00:00:00Z\",\n  \"end_time\": \"2024-12-31T23:59:59Z\"\n}\n```\n\n")

	writer.Write("**Search with Thinking Mode (Gemini 3 Pro):**\n```json\n{\n  \"query\": \"Compare the pros and cons of different cloud deployment strategies\",\n  \"model\": \"gemini-3-pro-preview\",\n  \"enable_thinking\": true,\n  \"thinking_level\": \"high\"\n}\n```\n\n")

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
	writer.Write("```json\n// Code review with multiple files\n{\n  \"query\": \"Review this code for potential issues and suggest improvements\",\n  \"model\": \"gemini-2.5-pro\",\n  \"file_paths\": [\n    \"/path/to/main.go\",\n    \"/path/to/utils.go\",\n    \"/path/to/config.yaml\"\n  ]\n}\n\n// Documentation analysis\n{\n  \"query\": \"Explain how these components interact and suggest documentation improvements\",\n  \"model\": \"gemini-flash-latest\",\n  \"file_paths\": [\n    \"/path/to/README.md\",\n    \"/path/to/api.go\"\n  ]\n}\n```\n\n")

	// Caching
	writer.Write("## Caching (gemini_ask only)\n\n")
	writer.Write("**Implicit Caching (Automatic):**\n")
	writer.Write("- 75%% token discount for requests with common prefixes\n")
	writer.Write("- Pro: 2048+ tokens minimum\n")
	writer.Write("- Flash: 1024+ tokens minimum\n")
	writer.Write("- Keep content at the beginning of requests the same, add variable content at the end\n\n")

	writer.Write("**Explicit Caching (Manual):**\n")
	writer.Write("- Available for Pro and Flash only\n")
	writer.Write("- Use `use_cache: true` parameter\n")
	writer.Write("- Custom TTL with `cache_ttl` (default: 10 minutes)\n\n")
	writer.Write("```json\n// Enable explicit caching\n{\n  \"query\": \"Analyze this codebase structure\",\n  \"model\": \"gemini-flash-latest\",\n  \"file_paths\": [\"/path/to/large/codebase\"],\n  \"use_cache\": true,\n  \"cache_ttl\": \"30m\"\n}\n```\n\n")

	// Thinking Mode
	writer.Write("## Thinking Mode (both tools)\n\n")
	writer.Write("Gemini 3 Pro uses the new `thinking_level` parameter for controlling reasoning depth:\n\n")
	writer.Write("**Gemini 3 Pro Thinking Levels:**\n")
	writer.Write("- `low`: Minimizes latency and cost. Best for simple instruction following, chat, or high-throughput applications\n")
	writer.Write("- `high` (default): Maximizes reasoning depth. The model may take longer for first token, but output will be more carefully reasoned\n")
	writer.Write("- `medium`: Coming soon, not supported at launch\n\n")
	writer.Write("**Important:** Cannot use both `thinking_level` and legacy `thinking_budget` parameter in the same request (returns 400 error).\n\n")
	writer.Write("```json\n// Gemini 3 Pro with thinking (default is high)\n{\n  \"query\": \"Solve this complex algorithm problem step by step\",\n  \"model\": \"gemini-3-pro-preview\",\n  \"enable_thinking\": true,\n  \"thinking_level\": \"high\"\n}\n\n// Low thinking level for faster responses\n{\n  \"query\": \"Quick code syntax check\",\n  \"model\": \"gemini-3-pro-preview\",\n  \"enable_thinking\": true,\n  \"thinking_level\": \"low\"\n}\n```\n\n")

	// Time Filtering
	writer.Write("## Time Filtering (gemini_search only)\n\n")
	writer.Write("Filter search results by publication date using RFC3339 format:\n\n")
	writer.Write("- Use `start_time` and `end_time` together (both required)\n")
	writer.Write("- Format: `YYYY-MM-DDTHH:MM:SSZ`\n\n")

	// Advanced Examples
	writer.Write("## Advanced Examples\n\n")
	writer.Write("```json\n// Comprehensive code review with thinking and caching (gemini_ask with Gemini 3 Pro)\n{\n  \"query\": \"Perform a thorough security and performance review of this codebase\",\n  \"model\": \"gemini-3-pro-preview\",\n  \"file_paths\": [\n    \"/path/to/main.go\",\n    \"/path/to/auth.go\",\n    \"/path/to/database.go\"\n  ],\n  \"enable_thinking\": true,\n  \"thinking_level\": \"high\",\n  \"use_cache\": true,\n  \"cache_ttl\": \"1h\"\n}\n\n// Custom system prompt with file context (gemini_ask with Gemini 3 Pro)\n{\n  \"query\": \"Suggest architectural improvements for better scalability\",\n  \"model\": \"gemini-3-pro-preview\",\n  \"systemPrompt\": \"You are a senior software architect. Focus on scalability, maintainability, and best practices.\",\n  \"file_paths\": [\"/path/to/architecture/overview.md\"],\n  \"enable_thinking\": true,\n  \"thinking_level\": \"high\"\n}\n```\n")

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