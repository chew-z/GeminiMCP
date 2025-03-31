# Gemini MCP Server

A production-grade MCP server integrating with Google's Gemini API, featuring advanced code review capabilities, file management, and cached context handling.

## Features

- **Multi-Model Support**: Choose from various Gemini models optimized for different tasks
- **Code Review Focus**: Built-in system prompt for detailed code analysis with markdown output
- **File Management**: Upload/delete text/code files (Go, Python, JS, Java, C/C++, Markdown, etc.)
- **Cached Contexts**: Create persistent contexts with files/text for repeated queries
- **Advanced Error Handling**: Graceful degradation with error mode logging
- **Retry Logic**: Automatic retries with exponential backoff for API calls
- **Security**: Configurable file type restrictions and size limits

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
| `GEMINI_MAX_FILE_SIZE` | Max upload size (bytes) | `10485760` (10MB) |
| `GEMINI_ALLOWED_FILE_TYPES` | Comma-separated MIME types | [Common text/code types] |

### Optimization Variables
| Variable | Description | Default |
|----------|-------------|---------|
| `GEMINI_TIMEOUT` | API timeout in seconds | `90` |
| `GEMINI_MAX_RETRIES` | Max API retries | `2` |
| `GEMINI_ENABLE_CACHING` | Enable context caching | `true` |

Example `.env`:
```env
GEMINI_API_KEY=your_api_key
GEMINI_MODEL=gemini-1.5-pro
GEMINI_MAX_FILE_SIZE=5242880  # 5MB
GEMINI_ALLOWED_FILE_TYPES=text/x-go,text/markdown
```

## Core API Tools

### Code Analysis & Query
```json
{
  "name": "gemini_ask",
  "arguments": {
    "query": "Review this Go code for concurrency issues...",
    "model": "gemini-2.5-pro-exp-03-25",
    "systemPrompt": "Optional custom review instructions"
  }
}
```

### File Management
```json
{
  "name": "gemini_upload_file",
  "arguments": {
    "filename": "review.go",
    "content": "base64EncodedContent",
    "display_name": "Core Application Code"
  }
}
```

### Cached Contexts
```json
{
  "name": "gemini_create_cache",
  "arguments": {
    "model": "gemini-1.5-pro",
    "file_ids": ["file1", "file2"],
    "content": "base64EncodedSupplementaryText",
    "ttl": "24h"
  }
}
```

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
| 25+ more  | (See `inferMIMEType` in files.go) |

## Operational Notes

- **Degraded Mode**: Automatically enters safe mode on initialization errors
- **Audit Logging**: All operations logged with timestamps and metadata
- **Performance**: Typical response latency <2s for code reviews
- **Security**: File content validated by MIME type and size before processing
```

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
# Gemini MCP
