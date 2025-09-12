# Release Notes

## v0.3.0 - 2025-09-12

### 🎉 New Features
- **Modernized JWT Authentication**: Replaced the custom JWT implementation with the standard `golang-jwt/jwt` library for improved security, reliability, and maintainability.
- **Enhanced GitHub File Fetching**: Added configurable limits for file size and total bytes when fetching from GitHub repositories, preventing excessive API usage. File fetching logic and logging have been significantly improved.
- **Automated Release Process**: Introduced initial framework and scripts to streamline the project release process.
- **Improved File Uploads**: Enhanced mime type detection for file uploads to the Gemini API, ensuring greater compatibility and fewer errors.

### 🐛 Bug Fixes
- **GitHub API Errors**: Improved error messages and reporting for GitHub API requests, making it easier to diagnose issues with file fetching.
- **Gemini API Mime Types**: Standardized the mime types sent to the Gemini API and a corresponding error messages for clarity.
- **Configuration**: Corrected the environment variable name for the GitHub API base URL to `GITHUB_API_BASE_URL`.
- **Authentication**: Fixed issues in token validation and improved error handling for authentication failures.

### 🔧 Improvements
- **JWT Validation**: Strengthened the JWT validation logic and improved the token extraction process.
- **Logging**: Added detailed logging for file fetching operations to improve traceability.
- **Tool Definitions**: Clarified the usage of `file_paths` vs. `github_files` in the `GeminiAskTool` to prevent confusion.

### ⚙️ Internal Changes
- **Dependency Cleanup**: Removed the unused `testify` dependency.