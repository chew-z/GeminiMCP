package main

import (
	"bytes"
	"context"
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
	query, err := validateRequiredString(req, "query")
	if err != nil {
		return createErrorResult(err.Error()), nil
	}

	// Create Gemini model configuration
	config, modelName, err := createModelConfig(ctx, req, s.config, s.config.GeminiModel)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Error creating model configuration: %v", err)), nil
	}

	// --- File Handling Logic ---
	logger.Info("Starting file handling logic")
	var uploads []*FileUploadRequest

	filePaths := extractArgumentStringArray(req, "file_paths")
	githubFiles := extractArgumentStringArray(req, "github_files")

	logger.Info("Extracted file parameters - local files: %d, github files: %d", len(filePaths), len(githubFiles))
	if len(filePaths) > 0 {
		logger.Info("Local file paths: %v", filePaths)
	}
	if len(githubFiles) > 0 {
		logger.Info("GitHub file paths: %v", githubFiles)
	}

	// Validation: Cannot use both file_paths and github_files
	if len(filePaths) > 0 && len(githubFiles) > 0 {
		logger.Error("Invalid request: both local and GitHub files specified")
		return createErrorResult("Cannot use both 'file_paths' and 'github_files' in the same request."), nil
	}

	// Handle GitHub files
	if len(githubFiles) > 0 {
		logger.Info("Processing GitHub files request")
		logger.Info("GitHub files count: %d", len(githubFiles))
		logger.Info("GitHub files list: %v", githubFiles)

		githubRepo := extractArgumentString(req, "github_repo", "")
		if githubRepo == "" {
			logger.Error("GitHub repository parameter missing")
			return createErrorResult("'github_repo' is required when using 'github_files'."), nil
		}
		logger.Info("GitHub repository: %s", githubRepo)

		githubRef := extractArgumentString(req, "github_ref", "")
		if githubRef == "" {
			// If no ref is provided and no default is set, it's not necessarily an error, the API might use the repo's default.
			logger.Info("No 'github_ref' provided, will use repository's default branch.")
		} else {
			logger.Info("GitHub reference: %s", githubRef)
		}

		// Validate GitHub file paths
		logger.Info("Validating GitHub file paths...")
		if err := validateFilePathArray(githubFiles, true); err != nil {
			logger.Error("GitHub file path validation failed: %v", err)
			return createErrorResult(err.Error()), nil
		}
		logger.Info("GitHub file path validation passed")

		fetchedUploads, fileErrs := fetchFromGitHub(ctx, s, githubRepo, githubRef, githubFiles)
		uploads = fetchedUploads
		if len(fileErrs) > 0 {
			for _, err := range fileErrs {
				logger.Error("Error processing github file: %v", err)
			}
			if len(uploads) == 0 {
				return createErrorResult(fmt.Sprintf("Error processing github files: %v", fileErrs)), nil
			}
		}
	} else if len(filePaths) > 0 {
		// Handle local file paths
		if isHTTPTransport(ctx) {
			return createErrorResult("'file_paths' is not supported in HTTP transport mode. Use 'github_files' instead."), nil
		}
		localUploads, err := readLocalFiles(ctx, filePaths, s.config)
		if err != nil {
			logger.Error("Error processing local files: %v", err)
			return createErrorResult(fmt.Sprintf("Error processing local files: %v", err)), nil
		}
		uploads = localUploads
	}

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
			if len(uploads) > 0 {
				cacheID, cacheErr = s.createCacheFromFiles(ctx, query, modelName, uploads, cacheTTL,
					extractArgumentString(req, "systemPrompt", s.config.GeminiSystemPrompt))
			}

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
	if len(uploads) > 0 {
		return s.processWithFiles(ctx, query, uploads, modelName, config, cacheErr)
	} else {
		return s.processWithoutFiles(ctx, query, modelName, config, cacheErr)
	}
}

// readLocalFiles reads files from the local filesystem and returns them as FileUploadRequest objects.
func readLocalFiles(ctx context.Context, filePaths []string, config *Config) ([]*FileUploadRequest, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Reading files from local filesystem (source: 'file_paths')")

	if config.FileReadBaseDir == "" {
		return nil, fmt.Errorf("local file reading is disabled: no base directory configured")
	}

	var uploads []*FileUploadRequest

	for _, filePath := range filePaths {
		// Clean the path to resolve ".." etc. and prevent shenanigans.
		cleanedPath := filepath.Clean(filePath)

		// Prevent absolute paths and path traversal attempts.
		if filepath.IsAbs(cleanedPath) || strings.HasPrefix(cleanedPath, "..") {
			return nil, fmt.Errorf("invalid path: %s. Only relative paths within the allowed directory are permitted", filePath)
		}

		fullPath := filepath.Join(config.FileReadBaseDir, cleanedPath)

		// Final, most important check: ensure the resolved path is still within the base directory.
		fileInfo, err := os.Lstat(fullPath)
		if err != nil {
			logger.Error("Failed to stat file %s: %v", filePath, err)
			continue
		}

		if fileInfo.IsDir() {
			logger.Warn("Skipping directory: %s", filePath)
			continue
		}

		if fileInfo.Mode()&os.ModeSymlink != 0 {
			linkDest, err := os.Readlink(fullPath)
			if err != nil {
				logger.Error("Failed to read symlink %s: %v", filePath, err)
				continue
			}
			if filepath.IsAbs(linkDest) || strings.HasPrefix(filepath.Clean(linkDest), "..") {
				logger.Error("Skipping unsafe symlink: %s -> %s", filePath, linkDest)
				continue
			}
		}

		if !strings.HasPrefix(fullPath, config.FileReadBaseDir) {
			return nil, fmt.Errorf("path traversal attempt detected: %s", filePath)
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			logger.Error("Failed to read file %s: %v", filePath, err)
			continue // Or return error immediately
		}

		if int64(len(content)) > config.MaxFileSize {
			logger.Warn("Skipping file %s because it is too large: %d bytes, limit is %d", filePath, len(content), config.MaxFileSize)
			continue
		}

		mimeType := getMimeTypeFromPath(filePath)
		fileName := filepath.Base(filePath)

		logger.Info("Adding file to context: %s", filePath)
		uploads = append(uploads, &FileUploadRequest{
			FileName:    fileName,
			MimeType:    mimeType,
			Content:     content,
			DisplayName: fileName,
		})
	}
	return uploads, nil
}

