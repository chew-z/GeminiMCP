# Release Notes

## v0.2.0 - 2025-08-12

### 🎉 New Features
- **GitHub Integration**: Enhanced file fetching with improved logging, error handling, and configurable limits. Secure file fetching from GitHub is now enabled.
- **Authentication**: Replaced the custom JWT implementation with the more robust `golang-jwt` library, improving token validation and error handling.
- **Gemini API**: Improved code fetching instructions and clarified GitHub file parameters for the GeminiAskTool.
- **Logging**: Added logging for file fetching operations to improve traceability.
- **Release Process**: Automated the project release process.
- **Retry Mechanism**: Implemented exponential backoff and retries for API calls to improve resilience.
- **Configuration**: Enabled HTTP transport by default when stdio is not specified.
- **Tooling**: Clarified file path usage for the GeminiAskTool.

### 🐛 Bug Fixes
- **GitHub**: Improved error messages for not found files and API requests.
- **Gemini**: Corrected file handling and model configuration issues.
- **Context**: Added the missing HTTP transport context key.
- **Configuration**: Corrected the GitHub API base URL environment variable name.

### 🔧 Improvements
- **Gemini**: Refactored to clarify GitHub file parameters in the GeminiAskTool.

### ⚙️ Internal Changes
- Removed the unused `testify` dependency.
