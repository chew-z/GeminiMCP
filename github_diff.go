package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"google.golang.org/genai"
)

// truncateDiff cuts a unified diff at hunk boundaries (never mid-hunk) so the
// result stays at or below maxBytes. It returns the possibly-shortened diff
// and a boolean indicating whether truncation occurred. If the diff contains
// no hunk markers, it falls back to byte truncation.
//
// The marker `[truncated: N more hunks omitted]` is appended when truncation
// removes hunks, so Gemini knows the view is partial.
func truncateDiff(patch string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 || int64(len(patch)) <= maxBytes {
		return patch, false
	}

	// Split on hunk markers — keep the leading file header and any hunks
	// that still fit under the budget.
	idx := strings.Index(patch, "@@ ")
	if idx == -1 {
		// No unified-diff hunks at all; do a plain byte cut.
		return patch[:maxBytes] + "\n[truncated: diff exceeds size limit]", true
	}

	// Split preserving the markers on the following chunks.
	header := patch[:idx]
	rest := patch[idx:]
	hunks := splitHunks(rest)

	var b strings.Builder
	b.WriteString(header)
	kept := 0
	for _, h := range hunks {
		if int64(b.Len()+len(h)) > maxBytes {
			break
		}
		b.WriteString(h)
		kept++
	}
	omitted := len(hunks) - kept
	if omitted <= 0 {
		// No hunks omitted — maxBytes was larger than we first thought.
		return patch, false
	}
	fmt.Fprintf(&b, "\n[truncated: %d more hunk(s) omitted]\n", omitted)
	return b.String(), true
}

// splitHunks splits a rest-of-patch string at `\n@@ ` boundaries while keeping
// each hunk prefixed with its own `@@ ` marker.
func splitHunks(rest string) []string {
	var out []string
	for len(rest) > 0 {
		// rest must currently start with "@@ "
		next := strings.Index(rest[3:], "\n@@ ")
		if next == -1 {
			out = append(out, rest)
			break
		}
		// next is relative to rest[3:], so the split point in rest is next+3+1 (skip the \n).
		end := next + 3 + 1
		out = append(out, rest[:end])
		rest = rest[end:]
	}
	return out
}

// githubAPIGet performs a GET against the GitHub REST API with auth, retry,
// and rate-limit classification via withRetry[T]. The returned body is
// length-limited to maxBytes+1 so callers can detect overflow.
func githubAPIGet(ctx context.Context, s *GeminiServer, url, accept string, maxBytes int64) ([]byte, error) {
	logger := getLoggerFromContext(ctx)
	client := &http.Client{Timeout: s.config.HTTPTimeout}

	return withRetry(ctx, s.config, logger, "github.api.get", func(ctx context.Context) ([]byte, error) {
		return githubAPIGetOnce(ctx, client, s, url, accept, maxBytes, logger)
	})
}

// githubAPIGetOnce issues a single GET attempt and classifies the response.
// It is split out so githubAPIGet's retry wrapper stays trivial.
func githubAPIGetOnce(
	ctx context.Context, client *http.Client, s *GeminiServer,
	url, accept string, maxBytes int64, logger Logger,
) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if s.config.GitHubToken != "" {
		req.Header.Set("Authorization", "token "+s.config.GitHubToken)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logger.Debug("error closing github api response body: %v", cerr)
		}
	}()

	if classifyErr := classifyGitHubStatus(resp); classifyErr != nil {
		return nil, classifyErr
	}

	limit := maxBytes
	if limit <= 0 {
		limit = 1 << 20 // 1MB default guard for metadata calls
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("failed reading github api response: %w", err)
	}
	return body, nil
}

// classifyGitHubStatus maps non-200 responses to retryable / fatal errors.
// Returns nil for a clean 200 OK.
func classifyGitHubStatus(resp *http.Response) error {
	// Treat rate limit and server errors as retryable by returning errors
	// that isRetryableError recognises.
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("github api rate limit exceeded (429)")
	}
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return fmt.Errorf("github api rate limit exceeded (403)")
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("github api server error %d (unavailable)", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if rerr != nil {
		return fmt.Errorf("github api %d: (failed to read body: %v)", resp.StatusCode, rerr)
	}
	return fmt.Errorf("github api %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

// makeTextPart wraps a labelled block into a genai text Part.
func makeTextPart(header, body string) *genai.Part {
	return genai.NewPartFromText(fmt.Sprintf("%s\n%s", header, body))
}
