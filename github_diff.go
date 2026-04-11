package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genai"
)

// diffTruncMarkerReserve is the byte budget we reserve for the trailing
// truncation marker. The longest realistic marker is
// "\n[truncated: 9999 more hunk(s) omitted]\n" (~40 bytes); 64 leaves slack.
const diffTruncMarkerReserve = 64

// truncateDiff cuts a unified diff at hunk boundaries (never mid-hunk) so the
// result stays at or below maxBytes. It returns the possibly-shortened diff
// and a boolean indicating whether truncation occurred. If the diff contains
// no hunk markers, or the file header alone exceeds the budget, it falls back
// to byteTruncateDiff which guarantees the same ≤ maxBytes invariant.
//
// The marker `[truncated: N more hunk(s) omitted]` is appended when truncation
// removes hunks, so Gemini knows the view is partial. The marker's byte cost
// is reserved up-front via diffTruncMarkerReserve so it cannot push the final
// result over the contract.
func truncateDiff(patch string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 || int64(len(patch)) <= maxBytes {
		return patch, false
	}

	// Very small budgets can't fit both content and a reserved marker footer
	// at hunk granularity. Byte-level truncation still preserves the
	// ≤ maxBytes guarantee.
	if maxBytes <= diffTruncMarkerReserve {
		return byteTruncateDiff(patch, maxBytes)
	}

	effectiveMax := maxBytes - diffTruncMarkerReserve

	// Split on hunk markers — keep the leading file header and any hunks
	// that still fit under the effective budget.
	idx := strings.Index(patch, "@@ ")
	if idx == -1 {
		return byteTruncateDiff(patch, maxBytes)
	}

	// Split preserving the markers on the following chunks.
	header := patch[:idx]
	if int64(len(header)) > effectiveMax {
		// Even the file header blows our budget. Byte-cut.
		return byteTruncateDiff(patch, maxBytes)
	}
	rest := patch[idx:]
	hunks := splitHunks(rest)

	var b strings.Builder
	b.WriteString(header)
	kept := 0
	for _, h := range hunks {
		if int64(b.Len()+len(h)) > effectiveMax {
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
	finalMarker := fmt.Sprintf("\n[truncated: %d more hunk(s) omitted]\n", omitted)
	b.WriteString(finalMarker)
	result := b.String()
	// Defensive guard: reserved budget should always hold, but if a future
	// edit ever breaks it, byte-cut to preserve the contract.
	if int64(len(result)) > maxBytes {
		return byteTruncateDiff(result, maxBytes)
	}
	return result, true
}

// byteTruncateDiff is the fallback when hunk-boundary truncation is not
// possible (no hunks, header alone exceeds budget, or very small budgets).
// It guarantees the returned string is ≤ maxBytes.
func byteTruncateDiff(patch string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 {
		return "", true
	}
	if int64(len(patch)) <= maxBytes {
		return patch, false
	}
	const marker = "\n[truncated: diff exceeds size limit]"
	if int64(len(marker)) >= maxBytes {
		// Budget too small even for the marker; hard cut.
		return patch[:maxBytes], true
	}
	cut := int(maxBytes) - len(marker)
	return patch[:cut] + marker, true
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

	if classifyErr := classifyGitHubStatus(ctx, resp, logger); classifyErr != nil {
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
// Returns nil for a clean 200 OK. On rate-limit responses it also sleeps
// (inside the current attempt) until the reset point when that is feasible,
// restoring the pre-b0c5d5b rate-limit-reset fidelity while still handing
// the retry loop a retryable error so withRetry's own jittered backoff can
// take one more round if needed.
func classifyGitHubStatus(ctx context.Context, resp *http.Response, logger Logger) error {
	// Treat rate limit and server errors as retryable by returning errors
	// that isRetryableError recognises.
	if resp.StatusCode == http.StatusTooManyRequests {
		waitForGitHubRateLimitReset(ctx, resp.Header, logger)
		return fmt.Errorf("github api rate limit exceeded (429)")
	}
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		waitForGitHubRateLimitReset(ctx, resp.Header, logger)
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

// waitForGitHubRateLimitReset honours the X-RateLimit-Reset header when the
// wait is feasible (≤ 2 minutes), sleeping the current attempt until the
// reset point. On long resets it returns immediately and lets the caller
// escalate via the standard retry backoff.
func waitForGitHubRateLimitReset(ctx context.Context, header http.Header, logger Logger) {
	// Parse X-RateLimit-Reset as a Unix timestamp; fall back to a 5-minute
	// synthetic reset when missing or malformed so we don't hot-loop.
	var waitTime time.Duration
	if resetStr := header.Get("X-RateLimit-Reset"); resetStr != "" {
		if resetTime, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			waitTime = time.Until(time.Unix(resetTime, 0))
		} else {
			waitTime = 5 * time.Minute
		}
	} else {
		waitTime = 5 * time.Minute
	}

	if waitTime <= 0 {
		return
	}
	if waitTime > 2*time.Minute {
		logger.Warn("GitHub rate limit reset is too far away (%s); deferring to retry backoff", waitTime)
		return
	}
	logger.Warn("GitHub rate limit hit; sleeping %s until reset", waitTime)
	select {
	case <-ctx.Done():
		return
	case <-time.After(waitTime):
		return
	}
}

// makeTextPart wraps a labelled block into a genai text Part.
func makeTextPart(header, body string) *genai.Part {
	return genai.NewPartFromText(fmt.Sprintf("%s\n%s", header, body))
}

// sanitizeUntrustedBlockContent neutralizes any line that looks like a
// server-emitted context-block header (starting with "--- ") inside
// attacker-controlled text. It prevents a malicious PR body / commit message /
// review comment from impersonating a legitimate block header such as
// "--- File: secrets.env ---". The fix is a minimal transform: two leading
// spaces in front of any offending line, which breaks the exact header prefix
// without altering semantics for Gemini.
func sanitizeUntrustedBlockContent(s string) string {
	if !strings.Contains(s, "--- ") {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "--- ") {
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
}
