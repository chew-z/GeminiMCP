package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gomcpgo/mcp/pkg/protocol"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// GeminiServer implements the ToolHandler interface for Gemini API interactions
type GeminiServer struct {
	config     *Config
	client     *genai.Client
	fileStore  *FileStore
	cacheStore *CacheStore
}

// NewGeminiServer creates a new GeminiServer with the provided configuration
func NewGeminiServer(ctx context.Context, config *Config) (*GeminiServer, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	if config.GeminiAPIKey == "" {
		return nil, errors.New("Gemini API key is required")
	}

	// Initialize the Gemini client
	client, err := genai.NewClient(ctx, option.WithAPIKey(config.GeminiAPIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	// Create the file and cache stores
	fileStore := NewFileStore(client, config)
	cacheStore := NewCacheStore(client, config, fileStore)

	return &GeminiServer{
		config:     config,
		client:     client,
		fileStore:  fileStore,
		cacheStore: cacheStore,
	}, nil
}

// Close closes the Gemini client connection
func (s *GeminiServer) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

// ListTools implements the ToolHandler interface for GeminiServer
func (s *GeminiServer) ListTools(ctx context.Context) (*protocol.ListToolsResponse, error) {
	tools := []protocol.Tool{
		{
			Name:        "gemini_ask",
			Description: "Use Google's Gemini AI model to ask about complex coding problems",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "The coding problem that we are asking Gemini AI to work on [question + code]"
					},
					"model": {
						"type": "string",
						"description": "Optional: Specific Gemini model to use (overrides default configuration)"
					},
					"systemPrompt": {
						"type": "string",
						"description": "Optional: Custom system prompt to use for this request (overrides default configuration)"
					}
				},
				"required": ["query"]
			}`),
		},
		{
			Name:        "gemini_models",
			Description: "List available Gemini models with descriptions",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {},
				"required": []
			}`),
		},
		{
			Name:        "gemini_upload_file",
			Description: "Upload a file to Gemini for processing",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"filename": {"type": "string", "description": "Name of the file"},
					"mime_type": {"type": "string", "description": "MIME type of the file"},
					"content": {"type": "string", "description": "Base64-encoded file content"},
					"display_name": {"type": "string", "description": "Optional human-readable name for the file"}
				},
				"required": ["filename", "mime_type", "content"]
			}`),
		},
		{
			Name:        "gemini_list_files",
			Description: "List all uploaded files",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {},
				"required": []
			}`),
		},
		{
			Name:        "gemini_delete_file",
			Description: "Delete an uploaded file",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_id": {"type": "string", "description": "ID of the file to delete"}
				},
				"required": ["file_id"]
			}`),
		},
	}

	// Add cache tools if caching is enabled
	if s.config.EnableCaching {
		tools = append(tools, []protocol.Tool{
			{
				Name:        "gemini_create_cache",
				Description: "Create a cached context for repeated queries",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"model": {"type": "string", "description": "Gemini model to use"},
						"system_prompt": {"type": "string", "description": "Optional system prompt for the context"},
						"file_ids": {"type": "array", "items": {"type": "string"}, "description": "Optional IDs of files to include in the context"},
						"content": {"type": "string", "description": "Optional text content to include in the context"},
						"ttl": {"type": "string", "description": "Optional time-to-live for the cache (e.g. '1h', '24h')"},
						"display_name": {"type": "string", "description": "Optional human-readable name for the cache"}
					},
					"required": ["model"]
				}`),
			},
			{
				Name:        "gemini_query_with_cache",
				Description: "Query Gemini using a cached context",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"cache_id": {"type": "string", "description": "ID of the cache to use"},
						"query": {"type": "string", "description": "Query to send to Gemini"}
					},
					"required": ["cache_id", "query"]
				}`),
			},
			{
				Name:        "gemini_list_caches",
				Description: "List all cached contexts",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {},
					"required": []
				}`),
			},
			{
				Name:        "gemini_delete_cache",
				Description: "Delete a cached context",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"cache_id": {"type": "string", "description": "ID of the cache to delete"}
					},
					"required": ["cache_id"]
				}`),
			},
		}...)
	}

	return &protocol.ListToolsResponse{
		Tools: tools,
	}, nil
}

// getLoggerFromContext safely extracts a logger from the context or creates a new one
func getLoggerFromContext(ctx context.Context) Logger {
	loggerValue := ctx.Value(loggerKey)
	if loggerValue != nil {
		if l, ok := loggerValue.(Logger); ok {
			return l
		}
	}
	// Create a new logger if one isn't in the context or type assertion fails
	return NewLogger(LevelInfo)
}

// createErrorResponse creates a standardized error response
func createErrorResponse(message string) *protocol.CallToolResponse {
	return &protocol.CallToolResponse{
		IsError: true,
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: message,
			},
		},
	}
}

// CallTool implements the ToolHandler interface for GeminiServer
func (s *GeminiServer) CallTool(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	// No need to get logger here as it's not used in this method
	switch req.Name {
	case "gemini_ask":
		return s.handleAskGemini(ctx, req)
	case "gemini_models":
		return s.handleGeminiModels(ctx)
	case "gemini_upload_file":
		return s.handleUploadFile(ctx, req)
	case "gemini_list_files":
		return s.handleListFiles(ctx)
	case "gemini_delete_file":
		return s.handleDeleteFile(ctx, req)
	case "gemini_create_cache":
		return s.handleCreateCache(ctx, req)
	case "gemini_query_with_cache":
		return s.handleQueryWithCache(ctx, req)
	case "gemini_list_caches":
		return s.handleListCaches(ctx)
	case "gemini_delete_cache":
		return s.handleDeleteCache(ctx, req)
	default:
		return createErrorResponse(fmt.Sprintf("unknown tool: %s", req.Name)), nil
	}
}

// handleAskGemini handles requests to the ask_gemini tool
func (s *GeminiServer) handleAskGemini(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)

	// Extract and validate query parameter (required)
	query, ok := req.Arguments["query"].(string)
	if !ok {
		return createErrorResponse("query must be a string"), nil
	}

	// Extract optional model parameter
	modelName := s.config.GeminiModel
	if customModel, ok := req.Arguments["model"].(string); ok && customModel != "" {
		// Validate the custom model
		if err := ValidateModelID(customModel); err != nil {
			logger.Error("Invalid model requested: %v", err)
			return createErrorResponse(fmt.Sprintf("Invalid model specified: %v", err)), nil
		}
		logger.Info("Using request-specific model: %s", customModel)
		modelName = customModel
	}

	// Extract optional systemPrompt parameter
	systemPrompt := s.config.GeminiSystemPrompt
	if customPrompt, ok := req.Arguments["systemPrompt"].(string); ok && customPrompt != "" {
		logger.Info("Using request-specific system prompt")
		systemPrompt = customPrompt
	}

	// Create Gemini model with configuration
	model := s.client.GenerativeModel(modelName)
	model.SystemInstruction = genai.NewUserContent(genai.Text(systemPrompt))

	// Use the configured temperature
	model.SetTemperature(float32(s.config.GeminiTemperature))
	logger.Debug("Using temperature: %v for model %s", s.config.GeminiTemperature, modelName)

	// Send request to Gemini API
	response, err := s.executeGeminiRequest(ctx, model, query)
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		return createErrorResponse(fmt.Sprintf("error from Gemini API: %v", err)), nil
	}

	return s.formatResponse(response), nil
}

// handleGeminiModels handles requests to the gemini_models tool
func (s *GeminiServer) handleGeminiModels(ctx context.Context) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Listing available Gemini models")

	// Get available models
	models := GetAvailableGeminiModels()

	// Create a formatted response using strings.Builder with error handling
	var formattedContent strings.Builder

	// Define a helper function to write with error checking
	writeStringf := func(format string, args ...interface{}) error {
		_, err := formattedContent.WriteString(fmt.Sprintf(format, args...))
		return err
	}

	// Write the header
	if err := writeStringf("# Available Gemini Models\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	// Write each model's information
	for _, model := range models {
		if err := writeStringf("## %s\n", model.Name); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		if err := writeStringf("- ID: `%s`\n", model.ID); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		if err := writeStringf("- Description: %s\n\n", model.Description); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}
	}

	// Add usage hint
	if err := writeStringf("## Usage\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("You can specify a model ID in the `model` parameter when using the `ask_gemini` tool:\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("```json\n{\n  \"query\": \"Your question here\",\n  \"model\": \"gemini-2.5-pro-exp-03-25\"\n}\n```\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: formattedContent.String(),
			},
		},
	}, nil
}

