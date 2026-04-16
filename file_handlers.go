package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
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

// buildHTTPClient creates a reusable HTTP client with the configured timeout.
func buildHTTPClient(cfg *Config) *http.Client {
	return &http.Client{Timeout: cfg.HTTPTimeout}
}

// drainResults collects values from the fan-out channels into ordered slices.
func drainResults(uploadsChan <-chan *FileUploadRequest, errChannel <-chan error) ([]*FileUploadRequest, []error) {
	var uploads []*FileUploadRequest
	var errs []error
	for err := range errChannel {
		errs = append(errs, err)
	}
	for upload := range uploadsChan {
		uploads = append(uploads, upload)
	}
	return uploads, errs
}

// logFetchSummary logs the per-file outcome of a batch fetch.
func logFetchSummary(logger Logger, uploads []*FileUploadRequest, errs []error) {
	logger.Info("GitHub file fetch operation completed")
	logger.Info("Successfully fetched files: %d", len(uploads))
	logger.Info("Errors encountered: %d", len(errs))

	if len(uploads) > 0 {
		logger.Info("Successfully fetched files:")
		for _, upload := range uploads {
			logger.Info("  - %s (%d bytes, %s)", upload.FileName, len(upload.Content), upload.MimeType)
		}
	}

	if len(errs) > 0 {
		logger.Error("Errors during fetch operation:")
		for i, err := range errs {
			logger.Error("  %d. %v", i+1, err)
		}
	}
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

	var wg sync.WaitGroup
	errChannel := make(chan error, len(files))
	uploadsChan := make(chan *FileUploadRequest, len(files))

	concurrencyLimit := 4
	semaphore := make(chan struct{}, concurrencyLimit)
	httpClient := buildHTTPClient(s.config)

	logger.Info("Starting concurrent file fetch for %d files with a limit of %d", len(files), concurrencyLimit)

	for _, file := range files {
		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()
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

	uploads, combinedErrs := drainResults(uploadsChan, errChannel)

	// Sort uploads by filename to ensure deterministic ordering.
	// Concurrent fetches return in completion order, but implicit caching
	// requires a stable prefix across identical requests.
	sort.Slice(uploads, func(i, j int) bool {
		return uploads[i].FileName < uploads[j].FileName
	})

	logFetchSummary(logger, uploads, combinedErrs)
	return uploads, combinedErrs
}

// handleRateLimitResponse inspects a 403/429 response. Returns:
//   - ctxErr != nil when the context was cancelled while waiting; body has been closed.
//   - retryErr != nil when the caller should set lastErr=retryErr and continue the retry loop; body has been closed.
//   - both nil when the response was not actually rate-limited or the reset window has elapsed;
//     body is left OPEN so the caller's status-code handler can read the genuine GitHub error payload.
func handleRateLimitResponse(ctx context.Context, resp *http.Response, logger Logger, filePath string) (ctxErr, retryErr error) {
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	logger.Debug("[%s] ratelimit headers: status=%d remaining=%q reset=%q",
		filePath, resp.StatusCode, remaining, resp.Header.Get("X-RateLimit-Reset"))
	if remaining != "0" && resp.StatusCode != http.StatusTooManyRequests {
		logger.Debug("[%s] ratelimit: not throttled, caller owns body", filePath)
		return nil, nil
	}

	resetTimeStr := resp.Header.Get("X-RateLimit-Reset")
	resetTime, parseErr := strconv.ParseInt(resetTimeStr, 10, 64)
	if parseErr != nil {
		resetTime = time.Now().Add(5 * time.Minute).Unix()
		logger.Debug("[%s] Failed to parse reset time '%s': %v, using fallback", filePath, resetTimeStr, parseErr)
	}
	waitTime := time.Until(time.Unix(resetTime, 0))

	if waitTime <= 0 {
		logger.Debug("[%s] ratelimit: reset window already elapsed, caller owns body", filePath)
		return nil, nil
	}

	// We're about to wait/retry — discard the response body now.
	if closeErr := resp.Body.Close(); closeErr != nil {
		logger.Debug("[%s] Error closing response body during rate limit handling: %v", filePath, closeErr)
	}

	logger.Warn("[%s] Rate limit exceeded. Reset in %v", filePath, waitTime)
	if waitTime <= 2*time.Minute {
		logger.Debug("[%s] ratelimit: waiting %v in-process before signalling retry", filePath, waitTime)
		select {
		case <-ctx.Done():
			return ctx.Err(), nil
		case <-time.After(waitTime):
			return nil, fmt.Errorf("rate limit exceeded")
		}
	}
	logger.Debug("[%s] ratelimit: wait %v exceeds 2m cap, returning retryable error", filePath, waitTime)
	return nil, fmt.Errorf("rate limit exceeded")
}

// readErrorBody reads and closes resp.Body, returning its contents as a string
// suitable for logging. On failure it returns a diagnostic message instead.
func readErrorBody(resp *http.Response, logger Logger, filePath string) string {
	body, readErr := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		logger.Debug("[%s] Error closing response body after error status %d: %v", filePath, resp.StatusCode, closeErr)
	}
	if readErr != nil {
		logger.Debug("[%s] Error reading response body for status %d: %v", filePath, resp.StatusCode, readErr)
		return fmt.Sprintf("failed to read error body: %v", readErr)
	}
	return string(body)
}

