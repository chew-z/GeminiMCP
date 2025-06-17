package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

// GeminiAskHandler is a handler for the gemini_ask tool that uses mcp-go types directly
func (s *GeminiServer) GeminiAskHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling gemini_ask request with direct handler")

	// Extract and validate query parameter (required)
	query, ok := req.GetArguments()["query"].(string)
	if !ok || query == "" {
		return createErrorResult("query must be a string and cannot be empty"), nil
	}

	// Create Gemini model configuration
	config, modelName, err := createModelConfig(ctx, req, s.config, s.config.GeminiModel)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Error creating model configuration: %v", err)), nil
	}

	// Extract file paths if provided
	filePaths := extractArgumentStringArray(req, "file_paths")

	// Check if caching is requested
	useCache := extractArgumentBool(req, "use_cache", false)
	cacheTTL := extractArgumentString(req, "cache_ttl", "")

	// Check if thinking mode is requested (used to determine if caching should be used)
	enableThinking := extractArgumentBool(req, "enable_thinking", s.config.EnableThinking)

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
		modelVersion := GetModelVersion(modelName)
		if modelVersion != nil && modelVersion.SupportsCaching {
			// Try to create cache from files if provided
			cacheID, cacheErr = s.createCacheFromFiles(ctx, query, modelName, filePaths, cacheTTL,
				extractArgumentString(req, "systemPrompt", s.config.GeminiSystemPrompt))

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
	query, ok := req.GetArguments()["query"].(string)
	if !ok || query == "" {
		return createErrorResult("query must be a string and cannot be empty"), nil
	}

	// Extract optional system prompt - use search-specific prompt as default
	systemPrompt := extractArgumentString(req, "systemPrompt", s.config.GeminiSearchSystemPrompt)

	// Extract optional model parameter - use search-specific model as default
	modelName := extractArgumentString(req, "model", s.config.GeminiSearchModel)
	if err := ValidateModelID(modelName); err != nil {
		logger.Error("Invalid model requested: %v", err)
		return createErrorResult(fmt.Sprintf("Invalid model specified: %v", err)), nil
	}

	// Resolve model ID to ensure we use a valid API-addressable version
	resolvedModelID := ResolveModelID(modelName)
	if resolvedModelID != modelName {
		logger.Info("Resolved model ID from '%s' to '%s'", modelName, resolvedModelID)
		modelName = resolvedModelID
	}
	logger.Info("Using %s model for Google Search integration", modelName)

	// Get model information for context window and thinking capability
	modelInfo := GetModelByID(modelName)
	if modelInfo == nil {
		logger.Warn("Model information not found for %s, using default parameters", modelName)
	}

	// Create the generate content configuration
	googleSearch := &genai.GoogleSearch{}

	// Extract and validate time range filter parameters
	startTimeStr := extractArgumentString(req, "start_time", "")
	endTimeStr := extractArgumentString(req, "end_time", "")

	// Both must be provided if either is provided
	if (startTimeStr != "" && endTimeStr == "") || (startTimeStr == "" && endTimeStr != "") {
		return createErrorResult("Both start_time and end_time must be provided for time range filtering"), nil
	}

	// If both time parameters are provided, create the time range filter
	if startTimeStr != "" && endTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			logger.Error("Invalid start_time format: %v", err)
			return createErrorResult(fmt.Sprintf("Invalid start_time format: %v. Must be RFC3339 format (e.g. '2024-01-01T00:00:00Z')", err)), nil
		}

		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			logger.Error("Invalid end_time format: %v", err)
			return createErrorResult(fmt.Sprintf("Invalid end_time format: %v. Must be RFC3339 format (e.g. '2024-12-31T23:59:59Z')", err)), nil
		}

		// Ensure start time is before or equal to end time
		if startTime.After(endTime) {
			return createErrorResult("start_time must be before or equal to end_time"), nil
		}

		// Create the time range filter
		googleSearch.TimeRangeFilter = &genai.Interval{
			StartTime: startTime,
			EndTime:   endTime,
		}
		logger.Info("Applying time range filter from %s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
		Temperature:       genai.Ptr(float32(s.config.GeminiTemperature)),
		Tools: []*genai.Tool{
			{
				GoogleSearch: googleSearch,
			},
		},
	}

	// Configure thinking
	enableThinking := extractArgumentBool(req, "enable_thinking", s.config.EnableThinking)
	if enableThinking && modelInfo != nil && modelInfo.SupportsThinking {
		thinkingConfig := &genai.ThinkingConfig{
			IncludeThoughts: true,
		}

		// Determine thinking budget
		thinkingBudget := 0

		// Check for level first
		args := req.GetArguments()
		if levelStr, ok := args["thinking_budget_level"].(string); ok && levelStr != "" {
			thinkingBudget = getThinkingBudgetFromLevel(levelStr)
			logger.Info("Setting thinking budget to %d tokens from level: %s", thinkingBudget, levelStr)
		} else if budgetRaw, ok := args["thinking_budget"].(float64); ok && budgetRaw >= 0 {
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
	configureMaxTokensOutput(ctx, config, req, modelInfo, 0.5)

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

	// Create formatted response using strings.Builder
	var formattedContent strings.Builder

	// Write the content using helper to make error checking cleaner
	write := func(format string, args ...interface{}) error {
		_, err := formattedContent.WriteString(fmt.Sprintf(format, args...))
		return err
	}

	// Write the header
	if err := write("# Available Gemini 2.5 Models\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("This server supports the official Gemini 2.5 model family:\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Gemini 2.5 Pro
	if err := write("## Gemini 2.5 Pro\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Model ID**: `gemini-2.5-pro`\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Description**: Our most powerful thinking model with maximum response accuracy and state-of-the-art performance\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Best for**: Complex reasoning, programming tasks, thinking mode\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Context Window**: 1M tokens\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Supports Thinking**: Yes\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Supports Caching**: Yes\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Gemini 2.5 Flash
	if err := write("## Gemini 2.5 Flash\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Model ID**: `gemini-2.5-flash`\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Description**: Best model in terms of price-performance, offering well-rounded capabilities\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Best for**: General tasks, balanced performance\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Context Window**: 32K tokens\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Supports Thinking**: Yes\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Supports Caching**: Yes\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Gemini 2.5 Flash Lite
	if err := write("## Gemini 2.5 Flash Lite\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Model ID**: `gemini-2.5-flash-lite-preview-06-17`\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Description**: Optimized for cost efficiency and low latency\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Best for**: Search queries, lightweight tasks\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Context Window**: 32K tokens\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Supports Thinking**: Yes\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Supports Caching**: No (preview version)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Add usage section
	if err := write("## Usage\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("Specify a model ID in the `model` parameter:\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Basic examples
	if err := write("```json\n// For complex reasoning (use Pro)\n{\n  \"query\": \"Your complex question here\",\n  \"model\": \"gemini-2.5-pro\",\n  \"enable_thinking\": true\n}\n\n// For general tasks (use Flash)\n{\n  \"query\": \"Your general question here\",\n  \"model\": \"gemini-2.5-flash\"\n}\n\n// For search queries (use Flash Lite)\n{\n  \"query\": \"Your search question here\",\n  \"model\": \"gemini-2.5-flash-lite-preview-06-17\"\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// System Prompt Information
	if err := write("## System Prompt\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("The default system prompt is optimized for code review and programming problems. For other tasks, consider using a custom system prompt:\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("```json\n// Using custom system prompt for creative writing\n{\n  \"query\": \"Write a short story about...\",\n  \"model\": \"gemini-2.5-pro\",\n  \"systemPrompt\": \"You are a creative writing assistant focused on storytelling, character development, and narrative structure.\"\n}\n\n// Using custom system prompt for data analysis\n{\n  \"query\": \"Analyze this dataset...\",\n  \"model\": \"gemini-2.5-flash\",\n  \"systemPrompt\": \"You are a data analyst expert. Provide clear insights, statistical analysis, and actionable recommendations.\"\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// File Attachments
	if err := write("## File Attachments\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("Attach files to provide context for your queries. This is particularly useful for code review, debugging, and analysis:\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("```json\n// Code review with multiple files\n{\n  \"query\": \"Review this code for potential issues and suggest improvements\",\n  \"model\": \"gemini-2.5-pro\",\n  \"file_paths\": [\n    \"/path/to/main.go\",\n    \"/path/to/utils.go\",\n    \"/path/to/config.yaml\"\n  ]\n}\n\n// Documentation analysis\n{\n  \"query\": \"Explain how these components interact and suggest documentation improvements\",\n  \"model\": \"gemini-2.5-flash\",\n  \"file_paths\": [\n    \"/path/to/README.md\",\n    \"/path/to/api.go\"\n  ]\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Caching
	if err := write("## Caching\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("Pro and Flash models support caching for improved performance on repeated queries. Flash Lite (preview) does not support caching yet.\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("```json\n// Enable caching with default TTL (10 minutes)\n{\n  \"query\": \"Analyze this codebase structure\",\n  \"model\": \"gemini-2.5-flash\",\n  \"file_paths\": [\"/path/to/large/codebase\"],\n  \"use_cache\": true\n}\n\n// Custom cache TTL\n{\n  \"query\": \"Long-term project analysis\",\n  \"model\": \"gemini-2.5-pro\",\n  \"file_paths\": [\"/path/to/project\"],\n  \"use_cache\": true,\n  \"cache_ttl\": \"30m\"\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("Caching is especially useful when:\n- Working with large codebases where context from multiple files is needed\n- Planning to ask multiple follow-up questions about the same code\n- Debugging issues that require file context\n- Code review scenarios where you need to discuss specific implementations\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Thinking Mode
	if err := write("## Thinking Mode\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("All Gemini 2.5 models support thinking mode, which shows the model's detailed reasoning process. This is particularly powerful with Pro models.\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("```json\n// Enable thinking with budget level\n{\n  \"query\": \"Solve this complex algorithm problem step by step\",\n  \"model\": \"gemini-2.5-pro\",\n  \"enable_thinking\": true,\n  \"thinking_budget_level\": \"high\"\n}\n\n// Custom thinking budget\n{\n  \"query\": \"Debug this complex issue\",\n  \"model\": \"gemini-2.5-pro\",\n  \"enable_thinking\": true,\n  \"thinking_budget\": 12000\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Thinking Budget Levels:**\n- `none`: 0 tokens (disabled)\n- `low`: 4,096 tokens\n- `medium`: 16,384 tokens  \n- `high`: 24,576 tokens (maximum)\n\nOr use `thinking_budget` to set a specific token count (0-24,576).\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Advanced Examples
	if err := write("## Advanced Examples\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("```json\n// Comprehensive code review with thinking and caching\n{\n  \"query\": \"Perform a thorough security and performance review of this codebase\",\n  \"model\": \"gemini-2.5-pro\",\n  \"file_paths\": [\n    \"/path/to/main.go\",\n    \"/path/to/auth.go\",\n    \"/path/to/database.go\"\n  ],\n  \"enable_thinking\": true,\n  \"thinking_budget_level\": \"medium\",\n  \"use_cache\": true,\n  \"cache_ttl\": \"1h\"\n}\n\n// Custom system prompt with file context\n{\n  \"query\": \"Suggest architectural improvements for better scalability\",\n  \"model\": \"gemini-2.5-pro\",\n  \"systemPrompt\": \"You are a senior software architect. Focus on scalability, maintainability, and best practices.\",\n  \"file_paths\": [\"/path/to/architecture/overview.md\"],\n  \"enable_thinking\": true\n}\n```\n"); err != nil {
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
