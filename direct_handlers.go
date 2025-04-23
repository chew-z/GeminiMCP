package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

// GeminiAskHandler is a handler for the gemini_ask tool that uses mcp-go types directly
func (s *GeminiServer) GeminiAskHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling gemini_ask request with direct handler")

	// Extract and validate query parameter (required)
	query, ok := req.Params.Arguments["query"].(string)
	if !ok || query == "" {
		return createErrorResult("query must be a string and cannot be empty"), nil
	}

	// Create Gemini model configuration
	config, modelName, err := createModelConfig(ctx, req.Params.Arguments, s.config, s.config.GeminiModel)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Error creating model configuration: %v", err)), nil
	}

	// Extract file paths if provided
	filePaths := extractArgumentStringArray(req.Params.Arguments, "file_paths")

	// Check if caching is requested
	useCache := extractArgumentBool(req.Params.Arguments, "use_cache", false)
	cacheTTL := extractArgumentString(req.Params.Arguments, "cache_ttl", "")

	// Check if thinking mode is requested (used to determine if caching should be used)
	enableThinking := extractArgumentBool(req.Params.Arguments, "enable_thinking", s.config.EnableThinking)

	// Caching and thinking conflict - prioritize thinking if both are requested
	if useCache && enableThinking {
		logger.Warn("Both caching and thinking mode were requested - prioritizing thinking mode")
		useCache = false
	}

	// Handle caching if enabled and supported by the model
	var cacheID string
	var cacheErr error

	if useCache && s.config.EnableCaching {
		// Get model information to check if it supports caching
		model := GetModelByID(modelName)
		if model != nil && model.SupportsCaching {
			// Try to create cache from files if provided
			cacheID, cacheErr = s.createCacheFromFiles(ctx, query, modelName, filePaths, cacheTTL,
				extractArgumentString(req.Params.Arguments, "systemPrompt", s.config.GeminiSystemPrompt))

			if cacheErr != nil {
				logger.Warn("Failed to create cache, falling back to regular request: %v", cacheErr)
			} else if cacheID != "" {
				logger.Info("Using cache with ID: %s", cacheID)
				return s.handleQueryWithCacheDirect(ctx, cacheID, query)
			}
		} else {
			logger.Warn("Model %s does not support caching, falling back to regular request", modelName)
		}
	}

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResult("Internal error: Gemini client not properly initialized"), nil
	}

	// Process with files if provided
	if len(filePaths) > 0 {
		return s.processWithFiles(ctx, query, filePaths, modelName, config, enableThinking, cacheErr)
	} else {
		return s.processWithoutFiles(ctx, query, modelName, config, enableThinking, cacheErr)
	}
}

// createCacheFromFiles creates a cache from the provided files and returns the cache ID
func (s *GeminiServer) createCacheFromFiles(ctx context.Context, query, modelName string,
	filePaths []string, cacheTTL, systemPrompt string) (string, error) {

	logger := getLoggerFromContext(ctx)

	// Check if file store is properly initialized
	if s.fileStore == nil {
		return "", fmt.Errorf("FileStore not properly initialized")
	}

	// Create a list of file IDs from uploaded files
	var fileIDs []string

	// Upload each file to the API
	for _, filePath := range filePaths {
		// Read the file
		content, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error("Failed to read file %s: %v", filePath, err)
			continue
		}

		// Get mime type and filename
		mimeType := getMimeTypeFromPath(filePath)
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

		logger.Info("Successfully uploaded file %s with ID: %s for caching", fileName, fileInfo.ID)
		fileIDs = append(fileIDs, fileInfo.ID)
	}

	// If no files were uploaded successfully, return error
	if len(fileIDs) == 0 && len(filePaths) > 0 {
		return "", fmt.Errorf("failed to upload any files for caching")
	}

	// Create cache request
	cacheReq := &CacheRequest{
		Model:        modelName,
		SystemPrompt: systemPrompt,
		FileIDs:      fileIDs,
		TTL:          cacheTTL,
		Content:      query,
	}

	// Create the cache
	cacheInfo, err := s.cacheStore.CreateCache(ctx, cacheReq)
	if err != nil {
		return "", fmt.Errorf("failed to create cache: %w", err)
	}

	return cacheInfo.ID, nil
}

// handleQueryWithCacheDirect handles a query with a previously created cache
func (s *GeminiServer) handleQueryWithCacheDirect(ctx context.Context, cacheID, query string) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)

	// Get the cache
	cacheInfo, err := s.cacheStore.GetCache(ctx, cacheID)
	if err != nil {
		logger.Error("Failed to get cache with ID %s: %v", cacheID, err)
		return createErrorResult(fmt.Sprintf("Failed to get cache: %v", err)), nil
	}

	// Use the cached content for the query
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	// Create the configuration
	config := &genai.GenerateContentConfig{
		CachedContent: cacheInfo.Name,
	}

	// Make the request to the API
	response, err := s.client.Models.GenerateContent(ctx, cacheInfo.Model, contents, config)
	if err != nil {
		logger.Error("Failed to generate content with cached content: %v", err)
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	// Extract thinking flag to determine if we should try to include thinking data in response
	enableThinking := s.config.EnableThinking
	model := GetModelByID(cacheInfo.Model)
	withThinking := enableThinking && model != nil && model.SupportsThinking

	// Convert to MCP result
	return convertGenaiResponseToMCPResult(response, withThinking), nil
}

