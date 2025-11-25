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

func (s *GeminiServer) parseAskRequest(ctx context.Context, req mcp.CallToolRequest) (string, *genai.GenerateContentConfig, string, error) {
	// Extract and validate query parameter (required)
	query, err := validateRequiredString(req, "query")
	if err != nil {
		return "", nil, "", err
	}

	// Create Gemini model configuration
	config, modelName, err := createModelConfig(ctx, req, s.config, s.config.GeminiModel)
	if err != nil {
		return "", nil, "", fmt.Errorf("error creating model configuration: %v", err)
	}

	return query, config, modelName, nil
}

// GeminiAskHandler is a handler for the gemini_ask tool that uses mcp-go types directly
func (s *GeminiServer) GeminiAskHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling gemini_ask request with direct handler")

	query, config, modelName, err := s.parseAskRequest(ctx, req)
	if err != nil {
		return createErrorResult(err.Error()), nil
	}

	// --- File Handling Logic ---
	logger.Info("Starting file handling logic")
	uploads, errResult := s.gatherFileUploads(ctx, req)
	if errResult != nil {
		return errResult, nil
	}

	// Handle caching if enabled
	cacheID, uploadedFiles, cacheErr := s.maybeCreateCache(ctx, req, query, modelName, uploads)
	if cacheID != "" {
		logger.Info("Using cache with ID: %s", cacheID)
		return s.handleQueryWithCacheDirect(ctx, cacheID, query)
	}

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResult("Internal error: Gemini client not properly initialized"), nil
	}

	// Process with files if provided
	if len(uploads) > 0 {
		return s.processWithFiles(ctx, query, uploads, uploadedFiles, modelName, config, cacheErr)
	} else {
		return s.processWithoutFiles(ctx, query, modelName, config, cacheErr)
	}
}

// gatherFileUploads handles the logic of selecting, validating, and fetching files
// from either local paths or a GitHub repository.
func (s *GeminiServer) gatherFileUploads(ctx context.Context, req mcp.CallToolRequest) ([]*FileUploadRequest, *mcp.CallToolResult) {
	logger := getLoggerFromContext(ctx)

	filePaths := extractArgumentStringArray(req, "file_paths")
	githubFiles := extractArgumentStringArray(req, "github_files")

	logger.Info("Extracted file parameters - local files: %d, github files: %d", len(filePaths), len(githubFiles))

	// Validation: Cannot use both file_paths and github_files
	if len(filePaths) > 0 && len(githubFiles) > 0 {
		logger.Error("Invalid request: both local and GitHub files specified")
		return nil, createErrorResult("Cannot use both 'file_paths' and 'github_files' in the same request.")
	}

	// Handle file sources
	if len(githubFiles) > 0 {
		return s.gatherGitHubFiles(ctx, req, githubFiles)
	} else if len(filePaths) > 0 {
		return s.gatherLocalFiles(ctx, filePaths)
	}

	return nil, nil // No files requested
}

// gatherGitHubFiles fetches files from a GitHub repository.
func (s *GeminiServer) gatherGitHubFiles(ctx context.Context, req mcp.CallToolRequest, githubFiles []string) ([]*FileUploadRequest, *mcp.CallToolResult) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Processing GitHub files request")

	githubRepo := extractArgumentString(req, "github_repo", "")
	if githubRepo == "" {
		logger.Error("GitHub repository parameter missing")
		return nil, createErrorResult("'github_repo' is required when using 'github_files'.")
	}

	githubRef := extractArgumentString(req, "github_ref", "")

	// Validate and fetch
	if err := validateFilePathArray(githubFiles, true); err != nil {
		logger.Error("GitHub file path validation failed: %v", err)
		return nil, createErrorResult(err.Error())
	}

	fetchedUploads, fileErrs := fetchFromGitHub(ctx, s, githubRepo, githubRef, githubFiles)
	if len(fileErrs) > 0 {
		for _, err := range fileErrs {
			logger.Error("Error processing github file: %v", err)
		}
		if len(fetchedUploads) == 0 {
			return nil, createErrorResult(fmt.Sprintf("Error processing github files: %v", fileErrs))
		}
	}
	return fetchedUploads, nil
}

// gatherLocalFiles fetches files from the local filesystem.
func (s *GeminiServer) gatherLocalFiles(ctx context.Context, filePaths []string) ([]*FileUploadRequest, *mcp.CallToolResult) {
	if isHTTPTransport(ctx) {
		return nil, createErrorResult("'file_paths' is not supported in HTTP transport mode. Use 'github_files' instead.")
	}

	localUploads, err := readLocalFiles(ctx, filePaths, s.config)
	if err != nil {
		getLoggerFromContext(ctx).Error("Error processing local files: %v", err)
		return nil, createErrorResult(fmt.Sprintf("Error processing local files: %v", err))
	}
	return localUploads, nil
}