// handleUploadFile handles requests to the upload_file tool
func (s *GeminiServer) handleUploadFile(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling file upload request")

	// Extract and validate required parameters
	filename, ok := req.Arguments["filename"].(string)
	if !ok || filename == "" {
		return createErrorResponse("filename must be a non-empty string"), nil
	}

	mimeType, ok := req.Arguments["mime_type"].(string)
	if !ok || mimeType == "" {
		return createErrorResponse("mime_type must be a non-empty string"), nil
	}

	contentBase64, ok := req.Arguments["content"].(string)
	if !ok || contentBase64 == "" {
		return createErrorResponse("content must be a non-empty base64-encoded string"), nil
	}

	// Get optional display name
	displayName, _ := req.Arguments["display_name"].(string)

	// Decode base64 content
	content, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		logger.Error("Failed to decode base64 content: %v", err)
		return createErrorResponse("invalid base64 encoding for content"), nil
	}

	// Create upload request
	uploadReq := &FileUploadRequest{
		FileName:    filename,
		MimeType:    mimeType,
		Content:     content,
		DisplayName: displayName,
	}

	// Upload the file
	fileInfo, err := s.fileStore.UploadFile(ctx, uploadReq)
	if err != nil {
		logger.Error("Failed to upload file: %v", err)
		return createErrorResponse(fmt.Sprintf("failed to upload file: %v", err)), nil
	}

	// Format the response
	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: fmt.Sprintf("File uploaded successfully:\n\n- File ID: `%s`\n- Name: %s\n- Size: %s\n- MIME Type: %s\n\nUse this File ID when creating a cache context.",
					fileInfo.ID, fileInfo.DisplayName, humanReadableSize(fileInfo.Size), fileInfo.MimeType),
			},
		},
	}, nil
}

