package main

import (
	"context"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

// GeminiServer implements the ToolHandler interface for Gemini API interactions
type GeminiServer struct {
	config     *Config
	client     *genai.Client
	fileStore  *FileStore
	cacheStore *CacheStore
	mutex      sync.Mutex // Added to protect client access
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
	URL   string `json:"url"`
	Type  string `json:"type"` // "web" or "retrieved_context"
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

	// Retry settings
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration

	// File handling settings
	MaxFileSize      int64    // Max file size in bytes
	AllowedFileTypes []string // Allowed MIME types

	// Cache settings
	EnableCaching   bool          // Enable/disable caching
	DefaultCacheTTL time.Duration // Default TTL if not specified

	// Thinking settings
	EnableThinking      bool   // Enable/disable thinking mode for supported models
	ThinkingBudget      int    // Maximum number of tokens to allocate for thinking
	ThinkingBudgetLevel string // Thinking budget level (none, low, medium, high)
}

// CacheRequest represents a request to create a cached context
type CacheRequest struct {
	Model        string   `json:"model"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	FileIDs      []string `json:"file_ids,omitempty"`
	Content      string   `json:"content,omitempty"`
	TTL          string   `json:"ttl,omitempty"` // Duration like "1h", "24h", etc.
	DisplayName  string   `json:"display_name,omitempty"`
}

// CacheInfo represents information about a cached context
type CacheInfo struct {
	ID          string    `json:"id"`   // The unique ID (last part of the Name)
	Name        string    `json:"name"` // The full resource name
	DisplayName string    `json:"display_name"`
	Model       string    `json:"model"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	FileIDs     []string  `json:"file_ids,omitempty"`
}

// CacheStore manages cache metadata
type CacheStore struct {
	client    *genai.Client
	config    *Config
	fileStore *FileStore
	mu        sync.RWMutex
	cacheInfo map[string]*CacheInfo // Map of ID -> CacheInfo
}

// GeminiModelInfo holds information about a Gemini model
type GeminiModelInfo struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	SupportsCaching   bool   `json:"supports_caching"`    // Whether this model supports caching
	SupportsThinking  bool   `json:"supports_thinking"`   // Whether this model supports thinking mode
	ContextWindowSize int    `json:"context_window_size"` // Maximum context window size in tokens
}

// FileUploadRequest represents a request to upload a file
type FileUploadRequest struct {
	FileName    string `json:"filename"`
	MimeType    string `json:"mime_type"`
	Content     []byte `json:"content"`
	DisplayName string `json:"display_name,omitempty"`
}

// FileInfo represents information about a stored file
type FileInfo struct {
	ID          string    `json:"id"`           // The unique ID (last part of the Name)
	Name        string    `json:"name"`         // The full resource name (e.g., "files/abc123")
	URI         string    `json:"uri"`          // The URI to use in requests
	DisplayName string    `json:"display_name"` // Human-readable name
	MimeType    string    `json:"mime_type"`
	Size        int64     `json:"size"`
	UploadedAt  time.Time `json:"uploaded_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// FileStore manages file metadata
type FileStore struct {
	client   *genai.Client
	config   *Config
	mu       sync.RWMutex
	fileInfo map[string]*FileInfo // Map of ID -> FileInfo
}

// GeminiServer implements the ToolHandler interface to provide research capabilities
// through Google's Gemini API.
// Defined in gemini.go

// ErrorGeminiServer implements the ToolHandler interface but returns error responses
// for all calls. Used when the Gemini server is in degraded mode due to initialization errors.
type ErrorGeminiServer struct {
	errorMessage string
	config       *Config // Added to check EnableCaching
}

// ListTools implements the ToolHandler interface for ErrorGeminiServer
// Returns the same tool signature as the normal Gemini server but in error mode
func (s *ErrorGeminiServer) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	tools := []mcp.Tool{
		mcp.NewTool(
			"gemini_ask",
			mcp.WithDescription("Use Google's Gemini AI model to ask about complex coding problems"),
			mcp.WithString("query", mcp.Required(), mcp.Description("The coding problem that we are asking Gemini AI to work on [question + code]")),
			mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
			mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
			mcp.WithArray("file_paths", mcp.Description("Optional: Paths to files to include in the request context")),
			mcp.WithBoolean("use_cache", mcp.Description("Optional: Whether to try using a cache for this request (only works with compatible models)")),
			mcp.WithString("cache_ttl", mcp.Description("Optional: TTL for cache if created (e.g., '10m', '1h'). Default is 10 minutes")),
			mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process (only works with Pro models)")),
			mcp.WithNumber("thinking_budget", mcp.Description("Optional: Maximum number of tokens to allocate for the model's thinking process (0-24576)")),
			mcp.WithString("thinking_budget_level", mcp.Description("Optional: Predefined thinking budget level (none, low, medium, high)")),
			mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
		),
		mcp.NewTool(
			"gemini_search",
			mcp.WithDescription("Use Google's Gemini AI model with Google Search to answer questions with grounded information"),
			mcp.WithString("query", mcp.Required(), mcp.Description("The question to ask Gemini using Google Search for grounding")),
			mcp.WithString("systemPrompt", mcp.Description("Optional: Custom system prompt to use for this request (overrides default configuration)")),
			mcp.WithBoolean("enable_thinking", mcp.Description("Optional: Enable thinking mode to see model's reasoning process (when supported)")),
			mcp.WithNumber("thinking_budget", mcp.Description("Optional: Maximum number of tokens to allocate for the model's thinking process (0-24576)")),
			mcp.WithString("thinking_budget_level", mcp.Description("Optional: Predefined thinking budget level (none, low, medium, high)")),
			mcp.WithNumber("max_tokens", mcp.Description("Optional: Maximum token limit for the response. Default is determined by the model")),
			mcp.WithString("model", mcp.Description("Optional: Specific Gemini model to use (overrides default configuration)")),
		),
		mcp.NewTool(
			"gemini_models",
			mcp.WithDescription("List available Gemini models with descriptions"),
		),
	}

	return tools, nil
}

// CallTool implements the ToolHandler interface for ErrorGeminiServer
// Always returns the initialization error regardless of which tool is called
func (s *ErrorGeminiServer) CallTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Log which tool was attempted, if a logger is in the context
	if loggerValue := ctx.Value(loggerKey); loggerValue != nil {
		if logger, ok := loggerValue.(Logger); ok {
			logger.Info("Tool '%s' called in error mode", req.Params.Name)
		}
	}

	// Return the same error message regardless of which tool is called
	return mcp.NewToolResultError(s.errorMessage), nil
}

// handleErrorResponse is a handler function that can be used with mark3labs/mcp-go's AddTool
func (s *ErrorGeminiServer) handleErrorResponse(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Log which tool was attempted, if a logger is in the context
	if loggerValue := ctx.Value(loggerKey); loggerValue != nil {
		if logger, ok := loggerValue.(Logger); ok {
			logger.Info("Tool '%s' called in error mode", req.Params.Name)
		}
	}

	// Return the same error message regardless of which tool is called
	return mcp.NewToolResultError(s.errorMessage), nil
}