// createCacheFromFiles creates a cache from the provided files and returns the cache ID
func (s *GeminiServer) createCacheFromFiles(ctx context.Context, query, modelName string,
	uploads []*FileUploadRequest, cacheTTL, systemPrompt string) (string, error) {

	logger := getLoggerFromContext(ctx)

	// Check if file store is properly initialized
	if s.fileStore == nil {
		return "", fmt.Errorf("FileStore not properly initialized")
	}

	// Create a list of file IDs from uploaded files
	var fileIDs []string

	// Upload each file to the API
	for _, uploadReq := range uploads {
		// Upload the file
		fileInfo, err := s.fileStore.UploadFile(ctx, uploadReq)
		if err != nil {
			logger.Error("Failed to upload file %s: %v", uploadReq.FileName, err)
			continue // Continue with other files
		}

		logger.Info("Successfully uploaded file %s with ID: %s for caching", uploadReq.FileName, fileInfo.ID)
		fileIDs = append(fileIDs, fileInfo.ID)
	}

	// If no files were uploaded successfully, return error
	if len(fileIDs) == 0 && len(uploads) > 0 {
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
	response, err := withRetry(ctx, s.config, logger, "gemini.models.generate_content", func(ctx context.Context) (*genai.GenerateContentResponse, error) {
		return s.client.Models.GenerateContent(ctx, cacheInfo.Model, contents, config)
	})
	if err != nil {
		logger.Error("Failed to generate content with cached content: %v", err)
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	// Convert to MCP result
	return convertGenaiResponseToMCPResult(response), nil
}

// processWithFiles handles a Gemini API request with file attachments
func (s *GeminiServer) processWithFiles(ctx context.Context, query string, uploads []*FileUploadRequest,
	modelName string, config *genai.GenerateContentConfig, cacheErr error) (*mcp.CallToolResult, error) {

	logger := getLoggerFromContext(ctx)

	// Create initial content with the query
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	// Process each file
	for _, upload := range uploads {
		// Upload the file to Gemini
		logger.Info("Uploading file %s with mime type %s", upload.FileName, upload.MimeType)
		uploadConfig := &genai.UploadFileConfig{
			MIMEType:    upload.MimeType,
			DisplayName: upload.FileName,
		}

		file, err := withRetry(ctx, s.config, logger, "gemini.files.upload", func(ctx context.Context) (*genai.File, error) {
			return s.client.Files.Upload(ctx, bytes.NewReader(upload.Content), uploadConfig)
		})
		if err != nil {
			logger.Error("Failed to upload file %s: %v - falling back to direct content", upload.FileName, err)
			// Fallback to direct content if upload fails
			contents = append(contents, genai.NewContentFromText(string(upload.Content), genai.RoleUser))
			continue
		}

		// Add file to contents using the URI
		contents = append(contents, genai.NewContentFromURI(file.URI, upload.MimeType, genai.RoleUser))
	}

	// Generate content with files
	response, err := withRetry(ctx, s.config, logger, "gemini.models.generate_content", func(ctx context.Context) (*genai.GenerateContentResponse, error) {
		return s.client.Models.GenerateContent(ctx, modelName, contents, config)
	})
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		if cacheErr != nil {
			// If there was also a cache error, include it in the response
			return createErrorResult(fmt.Sprintf("Error from Gemini API: %v\nCache error: %v", err, cacheErr)), nil
		}
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	// Convert to MCP result
	return convertGenaiResponseToMCPResult(response), nil
}

// processWithoutFiles handles a Gemini API request without file attachments
func (s *GeminiServer) processWithoutFiles(ctx context.Context, query string,
	modelName string, config *genai.GenerateContentConfig, cacheErr error) (*mcp.CallToolResult, error) {

	logger := getLoggerFromContext(ctx)

	// Create content with just the query
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	// Generate content
	response, err := withRetry(ctx, s.config, logger, "gemini.models.generate_content", func(ctx context.Context) (*genai.GenerateContentResponse, error) {
		return s.client.Models.GenerateContent(ctx, modelName, contents, config)
	})
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		if cacheErr != nil {
			// If there was also a cache error, include it in the response
			return createErrorResult(fmt.Sprintf("Error from Gemini API: %v\nCache error: %v", err, cacheErr)), nil
		}
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	// Convert to MCP result
	return convertGenaiResponseToMCPResult(response), nil
}
