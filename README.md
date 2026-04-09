# Gemini MCP Server

MCP (Model Control Protocol) server integrating with Google's Gemini API.

> **Important**: This server now supports **Gemini 3 Pro** (latest) along with the Gemini 2.5 family models for optimal thinking mode and implicit caching capabilities.

## Key Advantages

- **Single Self-Contained Binary**: Written in Go, the project compiles to a single binary with no dependencies, eliminating package manager issues and preventing unexpected changes without user knowledge
- **Dynamic Model Access**: Automatically fetches the latest available Gemini models at startup
- **Implicit Caching**: Optimized prompt structure for automatic Gemini implicit caching (~90% token discount)
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
go build -o ./bin/mcp-gemini .

## Start server with environment variables
export GEMINI_API_KEY=your_api_key
export GEMINI_MODEL=gemini-3.1-pro-preview
./bin/mcp-gemini

## Or start with HTTP transport
./bin/mcp-gemini --transport=http

## Or override settings via command line
./bin/mcp-gemini --transport=http --gemini-model=gemini-3-flash-preview
```

### Client Configuration

Add this server to any MCP-compatible client like Claude Desktop by adding to your client's configuration:

```json
{
    "gemini": {
        "command": "/Users/<user>/Path/to/bin/mcp-gemini",
        "env": {
            "GEMINI_API_KEY": "YOUR_GEMINI_API_KEY",
            "GEMINI_MODEL": "gemini-3.1-pro-preview",
            "GEMINI_SEARCH_MODEL": "gemini-flash-lite-latest",
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

### Command-Line Options

The server accepts several command-line flags to override environment variables:

```bash
./bin/mcp-gemini [OPTIONS]

Options:
  --gemini-model string          Gemini model name (overrides GEMINI_MODEL)
  --gemini-system-prompt string  System prompt (overrides GEMINI_SYSTEM_PROMPT)  
  --gemini-temperature float     Temperature 0.0-1.0 (overrides GEMINI_TEMPERATURE)
  --enable-thinking             Enable thinking mode (overrides GEMINI_ENABLE_THINKING)
  --transport string            Transport: 'stdio' (default) or 'http'
  --auth-enabled                Enable JWT authentication for HTTP transport
  --generate-token              Generate a JWT token and exit
  --token-username string       Username for token generation (default: "admin")
  --token-role string           Role for token generation (default: "admin")
  --token-expiration int        Token expiration in hours (default: 744 = 31 days)
  --help                        Show help information
```

**Transport Modes:**
- **stdio** (default): For MCP clients like Claude Desktop that communicate via stdin/stdout
- **http**: Enables REST API endpoints for web applications or direct HTTP access

### Authentication (HTTP Transport Only)

The server supports JWT-based authentication for HTTP transport:

**Security Note on CORS:** By default, the HTTP server allows Cross-Origin Resource Sharing (CORS) from all origins (`*`). This is convenient for development but can be a security risk in production. It is strongly recommended to configure the allowed origins for your production environment by setting the `GEMINI_HTTP_CORS_ORIGINS` environment variable to a comma-separated list of allowed domains (e.g., `https://your-app.com,https://your-other-app.com`).

```bash
# Enable authentication
export GEMINI_AUTH_ENABLED=true
export GEMINI_AUTH_SECRET_KEY="your-secret-key-at-least-32-characters"

# Start server with authentication enabled
./bin/mcp-gemini --transport=http --auth-enabled=true

# Generate authentication tokens (31 days expiration by default)
./bin/mcp-gemini --generate-token --token-username=admin --token-role=admin
```

**Using Authentication:**
```bash
# Include JWT token in requests
curl -X POST http://localhost:8081/mcp \
  -H "Authorization: Bearer your-jwt-token-here" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```

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

### Project Analysis with File Context

Say to your LLM:

> _Please use **Gemini** to analyze our project files. Include the main.go, config.go and models.go files from the GitHub repo and ask Gemini about our project architecture and how it could be improved._

The server automatically optimizes for Gemini's implicit caching by placing files before the query, so repeated queries over the same files benefit from ~90% token discount automatically.

This is particularly useful when:
- Working with complex codebases requiring multiple files for context
- Planning to ask follow-up questions about the same code
- Debugging issues that require file context
- Code review scenarios discussing implementation details

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
        "model": "gemini-3-flash-preview",
        "systemPrompt": "You are a senior Go developer. Focus on concurrency patterns, potential race conditions, and performance implications.",
        "github_files": ["main.go", "config.go"]
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
        "github_files": ["models.go"]
    }
}
```

File attachments with GitHub integration:

```json
{
    "name": "gemini_ask",
    "arguments": {
        "query": "Explain the main data structures in these files and how they interact",
        "model": "gemini-3-flash-preview",
        "github_repo": "owner/repo",
        "github_ref": "main",
        "github_files": ["models.go", "structs.go"]
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
        "thinking_level": "high",
        "max_tokens": 4096,
        "model": "gemini-3.1-pro-preview",
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

Lists all available Gemini models with capabilities.

```json
{
    "name": "gemini_models",
    "arguments": {}
}
```

Returns comprehensive model information including:

- Detailed descriptions of the supported Gemini 2.5 models (Pro, Flash, Flash Lite).
- Model IDs, context window sizes, and descriptions.
- Usage examples
- Thinking mode support.

### Model Management

This server is optimized for and exclusively supports the **Gemini 2.5 family of models**. The `gemini_models` tool provides a detailed, static list of these supported models and their specific capabilities as presented by the server.

Key supported models (as detailed by the `gemini_models` tool):

-   **`gemini-3.1-pro-preview`** (latest Pro):
    *   Most powerful model, 1M token context window.
    *   Best for: Complex reasoning, detailed analysis, comprehensive code review.
    *   Capabilities: Advanced thinking mode, automatic implicit caching.
-   **`gemini-3-flash-preview`** (latest Flash):
    *   Latest Flash model with improved performance, 1M token context window.
    *   Best for: General programming tasks, standard code review, balanced price-performance.
    *   Capabilities: Thinking mode, automatic implicit caching.
-   **`gemini-3.1-flash-lite-preview`** (latest Flash Lite):
    *   Fastest and most cost-efficient, 1M token context window.
    *   Best for: Search queries, lightweight tasks, quick responses.
    *   Capabilities: Thinking mode, automatic implicit caching.

**Always use the `gemini_models` tool to get the most current details, capabilities, and example usage for each model as presented by the server.**

### Implicit Caching

The server leverages Gemini's automatic implicit caching for cost savings:

- **Automatic**: No configuration needed — Gemini automatically caches and reuses shared request prefixes
- **Optimized Ordering**: File content is placed before the query to maximize cache hits across repeated requests
- **Deterministic Prefix**: GitHub files are sorted by filename to ensure consistent ordering
- **Token Thresholds**: Pro models require 4096+ tokens, Flash models require 1024+ tokens for cache eligibility
- **~90% Discount**: Cached tokens are billed at approximately 10% of standard input price

### File Handling

Robust file processing with:

- **GitHub Integration**: Fetch files directly from a GitHub repository using the `github_repo`, `github_ref`, and `github_files` arguments. `github_repo` is required when `github_files` is provided.
- **Local File Access (stdio only)**: The `file_paths` argument can be used to access local files, but only when the server is running in `stdio` mode. This method is deprecated for `http` transport due to security concerns.
- **Automatic Validation**: Size checking, MIME type detection, and content validation
- **Wide Format Support**: Handles 60+ code, text, config, and document formats

### Advanced Features

#### Thinking Mode

The server supports "thinking mode" for Gemini 3 Pro and compatible Gemini 2.5 models:

- **Gemini 3 Pro**: Uses the new `thinking_level` parameter (low, high; medium coming soon)
- **Gemini 2.5 Models**: Uses legacy `thinking_budget` and `thinking_budget_level` parameters
- **Model Compatibility**: Automatically validates thinking capability based on requested model
- **Tool Support**: Available in both `gemini_ask` and `gemini_search` tools

Example with Gemini 3 Pro thinking mode:

```json
{
    "name": "gemini_ask",
    "arguments": {
        "query": "Analyze the algorithmic complexity of merge sort vs. quick sort",
        "model": "gemini-3.1-pro-preview",
        "enable_thinking": true,
        "thinking_level": "high"
    }
}
```

##### Thinking Level Control (Gemini 3 Pro)

Configure the depth and detail of Gemini 3 Pro's thinking process using the `thinking_level` parameter:

- **Thinking Levels**:

    - `low`: Minimizes latency and cost. Best for simple instruction following, chat, or high-throughput applications
    - `high` (default): Maximizes reasoning depth. The model may take longer for first token, but output will be more carefully reasoned
    - `medium`: Coming soon, not supported at launch

**Important**: Cannot use both `thinking_level` and legacy `thinking_budget` parameter in the same request (returns 400 error).

Examples:

```json
// Gemini 3 Pro with high thinking level (default)
{
  "name": "gemini_ask",
  "arguments": {
    "query": "Analyze this complex algorithm...",
    "model": "gemini-3.1-pro-preview",
    "enable_thinking": true,
    "thinking_level": "high"
  }
}

// Gemini 3 Pro with low thinking level for faster responses
{
  "name": "gemini_search",
  "arguments": {
    "query": "Quick search for recent developments...",
    "model": "gemini-3.1-pro-preview",
    "enable_thinking": true,
    "thinking_level": "low"
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

### Retries and Backoff

The server automatically retries transient failures using exponential backoff with jitter.

- Retryable errors: network timeouts/temporary errors, Google API 429, and 5xx responses.
- Backoff strategy: delay grows ~2^attempt from `GEMINI_INITIAL_BACKOFF`, capped by `GEMINI_MAX_BACKOFF`, with full jitter (0.5–1.5x).
- Control via env vars: set `GEMINI_MAX_RETRIES` (0 disables retries), `GEMINI_INITIAL_BACKOFF`, and `GEMINI_MAX_BACKOFF`.

Example configuration:

```bash
export GEMINI_MAX_RETRIES=3
export GEMINI_INITIAL_BACKOFF=2s
export GEMINI_MAX_BACKOFF=15s
```

### Configuration Options

#### Essential Environment Variables

| Variable                      | Description                          | Default                  |
| ----------------------------- | ------------------------------------ | ------------------------ |
| `GEMINI_API_KEY`              | Google Gemini API key                | _Required_               |
| `GEMINI_MODEL`                | Default model for `gemini_ask`       | `gemini-2.5-pro`         |
| `GEMINI_SEARCH_MODEL`         | Default model for `gemini_search`    | `gemini-flash-lite-latest`  |
| `GEMINI_SYSTEM_PROMPT`        | System prompt for general queries    | _Custom review prompt_   |
| `GEMINI_SEARCH_SYSTEM_PROMPT` | System prompt for search             | _Custom search prompt_   |
| `GEMINI_GITHUB_TOKEN`         | GitHub token for private repo access | _Optional_               |
| `GEMINI_GITHUB_API_BASE_URL`  | GitHub API base URL for GHES   | `https://api.github.com` |
| `GEMINI_MAX_GITHUB_FILES`     | Max number of files per call         | `20`                     |
| `GEMINI_MAX_GITHUB_FILE_SIZE` | Max size per file in bytes           | `1048576` (1MB)          |
| `GEMINI_MAX_FILE_SIZE`        | Max upload size (bytes)              | `10485760` (10MB)        |
| `GEMINI_ALLOWED_FILE_TYPES`   | Comma-separated MIME types           | [Common text/code types] |

#### Optimization Variables

| Variable                       | Description                                          | Default | 
| ------------------------------ | ---------------------------------------------------- | ------- | 
| `GEMINI_TIMEOUT`               | API timeout (Go duration, e.g., `90s`)               | `90s`   | 
| `GEMINI_MAX_RETRIES`           | Max API retries                                      | `2`     | 
| `GEMINI_INITIAL_BACKOFF`       | Initial retry backoff (duration)                     | `1s`    | 
| `GEMINI_MAX_BACKOFF`           | Maximum retry backoff cap (duration)                 | `10s`   | 
| `GEMINI_TEMPERATURE`           | Model temperature (0.0-1.0)                          | `0.4`   | 
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

- **Gemini 3+ Support**: Server supports the latest Gemini 3.1 Pro, Gemini 3 Flash, and Gemini 3.1 Flash Lite models.
- **Implicit Caching Only**: Removed explicit caching in favor of Gemini's automatic implicit caching. Files are placed before queries to maximize cache prefix hits.
- **Expanded MIME Types**: 60+ file extensions and special filenames recognized for inline text injection.
- **Partial Failure Warnings**: Model is informed about files that couldn't be loaded via [System Note].
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