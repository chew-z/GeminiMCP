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
		return createErrorResponseWithMessage("query must be a string"), nil
	}

	// Extract optional model parameter
	modelName := extractModelParam(ctx, req.Arguments, s.config.GeminiModel)

	// Check if caching is requested
	useCache := extractBoolParam(req.Arguments, "use_cache", false)

	// Extract cache TTL if provided
	cacheTTL := extractStringParam(req.Arguments, "cache_ttl", "")

	// Check if thinking mode is also requested, as this can conflict with caching
	thinkingRequested := extractBoolParam(req.Arguments, "enable_thinking", s.config.EnableThinking)

	if thinkingRequested && useCache {
		logger.Warn("Both caching and thinking mode were requested - these features may conflict. Prioritizing thinking mode.")
		useCache = false
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

	// If caching is requested and the model supports it, use caching
	var cacheID string
	var cacheErr error
	
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
				return createErrorResponseWithMessage("Internal error: FileStore not properly initialized"), nil
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

			// Extract optional systemPrompt parameter
			systemPrompt := extractSystemPrompt(ctx, req.Arguments, s.config.GeminiSystemPrompt)

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
	// Create content config with system prompt and other settings
	config := createGenaiContentConfig(ctx, req.Arguments, s.config, modelName)

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResponseWithMessage("Internal error: Gemini client not properly initialized"), nil
	}

	// Process with files if provided
	if len(filePaths) > 0 {
		// Add file contents 
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

		// Generate content with files
		response, err := s.client.Models.GenerateContent(ctx, modelName, contents, config)
		if err != nil {
			logger.Error("Gemini API error: %v", err)
			if cacheErr != nil {
				// Include cache error in response if it exists
				return createErrorResponseWithMessage(fmt.Sprintf("Error from Gemini API: %v\nCache error: %v", err, cacheErr)), nil
			}
			return createErrorResponseWithMessage(fmt.Sprintf("Error from Gemini API: %v", err)), nil
		}

		return s.formatResponse(response), nil
	} else {
		// No files, just send the query as text
		contents := []*genai.Content{
			genai.NewContentFromText(query, genai.RoleUser),
		}

		// Generate content with simple text query
		response, err := s.client.Models.GenerateContent(ctx, modelName, contents, config)
		if err != nil {
			logger.Error("Gemini API error: %v", err)
			if cacheErr != nil {
				// Include cache error in response if it exists
				return createErrorResponseWithMessage(fmt.Sprintf("Error from Gemini API: %v\nCache error: %v", err, cacheErr)), nil
			}
			return createErrorResponseWithMessage(fmt.Sprintf("Error from Gemini API: %v", err)), nil
		}

		return s.formatResponse(response), nil
	}
}