// mapNonOKStatus converts a non-OK GitHub response into a user-facing error.
// Returns (err, retryable). When retryable is true the caller should set
// lastErr=err and continue the retry loop; otherwise it is a terminal error.
func mapNonOKStatus(statusCode int, owner, repo, filePath, ref string, tokenSet bool) (err error, retryable bool) {
	if statusCode >= 500 {
		return fmt.Errorf("server error %d", statusCode), true
	}

	var userMsg string
	switch statusCode {
	case http.StatusNotFound:
		if !tokenSet {
			userMsg = fmt.Sprintf("repository '%s/%s' not found or private (no GitHub token configured)", owner, repo)
		} else {
			userMsg = fmt.Sprintf("file '%s' not found in repository '%s/%s' (ref: %s)", filePath, owner, repo, ref)
		}
	case http.StatusUnauthorized:
		userMsg = "authentication failed - check GitHub token permissions"
	case http.StatusForbidden:
		userMsg = fmt.Sprintf("access denied to '%s/%s'", owner, repo)
	default:
		userMsg = fmt.Sprintf("GitHub API error: status %d", statusCode)
	}
	return fmt.Errorf("%s", userMsg), false
}

// fetchAttemptOutcome captures the result of a single fetch attempt. Exactly
// one of upload / retryErr / fatalErr is set.
type fetchAttemptOutcome struct {
	upload   *FileUploadRequest
	retryErr error
	fatalErr error
}

// fetchAttemptParams groups the immutable request context for a single fetch attempt.
type fetchAttemptParams struct {
	s         *GeminiServer
	client    *http.Client
	apiURL    string
	owner     string
	repo      string
	filePath  string
	ref       string
	startTime time.Time
}

func performFetchRequest(ctx context.Context, p fetchAttemptParams) (*http.Response, fetchAttemptOutcome) {
	logger := getLoggerFromContext(ctx)
	req, err := http.NewRequestWithContext(ctx, "GET", p.apiURL, nil)
	if err != nil {
		return nil, fetchAttemptOutcome{fatalErr: fmt.Errorf("failed to create request for %s: %w", p.filePath, err)}
	}
	req.Header.Set("Accept", "application/vnd.github.v3.raw")
	if p.s.config.GitHubToken != "" {
		req.Header.Set("Authorization", "token "+p.s.config.GitHubToken)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		logger.Warn("[%s] Request failed: %v", p.filePath, err)
		return nil, fetchAttemptOutcome{retryErr: fmt.Errorf("HTTP request failed: %w", err)}
	}
	return resp, fetchAttemptOutcome{}
}

func consumeFetchBody(ctx context.Context, resp *http.Response, p fetchAttemptParams) fetchAttemptOutcome {
	logger := getLoggerFromContext(ctx)

	if resp.ContentLength > p.s.config.MaxGitHubFileSize {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Debug("[%s] Error closing response body for oversized file: %v", p.filePath, closeErr)
		}
		return fetchAttemptOutcome{fatalErr: fmt.Errorf(
			"file %s is too large: %d bytes, limit is %d",
			p.filePath, resp.ContentLength, p.s.config.MaxGitHubFileSize,
		)}
	}

	content, readErr := io.ReadAll(io.LimitReader(resp.Body, p.s.config.MaxGitHubFileSize+1))
	if closeErr := resp.Body.Close(); closeErr != nil {
		logger.Debug("[%s] Error closing response body after reading content: %v", p.filePath, closeErr)
	}
	if readErr != nil {
		return fetchAttemptOutcome{retryErr: fmt.Errorf("failed to read content: %w", readErr)}
	}

	if int64(len(content)) > p.s.config.MaxGitHubFileSize {
		return fetchAttemptOutcome{fatalErr: fmt.Errorf(
			"file %s is too large (read %d bytes), limit is %d",
			p.filePath, len(content), p.s.config.MaxGitHubFileSize,
		)}
	}

	if len(content) == 0 {
		logger.Warn("[%s] File fetched successfully but content is empty (0 bytes)", p.filePath)
	}

	totalTime := time.Since(p.startTime)
	logger.Info("[%s] Successfully fetched file (%d bytes) in %v", p.filePath, len(content), totalTime)

	return fetchAttemptOutcome{upload: &FileUploadRequest{
		FileName: p.filePath,
		MimeType: getMimeTypeFromPath(p.filePath),
		Content:  content,
	}}
}

