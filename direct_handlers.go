package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	// --- File Handling Logic ---
	var uploads []*FileUploadRequest

	filePaths := extractArgumentStringArray(req, "file_paths")
	githubFiles := extractArgumentStringArray(req, "github_files")

	// Validation: Cannot use both file_paths and github_files
	if len(filePaths) > 0 && len(githubFiles) > 0 {
		return createErrorResult("Cannot use both 'file_paths' and 'github_files' in the same request."), nil
	}

	// Handle GitHub files
	if len(githubFiles) > 0 {
		githubRepo := extractArgumentString(req, "github_repo", "")
		if githubRepo == "" {
			return createErrorResult("'github_repo' is required when using 'github_files'."), nil
		}
		githubRef := extractArgumentString(req, "github_ref", "")
		if githubRef == "" {
			// If no ref is provided and no default is set, it's not necessarily an error, the API might use the repo's default.
			logger.Info("No 'github_ref' provided, will use repository's default branch.")
		}

		// Validate file paths
		for _, file := range githubFiles {
			if strings.Contains(file, "..") || strings.HasPrefix(file, "/") {
				return createErrorResult(fmt.Sprintf("invalid file path: %s. Path must be relative and within the repository.", file)), nil
			}
		}

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

	// Non-streaming search request with metadata extraction
	resp, err := withRetry(ctx, s.config, logger, "gemini.models.generate_content", func(ctx context.Context) (*genai.GenerateContentResponse, error) {
		return s.client.Models.GenerateContent(ctx, modelName, contents, config)
	})
	if err != nil {
		logger.Error("Gemini Search API error: %v", err)
		return createErrorResult(fmt.Sprintf("Error from Gemini Search API: %v", err)), nil
	}

	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		responseText = resp.Text()
		if metadata := resp.Candidates[0].GroundingMetadata; metadata != nil {
			if len(metadata.WebSearchQueries) > 0 && len(searchQueries) == 0 {
				searchQueries = metadata.WebSearchQueries
			}
			for _, chunk := range metadata.GroundingChunks {
				var source SourceInfo
				if web := chunk.Web; web != nil {
					if seenURLs[web.URI] {
						continue
					}
					source = SourceInfo{Title: web.Title, Type: "web"}
					seenURLs[web.URI] = true
				} else if retrievedCtx := chunk.RetrievedContext; retrievedCtx != nil {
					if seenURLs[retrievedCtx.URI] {
						continue
					}
					source = SourceInfo{Title: retrievedCtx.Title, Type: "retrieved_context"}
					seenURLs[retrievedCtx.URI] = true
				}
				if source.Title != "" {
					sources = append(sources, source)
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
	searchResp := SearchResponse{
		Answer:        responseText,
		Sources:       sources,
		SearchQueries: searchQueries,
	}

	// Convert to JSON and return as text content
	responseJSON, err := json.Marshal(searchResp)
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

// parseGitHubRepo parses a GitHub repository string into owner and repo.
// It accepts "owner/repo" or a full GitHub URL.
func parseGitHubRepo(repoStr string) (owner string, repo string, err error) {
	// Handle SSH URLs: git@github.com:owner/repo.git
	if strings.HasPrefix(repoStr, "git@") {
		repoStr = strings.Replace(repoStr, ":", "/", 1)
		repoStr = strings.Replace(repoStr, "git@", "https://", 1)
	}

	u, err := url.Parse(repoStr)
	if err != nil || u.Host == "" {
		// If parsing as a URL fails, it might be in "owner/repo" format
		parts := strings.Split(repoStr, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], nil
		}
		return "", "", fmt.Errorf("invalid github_repo format: %s", repoStr)
	}

	path := strings.TrimSuffix(u.Path, ".git")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid github_repo URL path: %s", u.Path)
	}
	return parts[0], parts[1], nil
}

// fetchFromGitHub fetches files from a GitHub repository.
func fetchFromGitHub(ctx context.Context, s *GeminiServer, repoURL, ref string, files []string) ([]*FileUploadRequest, []error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Fetching files from GitHub (source: 'github_files')")
	owner, repo, err := parseGitHubRepo(repoURL)
	if err != nil {
		return nil, []error{err}
	}
	logger.Info("Accessing GitHub repository: %s/%s", owner, repo)

	if len(files) > s.config.MaxGitHubFiles {
		return nil, []error{fmt.Errorf("too many files requested, limit is %d", s.config.MaxGitHubFiles)}
	}

	var uploads []*FileUploadRequest
	var wg sync.WaitGroup
	errChannel := make(chan error, len(files))
	uploadsChan := make(chan *FileUploadRequest, len(files))

	for _, file := range files {
		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()

			apiURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", s.config.GitHubAPIBaseURL, owner, repo, filePath)
			if ref != "" {
				apiURL += "?ref=" + ref
			}

			req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
			if err != nil {
				errChannel <- fmt.Errorf("failed to create request for %s: %w", filePath, err)
				return
			}

			req.Header.Set("Accept", "application/vnd.github.v3+json")
			if s.config.GitHubToken != "" {
				req.Header.Set("Authorization", "token "+s.config.GitHubToken)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errChannel <- fmt.Errorf("failed to fetch %s: %w", filePath, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				var bodyMsg string
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					bodyMsg = fmt.Sprintf("(could not read response body: %v)", err)
				} else {
					bodyMsg = string(body)
				}
				errChannel <- fmt.Errorf("failed to fetch %s: status %d, body: %s", filePath, resp.StatusCode, bodyMsg)
				return
			}
			logger.Info("Successfully connected to GitHub and fetched: %s", filePath)

			var fileContent struct {
				Content  string `json:"content"`
				Encoding string `json:"encoding"`
				Size     int64  `json:"size"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&fileContent); err != nil {
				errChannel <- fmt.Errorf("failed to decode response for %s: %w", filePath, err)
				return
			}

			if fileContent.Encoding != "base64" {
				errChannel <- fmt.Errorf("unsupported encoding for %s: %s", filePath, fileContent.Encoding)
				return
			}

			if fileContent.Size > s.config.MaxGitHubFileSize {
				errChannel <- fmt.Errorf("file %s is too large: %d bytes, limit is %d", filePath, fileContent.Size, s.config.MaxGitHubFileSize)
				return
			}

			decodedContent, err := base64.StdEncoding.DecodeString(fileContent.Content)
			if err != nil {
				errChannel <- fmt.Errorf("failed to decode content for %s: %w", filePath, err)
				return
			}
			logger.Info("Adding file to context: %s", filePath)

			uploadsChan <- &FileUploadRequest{
				FileName: filePath,
				MimeType: getMimeTypeFromPath(filePath),
				Content:  decodedContent,
			}
		}(file)
	}

	wg.Wait()
	close(errChannel)
	close(uploadsChan)

	var combinedErrs []error
	for err := range errChannel {
		combinedErrs = append(combinedErrs, err)
	}

	for upload := range uploadsChan {
		uploads = append(uploads, upload)
	}

	return uploads, combinedErrs
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

	if err := write("This server supports 3 Gemini 2.5 models and provides 2 main tools:\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- `gemini_ask`: For general queries, coding problems (default: code review system prompt)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- `gemini_search`: For search-grounded queries (default: search assistant system prompt)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Gemini 2.5 Pro
	if err := write("## Gemini 2.5 Pro\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Model ID**: `gemini-2.5-pro` (production)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Description**: Most powerful model with maximum accuracy and performance\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Context Window**: 1M tokens\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Best for**: Complex reasoning, detailed analysis, comprehensive code review\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Thinking Mode**: Yes (advanced reasoning capabilities)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Implicit Caching**: Yes (automatic optimization, 2048+ token minimum)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Explicit Caching**: Yes (user-controlled via `use_cache`)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Gemini 2.5 Flash
	if err := write("## Gemini 2.5 Flash\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Model ID**: `gemini-2.5-flash` (production)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Description**: Balanced price-performance with fast responses\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Context Window**: 32K tokens\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Best for**: General programming tasks, standard code review\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Thinking Mode**: Yes\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Implicit Caching**: Yes (automatic optimization, 1024+ token minimum)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Explicit Caching**: Yes (user-controlled via `use_cache`)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Gemini 2.5 Flash Lite
	if err := write("## Gemini 2.5 Flash Lite\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Model ID**: `gemini-2.5-flash-lite`\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Description**: Optimized for cost efficiency and low latency\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Context Window**: 32K tokens\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Best for**: Search queries, lightweight tasks, quick responses\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Thinking Mode**: Yes (off by default for speed/cost, can be enabled)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Implicit Caching**: No\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}
	if err := write("- **Explicit Caching**: No (preview limitation)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Tool Usage Examples
	if err := write("## Tool Usage Examples\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// gemini_ask examples
	if err := write("### gemini_ask Examples\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**General Problem (non-coding):**\n```json\n{\n  \"query\": \"Explain quantum computing in simple terms\",\n  \"model\": \"gemini-2.5-flash\",\n  \"systemPrompt\": \"You are an expert science communicator. Explain complex topics clearly for a general audience.\"\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Coding Problem with Files and Cache:**\n```json\n{\n  \"query\": \"Review this code for security vulnerabilities and performance issues\",\n  \"model\": \"gemini-2.5-pro\",\n  \"file_paths\": [\"/path/to/auth.go\", \"/path/to/database.go\"],\n  \"use_cache\": true,\n  \"cache_ttl\": \"30m\",\n  \"enable_thinking\": true\n}\n```\n*Note: Default system prompt optimized for code review will be used*\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Custom System Prompt Override:**\n```json\n{\n  \"query\": \"Analyze this code architecture\",\n  \"model\": \"gemini-2.5-pro\",\n  \"systemPrompt\": \"You are a senior software architect. Focus on design patterns, scalability, and maintainability.\",\n  \"file_paths\": [\"/path/to/main.go\"]\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// gemini_search examples
	if err := write("### gemini_search Examples\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Basic Search:**\n```json\n{\n  \"query\": \"What are the latest developments in Go programming language?\",\n  \"model\": \"gemini-2.5-flash-lite\"\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Search with Time Filtering:**\n```json\n{\n  \"query\": \"Recent security vulnerabilities in JavaScript frameworks\",\n  \"model\": \"gemini-2.5-flash\",\n  \"start_time\": \"2024-01-01T00:00:00Z\",\n  \"end_time\": \"2024-12-31T23:59:59Z\"\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Search with Thinking Mode (Flash Lite):**\n```json\n{\n  \"query\": \"Compare the pros and cons of different cloud deployment strategies\",\n  \"model\": \"gemini-2.5-flash-lite\",\n  \"enable_thinking\": true,\n  \"thinking_budget_level\": \"medium\"\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// System Prompt Details
	if err := write("## System Prompt Details\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Default System Prompts:**\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- **gemini_ask**: Optimized for thorough code review (senior developer perspective)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- **gemini_search**: Helpful search assistant for accurate, up-to-date information\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Override via:**\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- `systemPrompt` parameter in requests\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- `GEMINI_SYSTEM_PROMPT` env variable (for gemini_ask)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- `GEMINI_SEARCH_SYSTEM_PROMPT` env variable (for gemini_search)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- Command line flags: `--gemini-system-prompt`\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// File Attachments
	if err := write("## File Attachments (gemini_ask only)\n\n"); err != nil {
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
	if err := write("## Caching (gemini_ask only)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Implicit Caching (Automatic):**\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- 75% token discount for requests with common prefixes\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- Pro: 2048+ tokens minimum\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- Flash: 1024+ tokens minimum\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- Keep content at the beginning of requests the same, add variable content at the end\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Explicit Caching (Manual):**\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- Available for Pro and Flash only\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- Use `use_cache: true` parameter\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- Custom TTL with `cache_ttl` (default: 10 minutes)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("```json\n// Enable explicit caching\n{\n  \"query\": \"Analyze this codebase structure\",\n  \"model\": \"gemini-2.5-flash\",\n  \"file_paths\": [\"/path/to/large/codebase\"],\n  \"use_cache\": true,\n  \"cache_ttl\": \"30m\"\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Thinking Mode
	if err := write("## Thinking Mode (both tools)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("All Gemini 2.5 models support thinking mode, which shows the model's detailed reasoning process. Flash-Lite has thinking off by default for speed/cost optimization.\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("**Thinking Budget Levels:**\n- `none`: 0 tokens (disabled)\n- `low`: 4,096 tokens\n- `medium`: 16,384 tokens\n- `high`: 24,576 tokens (maximum)\n\nOr use `thinking_budget` to set a specific token count (0-24,576).\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("```json\n// Enable thinking with budget level\n{\n  \"query\": \"Solve this complex algorithm problem step by step\",\n  \"model\": \"gemini-2.5-pro\",\n  \"enable_thinking\": true,\n  \"thinking_budget_level\": \"high\"\n}\n\n// Custom thinking budget\n{\n  \"query\": \"Debug this complex issue\",\n  \"model\": \"gemini-2.5-pro\",\n  \"enable_thinking\": true,\n  \"thinking_budget\": 12000\n}\n```\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Time Filtering
	if err := write("## Time Filtering (gemini_search only)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("Filter search results by publication date using RFC3339 format:\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- Use `start_time` and `end_time` together (both required)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("- Format: `YYYY-MM-DDTHH:MM:SSZ`\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	// Advanced Examples
	if err := write("## Advanced Examples\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResult("Error generating model list"), nil
	}

	if err := write("```json\n// Comprehensive code review with thinking and caching (gemini_ask)\n{\n  \"query\": \"Perform a thorough security and performance review of this codebase\",\n  \"model\": \"gemini-2.5-pro\",\n  \"file_paths\": [\n    \"/path/to/main.go\",\n    \"/path/to/auth.go\",\n    \"/path/to/database.go\"\n  ],\n  \"enable_thinking\": true,\n  \"thinking_budget_level\": \"medium\",\n  \"use_cache\": true,\n  \"cache_ttl\": \"1h\"\n}\n\n// Custom system prompt with file context (gemini_ask)\n{\n  \"query\": \"Suggest architectural improvements for better scalability\",\n  \"model\": \"gemini-2.5-pro\",\n  \"systemPrompt\": \"You are a senior software architect. Focus on scalability, maintainability, and best practices.\",\n  \"file_paths\": [\"/path/to/architecture/overview.md\"],\n  \"enable_thinking\": true\n}\n```\n"); err != nil {
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
