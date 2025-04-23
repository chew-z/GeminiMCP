package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/genai"
)

// handleAskGemini handles requests to the ask_gemini tool
func (s *GeminiServer) handleAskGemini(ctx context.Context, req *internalCallToolRequest) (*internalCallToolResponse, error) {
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

	// Check if thinking mode is also requested, as this can conflict with caching
	thinkingRequested := s.config.EnableThinking
	if thinkingRaw, ok := req.Arguments["enable_thinking"].(bool); ok {
		thinkingRequested = thinkingRaw
	}

	if thinkingRequested && useCache {
		logger.Warn("Both caching and thinking mode were requested - these features may conflict. Prioritizing thinking mode.")
		useCache = false
	}

	if useCache && s.config.EnableCaching {
		// Check if model supports caching
		model := GetModelByID(modelName)
		if model != nil && model.SupportsCaching {
			// Estimate content size - caching requires at least ~32K tokens of context
			estimatedTokens := len(query) / 4 // Very rough estimation: ~4 chars per token
			if len(filePaths) == 0 && estimatedTokens < 32768 {
				logger.Warn("Query may be too small for caching (min ~32K tokens needed). Caching may fail.")
			}

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
		return s.handleQueryWithCache(ctx, &internalCallToolRequest{
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
	if modelInfo == nil {
		logger.Warn("Model information not found for %s, using default parameters", modelName)
	}

	// Check if max_tokens parameter was provided
	if maxTokensRaw, ok := req.Arguments["max_tokens"].(float64); ok {
		maxTokens := int(maxTokensRaw)
		// Set the maximum output token limit
		config.MaxOutputTokens = int32(maxTokens)
		logger.Info("Setting max output tokens to %d", maxTokens)

		// Warn if tokens exceed the model's context window
		if modelInfo != nil && maxTokens > modelInfo.ContextWindowSize {
			logger.Warn("Requested max_tokens (%d) exceeds model's context window size (%d)",
				maxTokens, modelInfo.ContextWindowSize)
		}
	} else {
		// Set a safe default if not specified
		if modelInfo != nil {
			// Use 75% of the context window as a safe default for max output tokens
			safeTokenLimit := int32(modelInfo.ContextWindowSize * 3 / 4)
			config.MaxOutputTokens = safeTokenLimit
			logger.Debug("Using default max output tokens: %d (75%% of context window)", safeTokenLimit)
		}
	}

	// Configure thinking mode if enabled and model supports it
	if enableThinking {
		if modelInfo != nil && modelInfo.SupportsThinking {
			thinkingConfig := &genai.ThinkingConfig{
				IncludeThoughts: true,
			}

			// Determine thinking budget - check for level first, then explicit value
			thinkingBudget := 0

			// Check if thinking_budget_level parameter was provided
			if levelStr, ok := req.Arguments["thinking_budget_level"].(string); ok && levelStr != "" {
				thinkingBudget = getThinkingBudgetFromLevel(levelStr)
				logger.Info("Setting thinking budget to %d tokens from level: %s", thinkingBudget, levelStr)
			} else if budgetRaw, ok := req.Arguments["thinking_budget"].(float64); ok && budgetRaw >= 0 {
				// If explicit budget was provided, use that instead of level
				thinkingBudget = int(budgetRaw)
				logger.Info("Setting thinking budget to %d tokens from explicit value", thinkingBudget)
			} else if s.config.ThinkingBudget > 0 {
				// Fall back to config value if neither level nor explicit budget provided
				thinkingBudget = s.config.ThinkingBudget
				logger.Info("Using default thinking budget of %d tokens from config", thinkingBudget)
			}

			// Only set the thinking budget if it's greater than 0
			if thinkingBudget > 0 {
				budget := int32(thinkingBudget)
				thinkingConfig.ThinkingBudget = &budget
			}

			config.ThinkingConfig = thinkingConfig
			logger.Info("Thinking mode enabled for request with model %s", modelName)
		} else {
			if modelInfo != nil {
				logger.Warn("Thinking mode requested but model %s doesn't support it", modelName)
			} else {
				logger.Warn("Thinking mode requested but unknown if model %s supports it", modelName)
			}
		}
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
