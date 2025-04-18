# GeminiMCP Project Context

## Project Overview
- GeminiMCP is a Model Control Protocol (MCP) server integrating with Google's Gemini API
- Written in Go, compiles to a single binary with no dependencies
- Provides advanced AI capabilities for code analysis, general queries, and search with grounding
- Located at: `/Users/rrj/Projekty/Go/src/GeminiMCP`

## Architecture
- **Main Server** (`main.go`): Entry point with configuration and initialization
- **Gemini Integration** (`gemini.go`): Core API interactions and tool implementations
- **Configuration** (`config.go`): Environment-based settings with command-line overrides
- **Models** (`models.go`): Model definitions, capabilities, and dynamic fetching
- **Cache System** (`cache.go`): Context persistence with TTL control
- **File Management** (`files.go`): File processing and metadata handling

## Available Tools
1. `gemini_ask`: For code analysis and general queries with optional file context
2. `gemini_search`: For grounded answers using Google Search
3. `gemini_models`: Lists available Gemini models with capabilities

## Recent Changes
The latest commit (April 18, 2025) added advanced model configuration options:
- Added `EnableThinking` capability for Gemini 2.5 models
- Added `ContextWindowSize` control (up to 1M tokens for some models)
- Enhanced model metadata with thinking capabilities and context limits
- Updated API integration with ThinkingConfig parameters

## Key Features
- **Dynamic Model Access**: Automatically fetches available models at startup
- **Advanced Caching**: Efficient context reuse with TTL control
- **Enhanced File Handling**: Seamless file integration with MIME detection
- **Production Reliability**: Robust error handling with graceful degradation
- **Reasoning Capabilities**: Support for advanced thinking in Gemini 2.5 models

## Client Integration
- Designed to work with Claude Desktop or any MCP-compatible client
- Configuration through environment variables or command-line flags
- Environment variables must be included in the client's configuration JSON

## Next Steps Options
1. Implement new features or enhancements
2. Improve existing functionality
3. Extend documentation
4. Add test coverage
5. Optimize performance

I've analyzed this project through its codebase and recent commit history. The project is built around the MCP protocol to provide a unified interface to Google's Gemini AI models, with particular focus on code analysis capabilities, efficient context handling, and integration with MCP-compatible clients like Claude Desktop.