// maybeCreateCache handles the logic for checking caching eligibility and creating a cache.
func (s *GeminiServer) maybeCreateCache(
	ctx context.Context,
	req mcp.CallToolRequest,
	query, modelName string,
	uploads []*FileUploadRequest,
) (string, []*FileInfo, error) {
	logger := getLoggerFromContext(ctx)

	useCache := extractArgumentBool(req, "use_cache", false)
	cacheTTL := extractArgumentString(req, "cache_ttl", "")
	enableThinking := extractArgumentBool(req, "enable_thinking", s.config.EnableThinking)

	if useCache && enableThinking {
		logger.Warn("Both caching and thinking mode were requested - prioritizing thinking mode")
		useCache = false
	}

	if !useCache || !s.config.EnableCaching {
		return "", nil, nil
	}

	modelVersion := GetModelVersion(modelName)
	if modelVersion == nil || !modelVersion.SupportsCaching {
		logger.Warn("Model %s does not support caching, falling back to regular request", modelName)
		return "", nil, nil
	}

	if len(uploads) == 0 {
		return "", nil, nil // Caching with no files is not implemented in this flow
	}

	cacheID, uploadedFiles, cacheErr := s.createCacheFromFiles(ctx, query, modelName, uploads, cacheTTL,
		extractArgumentString(req, "systemPrompt", s.config.GeminiSystemPrompt))

	if cacheErr != nil {
		logger.Warn("Failed to create cache, falling back to regular request: %v", cacheErr)
		// Return the error but also the uploaded files so they can be reused
		return "", uploadedFiles, cacheErr
	}

	return cacheID, uploadedFiles, nil
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

// createCacheFromFiles creates a cache from the provided files and returns the cache ID and file info
func (s *GeminiServer) createCacheFromFiles(ctx context.Context, query, modelName string,
	uploads []*FileUploadRequest, cacheTTL, systemPrompt string) (string, []*FileInfo, error) {

	logger := getLoggerFromContext(ctx)

	// Check if file store is properly initialized
	if s.fileStore == nil {
		return "", nil, fmt.Errorf("FileStore not properly initialized")
	}

	// Upload files and collect their info
	var fileInfos []*FileInfo
	var fileIDs []string
	for _, uploadReq := range uploads {
		fileInfo, err := s.fileStore.UploadFile(ctx, uploadReq)
		if err != nil {
			logger.Error("Failed to upload file %s: %v", uploadReq.FileName, err)
			continue // Continue with other files
		}
		logger.Info("Successfully uploaded file %s with ID: %s for caching", uploadReq.FileName, fileInfo.ID)
		fileInfos = append(fileInfos, fileInfo)
		fileIDs = append(fileIDs, fileInfo.ID)
	}

	// If no files were uploaded successfully, return error
	if len(fileIDs) == 0 && len(uploads) > 0 {
		return "", fileInfos, fmt.Errorf("failed to upload any files for caching")
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
		return "", fileInfos, fmt.Errorf("failed to create cache: %w", err)
	}

	return cacheInfo.ID, fileInfos, nil
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

// processWithFiles handles a Gemini API request with file attachments, reusing existing uploads if available
func (s *GeminiServer) processWithFiles(ctx context.Context, query string, uploads []*FileUploadRequest,
	uploadedFiles []*FileInfo, modelName string, config *genai.GenerateContentConfig, cacheErr error) (*mcp.CallToolResult, error) {

	logger := getLoggerFromContext(ctx)

	// Create initial content with the query
	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	// Use pre-uploaded files if available
	if len(uploadedFiles) > 0 {
		logger.Info("Reusing %d pre-uploaded files", len(uploadedFiles))
		for _, fileInfo := range uploadedFiles {
			contents = append(contents, genai.NewContentFromURI(fileInfo.URI, fileInfo.MimeType, genai.RoleUser))
		}
	} else if len(uploads) > 0 {
		logger.Info("No pre-uploaded files found, proceeding with manual upload")
		// Process each file by uploading it now
		for _, upload := range uploads {
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
				contents = append(contents, genai.NewContentFromText(string(upload.Content), genai.RoleUser))
				continue
			}
			contents = append(contents, genai.NewContentFromURI(file.URI, upload.MimeType, genai.RoleUser))
		}
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
