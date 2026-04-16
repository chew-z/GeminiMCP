package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/genai"
)

// githubPRMeta is the slice of /repos/.../pulls/{n} we care about.
type githubPRMeta struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
	Base struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"base"`
	Head struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"head"`
}

// githubPRReviewComment is a single line-anchored review comment.
type githubPRReviewComment struct {
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Position int    `json:"position"`
	Body     string `json:"body"`
}

// gatherPullRequest fetches a PR bundle and emits one or more text parts plus
// inventory metadata. Partial failures (e.g. review-comments endpoint down) are
// surfaced as warnings while preserving whatever content did succeed.
func (s *GeminiServer) gatherPullRequest(
	ctx context.Context, owner, repo string, prNumber int,
) ([]*genai.Part, *prInventory, []string, error) {

	logger := getLoggerFromContext(ctx)
	logger.Info("Fetching GitHub PR %s/%s#%d", owner, repo, prNumber)

	base := strings.TrimRight(s.config.GitHubAPIBaseURL, "/")
	prURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", base, owner, repo, prNumber)

	meta, err := s.fetchPRMeta(ctx, prURL, prNumber)
	if err != nil {
		return nil, nil, nil, err
	}

	var warnings []string
	diff, diffTruncated, diffWarn := s.fetchPRDiff(ctx, prURL, prNumber)
	if diffWarn != "" {
		warnings = append(warnings, diffWarn)
	}
	comments, commentWarns := s.fetchPRComments(ctx, base, owner, repo, prNumber)
	warnings = append(warnings, commentWarns...)

	parts := assemblePRParts(owner, repo, meta, diff, diffTruncated, comments)

	inv := &prInventory{
		Number:        meta.Number,
		Title:         strings.TrimSpace(meta.Title),
		ReviewCount:   len(comments),
		DiffTruncated: diffTruncated,
	}
	return parts, inv, warnings, nil
}

// fetchPRMeta retrieves and parses the PR's metadata payload.
func (s *GeminiServer) fetchPRMeta(ctx context.Context, prURL string, prNumber int) (githubPRMeta, error) {
	metaBytes, err := githubAPIGet(ctx, s, prURL, "application/vnd.github+json", 1<<20)
	if err != nil {
		return githubPRMeta{}, fmt.Errorf("fetch PR #%d metadata: %w", prNumber, err)
	}
	var meta githubPRMeta
	if uerr := json.Unmarshal(metaBytes, &meta); uerr != nil {
		return githubPRMeta{}, fmt.Errorf("parse PR #%d metadata: %w", prNumber, uerr)
	}
	return meta, nil
}

// fetchPRDiff retrieves the PR's unified diff, applies truncation, and returns
// the final string, a truncation flag, and a warning string (empty on success).
func (s *GeminiServer) fetchPRDiff(ctx context.Context, prURL string, prNumber int) (string, bool, string) {
	logger := getLoggerFromContext(ctx)
	diffBytes, err := githubAPIGet(ctx, s, prURL, "application/vnd.github.v3.diff", s.config.MaxGitHubDiffBytes+1)
	if err != nil {
		logger.Warn("Failed to fetch PR #%d diff: %v", prNumber, err)
		return "", false, fmt.Sprintf("PR #%d diff: %v", prNumber, err)
	}
	diff, truncated := truncateDiff(string(diffBytes), s.config.MaxGitHubDiffBytes)
	return diff, truncated, ""
}

// fetchPRComments retrieves PR review comments, capped at config limit. Errors
// return as warnings; comments are always returned (possibly empty).
func (s *GeminiServer) fetchPRComments(
	ctx context.Context, base, owner, repo string, prNumber int,
) ([]githubPRReviewComment, []string) {
	logger := getLoggerFromContext(ctx)
	if s.config.MaxGitHubPRReviewComments <= 0 {
		return nil, nil
	}

	commentsURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments?per_page=%d",
		base, owner, repo, prNumber, s.config.MaxGitHubPRReviewComments)
	commentsBytes, cerr := githubAPIGet(ctx, s, commentsURL, "application/vnd.github+json", 1<<20)
	if cerr != nil {
		logger.Warn("Failed to fetch PR #%d review comments: %v", prNumber, cerr)
		return nil, []string{fmt.Sprintf("PR #%d review comments: %v", prNumber, cerr)}
	}
	var comments []githubPRReviewComment
	if uerr := json.Unmarshal(commentsBytes, &comments); uerr != nil {
		logger.Warn("Failed to parse PR #%d review comments: %v", prNumber, uerr)
		return nil, []string{fmt.Sprintf("PR #%d review comments: parse error", prNumber)}
	}
	if len(comments) > s.config.MaxGitHubPRReviewComments {
		comments = comments[:s.config.MaxGitHubPRReviewComments]
	}
	return comments, nil
}

// assemblePRParts converts the fetched PR metadata, diff, and review comments
// into a single nested <pull_request> XML fragment handed to Gemini. The
// _ = owner and _ = repo parameters are retained for signature continuity;
// the repo is already recorded on the <context repo="..."> wrapper.
func assemblePRParts(
	_, _ string, meta githubPRMeta, diff string, diffTruncatedFlag bool, comments []githubPRReviewComment,
) []*genai.Part {
	var b strings.Builder
	fmt.Fprintf(&b,
		"  <pull_request number=\"%d\" author=\"%s\" state=\"%s\""+
			" base_ref=\"%s\" base_sha=\"%s\" head_ref=\"%s\" head_sha=\"%s\""+
			" title=\"%s\" diff_truncated=\"%s\">\n",
		meta.Number,
		xmlAttr("@"+meta.User.Login),
		xmlAttr(meta.State),
		xmlAttr(meta.Base.Ref),
		xmlAttr(shortSHA(meta.Base.SHA)),
		xmlAttr(meta.Head.Ref),
		xmlAttr(shortSHA(meta.Head.SHA)),
		xmlAttr(strings.TrimSpace(meta.Title)),
		boolStr(diffTruncatedFlag),
	)
	fmt.Fprintf(&b, "    <description>%s</description>\n", cdataWrap(strings.TrimSpace(meta.Body)))
	if diff != "" {
		fmt.Fprintf(&b, "    <patch>%s</patch>\n", cdataWrap(diff))
	}
	for _, c := range comments {
		fmt.Fprintf(&b, "    <review author=\"%s\" path=\"%s\" line=\"%d\">%s</review>\n",
			xmlAttr("@"+c.User.Login),
			xmlAttr(c.Path),
			c.Line,
			cdataWrap(strings.TrimSpace(c.Body)),
		)
	}
	b.WriteString("  </pull_request>\n")
	return []*genai.Part{genai.NewPartFromText(b.String())}
}

// shortSHA returns the leading 7 characters of a commit sha, or the whole
// string if it is shorter.
func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
}

// encodeRefForURL escapes a git ref for use as a URL path segment while
// preserving slashes inside branch names (GitHub allows `feature/foo`).
func encodeRefForURL(ref string) string {
	parts := strings.Split(ref, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
