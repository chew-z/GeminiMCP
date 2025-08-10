### FINAL PROPOSED PLAN

**Objective:** To refactor the `gemini_ask` tool to securely access file content via the GitHub API. This will make the tool safe for remote use over HTTP, provide intelligent defaults for local development, and clearly separate remote fetching from local file access.

---

**Phase 1: Configuration & Startup Integration**

1.  **Update `structs.go` (Configuration):**
    *   The `Config` struct will be expanded with the following fields:
        *   `GitHubToken string`: For authenticating to the GitHub API. Loaded from the `GEMINI_GITHUB_TOKEN` environment variable.
        *   `GitHubAPIBaseURL string`: Optional, for GitHub Enterprise support. Defaults to `https://api.github.com`.
        *   `DefaultGitHubRepo string`: The `owner/repo` of the project, discovered at startup.
        *   `DefaultGitHubRef string`: The default branch name, discovered at startup.
        *   `MaxGitHubFiles int`: The maximum number of files allowed in a single API call (e.g., 20).
        *   `MaxGitHubFileSize int64`: The maximum size for a single file in bytes (e.g., 1MB).

2.  **Update `main.go` (Git Discovery):**
    *   At startup, the server will execute `git` commands to auto-detect the repository context:
        *   It will get the remote URL and parse it to an `owner/repo` format, storing it in `config.DefaultGitHubRepo`. It will correctly handle both HTTPS and SSH URLs.
        *   It will get the default branch name (e.g., `main`) and store it in `config.DefaultGitHubRef`.
    *   This functionality will depend on `git` being installed in the server's environment. If not in a git repo, these values will be empty, and the tool will require them to be provided explicitly.

---

**Phase 2: Tool Logic & Secure Fetching**

3.  **Update `tools.go` (Tool Definition):**
    *   The `gemini_ask` tool schema will be updated:
        *   **New Arguments:** `github_repo` (string), `github_ref` (string), and `github_files` (array of strings) will be added as optional parameters.
        *   **Deprecation:** The description for the existing `file_paths` argument will be updated to mark it as deprecated and for `stdio` (local) use only.

4.  **Implement GitHub Fetcher (`direct_handlers.go`):**
    *   A new, internal function `fetchFromGitHub(...)` will be created.
    *   **Functionality:** It will concurrently fetch up to `MaxGitHubFiles` from the GitHub Contents API using `GET /repos/{owner}/{repo}/contents/{path}`.
    *   **Security:** It will use the `GEMINI_GITHUB_TOKEN` for authentication if provided. It will validate all file paths to prevent directory traversal (`../`) and enforce size limits.
    *   **Error Handling:** It will gracefully handle GitHub API errors, including "Not Found" (404), and provide clear messages for rate-limiting issues.

5.  **Refactor `GeminiAskHandler` (`direct_handlers.go`):**
    *   The handler's core logic will be rewritten to enforce a strict separation of concerns:
        *   **GitHub First:** If `github_files` are provided, the handler will use the new `fetchFromGitHub` function. It will use the auto-detected repo and branch as defaults if the arguments are omitted.
        *   **`file_paths` Restriction:** If `file_paths` are provided:
            *   The handler will first check the transport mode. If it's `http`, the request will be **rejected** with an error explaining that `github_files` must be used.
            *   If the transport is `stdio`, it will proceed to read the local files.
        *   It will be an error to provide both `github_files` and `file_paths` in the same call.

6.  **Unify Core Logic (`direct_handlers.go`):**
    *   The functions `processWithFiles` and `createCacheFromFiles` will be modified to accept a slice of `FileUploadRequest` structs (`[]*FileUploadRequest`). This makes them source-agnostic, able to process data whether it comes from GitHub or the local filesystem, completing the refactor.

---

**Phase 3: Documentation & Verification**

7.  **Update `README.md`:**
    *   The documentation will be thoroughly updated to explain the new `github_files` workflow as the primary, recommended method for providing files.
    *   All tool examples will be updated to use `github_files`.
    *   The `GEMINI_GITHUB_TOKEN` environment variable and its purpose will be clearly documented.

8.  **Implement New Tests:**
    *   Unit tests will be added for the `fetchFromGitHub` function, using a mocked HTTP client to simulate various GitHub API responses (success, file not found, rate limit error).
    *   Handler-level tests will be added to verify the new security rules, such as the rejection of `file_paths` over HTTP and the correct use of default repository information.

---