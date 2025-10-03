# Release Notes

## v0.4.0 - 2025-10-03

### 🎉 New Features
- **Authentication**: Improved JWT validation and token extraction, and replaced the custom JWT implementation with the standard `golang-jwt` library for better security and maintenance.
- **File Handling**: Enhanced mime type detection for file uploads and improved file fetching from GitHub with better logging and error handling.
- **Gemini Tools**: Clarified file path usage for the `gemini_ask` tool and improved code fetching instructions.

### 🐛 Bug Fixes
- **Gemini Client**: Standardized mime types and improved error messages.
- **GitHub Integration**: Fixed issues with file handling and model configuration.

### 🔧 Improvements
- **Code Quality**: Refactored model names to use `latest` suffixes and made other internal improvements.
- **Dependencies**: Updated Go module dependencies.
