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