// fetchAttempt performs a single HTTP fetch for a file, returning one of a
// successful upload, a retryable error, or a fatal error.
func fetchAttempt(ctx context.Context, p fetchAttemptParams) fetchAttemptOutcome {
	logger := getLoggerFromContext(ctx)

	resp, pre := performFetchRequest(ctx, p)
	if resp == nil {
		return pre
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		ctxErr, retryErr := handleRateLimitResponse(ctx, resp, logger, p.filePath)
		if ctxErr != nil {
			return fetchAttemptOutcome{fatalErr: ctxErr}
		}
		if retryErr != nil {
			return fetchAttemptOutcome{retryErr: retryErr}
		}
		// Rate-limit headers said "not actually limited"; body is left OPEN so
		// the status-code check below can feed readErrorBody with the real
		// GitHub payload and decide whether the bare 403 is fatal.
	}

	if resp.StatusCode != http.StatusOK {
		bodyMsg := readErrorBody(resp, logger, p.filePath)
		logger.Error("[%s] HTTP error %d: %s", p.filePath, resp.StatusCode, bodyMsg)

		userErr, retryable := mapNonOKStatus(resp.StatusCode, p.owner, p.repo, p.filePath, p.ref, p.s.config.GitHubToken != "")
		if retryable {
			return fetchAttemptOutcome{retryErr: userErr}
		}
		return fetchAttemptOutcome{fatalErr: userErr}
	}

	return consumeFetchBody(ctx, resp, p)
}

// waitBeforeRetry sleeps for the exponential-backoff-plus-jitter interval for
// the given attempt. Returns ctx.Err() if the context is cancelled.
func waitBeforeRetry(ctx context.Context, logger Logger, filePath string, attempt, maxRetries int) error {
	backoff := time.Duration(1<<uint(attempt)) * time.Second
	jitter := time.Duration(rand.Int63n(int64(500 * time.Millisecond)))
	sleepTime := backoff + jitter
	logger.Info("[%s] Retry attempt %d/%d. Sleeping for %v", filePath, attempt, maxRetries, sleepTime)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(sleepTime):
		return nil
	}
}

// buildContentsAPIURL builds the raw-content endpoint URL for a file path.
func buildContentsAPIURL(baseURL, owner, repo, filePath, ref string) string {
	// URL-escape individual path segments to handle filenames with spaces or
	// special chars, while preserving the directory separator slashes.
	segments := strings.Split(filePath, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	apiURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", baseURL, owner, repo, strings.Join(segments, "/"))
	if ref != "" {
		apiURL += "?ref=" + url.QueryEscape(ref)
	}
	return apiURL
}

// fetchSingleFile fetches a single file from GitHub with retries and rate limit handling.
func fetchSingleFile(ctx context.Context, s *GeminiServer, client *http.Client, owner, repo, filePath, ref string) (*FileUploadRequest, error) {
	logger := getLoggerFromContext(ctx)
	startTime := time.Now()
	logger.Info("[%s] Starting fetch at %s", filePath, startTime.Format(time.RFC3339))

	apiURL := buildContentsAPIURL(s.config.GitHubAPIBaseURL, owner, repo, filePath, ref)
	logger.Info("[%s] Constructed API URL: %s", filePath, apiURL)

	params := fetchAttemptParams{
		s:         s,
		client:    client,
		apiURL:    apiURL,
		owner:     owner,
		repo:      repo,
		filePath:  filePath,
		ref:       ref,
		startTime: startTime,
	}

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := waitBeforeRetry(ctx, logger, filePath, attempt, maxRetries); err != nil {
				return nil, err
			}
		}

		outcome := fetchAttempt(ctx, params)
		if outcome.fatalErr != nil {
			return nil, outcome.fatalErr
		}
		if outcome.upload != nil {
			return outcome.upload, nil
		}
		lastErr = outcome.retryErr
	}

	return nil, fmt.Errorf("failed to fetch %s after %d retries: %v", filePath, maxRetries, lastErr)
}
