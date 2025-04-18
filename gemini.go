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
	"sync"

	"github.com/gomcpgo/mcp/pkg/protocol"
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

			// Check if file store is properly initialized
			if s.fileStore == nil {
				logger.Error("FileStore not properly initialized")
				return createErrorResponse("Internal error: FileStore not properly initialized"), nil
			}

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

				// Create upload request with display name
				uploadReq := &FileUploadRequest{
					FileName:    fileName,
					MimeType:    mimeType,
					Content:     content,
					DisplayName: fileName,
				}

				// Upload the file using the updated UploadFile method
				fileInfo, err := s.fileStore.UploadFile(ctx, uploadReq)
				if err != nil {
					logger.Error("Failed to upload file %s: %v", filePath, err)
					continue
				}

				logger.Info("Successfully uploaded file %s with ID: %s for caching", fileName, fileInfo.ID)

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
		Temperature:       genai.Ptr(float32(s.config.GeminiTemperature)),
	}

	// Check if thinking mode should be enabled
	enableThinking := s.config.EnableThinking
	if thinkingRaw, ok := req.Arguments["enable_thinking"].(bool); ok {
		enableThinking = thinkingRaw
	}

	// Get model information
	modelInfo := GetModelByID(modelName)

	// Configure thinking mode if enabled and model supports it
	if enableThinking && modelInfo != nil && modelInfo.SupportsThinking {
		config.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: true,
		}
		logger.Info("Thinking mode enabled for request with model %s", modelName)
	}

	// Log the temperature setting
	logger.Debug("Using temperature: %v for model %s", s.config.GeminiTemperature, modelName)

	// Before processing files, set the environment variable explicitly
	originalAPIKey := os.Getenv("GOOGLE_API_KEY")
	os.Setenv("GOOGLE_API_KEY", s.config.GeminiAPIKey)

	// This will be executed at the end of this function to reset the environment
	defer func() {
		if originalAPIKey != "" {
			os.Setenv("GOOGLE_API_KEY", originalAPIKey)
		} else {
			os.Unsetenv("GOOGLE_API_KEY")
		}
	}()

	// Add file contents if provided
	if len(filePaths) > 0 {
		// Validate client first
		if s.client == nil {
			logger.Error("Gemini client not properly initialized")
			return createErrorResponse("Internal error: Gemini client not properly initialized"), nil
		}

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
				MIMEType:    mimeType,
				DisplayName: fileName,
			}
			file, err := s.client.Files.Upload(ctx, bytes.NewReader(content), uploadConfig)

			if err != nil {
				logger.Error("Failed to upload file %s: %v - falling back to direct content", filePath, err)
				// Fallback to direct content if upload fails
				contents = append(contents, genai.NewContentFromText(string(content), genai.RoleUser))
				continue
			}

			// Add file to contents using the URI
			contents = append(contents, genai.NewContentFromURI(file.URI, mimeType, genai.RoleUser))
		}

		// Validate client and models before proceeding
		if s.client == nil || s.client.Models == nil {
			logger.Error("Gemini client or Models service not properly initialized")
			return createErrorResponse("Internal error: Gemini client not properly initialized"), nil
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

		// Validate client and models before proceeding
		if s.client == nil || s.client.Models == nil {
			logger.Error("Gemini client or Models service not properly initialized")
			return createErrorResponse("Internal error: Gemini client not properly initialized"), nil
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

	// Initialize response data
	responseText := ""
	var sources []SourceInfo
	var searchQueries []string

	// Track seen URLs to avoid duplicates
	seenURLs := make(map[string]bool)

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResponse("Internal error: Gemini client not properly initialized"), nil
	}

	// Stream the response
	for result, err := range s.client.Models.GenerateContentStream(ctx, modelName, contents, config) {
		if err != nil {
			logger.Error("Gemini Search API error: %v", err)
			return createErrorResponse(fmt.Sprintf("Error from Gemini Search API: %v", err)), nil
		}

		// Extract text from each candidate
		if len(result.Candidates) > 0 && result.Candidates[0].Content != nil {
			responseText += result.Text()

			// Extract metadata if available
			if metadata := result.Candidates[0].GroundingMetadata; metadata != nil {
				// Collect search queries
				if len(metadata.WebSearchQueries) > 0 && len(searchQueries) == 0 {
					searchQueries = metadata.WebSearchQueries
				}

				// Collect sources from grounding chunks
				for _, chunk := range metadata.GroundingChunks {
					var source SourceInfo

					if web := chunk.Web; web != nil {
						// Skip if we've seen this URL already
						if seenURLs[web.URI] {
							continue
						}

						source = SourceInfo{
							Title: web.Title,
							URL:   web.URI,
							Type:  "web",
						}
						seenURLs[web.URI] = true
					} else if ctx := chunk.RetrievedContext; ctx != nil {
						// Skip if we've seen this URL already
						if seenURLs[ctx.URI] {
							continue
						}

						source = SourceInfo{
							Title: ctx.Title,
							URL:   ctx.URI,
							Type:  "retrieved_context",
						}
						seenURLs[ctx.URI] = true
					}

					if source.URL != "" {
						sources = append(sources, source)
					}
				}
			}
		}
	}

	// Check for empty content and provide a fallback message
	if responseText == "" {
		responseText = "The Gemini Search model returned an empty response. This might indicate an issue with the search functionality or that no relevant information was found. Please try rephrasing your question or providing more specific details."
	}

	// Create the response JSON
	response := SearchResponse{
		Answer:        responseText,
		Sources:       sources,
		SearchQueries: searchQueries,
	}

	// Convert to JSON
	responseJSON, err := json.Marshal(response)
	if err != nil {
		logger.Error("Failed to marshal search response: %v", err)
		return createErrorResponse(fmt.Sprintf("Failed to format search response: %v", err)), nil
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: string(responseJSON),
			},
		},
	}, nil
}

