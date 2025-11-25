package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

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
// fetchFromGitHub fetches files from a GitHub repository.
func fetchFromGitHub(ctx context.Context, s *GeminiServer, repoURL, ref string, files []string) ([]*FileUploadRequest, []error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Starting GitHub file fetch operation")
	logger.Info("Repository URL: %s", repoURL)
	logger.Info("Reference: %s", ref)
	logger.Info("Files to fetch: %v", files)

	owner, repo, err := parseGitHubRepo(repoURL)
	if err != nil {
		logger.Error("Failed to parse GitHub repository URL '%s': %v", repoURL, err)
		return nil, []error{err}
	}
	logger.Info("Parsed repository - Owner: %s, Repo: %s", owner, repo)

	if len(files) > s.config.MaxGitHubFiles {
		logger.Error("Too many files requested: %d, limit is %d", len(files), s.config.MaxGitHubFiles)
		return nil, []error{fmt.Errorf("too many files requested, limit is %d", s.config.MaxGitHubFiles)}
	}

	logger.Info("GitHub API configuration - Base URL: %s", s.config.GitHubAPIBaseURL)
	logger.Info("GitHub API configuration - Token available: %t", s.config.GitHubToken != "")
	logger.Info("GitHub API configuration - Max file size: %d bytes", s.config.MaxGitHubFileSize)

	var uploads []*FileUploadRequest
	var wg sync.WaitGroup
	errChannel := make(chan error, len(files))
	uploadsChan := make(chan *FileUploadRequest, len(files))

	// Create a semaphore to limit concurrency
	concurrencyLimit := 4 // Limit to 4 concurrent downloads
	semaphore := make(chan struct{}, concurrencyLimit)

	// Create a reusable HTTP client with a timeout
	httpClient := &http.Client{
		Timeout: s.config.HTTPTimeout,
	}

	logger.Info("Starting concurrent file fetch for %d files with a limit of %d", len(files), concurrencyLimit)

	for _, file := range files {
		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			upload, err := fetchSingleFile(ctx, s, httpClient, owner, repo, filePath, ref)
			if err != nil {
				errChannel <- err
				return
			}
			uploadsChan <- upload
		}(file)
	}

	logger.Info("Waiting for all file fetch operations to complete...")
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

	logger.Info("GitHub file fetch operation completed")
	logger.Info("Successfully fetched files: %d", len(uploads))
	logger.Info("Errors encountered: %d", len(combinedErrs))

	if len(uploads) > 0 {
		logger.Info("Successfully fetched files:")
		for _, upload := range uploads {
			logger.Info("  - %s (%d bytes, %s)", upload.FileName, len(upload.Content), upload.MimeType)
		}
	}

	if len(combinedErrs) > 0 {
		logger.Error("Errors during fetch operation:")
		for i, err := range combinedErrs {
			logger.Error("  %d. %v", i+1, err)
		}
	}

	return uploads, combinedErrs
}

// fetchSingleFile fetches a single file from GitHub with retries and rate limit handling.
func fetchSingleFile(ctx context.Context, s *GeminiServer, client *http.Client, owner, repo, filePath, ref string) (*FileUploadRequest, error) {
	logger := getLoggerFromContext(ctx)
	startTime := time.Now()
	logger.Info("[%s] Starting fetch at %s", filePath, startTime.Format(time.RFC3339))

	apiURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", s.config.GitHubAPIBaseURL, owner, repo, filePath)
	if ref != "" {
		apiURL += "?ref=" + ref
	}
	logger.Info("[%s] Constructed API URL: %s", filePath, apiURL)

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			jitter := time.Duration(rand.Int63n(int64(500 * time.Millisecond)))
			sleepTime := backoff + jitter
			logger.Info("[%s] Retry attempt %d/%d. Sleeping for %v", filePath, attempt, maxRetries, sleepTime)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleepTime):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request for %s: %w", filePath, err)
		}

		// Use raw content type to get the file content directly
		req.Header.Set("Accept", "application/vnd.github.v3.raw")
		if s.config.GitHubToken != "" {
			req.Header.Set("Authorization", "token "+s.config.GitHubToken)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			logger.Warn("[%s] Request failed: %v", filePath, err)
			continue
		}
		defer resp.Body.Close()

		// Handle Rate Limits
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			remaining := resp.Header.Get("X-RateLimit-Remaining")
			if remaining == "0" || resp.StatusCode == http.StatusTooManyRequests {
				resetTimeStr := resp.Header.Get("X-RateLimit-Reset")
				resetTime, _ := strconv.ParseInt(resetTimeStr, 10, 64)
				waitTime := time.Until(time.Unix(resetTime, 0))

				if waitTime > 0 {
					logger.Warn("[%s] Rate limit exceeded. Reset in %v", filePath, waitTime)
					// If wait time is reasonable, we could wait, but for now just fail or retry if it's short
					// For this implementation, we'll treat it as a retryable error if we haven't exhausted retries
					lastErr = fmt.Errorf("rate limit exceeded")
					continue
				}
			}
		}

		if resp.StatusCode != http.StatusOK {
			// Read body for error message
			body, _ := io.ReadAll(resp.Body)
			bodyMsg := string(body)
			logger.Error("[%s] HTTP error %d: %s", filePath, resp.StatusCode, bodyMsg)

			if resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("server error %d", resp.StatusCode)
				continue
			}

			// Non-retryable errors
			var userMsg string
			switch resp.StatusCode {
			case http.StatusNotFound:
				if s.config.GitHubToken == "" {
					userMsg = fmt.Sprintf("repository '%s/%s' not found or private (no GitHub token configured)", owner, repo)
				} else {
					userMsg = fmt.Sprintf("file '%s' not found in repository '%s/%s' (ref: %s)", filePath, owner, repo, ref)
				}
			case http.StatusUnauthorized:
				userMsg = fmt.Sprintf("authentication failed - check GitHub token permissions")
			case http.StatusForbidden:
				userMsg = fmt.Sprintf("access denied to '%s/%s'", owner, repo)
			default:
				userMsg = fmt.Sprintf("GitHub API error: status %d", resp.StatusCode)
			}
			return nil, fmt.Errorf("%s", userMsg)
		}

		// Check content length if available
		if resp.ContentLength > s.config.MaxGitHubFileSize {
			return nil, fmt.Errorf("file %s is too large: %d bytes, limit is %d", filePath, resp.ContentLength, s.config.MaxGitHubFileSize)
		}

		// Read content
		content, err := io.ReadAll(io.LimitReader(resp.Body, s.config.MaxGitHubFileSize+1))
		if err != nil {
			lastErr = fmt.Errorf("failed to read content: %w", err)
			continue
		}

		if int64(len(content)) > s.config.MaxGitHubFileSize {
			return nil, fmt.Errorf("file %s is too large (read %d bytes), limit is %d", filePath, len(content), s.config.MaxGitHubFileSize)
		}

		totalTime := time.Since(startTime)
		logger.Info("[%s] Successfully fetched file (%d bytes) in %v", filePath, len(content), totalTime)

		return &FileUploadRequest{
			FileName: filePath,
			MimeType: getMimeTypeFromPath(filePath),
			Content:  content,
		}, nil
	}

	return nil, fmt.Errorf("failed to fetch %s after %d retries: %v", filePath, maxRetries, lastErr)
}
