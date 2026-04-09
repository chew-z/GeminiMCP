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

// maxReportedWarnings is the cap for file failure warnings surfaced to the model.
const maxReportedWarnings = 10

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
	uploads, warnings, errResult := s.gatherFileUploads(ctx, req)
	if errResult != nil {
		return errResult, nil
	}

	// Surface partial file failures to the model so it doesn't hallucinate about missing files
	if len(warnings) > 0 {
		reported := warnings
		suffix := ""
		if len(reported) > maxReportedWarnings {
			suffix = fmt.Sprintf("\n- ... and %d other file(s)", len(reported)-maxReportedWarnings)
			reported = reported[:maxReportedWarnings]
		}
		query += "\n\n[System Note: The following requested files could not be loaded:\n- " +
			strings.Join(reported, "\n- ") + suffix + "]"
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
// Returns uploads, warning messages for files that failed, and an optional error result.
func (s *GeminiServer) gatherFileUploads(ctx context.Context, req mcp.CallToolRequest) ([]*FileUploadRequest, []string, *mcp.CallToolResult) {
	logger := getLoggerFromContext(ctx)

	filePaths := extractArgumentStringArray(req, "file_paths")
	githubFiles := extractArgumentStringArray(req, "github_files")

	logger.Info("Extracted file parameters - local files: %d, github files: %d", len(filePaths), len(githubFiles))

	// Validation: Cannot use both file_paths and github_files
	if len(filePaths) > 0 && len(githubFiles) > 0 {
		logger.Error("Invalid request: both local and GitHub files specified")
		return nil, nil, createErrorResult("Cannot use both 'file_paths' and 'github_files' in the same request.")
	}

	filesRequested := len(filePaths) > 0 || len(githubFiles) > 0

	// Handle file sources
	var uploads []*FileUploadRequest
	var warnings []string
	var errResult *mcp.CallToolResult
	if len(githubFiles) > 0 {
		uploads, warnings, errResult = s.gatherGitHubFiles(ctx, req, githubFiles)
	} else if len(filePaths) > 0 {
		uploads, warnings, errResult = s.gatherLocalFiles(ctx, filePaths)
	}

	if errResult != nil {
		return nil, nil, errResult
	}

	// Guard: if files were explicitly requested but none were gathered,
	// return an error instead of silently falling through to processWithoutFiles.
	if filesRequested && len(uploads) == 0 {
		logger.Error("Files were requested but none could be gathered")
		return nil, nil, createErrorResult("Failed to retrieve any of the requested files. Cannot proceed without file context.")
	}

	return uploads, warnings, nil
}

// gatherGitHubFiles fetches files from a GitHub repository.
// Returns uploads, warning messages for failed files, and an optional error result.
func (s *GeminiServer) gatherGitHubFiles(
	ctx context.Context, req mcp.CallToolRequest, githubFiles []string,
) ([]*FileUploadRequest, []string, *mcp.CallToolResult) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Processing GitHub files request")

	githubRepo := extractArgumentString(req, "github_repo", "")
	if githubRepo == "" {
		logger.Error("GitHub repository parameter missing")
		return nil, nil, createErrorResult("'github_repo' is required when using 'github_files'.")
	}

	githubRef := extractArgumentString(req, "github_ref", "")

	// Validate and fetch
	if err := validateFilePathArray(githubFiles, true); err != nil {
		logger.Error("GitHub file path validation failed: %v", err)
		return nil, nil, createErrorResult(err.Error())
	}

	fetchedUploads, fileErrs := fetchFromGitHub(ctx, s, githubRepo, githubRef, githubFiles)
	var warnings []string
	if len(fileErrs) > 0 {
		for _, err := range fileErrs {
			logger.Error("Error processing github file: %v", err)
			warnings = append(warnings, err.Error())
		}
		if len(fetchedUploads) == 0 {
			return nil, nil, createErrorResult(fmt.Sprintf("Error processing github files: %v", fileErrs))
		}
		// Partial failure: some files succeeded, some failed
		logger.Warn("Partial GitHub fetch: %d/%d files succeeded, %d failed",
			len(fetchedUploads), len(githubFiles), len(fileErrs))
	}
	return fetchedUploads, warnings, nil
}