// handleGeminiModels handles requests to the gemini_models tool
func (s *GeminiServer) handleGeminiModels(ctx context.Context) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Listing available Gemini models")

	// Direct API access to get the most up-to-date model list
	var models []GeminiModelInfo
	
	if s.config.GeminiAPIKey != "" {
		// We'll try to fetch models directly here for the most accurate list
		logger.Info("Fetching models directly from API for most current list...")
		
		// Create a new client specifically for this request
		clientConfig := &genai.ClientConfig{
			APIKey: s.config.GeminiAPIKey,
		}
		
		tempClient, err := genai.NewClient(ctx, clientConfig)
		if err != nil {
			logger.Warn("Could not create temporary client for model listing: %v. Will use cached list.", err)
		} else {
			// Try to fetch models directly
			var fetchedModels []GeminiModelInfo
			modelCount := 0
			
			// Iterate through all available models
			for model, err := range tempClient.Models.All(ctx) {
				modelCount++
				if err != nil {
					logger.Warn("Error while fetching model: %v", err)
					continue
				}
				
				// Look for Gemini models
				modelName := strings.ToLower(model.Name)
				if strings.Contains(modelName, "gemini") {
					id := model.Name
					if strings.HasPrefix(id, "models/") {
						id = strings.TrimPrefix(id, "models/")
					}
					
					logger.Debug("Found model: %s", id)
					
					// Determine capabilities based on model type
					supportsCaching := strings.HasSuffix(id, "-001")
					supportsThinking := strings.Contains(strings.ToLower(id), "pro")
					contextWindowSize := 32768 // Default for Flash models
					
					if supportsThinking {
						contextWindowSize = 1048576 // Pro models have 1M context
					}
					
					// Create user-friendly name
					name := strings.TrimPrefix(id, "gemini-")
					name = strings.ReplaceAll(name, "-", " ")
					name = strings.Title(name)
					name = "Gemini " + name
					
					// Create appropriate description
					description := "Google Gemini model"
					if strings.Contains(strings.ToLower(id), "pro") {
						description = "Pro model with advanced reasoning capabilities"
					} else if strings.Contains(strings.ToLower(id), "flash") {
						description = "Flash model optimized for efficiency and speed"
					}
					
					// Add preview designation if applicable
					if strings.Contains(strings.ToLower(id), "preview") || strings.Contains(strings.ToLower(id), "exp") {
						description = "Preview/Experimental " + description
					}
					
					// Add the model to our list
					fetchedModels = append(fetchedModels, GeminiModelInfo{
						ID:                id,
						Name:              name,
						Description:       description,
						SupportsCaching:   supportsCaching,
						SupportsThinking:  supportsThinking,
						ContextWindowSize: contextWindowSize,
					})
				}
			}
			
			if len(fetchedModels) > 0 {
				// Use the directly fetched models for this response
				models = fetchedModels
				logger.Info("Successfully fetched %d models directly from API for display", len(models))
				
				// Also update the store for future use
				modelStore.Lock()
				modelStore.models = fetchedModels
				modelStore.Unlock()
			} else {
				logger.Warn("No models found from direct API call (from %d total). Using cached list.", modelCount)
				models = GetAvailableGeminiModels()
			}
		}
	}
	
	// Fallback if we couldn't get models directly
	if len(models) == 0 {
		models = GetAvailableGeminiModels()
		logger.Info("Using cached model list with %d models", len(models))
	}

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
		if err := writeStringf("- Supports Caching: %v\n", model.SupportsCaching); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}
		
		// Add thinking support info
		if err := writeStringf("- Supports Thinking: %v\n", model.SupportsThinking); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}
		
		// Add context window size
		if err := writeStringf("- Context Window Size: %d tokens\n\n", model.ContextWindowSize); err != nil {
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
	
	// Add info about thinking mode
	if err := writeStringf("\n## Thinking Mode\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("Pro models support thinking mode, which shows the model's detailed reasoning process.\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("Enable thinking mode with the `enable_thinking` parameter. Example:\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}
	
	if err := writeStringf("```json\n{\n  \"query\": \"Your complex question here\",\n  \"model\": \"gemini-1.5-pro\",\n  \"enable_thinking\": true\n}\n```\n"); err != nil {
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
		Temperature:   genai.Ptr(float32(s.config.GeminiTemperature)),
	}

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResponse("Internal error: Gemini client not properly initialized"), nil
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

		// Validate client and models before proceeding
		if s.client == nil || s.client.Models == nil {
			return fmt.Errorf("gemini client or Models service not properly initialized")
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
	thinking := ""

	// Extract text from the response
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		content = resp.Text()

		// Try to extract thinking output from candidate
		candidate := resp.Candidates[0]
		// The ThinkingOutput field is not directly exposed in the Go API
		// We'll need to check the raw JSON if available
		if data, err := json.Marshal(candidate); err == nil {
			var candidateMap map[string]interface{}
			if err := json.Unmarshal(data, &candidateMap); err == nil {
				if thinkingOutput, ok := candidateMap["thinkingOutput"].(map[string]interface{}); ok {
					if thinkingText, ok := thinkingOutput["thinking"].(string); ok {
						thinking = thinkingText
					}
				}
			}
		}
	}

	// Check for empty content and provide a fallback message
	if content == "" {
		content = "The Gemini model returned an empty response. This might indicate that the model couldn't generate an appropriate response for your query. Please try rephrasing your question or providing more context."
	}

	// If thinking output was found, include it in the response
	if thinking != "" {
		// Create a JSON response with thinking included
		thinkingResp := map[string]string{
			"answer":   content,
			"thinking": thinking,
		}

		// Convert to JSON
		thinkingJSON, err := json.Marshal(thinkingResp)
		if err == nil {
			return &protocol.CallToolResponse{
				Content: []protocol.ToolContent{
					{
						Type: "text",
						Text: string(thinkingJSON),
					},
				},
			}
		}
		// Fall back to just content if JSON conversion fails
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
		return "text/plain" // Changed from "text/x-go" to "text/plain"
	case ".py":
		return "text/plain" // Changed from "text/x-python" to "text/plain"
	case ".java":
		return "text/plain" // Changed from "text/x-java" to "text/plain"
	case ".c", ".cpp", ".h", ".hpp":
		return "text/plain" // Changed from "text/x-c" to "text/plain"
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
