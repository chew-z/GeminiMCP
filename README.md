# Gemini MCP Server

This is an MCP (Machine Conversation Protocol) server that integrates with Google's Gemini API. It provides a research tool capability that can be used by MCP clients to fetch detailed information on various topics.

## Features

- Integration with Google's Gemini API
- Support for multiple models (gemini-pro is the default)
- Configurable timeout and retry behavior
- Comprehensive logging
- Error handling with graceful degradation

## Prerequisites

- Go 1.24 or later
- Google Gemini API key

## Installation

1. Clone the repository
2. Install dependencies:
   ```bash
   go mod download
   ```
3. Set up environment variables (see Configuration section)

## Configuration

The server can be configured using environment variables:

### Required Environment Variables

- `GEMINI_API_KEY` (Required): Your Google Gemini API key

### Optional Environment Variables

- `GEMINI_MODEL` (Optional): Gemini model to use (default: "gemini-pro")
- `GEMINI_TIMEOUT` (Optional): Timeout for API requests in seconds (default: 90)
- `GEMINI_MAX_RETRIES` (Optional): Maximum number of retries for failed requests (default: 2)
- `GEMINI_INITIAL_BACKOFF` (Optional): Initial backoff time in seconds (default: 1)
- `GEMINI_MAX_BACKOFF` (Optional): Maximum backoff time in seconds (default: 10)

You can create a `.env` file in the project root with these variables:

```env
GEMINI_API_KEY=your_gemini_api_key
GEMINI_MODEL=gemini-pro
GEMINI_TIMEOUT=120
```

## Usage

To run the server:

```bash
go run *.go
```

Or build and run:

```bash
go build -o gemini-mcp
./gemini-mcp
```

The server will start and listen for MCP client connections on the default MCP port.

## API

The server implements the following MCP tool:

- **research**: Accepts a text query and returns a comprehensive research report on the topic.

Example request:
```json
{
  "name": "research",
  "arguments": {
    "query": "What is quantum computing?"
  }
}
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
