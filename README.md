# Gemini MCP Server

MCP (Model Control Protocol) server integrating with Google's Gemini API.

> **Important**: This server exclusively supports **Gemini 2.5 family models** for optimal thinking mode and implicit caching capabilities.

## Key Advantages

- **Single Self-Contained Binary**: Written in Go, the project compiles to a single binary with no dependencies, eliminating package manager issues and preventing unexpected changes without user knowledge
- **Dynamic Model Access**: Automatically fetches the latest available Gemini models at startup
- **Advanced Context Handling**: Efficient caching system with TTL control for repeated queries
- **Enhanced File Handling**: Seamless file integration with intelligent MIME detection
- **Production Reliability**: Robust error handling, automatic retries, and graceful degradation
- **Comprehensive Capabilities**: Full support for code analysis, general queries, and search with grounding

## Installation and Configuration

### Prerequisites

- Google Gemini API key

### Building from Source

```bash
## Clone and build
git clone https://github.com/chew-z/GeminiMCP
cd GeminiMCP
go build -o mcp-gemini

## Start server with environment variables
export GEMINI_API_KEY=your_api_key
export GEMINI_MODEL=gemini-2.5-pro
./bin/mcp-gemini # Assuming build output is in ./bin
```

### Client Configuration

Add this server to any MCP-compatible client like Claude Desktop by adding to your client's configuration:

```json
{
    "gemini": {
        "command": "/Users/<user>/Path/to/bin/mcp-gemini",
        "env": {
            "GEMINI_API_KEY": "YOUR_GEMINI_API_KEY",
            "GEMINI_MODEL": "gemini-2.5-pro",
            "GEMINI_SEARCH_MODEL": "gemini-2.5-flash-lite",
            "GEMINI_SYSTEM_PROMPT": "You are a senior developer. Your job is to do a thorough code review of this code...",
            "GEMINI_SEARCH_SYSTEM_PROMPT": "You are a search assistant. Your job is to find the most relevant information about this topic..."
        }
    }
}
```

**Important Notes:**

- **Environment Variables**: For Claude Desktop app all configuration variables must be included in the MCP configuration JSON shown above (in the `env` section), not as system environment variables or in .env files. Variables set outside the config JSON will not take effect for the client application.

- **Claude Desktop Config Location**:

    - On macOS: `~/Library/Application\ Support/Claude/claude_desktop_config.json`
    - On Windows: `%APPDATA%\Claude\claude_desktop_config.json`

