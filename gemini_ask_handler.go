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

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResult("Internal error: Gemini client not properly initialized"), nil
	}

	// Process with files if provided
	if len(uploads) > 0 {
		return s.processWithFiles(ctx, query, uploads, modelName, config)
	}
	return s.processWithoutFiles(ctx, query, modelName, config)
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
		// Build a set of successfully fetched filenames to identify which ones failed
		fetched := make(map[string]bool, len(fetchedUploads))
		for _, u := range fetchedUploads {
			fetched[u.FileName] = true
		}
		for _, file := range githubFiles {
			if !fetched[file] {
				warnings = append(warnings, fmt.Sprintf("%s: could not be fetched from GitHub", file))
			}
		}
		for _, err := range fileErrs {
			logger.Error("Error processing github file: %v", err)
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
			warnings = append(warnings, fmt.Sprintf("%s: file not found or inaccessible", filePath))
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

		// Check file size before reading to prevent OOM on huge files.
		// Use os.Stat (follows symlinks) so we get the target's real size,
		// not the symlink entry size from Lstat above.
		realInfo, statErr := os.Stat(fullPath)
		if statErr != nil {
			logger.Error("Failed to stat target of %s: %v", filePath, statErr)
			warnings = append(warnings, fmt.Sprintf("%s: could not determine file size", filePath))
			continue
		}
		if realInfo.Size() > config.MaxFileSize {
			logger.Warn("Skipping file %s because it is too large: %d bytes, limit is %d", filePath, realInfo.Size(), config.MaxFileSize)
			warnings = append(warnings, fmt.Sprintf("%s: file too large (%d bytes, limit %d)", filePath, realInfo.Size(), config.MaxFileSize))
			continue
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			logger.Error("Failed to read file %s: %v", filePath, err)
			warnings = append(warnings, fmt.Sprintf("%s: could not read file", filePath))
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

// processWithFiles handles a Gemini API request with file attachments.
// Files are placed BEFORE the query to maximize implicit caching — Gemini caches
// the shared prefix of repeated requests, so stable file content at the front
// gets cached automatically across calls.
func (s *GeminiServer) processWithFiles(ctx context.Context, query string, uploads []*FileUploadRequest,
	modelName string, config *genai.GenerateContentConfig) (*mcp.CallToolResult, error) {

	logger := getLoggerFromContext(ctx)

	// Build parts: files first (cacheable prefix), then query last (variable suffix).
	// All parts must be in a single Content object — separate Content objects with the
	// same role are treated as distinct conversation turns and file context is dropped.
	var parts []*genai.Part

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

	// Query goes last — this is the variable part that changes between requests
	parts = append(parts, genai.NewPartFromText(query))

	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	// Generate content with files
	response, err := withRetry(ctx, s.config, logger, "gemini.models.generate_content", func(ctx context.Context) (*genai.GenerateContentResponse, error) {
		return s.client.Models.GenerateContent(ctx, modelName, contents, config)
	})
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	checkModelStatus(ctx, response, modelName)
	return convertGenaiResponseToMCPResult(response), nil
}

// processWithoutFiles handles a Gemini API request without file attachments
func (s *GeminiServer) processWithoutFiles(ctx context.Context, query string,
	modelName string, config *genai.GenerateContentConfig) (*mcp.CallToolResult, error) {

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
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	checkModelStatus(ctx, response, modelName)
	return convertGenaiResponseToMCPResult(response), nil
}