func (s *GeminiServer) handleListFiles(ctx context.Context) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling list files request")

	// Get files
	files, err := s.fileStore.ListFiles(ctx)
	if err != nil {
		logger.Error("Failed to list files: %v", err)
		return createErrorResponse(fmt.Sprintf("failed to list files: %v", err)), nil
	}

	// Format the response
	var sb strings.Builder
	sb.WriteString("# Uploaded Files\n\n")

	if len(files) == 0 {
		sb.WriteString("No files found.")
	} else {
		sb.WriteString("| ID | Name | MIME Type | Size | Upload Time |\n")
		sb.WriteString("|-----|-------|-----------|------|-------------|\n")

		for _, file := range files {
			displayName := file.DisplayName
			if displayName == "" {
				displayName = file.Name
			}

			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
				file.ID,
				displayName,
				file.MimeType,
				humanReadableSize(file.Size),
				file.UploadedAt.Format(time.RFC3339),
			))
		}
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: sb.String(),
			},
		},
	}, nil
}

func (s *GeminiServer) handleDeleteFile(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling delete file request")

	// Extract and validate required parameters
	fileID, ok := req.Arguments["file_id"].(string)
	if !ok || fileID == "" {
		return createErrorResponse("file_id must be a non-empty string"), nil
	}

	// Delete the file
	if err := s.fileStore.DeleteFile(ctx, fileID); err != nil {
		logger.Error("Failed to delete file: %v", err)
		return createErrorResponse(fmt.Sprintf("failed to delete file: %v", err)), nil
	}

	// Format the response
	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: fmt.Sprintf("File with ID `%s` was successfully deleted.", fileID),
			},
		},
	}, nil
}

