package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gomcpgo/mcp/pkg/protocol"
	"google.golang.org/genai"
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
	clientConfig := &genai.ClientConfig{
		APIKey: config.GeminiAPIKey,
	}
	client, err := genai.NewClient(ctx, clientConfig)
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

// Close closes the Gemini client connection (client doesn't need to be closed in the new API)
func (s *GeminiServer) Close() {
	// No need to close the client in the new API
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
					},
					"file_paths": {
						"type": "array",
						"items": {
							"type": "string"
						},
						"description": "Optional: Paths to files to include in the request context"
					},
					"use_cache": {
						"type": "boolean",
						"description": "Optional: Whether to try using a cache for this request (only works with compatible models)"
					},
					"cache_ttl": {
						"type": "string",
						"description": "Optional: TTL for cache if created (e.g., '10m', '1h'). Default is 10 minutes"
					}
				},
				"required": ["query"]
			}`),
		},
		{
			Name:        "gemini_search",
			Description: "Use Google's Gemini AI model with Google Search to answer questions with grounded information",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "The question to ask Gemini using Google Search for grounding"
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
	switch req.Name {
	case "gemini_ask":
		return s.handleAskGemini(ctx, req)
	case "gemini_search":
		return s.handleGeminiSearch(ctx, req)
	case "gemini_models":
		return s.handleGeminiModels(ctx)
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

	// Extract file paths if provided
	var filePaths []string
	if filePathsRaw, ok := req.Arguments["file_paths"].([]interface{}); ok {
		for _, pathRaw := range filePathsRaw {
			if path, ok := pathRaw.(string); ok {
				filePaths = append(filePaths, path)
			}
		}
	}

	// Check if caching is requested
	useCache := false
	if useCacheRaw, ok := req.Arguments["use_cache"].(bool); ok {
		useCache = useCacheRaw
	}

	// Extract cache TTL if provided
	cacheTTL := ""
	if ttl, ok := req.Arguments["cache_ttl"].(string); ok {
		cacheTTL = ttl
	}

	// If caching is requested and the model supports it, use caching
	var cacheID string
	var cacheErr error
	if useCache && s.config.EnableCaching {
		// Check if model supports caching
		model := GetModelByID(modelName)
		if model != nil && model.SupportsCaching {
			// Create a cache context from file paths
			var fileIDs []string

			// Upload files if provided
			for _, filePath := range filePaths {
				// Read the file
				content, err := os.ReadFile(filePath)
				if err != nil {
					logger.Error("Failed to read file %s: %v", filePath, err)
					continue
				}

				// Get mime type from file path
				mimeType := getMimeTypeFromPath(filePath)

				// Get filename from path
				fileName := filepath.Base(filePath)

				// Create upload request
				uploadReq := &FileUploadRequest{
					FileName:    fileName,
					MimeType:    mimeType,
					Content:     content,
					DisplayName: fileName,
				}

				// Upload the file
				fileInfo, err := s.fileStore.UploadFile(ctx, uploadReq)
				if err != nil {
					logger.Error("Failed to upload file %s: %v", filePath, err)
					continue
				}

				// Add file ID to the list
				fileIDs = append(fileIDs, fileInfo.ID)
			}

			// Create cache request
			cacheReq := &CacheRequest{
				Model:        modelName,
				SystemPrompt: systemPrompt,
				FileIDs:      fileIDs,
				TTL:          cacheTTL,
			}

			// Create the cache
			cacheInfo, err := s.cacheStore.CreateCache(ctx, cacheReq)
			if err != nil {
				// Log the error but continue without caching
				logger.Warn("Failed to create cache, falling back to regular request: %v", err)
				cacheErr = err
			} else {
				cacheID = cacheInfo.ID
				logger.Info("Created cache with ID: %s", cacheID)
			}
		} else {
			// Model doesn't support caching, log warning and continue
			logger.Warn("Model %s does not support caching, falling back to regular request", modelName)
		}
	}

	// If we successfully created a cache, use it
	if cacheID != "" {
		return s.handleQueryWithCache(ctx, &protocol.CallToolRequest{
			Arguments: map[string]interface{}{
				"cache_id": cacheID,
				"query":    query,
			},
		})
	}

	// If caching failed or wasn't requested, use regular API
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
		Temperature: genai.Ptr(float32(s.config.GeminiTemperature)),
	}

	// Log the temperature setting
	logger.Debug("Using temperature: %v for model %s", s.config.GeminiTemperature, modelName)

	// Add file contents if provided
	if len(filePaths) > 0 {
		contents := []*genai.Content{
			genai.NewContentFromText(query, genai.RoleUser),
		}

		// Add each file
		for _, filePath := range filePaths {
			// Read the file
			content, err := os.ReadFile(filePath)
			if err != nil {
				logger.Error("Failed to read file %s: %v", filePath, err)
				continue
			}

			// Get mime type from file path
			mimeType := getMimeTypeFromPath(filePath)

			// Get filename from path
			fileName := filepath.Base(filePath)

			// Upload to Gemini
			logger.Info("Uploading file %s with mime type %s", fileName, mimeType)
			uploadConfig := &genai.UploadFileConfig{
				MIMEType: mimeType,
			}
			file, err := s.client.Files.Upload(ctx, bytes.NewReader(content), uploadConfig)

			if err != nil {
				logger.Error("Failed to upload file %s: %v", filePath, err)
				continue
			}

			// Add file to contents
			contents = append(contents, genai.NewContentFromURI(file.URI, mimeType, genai.RoleUser))
		}

		// Generate content with files
		response, err := s.client.Models.GenerateContent(ctx, modelName, contents, config)
		if err != nil {
			logger.Error("Gemini API error: %v", err)
			if cacheErr != nil {
				// Include cache error in response if it exists
				return createErrorResponse(fmt.Sprintf("Error from Gemini API: %v\nCache error: %v", err, cacheErr)), nil
			}
			return createErrorResponse(fmt.Sprintf("Error from Gemini API: %v", err)), nil
		}

		return s.formatResponse(response), nil
	} else {
		// No files, just send the query as text
		contents := []*genai.Content{
			genai.NewContentFromText(query, genai.RoleUser),
		}
		response, err := s.client.Models.GenerateContent(ctx, modelName, contents, config)
		if err != nil {
			logger.Error("Gemini API error: %v", err)
			if cacheErr != nil {
				// Include cache error in response if it exists
				return createErrorResponse(fmt.Sprintf("Error from Gemini API: %v\nCache error: %v", err, cacheErr)), nil
			}
			return createErrorResponse(fmt.Sprintf("Error from Gemini API: %v", err)), nil
		}

		return s.formatResponse(response), nil
	}
}

// handleGeminiSearch handles requests to the gemini_search tool
func (s *GeminiServer) handleGeminiSearch(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)

	// Extract and validate query parameter (required)
	query, ok := req.Arguments["query"].(string)
	if !ok {
		return createErrorResponse("query must be a string"), nil
	}

	// Extract optional systemPrompt parameter
	systemPrompt := s.config.GeminiSearchSystemPrompt
	if customPrompt, ok := req.Arguments["systemPrompt"].(string); ok && customPrompt != "" {
		logger.Info("Using request-specific search system prompt")
		systemPrompt = customPrompt
	}

	// Always use gemini-2.0-flash model for search
	modelName := "gemini-2.0-flash"
	logger.Info("Using %s model for Google Search integration", modelName)

	// Create the generate content configuration
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
		Temperature:       genai.Ptr(float32(s.config.GeminiTemperature)),
		Tools: []*genai.Tool{
			{
				GoogleSearch: &genai.GoogleSearch{},
			},
		},
	}

	// Create query content
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	// Stream the response
	responseText := ""
	for result, err := range s.client.Models.GenerateContentStream(ctx, modelName, contents, config) {
		if err != nil {
			logger.Error("Gemini Search API error: %v", err)
			return createErrorResponse(fmt.Sprintf("Error from Gemini Search API: %v", err)), nil
		}

		// Extract text from each candidate
		if len(result.Candidates) > 0 && result.Candidates[0].Content != nil {
			responseText += result.Text()
		}
	}

	// Check for empty content and provide a fallback message
	if responseText == "" {
		responseText = "The Gemini Search model returned an empty response. This might indicate an issue with the search functionality or that no relevant information was found. Please try rephrasing your question or providing more specific details."
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: responseText,
			},
		},
	}, nil
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

		if err := writeStringf("- Description: %s\n", model.Description); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		// Add caching support info
		if err := writeStringf("- Supports Caching: %v\n\n", model.SupportsCaching); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}
	}

	// Add usage hint
	if err := writeStringf("## Usage\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("You can specify a model ID in the `model` parameter when using the `gemini_ask` tool:\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("```json\n{\n  \"query\": \"Your question here\",\n  \"model\": \"gemini-1.5-pro-001\",\n  \"use_cache\": true\n}\n```\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	// Add info about caching
	if err := writeStringf("\n## Caching\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("Only models with version suffixes (e.g., ending with `-001`) support caching.\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("When using a cacheable model, you can enable caching with the `use_cache` parameter. This will create a temporary cache that automatically expires after 10 minutes by default. You can specify a custom TTL with the `cache_ttl` parameter.\n"); err != nil {
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

// handleQueryWithCache handles internal requests to query with a cached context
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

	// Send the query with cached content
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	config := &genai.GenerateContentConfig{
		CachedContent: cacheInfo.Name,
		Temperature: genai.Ptr(float32(s.config.GeminiTemperature)),
	}

	response, err := s.client.Models.GenerateContent(ctx, cacheInfo.Model, contents, config)
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		return createErrorResponse(fmt.Sprintf("error from Gemini API: %v", err)), nil
	}

	return s.formatResponse(response), nil
}

// executeGeminiRequest makes the request to the Gemini API with retry capability
func (s *GeminiServer) executeGeminiRequest(ctx context.Context, model string, query string) (*genai.GenerateContentResponse, error) {
	logger := getLoggerFromContext(ctx)

	var response *genai.GenerateContentResponse

	// Define the operation to retry
	operation := func() error {
		var err error
		// Set timeout context for the API call
		timeoutCtx, cancel := context.WithTimeout(ctx, s.config.HTTPTimeout)
		defer cancel()

		contents := []*genai.Content{
			genai.NewContentFromText(query, genai.RoleUser),
		}
		config := &genai.GenerateContentConfig{
			Temperature: genai.Ptr(float32(s.config.GeminiTemperature)),
		}
		response, err = s.client.Models.GenerateContent(timeoutCtx, model, contents, config)
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
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		content = resp.Text()
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

// Helper function to get MIME type from file path
func getMimeTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	case ".wav":
		return "audio/wav"
	case ".doc", ".docx":
		return "application/msword"
	case ".xls", ".xlsx":
		return "application/vnd.ms-excel"
	case ".ppt", ".pptx":
		return "application/vnd.ms-powerpoint"
	case ".zip":
		return "application/zip"
	case ".csv":
		return "text/csv"
	case ".go":
		return "text/x-go"
	case ".py":
		return "text/x-python"
	case ".java":
		return "text/x-java"
	case ".c", ".cpp", ".h", ".hpp":
		return "text/x-c"
	case ".rb":
		return "text/plain"
	case ".php":
		return "text/plain"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}