// processWithFiles handles a Gemini API request with file attachments
func (s *GeminiServer) processWithFiles(ctx context.Context, query string, filePaths []string,
	modelName string, config *genai.GenerateContentConfig, enableThinking bool, cacheErr error) (*mcp.CallToolResult, error) {

	logger := getLoggerFromContext(ctx)

	// Create initial content with the query
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	// Process each file
	for _, filePath := range filePaths {
		// Read the file
		content, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error("Failed to read file %s: %v", filePath, err)
			continue
		}

		// Get mime type and filename
		mimeType := getMimeTypeFromPath(filePath)
		fileName := filepath.Base(filePath)

		// Upload the file to Gemini
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

	// Generate content with files
	response, err := s.client.Models.GenerateContent(ctx, modelName, contents, config)
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		if cacheErr != nil {
			// If there was also a cache error, include it in the response
			return createErrorResult(fmt.Sprintf("Error from Gemini API: %v\nCache error: %v", err, cacheErr)), nil
		}
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	// Convert to MCP result
	return convertGenaiResponseToMCPResult(response, enableThinking), nil
}

// processWithoutFiles handles a Gemini API request without file attachments
func (s *GeminiServer) processWithoutFiles(ctx context.Context, query string,
	modelName string, config *genai.GenerateContentConfig, enableThinking bool, cacheErr error) (*mcp.CallToolResult, error) {

	logger := getLoggerFromContext(ctx)

	// Create content with just the query
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	// Generate content
	response, err := s.client.Models.GenerateContent(ctx, modelName, contents, config)
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		if cacheErr != nil {
			// If there was also a cache error, include it in the response
			return createErrorResult(fmt.Sprintf("Error from Gemini API: %v\nCache error: %v", err, cacheErr)), nil
		}
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	// Convert to MCP result
	return convertGenaiResponseToMCPResult(response, enableThinking), nil
}

// GeminiSearchHandler is a handler for the gemini_search tool that uses mcp-go types directly
func (s *GeminiServer) GeminiSearchHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling gemini_search request with direct handler")

	// Extract and validate query parameter (required)
	query, ok := req.Params.Arguments["query"].(string)
	if !ok || query == "" {
		return createErrorResult("query must be a string and cannot be empty"), nil
	}

	// Extract optional system prompt - use search-specific prompt as default
	systemPrompt := extractArgumentString(req.Params.Arguments, "systemPrompt", s.config.GeminiSearchSystemPrompt)

	// Extract optional model parameter - use search-specific model as default
	modelName := extractArgumentString(req.Params.Arguments, "model", s.config.GeminiSearchModel)
	if err := ValidateModelID(modelName); err != nil {
		logger.Error("Invalid model requested: %v", err)
		return createErrorResult(fmt.Sprintf("Invalid model specified: %v", err)), nil
	}
	logger.Info("Using %s model for Google Search integration", modelName)

	// Get model information for context window and thinking capability
	modelInfo := GetModelByID(modelName)
	if modelInfo == nil {
		logger.Warn("Model information not found for %s, using default parameters", modelName)
	}

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

	// Configure thinking
	enableThinking := extractArgumentBool(req.Params.Arguments, "enable_thinking", s.config.EnableThinking)
	if enableThinking && modelInfo != nil && modelInfo.SupportsThinking {
		thinkingConfig := &genai.ThinkingConfig{
			IncludeThoughts: true,
		}

		// Determine thinking budget
		thinkingBudget := 0

		// Check for level first
		if levelStr, ok := req.Params.Arguments["thinking_budget_level"].(string); ok && levelStr != "" {
			thinkingBudget = getThinkingBudgetFromLevel(levelStr)
			logger.Info("Setting thinking budget to %d tokens from level: %s", thinkingBudget, levelStr)
		} else if budgetRaw, ok := req.Params.Arguments["thinking_budget"].(float64); ok && budgetRaw >= 0 {
			// If explicit budget was provided
			thinkingBudget = int(budgetRaw)
			logger.Info("Setting thinking budget to %d tokens from explicit value", thinkingBudget)
		} else if s.config.ThinkingBudget > 0 {
			// Fall back to config value
			thinkingBudget = s.config.ThinkingBudget
			logger.Info("Using default thinking budget of %d tokens", thinkingBudget)
		}

		// Set thinking budget if greater than 0
		if thinkingBudget > 0 {
			budget := int32(thinkingBudget)
			thinkingConfig.ThinkingBudget = &budget
		}

		config.ThinkingConfig = thinkingConfig
		logger.Info("Thinking mode enabled for search request with model %s", modelName)
	} else if enableThinking {
		if modelInfo != nil {
			logger.Warn("Thinking mode requested but model %s doesn't support it", modelName)
		} else {
			logger.Warn("Thinking mode requested but unknown if model supports it")
		}
	}

	// Configure max tokens (50% of context window by default for search)
	configureMaxTokensOutput(ctx, config, req.Params.Arguments, modelInfo, 0.5)

	// Create content with the query
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResult("Internal error: Gemini client not properly initialized"), nil
	}

	// Initialize response data
	responseText := ""
	var sources []SourceInfo
	var searchQueries []string

	// Track seen URLs to avoid duplicates
	seenURLs := make(map[string]bool)

	// Stream the response
	for result, err := range s.client.Models.GenerateContentStream(ctx, modelName, contents, config) {
		if err != nil {
			logger.Error("Gemini Search API error: %v", err)
			return createErrorResult(fmt.Sprintf("Error from Gemini Search API: %v", err)), nil
		}

		// Extract text and metadata from the response
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
		responseText = `The Gemini Search model returned an empty response.
			This might indicate an issue with the search functionality or that
			no relevant information was found. Please try rephrasing your question
			or providing more specific details.`
	}

	// Create the response JSON
	response := SearchResponse{
		Answer:        responseText,
		Sources:       sources,
		SearchQueries: searchQueries,
	}

	// Convert to JSON and return as text content
	responseJSON, err := json.Marshal(response)
	if err != nil {
		logger.Error("Failed to marshal search response: %v", err)
		return createErrorResult(fmt.Sprintf("Failed to format search response: %v", err)), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(string(responseJSON)),
		},
	}, nil
}