- **Configuration Help**: If you encounter any issues configuring the Claude desktop app, refer to the [MCP Quickstart Guide](https://modelcontextprotocol.io/quickstart/user) for additional assistance.

## Using this MCP server from Claude Desktop app

You can use Gemini tools directly from an LLM console by creating prompt examples that invoke the tools. Here are some example prompts for different use cases:

### Listing Available Models

Say to your LLM:

> _Please use the gemini_models tool to show me the list of available Gemini models._

The LLM will invoke the **`gemini_models`** tool and return the list of available models, organized by preference and capability. The output prioritizes recommended models for specific tasks, then organizes remaining models by version (newest to oldest).

### Code Analysis with **`gemini_ask`**

Say to your LLM:

> _Use the **`gemini_ask`** tool to analyze this Go code for potential concurrency issues:_
>
> ```
> func processItems(items []string) {
>     var wg sync.WaitGroup
>     results := make([]string, len(items))
>
>     for i, item := range items {
>         wg.Add(1)
>         go func(i int, item string) {
>             results[i] = processItem(item)
>             wg.Done()
>         }(i, item)
>     }
>
>     wg.Wait()
>     return results
> }
> ```
>
> _Please use a system prompt that focuses on code review and performance optimization._

### Creative Writing with **`gemini_ask`**

Say to your LLM:

> _Use the **`gemini_ask`** tool to create a short story about a space explorer discovering a new planet. Set a custom system prompt that encourages creative, descriptive writing with vivid imagery._

### Factual Research with **`gemini_search`**

Say to your LLM:

> _Use the **`gemini_search`** tool to find the latest information about advancements in fusion energy research from the past year. Set the start_time to one year ago and end_time to today. Include sources in your response._

### Complex Reasoning with Thinking Mode

Say to your LLM:

> _Use the `gemini_ask` tool with a thinking-capable model to solve this algorithmic problem:_
>
> _"Given an array of integers, find the longest consecutive sequence of integers. For example, given [100, 4, 200, 1, 3, 2], the longest consecutive sequence is [1, 2, 3, 4], so return 4."_
>
> _Enable thinking mode with a high budget level so I can see the detailed step-by-step reasoning process._

This will show both the final answer and the model's comprehensive reasoning process with maximum detail.

### Simple Project Analysis with Caching

Say to your LLM:

> _Please use a caching-enabled **Gemini model** to analyze our project files. Include the main.go, config.go and models.go files and ask Gemini a series of questions about our project architecture and how it could be improved. Use appropriate system prompts for each question._

With this simple prompt, the LLM will:

- Select a caching-compatible model (with -001 suffix)
- Include the specified project files
- Enable caching automatically
- Ask multiple questions while maintaining context
- Customize system prompts for each question type

This approach makes it easy to have an extended conversation about your codebase without complex configuration.

### Combined File Attachments with Caching

For programming tasks, you can directly use the file attachments feature with caching to create a more efficient workflow:

> _Use gemini_ask with model gemini-2.0-flash-001 to analyze these Go files. Please add both structs.go and models.go to the context, enable caching with a 30-minute TTL, and ask about how the model management system works in this application._
> _Use gemini_ask with model `gemini-2.5-flash` to analyze these Go files. Please add both structs.go and models.go to the context, enable caching with a 30-minute TTL, and ask about how the model management system works in this application._

The server has special optimizations for this use case, particularly useful when:
- Working with complex codebases requiring multiple files for context
- Planning to ask follow-up questions about the same code
- Debugging issues that require file context
- Code review scenarios discussing implementation details

When combining file attachments with caching, files are analyzed once and stored in the cache, making subsequent queries much faster and more cost-effective.

### Managing Multiple Caches and Reducing Costs

During a conversation, you can create and use multiple caches for different sets of files or contexts:

> _Please create a new **cache** for our frontend code (App.js, components/_.js) and analyze it separately from our backend code cache we created earlier.\*

The LLM can intelligently manage these different caches, switching between them as needed based on your queries. This capability is particularly valuable for projects with distinct components that require different analysis approaches.

**Cost Savings**: Using caching significantly reduces API costs, especially when working with large codebases or having extended conversations. By caching the context:

- Files are processed and tokenized only once instead of with every query
- Follow-up questions reuse the existing context instead of creating new API requests
- Complex analyses can be performed incrementally without re-uploading files
- Multi-session analysis becomes more economical, with some users reporting 40-60% cost reductions for extended code reviews

### Customizing System Prompts

The **`gemini_ask`** and **`gemini_search`** tools are highly versatile and not limited to programming-related queries. You can customize the system prompt for various use cases:

- **Educational content**: _"You are an expert teacher who explains complex concepts in simple terms..."_
- **Creative writing**: _"You are a creative writer specializing in vivid, engaging narratives..."_
- **Technical documentation**: _"You are a technical writer creating clear, structured documentation..."_
- **Data analysis**: _"You are a data scientist analyzing patterns and trends in information..."_

When using these tools from an LLM console, always encourage the LLM to set appropriate system prompts and parameters for the specific use case. The flexibility of system prompts allows these tools to be effective for virtually any type of query.

## Detailed Documentation

### Available Tools

The server provides three primary tools:

#### 1. **`gemini_ask`**

For code analysis, general queries, and creative tasks with optional file context.

```json
{
    "name": "gemini_ask",
    "arguments": {
        "query": "Review this Go code for concurrency issues...",
        "model": "gemini-2.5-flash",
        "systemPrompt": "You are a senior Go developer. Focus on concurrency patterns, potential race conditions, and performance implications.",
        "file_paths": ["main.go", "config.go"],
        "use_cache": true,
        "cache_ttl": "1h"
    }
}
```

Simple code analysis with file attachments:

```json
{
    "name": "gemini_ask",
    "arguments": {
        "query": "Analyze this code and suggest improvements",
        "model": "gemini-2.5-pro",
        "file_paths": ["models.go"]
    }
}
```

Combining file attachments with caching for repeated queries:

```json
{
    "name": "gemini_ask",
    "arguments": {
        "query": "Explain the main data structures in these files and how they interact",
        "model": "gemini-2.5-flash",
        "file_paths": ["models.go", "structs.go"],
        "use_cache": true,
        "cache_ttl": "30m"
    }
}
```

#### 2. **`gemini_search`**

Provides grounded answers using Google Search integration with enhanced model capabilities.

```json
{
    "name": "gemini_search",
    "arguments": {
        "query": "What is the current population of Warsaw, Poland?",
        "systemPrompt": "Optional custom search instructions",
        "enable_thinking": true,
        "thinking_budget": 8192,
        "thinking_budget_level": "medium",
        "max_tokens": 4096,
        "model": "gemini-2.5-pro",
        "start_time": "2024-01-01T00:00:00Z",
        "end_time": "2024-12-31T23:59:59Z"
    }
}
```

Returns structured responses with sources and optional thinking process:

```json
{
    "answer": "Detailed answer text based on search results...",
    "thinking": "Optional detailed reasoning process when thinking mode is enabled",
    "sources": [
        {
            "title": "Source Title",
            "url": "https://example.com/source-page",
            "type": "web"
        }
    ],
    "search_queries": ["population Warsaw Poland 2025"]
}
```

#### 3. **`gemini_models`**

Lists all available Gemini models with capabilities and caching support.

```json
{
    "name": "gemini_models",
    "arguments": {}
}
```

Returns comprehensive model information including:

- Detailed descriptions of the supported Gemini 2.5 models (Pro, Flash, Flash Lite).
- Model IDs, context window sizes, and descriptions.
- Caching capabilities (implicit and explicit).
- Usage examples
- Thinking mode support.

### Model Management

This server is optimized for and exclusively supports the **Gemini 2.5 family of models**. The `gemini_models` tool provides a detailed, static list of these supported models and their specific capabilities as presented by the server.

Key supported models (as detailed by the `gemini_models` tool):

-   **`gemini-2.5-pro`** (production):
    *   Most powerful model, 1M token context window.
    *   Best for: Complex reasoning, detailed analysis, comprehensive code review.
    *   Capabilities: Advanced thinking mode, implicit caching (2048+ token minimum), explicit caching.
-   **`gemini-2.5-flash`** (production):
    *   Balanced price-performance, 32K token context window.
    *   Best for: General programming tasks, standard code review.
    *   Capabilities: Thinking mode, implicit caching (1024+ token minimum), explicit caching.
-   **`gemini-2.5-flash-lite-preview-06-17`** (preview):
    *   Optimized for cost efficiency and low latency, 32K token context window.
    *   Best for: Search queries, lightweight tasks, quick responses.
    *   Capabilities: Thinking mode (off by default), no implicit or explicit caching (preview limitation).

**Always use the `gemini_models` tool to get the most current details, capabilities, and example usage for each model as presented by the server.**

### Caching System

The server offers sophisticated context caching:

- **Model Compatibility**:
    - **Gemini 2.5 Pro & Flash**: Support both implicit caching (automatic optimization by Google for repeated prefixes if content is long enough – 2048+ tokens for Pro, 1024+ for Flash) and explicit caching (user-controlled via `use_cache: true`).
    - **Gemini 2.5 Flash Lite (Preview)**: Preview versions typically do not support implicit or explicit caching.
- **Cache Control**: Set `use_cache: true` and specify `cache_ttl` (e.g., "10m", "2h")
- **File Association**: Automatically stores files and associates with cache context
- **Performance Optimization**: Local metadata caching for quick lookups

Example with caching:

```json
{
    "name": "gemini_ask",
    "arguments": {
        "query": "Follow up on our previous discussion...",
        "model": "gemini-2.5-pro",
        "use_cache": true,
        "cache_ttl": "1h"
    }
}
```

### File Handling

Robust file processing with:

- **Direct Path Integration**: Simply specify local file paths in `file_paths` array
- **Automatic Validation**: Size checking, MIME type detection, and content validation
- **Wide Format Support**: Handles common code, text, and document formats
- **Metadata Caching**: Stores file information for quick future reference

### Advanced Features

#### Thinking Mode

The server supports "thinking mode" for all compatible Gemini 2.5 models (Pro, Flash, and Flash Lite, though it's off by default for Flash Lite):

- **Model Compatibility**: Automatically validates thinking capability based on requested model
- **Tool Support**: Available in both `gemini_ask` and `gemini_search` tools
- **Configurable Budget**: Control thinking depth with budget levels or explicit token counts

Example with thinking mode:

```json
{
    "name": "gemini_ask",
    "arguments": {
        "query": "Analyze the algorithmic complexity of merge sort vs. quick sort",
        "model": "gemini-2.5-pro",
        "enable_thinking": true,
        "thinking_budget_level": "high"
    }
}
```

##### Thinking Budget Control

Configure the depth and detail of the model's thinking process:

- **Predefined Budget Levels**:

    - `none`: 0 tokens (thinking disabled)
    - `low`: 4096 tokens (default, quick analysis)
    - `medium`: 16384 tokens (detailed reasoning)
    - `high`: 24576 tokens (maximum depth for complex problems)

- **Custom Token Budget**: Alternatively, set a specific token count with `thinking_budget` parameter (0-24576)

Examples:

```json
// Using predefined level
{
  "name": "gemini_ask",
  "arguments": {
    "query": "Analyze this algorithm...",
    "model": "gemini-2.5-pro",
    "enable_thinking": true,
    "thinking_budget_level": "medium"
  }
}

// Using explicit token count
{
  "name": "gemini_search",
  "arguments": {
    "query": "Research quantum computing developments...",
    "model": "gemini-2.5-pro", // Or gemini-2.5-flash / gemini-2.5-flash-lite
    "enable_thinking": true,
    "thinking_budget": 12000
  }
}
```

#### Context Window Size Management

The server intelligently manages token limits:

- **Custom Sizing**: Set `max_tokens` parameter to control response length
- **Model-Aware Defaults**: Automatically sets appropriate defaults based on model capabilities
- **Capacity Warnings**: Provides warnings when requested tokens exceed model limits
- **Proportional Defaults**: Uses percentage-based defaults (75% for general queries, 50% for search)

Example with context window size management:

```json
{
    "name": "gemini_ask",
    "arguments": {
        "query": "Generate a detailed analysis of this code...",
        "model": "gemini-2.5-pro",
        "max_tokens": 8192
    }
}
```

### Configuration Options

#### Essential Environment Variables

| Variable                      | Description                          | Default                  |
| ----------------------------- | ------------------------------------ | ------------------------ |
| `GEMINI_API_KEY`              | Google Gemini API key                | _Required_               |
| `GEMINI_MODEL`                | Default model for `gemini_ask`       | `gemini-2.5-pro`         |
| `GEMINI_SEARCH_MODEL`         | Default model for `gemini_search`    | `gemini-2.5-flash-lite`  |
| `GEMINI_SYSTEM_PROMPT`        | System prompt for general queries    | _Custom review prompt_   |
| `GEMINI_SEARCH_SYSTEM_PROMPT` | System prompt for search             | _Custom search prompt_   |
| `GEMINI_MAX_FILE_SIZE`        | Max upload size (bytes)              | `10485760` (10MB)        |
| `GEMINI_ALLOWED_FILE_TYPES`   | Comma-separated MIME types           | [Common text/code types] |

#### Optimization Variables

| Variable                       | Description                                          | Default |
| ------------------------------ | ---------------------------------------------------- | ------- |
| `GEMINI_TIMEOUT`               | API timeout in seconds                               | `90`    |
| `GEMINI_MAX_RETRIES`           | Max API retries                                      | `2`     |
| `GEMINI_TEMPERATURE`           | Model temperature (0.0-1.0)                          | `0.4`   |
| `GEMINI_ENABLE_CACHING`        | Enable context caching                               | `true`  |
| `GEMINI_DEFAULT_CACHE_TTL`     | Default cache time-to-live                           | `1h`    |
| `GEMINI_ENABLE_THINKING`       | Enable thinking mode capability                      | `true`  |
| `GEMINI_THINKING_BUDGET_LEVEL` | Default thinking budget level (none/low/medium/high) | `low`   |
| `GEMINI_THINKING_BUDGET`       | Explicit thinking token budget (0-24576)             | `4096`  |

### Operational Features

- **Degraded Mode**: Automatically enters safe mode on initialization errors
- **Retry Logic**: Configurable exponential backoff for reliable API communication
- **Structured Logging**: Comprehensive event logging with severity levels
- **File Validation**: Secure handling with size and type restrictions

## Development

### Running Tests

```bash
go test -v
```

### Running Linter

```bash
./run_lint.sh
```

### Formatting Code

```bash
./run_format.sh
```

## Recent Changes

- **Exclusive Gemini 2.5 Support**: Server now exclusively supports the Gemini 2.5 family of models (Pro, Flash, Flash Lite) for optimal performance and access to the latest features.
- **Streamlined Model Information**: The `gemini_models` tool provides detailed, up-to-date information on supported Gemini 2.5 models, their context windows, and specific capabilities like caching and thinking mode.
- **Enhanced Caching for Gemini 2.5**: Leverages implicit caching (automatic for Pro/Flash with sufficient context) and provides robust explicit caching for Gemini 2.5 Pro and Flash models.
- **Time Range Filtering**: Added `start_time` and `end_time` to `gemini_search` for filtering results by publication date.

## License

[MIT License](LICENSE)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the project
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request
