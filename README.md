# Gemini MCP Server

MCP (Model Control Protocol) server integrating with Google's Gemini API.

## Quick Start

### Prerequisites
- Google Gemini API key

### Installation

```bash
# Clone and build
git clone https://github.com/chew-z/GeminiMCP
cd GeminiMCP
go build -o gemini-mcp

# Start server with environment variables
export GEMINI_API_KEY=your_api_key
export GEMINI_MODEL=gemini-1.5-pro
./gemini-mcp
```

## Key Advantages

- **Dynamic Model Access**: Automatically fetches the latest available Gemini models at startup
- **Advanced Context Handling**: Efficient caching system with TTL control for repeated queries
- **Enhanced File Handling**: Seamless file integration with intelligent MIME detection
- **Production Reliability**: Robust error handling, automatic retries, and graceful degradation
- **Comprehensive Capabilities**: Full support for code analysis, general queries, and search with grounding

## Client Configuration

Add this server to any MCP-compatible client like Claude Desktop by adding to your client's configuration:

```json
{
        "gemini": {
            "command": "/Users/<user>/Path/to/bin/mcp-gemini",
            "env": {
                "GEMINI_API_KEY": "YOUR_API_KEY_HERE",
                "GEMINI_MODEL": "gemini-2.0-flash-001",
                "GEMINI_SYSTEM_PROMPT": "You are a senior developer. Your job is to do a thorough code review of this code..."
            }
        }
}
```

## Using Gemini Tools from LLM Console

You can use Gemini tools directly from an LLM console by creating prompt examples that invoke the tools. Here are some example prompts for different use cases:

### Listing Available Models

Say to your LLM:

> Please use the gemini_models tool to show me the list of available Gemini models.

The LLM will invoke the `gemini_models` tool and return the list of available models, their capabilities, and caching support status.

### Code Analysis with gemini_ask

Say to your LLM:

> Use the gemini_ask tool to analyze this Go code for potential concurrency issues:
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
> Please use a system prompt that focuses on code review and performance optimization.

### Creative Writing with gemini_ask

Say to your LLM:

> Use the gemini_ask tool to create a short story about a space explorer discovering a new planet. Set a custom system prompt that encourages creative, descriptive writing with vivid imagery.

### Factual Research with gemini_search

Say to your LLM:

> Use the gemini_search tool to find the latest information about advancements in fusion energy research. Include sources in your response.

### File Analysis with Caching

Say to your LLM:

> Use the gemini_ask tool to analyze the code in main.go and config.go files. Please enable caching with a 1-hour TTL so we can discuss these files in follow-up questions. Set a system prompt that focuses on understanding the architecture and relationships between components.
>
> Use these parameters:
> - model: gemini-1.5-pro-001 (make sure to use a model that supports caching)
> - file_paths: ["main.go", "config.go"]
> - use_cache: true
> - cache_ttl: "1h"

For follow-up questions, you can then say:

> Using the cached context from our previous conversation about the code files, use the gemini_ask tool to explain how error handling is implemented across these files. Make sure to use the same cached context we created earlier.

This approach lets you have an extended conversation about the same set of files without having to re-upload them for each query, improving response times and maintaining context across multiple questions.

### Customizing System Prompts

The `gemini_ask` and `gemini_search` tools are highly versatile and not limited to programming-related queries. You can customize the system prompt for various use cases:

- **Educational content**: "You are an expert teacher who explains complex concepts in simple terms..."
- **Creative writing**: "You are a creative writer specializing in vivid, engaging narratives..."
- **Technical documentation**: "You are a technical writer creating clear, structured documentation..."
- **Data analysis**: "You are a data scientist analyzing patterns and trends in information..."

When using these tools from an LLM console, always encourage the LLM to set appropriate system prompts and parameters for the specific use case. The flexibility of system prompts allows these tools to be effective for virtually any type of query.

## Detailed Documentation

### Available Tools

The server provides three primary tools:

#### 1. gemini_ask
For code analysis, general queries, and creative tasks with optional file context.

```json
{
  "name": "gemini_ask",
  "arguments": {
    "query": "Review this Go code for concurrency issues...",
    "model": "gemini-2.0-flash-001",
    "systemPrompt": "Optional custom instructions",
    "file_paths": ["main.go", "config.go"],
    "use_cache": true,
    "cache_ttl": "1h"
  }
}
```

#### 2. gemini_search
Provides grounded answers using Google Search integration.

```json
{
  "name": "gemini_search",
  "arguments": {
    "query": "What is the current population of Warsaw, Poland?",
    "systemPrompt": "Optional custom search instructions"
  }
}
```

Returns structured responses with sources:
```json
{
  "answer": "Detailed answer text based on search results...",
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

#### 3. gemini_models
Lists all available Gemini models with capabilities and caching support.

```json
{
  "name": "gemini_models",
  "arguments": {}
}
```

Returns comprehensive model information including:
- Complete list of available models (dynamically fetched at startup)
- Model IDs and descriptions
- Caching support status
- Usage examples

### Model Management

The server dynamically fetches available Gemini models from the Google API at startup. Common models include:

| Model ID | Description | Caching Support |
|----------|-------------|----------------|
| `gemini-2.5-pro-exp-03-25` | State-of-the-art thinking model | No |
| `gemini-2.0-flash-001` | Optimized for speed with version suffix | Yes |
| `gemini-1.5-pro-001` | Previous generation with stability | Yes |

Use the `gemini_models` tool for a complete, up-to-date list.

### Caching System

The server offers sophisticated context caching:

- **Model Compatibility**: Only models with version suffixes (e.g., `-001`) support caching
- **Cache Control**: Set `use_cache: true` and specify `cache_ttl` (e.g., "10m", "2h")
- **File Association**: Automatically stores files and associates with cache context
- **Performance Optimization**: Local metadata caching for quick lookups

Example with caching:
```json
{
  "name": "gemini_ask",
  "arguments": {
    "query": "Follow up on our previous discussion...",
    "model": "gemini-1.5-pro-001",
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

### Configuration Options

#### Essential Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GEMINI_API_KEY` | Google Gemini API key | *Required* |
| `GEMINI_MODEL` | Default model ID | `gemini-1.5-pro` |
| `GEMINI_SYSTEM_PROMPT` | System prompt for general queries | *Custom review prompt* |
| `GEMINI_SEARCH_SYSTEM_PROMPT` | System prompt for search | *Custom search prompt* |
| `GEMINI_MAX_FILE_SIZE` | Max upload size (bytes) | `10485760` (10MB) |
| `GEMINI_ALLOWED_FILE_TYPES` | Comma-separated MIME types | [Common text/code types] |

#### Optimization Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GEMINI_TIMEOUT` | API timeout in seconds | `90` |
| `GEMINI_MAX_RETRIES` | Max API retries | `2` |
| `GEMINI_TEMPERATURE` | Model temperature (0.0-1.0) | `0.4` |
| `GEMINI_ENABLE_CACHING` | Enable context caching | `true` |
| `GEMINI_DEFAULT_CACHE_TTL` | Default cache time-to-live | `1h` |

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

- **Dynamic Model Fetching**: Automatic retrieval of available Gemini models at startup
- **Enhanced Client Integration**: Added configuration guides for MCP clients
- **Expanded Model Support**: Updated compatibility with latest Gemini 2.5 Pro and 2.0 Flash models
- **Search Capabilities**: Added Google Search integration with source attribution
- **Improved File Handling**: Enhanced MIME detection and validation
- **Caching Enhancements**: Better support for models with version suffixes
- **Reliability Improvements**: Refactored retry logic with configurable parameters

## License

[MIT License](LICENSE)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the project
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request
