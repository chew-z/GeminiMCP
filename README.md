# Gemini MCP Server

A production-grade MCP server integrating with Google's Gemini API, featuring advanced code review capabilities, efficient file management, and sophisticated cached context handling.

## Features

- **Multi-Model Support**: Choose from various Gemini models including Gemini 2.5 Pro and Gemini 2.0 Flash variants
- **Code Review Focus**: Built-in system prompt for detailed code analysis with markdown output
- **Automatic File Handling**: Built-in file management with direct path integration
- **Enhanced Caching**: Create persistent contexts with user-defined TTL for repeated queries
- **Advanced Error Handling**: Graceful degradation with structured error logging
- **Improved Retry Logic**: Automatic retries with configurable exponential backoff for API calls
- **Security**: Configurable file type restrictions and size limits
- **Performance Monitoring**: Built-in metrics collection for request latency and throughput

## Prerequisites

- Go 1.24+
- Google Gemini API key
- Basic understanding of MCP protocol

## Installation & Quick Start

```bash
# Clone and build
git clone https://github.com/yourorg/gemini-mcp
cd gemini-mcp
go build -o gemini-mcp

# Start server with environment variables
export GEMINI_API_KEY=your_api_key
export GEMINI_MODEL=gemini-1.5-pro
./gemini-mcp
```

## Configuration

### Essential Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GEMINI_API_KEY` | Google Gemini API key | *Required* |
| `GEMINI_MODEL` | Model ID from `models.go` | `gemini-1.5-pro` |
| `GEMINI_SYSTEM_PROMPT` | System prompt for code review | *Custom review prompt* |
| `GEMINI_SEARCH_SYSTEM_PROMPT` | System prompt for search queries | *Custom search prompt* |
| `GEMINI_MAX_FILE_SIZE` | Max upload size (bytes) | `10485760` (10MB) |
| `GEMINI_ALLOWED_FILE_TYPES` | Comma-separated MIME types | [Common text/code types] |

### Optimization Variables
| Variable | Description | Default |
|----------|-------------|---------|
| `GEMINI_TIMEOUT` | API timeout in seconds | `90` |
| `GEMINI_MAX_RETRIES` | Max API retries | `2` |
| `GEMINI_INITIAL_BACKOFF` | Initial backoff time (seconds) | `1` |
| `GEMINI_MAX_BACKOFF` | Maximum backoff time (seconds) | `10` |
| `GEMINI_TEMPERATURE` | Model temperature (0.0-1.0) | `0.4` |
| `GEMINI_ENABLE_CACHING` | Enable context caching | `true` |
| `GEMINI_DEFAULT_CACHE_TTL` | Default cache time-to-live | `1h` |

Example `.env`:
```env
GEMINI_API_KEY=your_api_key
GEMINI_MODEL=gemini-1.5-pro
GEMINI_SYSTEM_PROMPT="Your custom code review prompt here"
GEMINI_SEARCH_SYSTEM_PROMPT="Your custom search prompt here"
GEMINI_MAX_FILE_SIZE=5242880  # 5MB
GEMINI_ALLOWED_FILE_TYPES=text/x-go,text/markdown
```

## Core API Tools

Currently, the server provides three main tools:

### gemini_ask

Used for code analysis, review, and general queries with optional file path inclusion and caching.

```json
{
  "name": "gemini_ask",
  "arguments": {
    "query": "Review this Go code for concurrency issues...",
    "model": "gemini-2.5-pro-exp-03-25",
    "systemPrompt": "Optional custom review instructions",
    "file_paths": ["main.go", "config.go"],
    "use_cache": true,
    "cache_ttl": "1h"
  }
}
```

### gemini_search

Uses Google Search integration with Gemini to provide grounded answers to questions. This tool uses the Gemini 2.0 Flash model specifically optimized for search-based queries.

```json
{
  "name": "gemini_search",
  "arguments": {
    "query": "What is the current population of Warsaw, Poland?",
    "systemPrompt": "Optional custom search instructions"
  }
}
```

### gemini_models

Lists all available Gemini models with their capabilities and caching support.

```json
{
  "name": "gemini_models",
  "arguments": {}
}
```

## Supported Models

The following Gemini models are supported:

| Model ID | Description | Caching Support |
|----------|-------------|----------------|
| `gemini-2.5-pro-exp-03-25` | State-of-the-art thinking model | No |
| `gemini-2.0-flash-lite` | Optimized for speed and cost efficiency | No |
| `gemini-2.0-flash-001` | Version with text-only output | Yes |
| `gemini-1.5-pro` | Previous generation pro model | No |
| `gemini-1.5-pro-001` | Stable version with version suffix | Yes |
| `gemini-1.5-flash` | Optimized for efficiency and speed | No |
| `gemini-1.5-flash-001` | Stable version with version suffix | Yes |

## Supported File Types
| Extension | MIME Type | 
|-----------|-----------|
| .go       | text/x-go |
| .py       | text/x-python |
| .js       | text/javascript |
| .md       | text/markdown |
| .java     | text/x-java |
| .c/.h     | text/x-c |
| .cpp/.hpp | text/x-c++ |
| 25+ more  | (See `getMimeTypeFromPath` in gemini.go) |

## Operational Notes

- **Degraded Mode**: Automatically enters safe mode on initialization errors
- **Audit Logging**: All operations logged with timestamps and metadata
- **Performance**: Typical response latency <2s for code reviews
- **Security**: File content validated by MIME type and size before processing

## File Handling

The server now handles files directly through the `gemini_ask` tool:

1. Specify local file paths in the `file_paths` array parameter
2. The server automatically:
   - Reads the files from the provided paths
   - Determines the correct MIME type based on file extension
   - Uploads the file content to the Gemini API
   - Uses the files as context for the query

This direct file handling approach eliminates the need for separate file upload/management endpoints.

## Caching Functionality

The server supports enhanced caching capabilities:

- **Automatic Caching**: Simply set `use_cache: true` in the `gemini_ask` request
- **TTL Control**: Specify cache expiration with the `cache_ttl` parameter (e.g., "10m", "2h")
- **Model Support**: Only models with version suffixes (ending with `-001`) support caching
- **Context Persistence**: Uploaded files are automatically stored and associated with the cache

Example with caching:
```json
{
  "name": "gemini_ask",
  "arguments": {
    "query": "Review this code considering the best practices we discussed earlier",
    "model": "gemini-1.5-pro-001",
    "use_cache": true,
    "cache_ttl": "1h",
    "file_paths": ["main.go", "config.go"]
  }
}
```

## Recent Changes

- Reimplemented proper file upload functionality using the fixed v1.1.0 version of `google.golang.org/genai` library
- Added `gemini_search` tool with Google Search integration
- Added support for Gemini 2.5 Pro and Gemini 2.0 Flash models
- Simplified the API by integrating file handling directly into the `gemini_ask` tool
- Enhanced caching system with user-configurable TTL
- Refactored retry logic with configurable backoff parameters
- Improved error handling and logging throughout the codebase
- Removed deprecated functions and tools for a cleaner implementation

## Development

### Running Tests

To run tests:

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

## License

[MIT License](LICENSE)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the project
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request