func (s *GeminiServer) handleCreateCache(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling create cache request")

	// Check if caching is enabled
	if !s.config.EnableCaching {
		return createErrorResponse("caching is disabled"), nil
	}

	// Extract and validate required parameters
	model, ok := req.Arguments["model"].(string)
	if !ok || model == "" {
		return createErrorResponse("model must be a non-empty string"), nil
	}

	// Extract optional parameters
	systemPrompt, _ := req.Arguments["system_prompt"].(string)
	contentBase64, _ := req.Arguments["content"].(string)
	ttl, _ := req.Arguments["ttl"].(string)
	displayName, _ := req.Arguments["display_name"].(string)

	// Decode base64 content if provided
	var content []byte
	var err error
	if contentBase64 != "" {
		content, err = base64.StdEncoding.DecodeString(contentBase64)
		if err != nil {
			logger.Error("Failed to decode base64 content: %v", err)
			return createErrorResponse("invalid base64 encoding for content"), nil
		}
	}

	// Extract file IDs if provided
	var fileIDs []string
	if fileIDsRaw, ok := req.Arguments["file_ids"]; ok {
		if fileIDsList, ok := fileIDsRaw.([]interface{}); ok {
			for _, fileIDRaw := range fileIDsList {
				if fileID, ok := fileIDRaw.(string); ok {
					fileIDs = append(fileIDs, fileID)
				}
			}
		}
	}

	// Validate that either content or file IDs are provided
	if len(content) == 0 && len(fileIDs) == 0 {
		return createErrorResponse("either content or file_ids must be provided"), nil
	}

	// Create cache request
	cacheReq := &CacheRequest{
		Model:        model,
		SystemPrompt: systemPrompt,
		FileIDs:      fileIDs,
		Content:      string(content), // Convert []byte to string
		TTL:          ttl,
		DisplayName:  displayName,
	}

	// Create the cache
	cacheInfo, err := s.cacheStore.CreateCache(ctx, cacheReq)
	if err != nil {
		logger.Error("Failed to create cache: %v", err)
		return createErrorResponse(fmt.Sprintf("failed to create cache: %v", err)), nil
	}

	// Format the response
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Cache created successfully:\n\n"))
	sb.WriteString(fmt.Sprintf("- Cache ID: `%s`\n", cacheInfo.ID))

	if cacheInfo.DisplayName != "" {
		sb.WriteString(fmt.Sprintf("- Name: %s\n", cacheInfo.DisplayName))
	}

	sb.WriteString(fmt.Sprintf("- Model: %s\n", cacheInfo.Model))
	sb.WriteString(fmt.Sprintf("- Created: %s\n", cacheInfo.CreatedAt.Format(time.RFC3339)))

	if !cacheInfo.ExpiresAt.IsZero() {
		sb.WriteString(fmt.Sprintf("- Expires: %s\n", cacheInfo.ExpiresAt.Format(time.RFC3339)))
	}

	if len(fileIDs) > 0 {
		sb.WriteString("\nIncluded files:\n")
		for _, fileID := range fileIDs {
			sb.WriteString(fmt.Sprintf("- `%s`\n", fileID))
		}
	}

	sb.WriteString("\nUse this Cache ID with the `query_with_cache` tool to perform queries using this cached context.")

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: sb.String(),
			},
		},
	}, nil
}

func (s *GeminiServer) handleQueryWithCache(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling query with cache request")

	// Check if caching is enabled
	if !s.config.EnableCaching {
		return createErrorResponse("caching is disabled"), nil
	}

	// Extract and validate required parameters
	cacheID, ok := req.Arguments["cache_id"].(string)
	if !ok || cacheID == "" {
		return createErrorResponse("cache_id must be a non-empty string"), nil
	}

	query, ok := req.Arguments["query"].(string)
	if !ok || query == "" {
		return createErrorResponse("query must be a non-empty string"), nil
	}

	// Get cache info
	cacheInfo, err := s.cacheStore.GetCache(ctx, cacheID)
	if err != nil {
		logger.Error("Failed to get cache info: %v", err)
		return createErrorResponse(fmt.Sprintf("failed to get cache: %v", err)), nil
	}

	// Create Gemini model with the cached content
	name := cacheInfo.Name
	if !strings.HasPrefix(name, "cachedContents/") {
		name = "cachedContents/" + name
	}

	model := s.client.GenerativeModel(cacheInfo.Model)
	model.CachedContentName = name

	// Set the temperature from config
	model.SetTemperature(float32(s.config.GeminiTemperature))

	// Send the query
	response, err := s.executeGeminiRequest(ctx, model, query)
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		return createErrorResponse(fmt.Sprintf("error from Gemini API: %v", err)), nil
	}

	return s.formatResponse(response), nil
}