// gatherLocalFiles fetches files from the local filesystem.
// Returns uploads, warning messages for skipped files, and an optional error result.
func (s *GeminiServer) gatherLocalFiles(ctx context.Context, filePaths []string) ([]*FileUploadRequest, []string, *mcp.CallToolResult) {
	if isHTTPTransport(ctx) {
		return nil, nil, createErrorResult("'file_paths' is not supported in HTTP transport mode. Use 'github_files' instead.")
	}

	localUploads, warnings, err := readLocalFiles(ctx, filePaths, s.config)
	if err != nil {
		getLoggerFromContext(ctx).Error("Error processing local files: %v", err)
		return nil, nil, createErrorResult(fmt.Sprintf("Error processing local files: %v", err))
	}
	return localUploads, warnings, nil
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
// Returns uploads, warning messages for skipped files, and an error for fatal issues.
func readLocalFiles(ctx context.Context, filePaths []string, config *Config) ([]*FileUploadRequest, []string, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Reading files from local filesystem (source: 'file_paths')")

	if config.FileReadBaseDir == "" {
		return nil, nil, fmt.Errorf("local file reading is disabled: no base directory configured")
	}

	var uploads []*FileUploadRequest
	var warnings []string

	for _, filePath := range filePaths {
		// Clean the path to resolve ".." etc. and prevent shenanigans.
		cleanedPath := filepath.Clean(filePath)

		// Prevent absolute paths and path traversal attempts.
		if filepath.IsAbs(cleanedPath) || strings.HasPrefix(cleanedPath, "..") {
			return nil, nil, fmt.Errorf("invalid path: %s. Only relative paths within the allowed directory are permitted", filePath)
		}

		fullPath := filepath.Join(config.FileReadBaseDir, cleanedPath)

		// Final, most important check: ensure the resolved path is still within the base directory.
		fileInfo, err := os.Lstat(fullPath)
		if err != nil {
			logger.Error("Failed to stat file %s: %v", filePath, err)
			warnings = append(warnings, fmt.Sprintf("%s: %v", filePath, err))
			continue
		}

		if fileInfo.IsDir() {
			logger.Warn("Skipping directory: %s", filePath)
			warnings = append(warnings, fmt.Sprintf("%s: is a directory", filePath))
			continue
		}

		if fileInfo.Mode()&os.ModeSymlink != 0 {
			linkDest, linkErr := os.Readlink(fullPath)
			if linkErr != nil {
				logger.Error("Failed to read symlink %s: %v", filePath, linkErr)
				warnings = append(warnings, fmt.Sprintf("%s: failed to read symlink", filePath))
				continue
			}
			if filepath.IsAbs(linkDest) || strings.HasPrefix(filepath.Clean(linkDest), "..") {
				logger.Error("Skipping unsafe symlink: %s -> %s", filePath, linkDest)
				warnings = append(warnings, fmt.Sprintf("%s: unsafe symlink", filePath))
				continue
			}
		}

		if !strings.HasPrefix(fullPath, config.FileReadBaseDir) {
			return nil, nil, fmt.Errorf("path traversal attempt detected: %s", filePath)
		}

		// Check file size before reading to prevent OOM on huge files
		if fileInfo.Size() > config.MaxFileSize {
			logger.Warn("Skipping file %s because it is too large: %d bytes, limit is %d", filePath, fileInfo.Size(), config.MaxFileSize)
			warnings = append(warnings, fmt.Sprintf("%s: file too large (%d bytes, limit %d)", filePath, fileInfo.Size(), config.MaxFileSize))
			continue
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			logger.Error("Failed to read file %s: %v", filePath, err)
			warnings = append(warnings, fmt.Sprintf("%s: %v", filePath, err))
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
	return uploads, warnings, nil
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
			logger.Error("Failed to upload file %s for caching: %v", uploadReq.FileName, err)
			// Fail fast on any upload failure — the caller will fall back to inline injection
			return "", fileInfos, fmt.Errorf("failed to upload file %s for caching: %w", uploadReq.FileName, err)
		}
		logger.Info("Successfully uploaded file %s with ID: %s for caching", uploadReq.FileName, fileInfo.ID)
		fileInfos = append(fileInfos, fileInfo)
		fileIDs = append(fileIDs, fileInfo.ID)
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

	checkModelStatus(ctx, response, cacheInfo.Model)
	return convertGenaiResponseToMCPResult(response), nil
}

// processWithFiles handles a Gemini API request with file attachments, reusing existing uploads if available
func (s *GeminiServer) processWithFiles(ctx context.Context, query string, uploads []*FileUploadRequest,
	uploadedFiles []*FileInfo, modelName string, config *genai.GenerateContentConfig, cacheErr error) (*mcp.CallToolResult, error) {

	logger := getLoggerFromContext(ctx)

	// Build all parts for a single user Content: query text first, then file parts.
	// Files must be in the same Content as the query — separate Content objects with the
	// same role are treated as distinct conversation turns and file context is dropped.
	parts := []*genai.Part{genai.NewPartFromText(query)}

	// Use pre-uploaded file URIs only when cache creation succeeded (cacheErr == nil).
	// When cache failed, the uploaded URIs may be stale or inaccessible — always
	// fall through to fresh inline injection from uploads instead.
	if len(uploadedFiles) > 0 && cacheErr == nil {
		logger.Info("Reusing %d pre-uploaded files (cache succeeded)", len(uploadedFiles))
		for _, fileInfo := range uploadedFiles {
			parts = append(parts, genai.NewPartFromURI(fileInfo.URI, fileInfo.MimeType))
		}
	} else if len(uploads) > 0 {
		logger.Info("Processing %d file(s) for inline injection", len(uploads))
		for _, upload := range uploads {
			// Inject text files directly as inline content — avoids Files API upload latency,
			// URI propagation delays, and silent empty-URI failures. Text content is well
			// within Gemini's 1M token context window and doesn't need Files API storage.
			if isTextMimeType(upload.MimeType) {
				logger.Info("Injecting %s (%d bytes) as inline text", upload.FileName, len(upload.Content))
				parts = append(parts, genai.NewPartFromText(fmt.Sprintf("--- File: %s ---\n%s", upload.FileName, string(upload.Content))))
				continue
			}

			// For binary/media files, use the Files API upload path
			logger.Info("Uploading binary file %s (%s) via Files API", upload.FileName, upload.MimeType)
			uploadConfig := &genai.UploadFileConfig{
				MIMEType:    upload.MimeType,
				DisplayName: upload.FileName,
			}
			file, err := withRetry(ctx, s.config, logger, "gemini.files.upload", func(ctx context.Context) (*genai.File, error) {
				return s.client.Files.Upload(ctx, bytes.NewReader(upload.Content), uploadConfig)
			})
			if err != nil || file.URI == "" {
				if err != nil {
					logger.Error("Failed to upload file %s: %v - skipping binary file", upload.FileName, err)
				} else {
					logger.Error("File %s uploaded but URI is empty - skipping binary file", upload.FileName)
				}
				parts = append(parts, genai.NewPartFromText(fmt.Sprintf(
					"--- File: %s ---\n[Error: This binary file (%s) could not be uploaded and cannot be displayed inline.]",
					upload.FileName, upload.MimeType)))
				continue
			}
			parts = append(parts, genai.NewPartFromURI(file.URI, upload.MimeType))
		}
	}

	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	// Generate content with files
	response, err := withRetry(ctx, s.config, logger, "gemini.models.generate_content", func(ctx context.Context) (*genai.GenerateContentResponse, error) {
		return s.client.Models.GenerateContent(ctx, modelName, contents, config)
	})
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		if cacheErr != nil {
			return createErrorResult(fmt.Sprintf("Error from Gemini API: %v\nCache error: %v", err, cacheErr)), nil
		}
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	checkModelStatus(ctx, response, modelName)
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
			return createErrorResult(fmt.Sprintf("Error from Gemini API: %v\nCache error: %v", err, cacheErr)), nil
		}
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	checkModelStatus(ctx, response, modelName)
	return convertGenaiResponseToMCPResult(response), nil
}
