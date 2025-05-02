package main

import (
	"context"
	"fmt"
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

// ModelVersion represents a specific version of a Gemini model
type ModelVersion struct {
	ID              string `json:"id"`               // The version ID (e.g., "gemini-2.0-flash-001")
	Name            string `json:"name"`             // Human-readable name of the version
	SupportsCaching bool   `json:"supports_caching"` // Whether this version supports caching
}

// GeminiModelInfo holds information about a Gemini model
type GeminiModelInfo struct {
	ID                   string         `json:"id"`                     // Base model ID
	Name                 string         `json:"name"`                   // Human-readable name
	Description          string         `json:"description"`            // Description of the model
	SupportsCaching      bool           `json:"supports_caching"`       // Whether this model version supports caching
	SupportsThinking     bool           `json:"supports_thinking"`      // Whether this model supports thinking mode
	ContextWindowSize    int            `json:"context_window_size"`    // Maximum context window size in tokens
	PreferredForThinking bool           `json:"preferred_for_thinking"` // Whether this model is preferred for thinking tasks
	PreferredForCaching  bool           `json:"preferred_for_caching"`  // Whether this model is preferred for repeated tasks with caching
	PreferredForSearch   bool           `json:"preferred_for_search"`   // Whether this model is preferred for search tasks
	Versions             []ModelVersion `json:"versions,omitempty"`     // Available versions of this model
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
// Note: This method is no longer needed since tools are registered directly
// in main.go using the shared definitions from tools.go
func (s *ErrorGeminiServer) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	tools := []mcp.Tool{
		GeminiAskTool,
		GeminiSearchTool,
		GeminiModelsTool,
	}
	return tools, nil
}

// CallTool implements the ToolHandler interface for ErrorGeminiServer
// Note: This method is no longer needed since handlers are registered directly
// in main.go. It's kept for potential backwards compatibility.
func (s *ErrorGeminiServer) CallTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Log which tool was attempted, if a logger is in the context
	if loggerValue := ctx.Value(loggerKey); loggerValue != nil {
		if logger, ok := loggerValue.(Logger); ok {
			logger.Info("Tool '%s' called in error mode via deprecated CallTool method", req.Params.Name)
		}
	}

	// Return the same error message regardless of which tool is called
	return mcp.NewToolResultError(s.errorMessage), nil
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
