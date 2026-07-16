package main

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// PromptDefinition defines the structure for a prompt.
//
// HandlerFactory is optional. When nil, the default generic handler
// (promptHandler) is used — which expects a "problem_statement" argument and
// emits a gemini_ask invocation. The server picks the system prompt via
// pre-qualification; clients cannot inject one. When non-nil, the factory is
// invoked with the live GeminiServer to produce a custom handler; this is the
// hook used by the github-workflow prompts (review_pr, explain_commit,
// compare_refs) that need bespoke arguments.
type PromptDefinition struct {
	*mcp.Prompt
	HandlerFactory func(s *GeminiServer) mcpPromptHandlerFunc
}

// mcpPromptHandlerFunc mirrors server.PromptHandlerFunc without pulling the
// import into structs.go. The real type alias lives in server_handlers.go.
type mcpPromptHandlerFunc func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error)

// NewPromptDefinition creates a new generic prompt definition. The server
// resolves the system prompt server-side via pre-qualification.
func NewPromptDefinition(name, description string) *PromptDefinition {
	return &PromptDefinition{
		Prompt: &mcp.Prompt{
			Name:        name,
			Description: description,
			Arguments: []mcp.PromptArgument{
				{
					Name:        "problem_statement",
					Description: "A clear and concise description of the programming problem or task.",
					Required:    true,
				},
			},
		},
	}
}

// GeminiServer implements the ToolHandler interface for Gemini API interactions
type GeminiServer struct {
	config   *Config
	provider Provider
}

// Config holds all configuration parameters for the application
type Config struct {
	// Gemini API settings
	GeminiAPIKey      string
	GeminiModel       string // Default model for 'gemini_ask'
	GeminiTemperature float64
	ProviderMaxTokens int32 // Max provider output tokens; 0 uses API default.

	// HTTP client settings
	HTTPTimeout time.Duration

	// HTTPWriteTimeout bounds the inbound HTTP server WriteTimeout independently
	// from the outbound per-call budget (HTTPTimeout). It must outlive the
	// outbound budget so a long tool call can deliver its response.
	HTTPWriteTimeout time.Duration

	// HTTP transport settings
	EnableHTTP      bool          // Enable HTTP transport
	HTTPAddress     string        // Server address (default: ":8080")
	HTTPPath        string        // Base path (default: "/mcp")
	HTTPStateless   bool          // Stateless mode
	HTTPHeartbeat   time.Duration // Heartbeat interval
	HTTPCORSEnabled bool          // Enable CORS
	HTTPCORSOrigins []string      // Allowed origins
	// HTTPPublicURL is the externally-facing resource identifier per RFC 9728.
	// Validated at startup: scheme must be https for any host, OR http only when
	// host is loopback (localhost, 127.0.0.1, [::1]). Empty → derive from request.
	HTTPPublicURL string
	// HTTPDisableLocalhostProtection disables mcp-go's DNS-rebinding guard for
	// loopback-bound deployments behind a trusted reverse proxy.
	HTTPDisableLocalhostProtection bool

	// Progress notification settings
	ProgressInterval time.Duration // Interval for notifications/progress; <=0 disables.

	// Task-augmented tool settings
	MaxConcurrentTasks int // Upper bound on concurrently-executing task tools. <=0 disables.

	// Authentication settings
	AuthEnabled   bool   // Enable JWT authentication for HTTP transport
	AuthSecretKey string // Secret key for JWT signing and verification

	// Retry settings
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration

	// GitHub settings
	GitHubToken               string // Token for private repo access
	GitHubAPIBaseURL          string // For GitHub Enterprise
	MaxGitHubFiles            int    // Max number of files per call
	MaxGitHubFileSize         int64  // Max size per file in bytes
	MaxGitHubDiffBytes        int64  // Max bytes of a single unified diff payload (PR / commit / compare)
	MaxGitHubCommits          int    // Max number of commits accepted via github_commits
	MaxGitHubPRReviewComments int    // Max number of PR review comments fetched

	// Pre-qualification settings
	Prequalify bool // Enable query pre-qualification for automatic system prompt selection
}

// FileUploadRequest represents a request to upload a file
type FileUploadRequest struct {
	FileName    string `json:"filename"`
	MimeType    string `json:"mime_type"`
	Content     []byte `json:"content"`
	DisplayName string `json:"display_name,omitempty"`
}

// contextInventory records which GitHub-sourced context blocks were attached
// to a gemini_ask request so the server can generate a small, deterministic
// descriptive addendum for the system prompt. It is intentionally descriptive
// only — it never injects task instructions.
type contextInventory struct {
	Repo     string // "owner/repo" (empty if no GitHub context was used)
	Files    fileInventory
	PR       *prInventory
	Commits  []commitInventory
	Diff     *diffInventory
	Warnings []string // non-fatal warnings accumulated while fetching
}

// fileInventory describes the file blocks attached to the request.
type fileInventory struct {
	Count int
	Ref   string
}

// prInventory describes a PR bundle attached to the request.
type prInventory struct {
	Number        int
	Title         string
	ReviewCount   int
	DiffTruncated bool
}

// commitInventory describes a single commit patch attached to the request.
type commitInventory struct {
	SHA       string
	Subject   string
	Truncated bool
}

// diffInventory describes a compare-refs diff attached to the request.
type diffInventory struct {
	Base      string
	Head      string
	Truncated bool
}

// HasAny reports whether any GitHub-sourced context was attached.
func (ci *contextInventory) HasAny() bool {
	if ci == nil {
		return false
	}
	return ci.Files.Count > 0 || ci.PR != nil || len(ci.Commits) > 0 || ci.Diff != nil
}

// GeminiServer implements the ToolHandler interface to provide research capabilities
// through Google's Gemini API.
// Defined in gemini.go

// ErrorGeminiServer implements the ToolHandler interface but returns error responses
// for all calls. Used when the Gemini server is in degraded mode due to initialization errors.
type ErrorGeminiServer struct {
	errorMessage string
}

// handleErrorResponse is a handler function that can be used with mark3labs/mcp-go's AddTool
func (s *ErrorGeminiServer) handleErrorResponse(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get logger from context
	logger := getLoggerFromContext(ctx)

	// Log which tool was attempted
	toolName := req.Params.Name
	logger.Info("Tool '%s' called in error mode", toolName)

	// Return an error result with the initialization error message
	// Include the tool name for better debugging
	errorMessage := fmt.Sprintf("Error in tool '%s': %s", toolName, s.errorMessage)
	return createErrorResult(errorMessage), nil
}