// GeminiModelsHandler is a handler for the gemini_models tool that uses mcp-go types directly
func (s *GeminiServer) GeminiModelsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling gemini_models request with direct handler")

	// Get the available models
	var models []GeminiModelInfo

	// Try to fetch models directly from the API if we have an API key
	if s.config.GeminiAPIKey != "" {
		logger.Info("Fetching models directly from API for most current list...")

		// Create a temporary client for this request
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

	// Create formatted response using strings.Builder
	var formattedContent strings.Builder

	// Write the content using helper to make error checking cleaner
	write := func(format string, args ...interface{}) error {
		_, err := formattedContent.WriteString(fmt.Sprintf(format, args...))
		return err
	}

	// Write the header
	if err := write("# Available Gemini Models\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Write each model's information
	for _, model := range models {
		if err := write("## %s\n", model.Name); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResult("Error generating model list"), nil
		}

		if err := write("- ID: `%s`\n", model.ID); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResult("Error generating model list"), nil
		}

		if err := write("- Description: %s\n", model.Description); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResult("Error generating model list"), nil
		}

		// Add caching support info
		if err := write("- Supports Caching: %v\n", model.SupportsCaching); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResult("Error generating model list"), nil
		}

		// Add thinking support info
		if err := write("- Supports Thinking: %v\n", model.SupportsThinking); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResult("Error generating model list"), nil
		}

		// Add context window size
		if err := write("- Context Window Size: %d tokens\n\n", model.ContextWindowSize); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResult("Error generating model list"), nil
		}
	}

	// Add usage section with detailed information
	if err := write("## Usage\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("You can specify a model ID in the `model` parameter when using the `gemini_ask` or `gemini_search` tools:\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("```json\n// For gemini_ask\n{\n  \"query\": \"Your question here\",\n  \"model\": \"gemini-1.5-pro-001\",\n  \"use_cache\": true\n}\n\n// For gemini_search\n{\n  \"query\": \"Your search question here\",\n  \"model\": \"gemini-2.5-pro-exp-03-25\",\n  \"enable_thinking\": true\n}\n```\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Add info about caching
	if err := write("\n## Caching\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("Only models with version suffixes (e.g., ending with `-001`) support caching.\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("When using a cacheable model, you can enable caching with the `use_cache` parameter. This will create a temporary cache that automatically expires after 10 minutes by default. You can specify a custom TTL with the `cache_ttl` parameter.\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Add info about thinking mode
	if err := write("\n## Thinking Mode\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("Pro models support thinking mode, which shows the model's detailed reasoning process.\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("You can control thinking mode using these parameters:\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("* `enable_thinking`: Enables or disables thinking mode (boolean)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("* `thinking_budget_level`: Sets predefined token budgets (\"none\", \"low\", \"medium\", \"high\")\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("  - none: 0 tokens (disabled)\n  - low: 4096 tokens\n  - medium: 16384 tokens\n  - high: 24576 tokens (maximum)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("* `thinking_budget`: Sets a specific token count (0-24576)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Add example usage for thinking mode
	if err := write("Example:\n\n```json\n{\n  \"query\": \"Your complex question here\",\n  \"model\": \"gemini-1.5-pro\",\n  \"enable_thinking\": true,\n  \"thinking_budget_level\": \"medium\"\n}\n```\n\nOr with explicit budget:\n\n```json\n{\n  \"query\": \"Your complex question here\",\n  \"model\": \"gemini-1.5-pro\",\n  \"enable_thinking\": true,\n  \"thinking_budget\": 8192\n}\n```\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Return the formatted content
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(formattedContent.String()),
		},
	}, nil
}