func (s *GeminiServer) handleListCaches(ctx context.Context) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling list caches request")

	// Check if caching is enabled
	if !s.config.EnableCaching {
		return createErrorResponse("caching is disabled"), nil
	}

	// Get caches
	caches, err := s.cacheStore.ListCaches(ctx)
	if err != nil {
		logger.Error("Failed to list caches: %v", err)
		return createErrorResponse(fmt.Sprintf("failed to list caches: %v", err)), nil
	}

	// Format the response
	var sb strings.Builder
	sb.WriteString("# Cached Contexts\n\n")

	if len(caches) == 0 {
		sb.WriteString("No cached contexts found.")
	} else {
		sb.WriteString("| ID | Name | Model | Created | Expires |\n")
		sb.WriteString("|-----|------|-------|---------|----------|\n")

		for _, cache := range caches {
			displayName := cache.DisplayName
			if displayName == "" {
				displayName = cache.ID
			}

			expires := "Never"
			if !cache.ExpiresAt.IsZero() {
				expires = cache.ExpiresAt.Format(time.RFC3339)
			}

			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
				cache.ID,
				displayName,
				cache.Model,
				cache.CreatedAt.Format(time.RFC3339),
				expires,
			))
		}
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: sb.String(),
			},
		},
	}, nil
}

func (s *GeminiServer) handleDeleteCache(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling delete cache request")

	// Check if caching is enabled
	if !s.config.EnableCaching {
		return createErrorResponse("caching is disabled"), nil
	}

	// Extract and validate required parameters
	cacheID, ok := req.Arguments["cache_id"].(string)
	if !ok || cacheID == "" {
		return createErrorResponse("cache_id must be a non-empty string"), nil
	}

	// Delete the cache
	if err := s.cacheStore.DeleteCache(ctx, cacheID); err != nil {
		logger.Error("Failed to delete cache: %v", err)
		return createErrorResponse(fmt.Sprintf("failed to delete cache: %v", err)), nil
	}

	// Format the response
	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: fmt.Sprintf("Cache with ID `%s` was successfully deleted.", cacheID),
			},
		},
	}, nil
}

// executeGeminiRequest makes the request to the Gemini API with retry capability
func (s *GeminiServer) executeGeminiRequest(ctx context.Context, model *genai.GenerativeModel, query string) (*genai.GenerateContentResponse, error) {
	logger := getLoggerFromContext(ctx)

	var response *genai.GenerateContentResponse

	// Define the operation to retry
	operation := func() error {
		var err error
		// Set timeout context for the API call
		timeoutCtx, cancel := context.WithTimeout(ctx, s.config.HTTPTimeout)
		defer cancel()

		response, err = model.GenerateContent(timeoutCtx, genai.Text(query))
		if err != nil {
			// Check specifically for timeout errors
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("request timed out after %v: consider increasing GEMINI_TIMEOUT: %w", s.config.HTTPTimeout, err)
			}

			// Handle other types of errors
			return fmt.Errorf("failed to generate content: %w", err)
		}

		// Check for empty response
		if response == nil || len(response.Candidates) == 0 {
			return errors.New("no response candidates returned from Gemini API")
		}

		return nil
	}

	// Execute the operation with retry logic
	err := RetryWithBackoff(
		ctx,
		s.config.MaxRetries,
		s.config.InitialBackoff,
		s.config.MaxBackoff,
		operation,
		IsTimeoutError, // Using the IsTimeoutError from retry.go
		logger,
	)

	if err != nil {
		return nil, err
	}

	return response, nil
}

// formatResponse formats the Gemini API response
func (s *GeminiServer) formatResponse(resp *genai.GenerateContentResponse) *protocol.CallToolResponse {
	var content string

	// Extract text from the response
	for _, candidate := range resp.Candidates {
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				// Use type assertion for text parts
				if textPart, ok := part.(genai.Text); ok {
					content += string(textPart)
				}
			}
		}
	}

	// Check for empty content and provide a fallback message
	if content == "" {
		content = "The Gemini model returned an empty response. This might indicate that the model couldn't generate an appropriate response for your query. Please try rephrasing your question or providing more context."
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: content,
			},
		},
	}
}
