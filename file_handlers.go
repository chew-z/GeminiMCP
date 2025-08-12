package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

	logger.Info("Starting concurrent file fetch for %d files", len(files))

	for _, file := range files {
		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()

			startTime := time.Now()
			logger.Info("[%s] Starting fetch at %s", filePath, startTime.Format(time.RFC3339))

			apiURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", s.config.GitHubAPIBaseURL, owner, repo, filePath)
			if ref != "" {
				apiURL += "?ref=" + ref
			}
			logger.Info("[%s] Constructed API URL: %s", filePath, apiURL)

			req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
			if err != nil {
				logger.Error("[%s] Failed to create HTTP request: %v", filePath, err)
				errChannel <- fmt.Errorf("failed to create request for %s: %w", filePath, err)
				return
			}

			req.Header.Set("Accept", "application/vnd.github.v3+json")
			if s.config.GitHubToken != "" {
				req.Header.Set("Authorization", "token "+s.config.GitHubToken)
				logger.Info("[%s] Using GitHub token for authentication", filePath)
			} else {
				logger.Info("[%s] No GitHub token provided, using unauthenticated request", filePath)
			}

			logger.Info("[%s] Sending request to GitHub API...", filePath)

			resp, err := http.DefaultClient.Do(req)
			httpTime := time.Now()
			if err != nil {
				logger.Error("[%s] HTTP request failed after %v: %v", filePath, httpTime.Sub(startTime), err)
				errChannel <- fmt.Errorf("failed to fetch %s: %w", filePath, err)
				return
			}
			defer resp.Body.Close()

			logger.Info("[%s] Received HTTP response after %v - Status: %d (%s)", filePath, httpTime.Sub(startTime), resp.StatusCode, resp.Status)
			logger.Info("[%s] Response headers - Content-Type: %s, Content-Length: %s", filePath,
				resp.Header.Get("Content-Type"), resp.Header.Get("Content-Length"))

			if resp.StatusCode != http.StatusOK {
				logger.Error("[%s] HTTP request failed with status %d", filePath, resp.StatusCode)
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					logger.Error("[%s] Failed to read error response body: %v", filePath, err)
				} else {
					bodyMsg := string(body)
					logger.Error("[%s] Error response body: %s", filePath, bodyMsg)
				}

				// Provide more user-friendly error messages based on HTTP status
				var userMsg string
				switch resp.StatusCode {
				case http.StatusNotFound:
					if s.config.GitHubToken == "" {
						userMsg = fmt.Sprintf("repository '%s/%s' not found or private (no GitHub token configured)", owner, repo)
					} else {
						userMsg = fmt.Sprintf("file '%s' not found in repository '%s/%s' (ref: %s) - repository may be private or file doesn't exist", filePath, owner, repo, ref)
					}
				case http.StatusUnauthorized:
					userMsg = fmt.Sprintf("authentication failed - check GitHub token permissions for '%s/%s'", owner, repo)
				case http.StatusForbidden:
					userMsg = fmt.Sprintf("access denied to '%s/%s' - token may lack required permissions", owner, repo)
				case http.StatusTooManyRequests:
					userMsg = "rate limit exceeded for GitHub API - try again later"
				default:
					userMsg = fmt.Sprintf("GitHub API error for %s: status %d", filePath, resp.StatusCode)
				}

				errChannel <- fmt.Errorf("%s", userMsg)
				return
			}
			logger.Info("[%s] Successfully received response from GitHub API", filePath)

			logger.Info("[%s] Parsing JSON response...", filePath)
			var fileContent struct {
				Content  string `json:"content"`
				Encoding string `json:"encoding"`
				Size     int64  `json:"size"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&fileContent); err != nil {
				logger.Error("[%s] Failed to parse JSON response: %v", filePath, err)
				errChannel <- fmt.Errorf("failed to decode response for %s: %w", filePath, err)
				return
			}

			logger.Info("[%s] File metadata - Size: %d bytes, Encoding: %s", filePath, fileContent.Size, fileContent.Encoding)

			if fileContent.Encoding != "base64" {
				logger.Error("[%s] Unsupported encoding: %s (expected base64)", filePath, fileContent.Encoding)
				errChannel <- fmt.Errorf("unsupported encoding for %s: %s", filePath, fileContent.Encoding)
				return
			}

			if fileContent.Size > s.config.MaxGitHubFileSize {
				logger.Error("[%s] File too large: %d bytes, limit is %d bytes", filePath, fileContent.Size, s.config.MaxGitHubFileSize)
				errChannel <- fmt.Errorf("file %s is too large: %d bytes, limit is %d", filePath, fileContent.Size, s.config.MaxGitHubFileSize)
				return
			}

			logger.Info("[%s] Decoding base64 content...", filePath)
			decodedContent, err := base64.StdEncoding.DecodeString(fileContent.Content)
			if err != nil {
				logger.Error("[%s] Failed to decode base64 content: %v", filePath, err)
				errChannel <- fmt.Errorf("failed to decode content for %s: %w", filePath, err)
				return
			}

			totalTime := time.Since(startTime)
			logger.Info("[%s] Successfully processed file - Decoded size: %d bytes", filePath, len(decodedContent))
			logger.Info("[%s] Adding file to context with MIME type: %s", filePath, getMimeTypeFromPath(filePath))
			logger.Info("[%s] File fetch completed in %v", filePath, totalTime)

			uploadsChan <- &FileUploadRequest{
				FileName: filePath,
				MimeType: getMimeTypeFromPath(filePath),
				Content:  decodedContent,
			}
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
