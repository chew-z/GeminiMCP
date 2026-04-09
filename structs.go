package main

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

// PromptDefinition defines the structure for a prompt with its system prompt
type PromptDefinition struct {
	*mcp.Prompt
	SystemPrompt SystemPromptProvider
}

// SystemPromptProvider is an interface for providing system prompts
type SystemPromptProvider interface {
	GetSystemPrompt() string
}

// StaticSystemPrompt provides a fixed system prompt
type StaticSystemPrompt string

// GetSystemPrompt returns the system prompt
func (s StaticSystemPrompt) GetSystemPrompt() string {
	return string(s)
}

// NewPromptDefinition creates a new prompt definition
func NewPromptDefinition(name, description string, systemPrompt string) *PromptDefinition {
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
				{
					Name:        "model",
					Description: "Optional: Specific Gemini model to use (supports auto-completion).",
				},
				{
					Name:        "thinking_level",
					Description: "Optional: Thinking level — minimal, low, medium, or high (supports auto-completion).",
				},
			},
		},
		SystemPrompt: StaticSystemPrompt(systemPrompt),
	}
}

// GeminiServer implements the ToolHandler interface for Gemini API interactions
type GeminiServer struct {
	config *Config
	client *genai.Client
}

// SearchResponse is the JSON response format for the gemini_search tool
type SearchResponse struct {
	Answer        string       `json:"answer"`
	Sources       []SourceInfo `json:"sources,omitempty"`
	SearchQueries []string     `json:"search_queries,omitempty"`
}

// SourceInfo represents a source from search results
type SourceInfo struct {
	Title string `json:"title"`
	URL   string `json:"-"` // "web" or "retrieved_context"
	Type  string `json:"type"`
}

// Config holds all configuration parameters for the application
type Config struct {
	// Gemini API settings
	GeminiAPIKey             string
	GeminiModel              string // Default model for 'gemini_ask'
	GeminiSearchModel        string // Default model for 'gemini_search'
	GeminiSystemPrompt       string
	GeminiSearchSystemPrompt string
	GeminiTemperature        float64

	// HTTP client settings
	HTTPTimeout time.Duration

	// HTTP transport settings
	EnableHTTP      bool          // Enable HTTP transport
	HTTPAddress     string        // Server address (default: ":8080")
	HTTPPath        string        // Base path (default: "/mcp")
	HTTPStateless   bool          // Stateless mode
	HTTPHeartbeat   time.Duration // Heartbeat interval
	HTTPCORSEnabled bool          // Enable CORS
	HTTPCORSOrigins []string      // Allowed origins

	// Authentication settings
	AuthEnabled   bool   // Enable JWT authentication for HTTP transport
	AuthSecretKey string // Secret key for JWT signing and verification

	// Retry settings
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration

	// File handling settings
	MaxFileSize     int64  // Max file size in bytes
	FileReadBaseDir string // Base directory for local file reads

	// GitHub settings
	GitHubToken       string // Token for private repo access
	GitHubAPIBaseURL  string // For GitHub Enterprise
	MaxGitHubFiles    int    // Max number of files per call
	MaxGitHubFileSize int64  // Max size per file in bytes

	// Thinking settings
	EnableThinking      bool   // Enable/disable thinking mode for supported models
	ThinkingLevel       string // Thinking level for gemini_ask: minimal, low, medium, high
	SearchThinkingLevel string // Thinking level for gemini_search: minimal, low, medium, high

	// Service tier settings
	ServiceTier string // Service tier: flex, standard, priority (default: standard)
}

// ModelVersion represents an actual API-addressable Gemini model
type ModelVersion struct {
	ID          string `json:"id"`           // The version ID used by the API (e.g., "gemini-2.5-pro-exp-03-25")
	IsPreferred bool   `json:"is_preferred"` // Whether this is the preferred version of the model family
}

// GeminiModelInfo represents a family of related models
type GeminiModelInfo struct {
	FamilyID          string         `json:"family_id"`           // Model family identifier (e.g., "gemini-2.5-pro")
	Name              string         `json:"name"`                // Human-readable family name
	Description       string         `json:"description"`         // Description of the model family
	SupportsThinking  bool           `json:"supports_thinking"`   // Whether this model family supports thinking mode
	ContextWindowSize int            `json:"context_window_size"` // Maximum context window size in tokens
	Versions          []ModelVersion `json:"versions"`            // Available versions of this model family
}

// FileUploadRequest represents a request to upload a file
type FileUploadRequest struct {
	FileName    string `json:"filename"`
	MimeType    string `json:"mime_type"`
	Content     []byte `json:"content"`
	DisplayName string `json:"display_name,omitempty"`
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
