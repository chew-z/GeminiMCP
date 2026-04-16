package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// githubCommitMeta is the slice of /repos/.../commits/{sha} we care about.
type githubCommitMeta struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
}

// gatherCommits fetches per-commit patches preserving user-supplied order.
// Each commit yields one text part. Failed fetches are reported as warnings
// so a single bad SHA does not poison the whole batch.
func (s *GeminiServer) gatherCommits(
	ctx context.Context, owner, repo string, shas []string,
) ([]*genai.Part, []commitInventory, []string, error) {

	logger := getLoggerFromContext(ctx)
	if len(shas) == 0 {
		return nil, nil, nil, nil
	}
	if len(shas) > s.config.MaxGitHubCommits {
		return nil, nil, nil, fmt.Errorf("too many commits requested: %d (limit %d)", len(shas), s.config.MaxGitHubCommits)
	}

	base := strings.TrimRight(s.config.GitHubAPIBaseURL, "/")

	var parts []*genai.Part
	var inv []commitInventory
	var warnings []string

	for _, sha := range shas {
		sha = strings.TrimSpace(sha)
		if sha == "" {
			continue
		}
		part, cinv, warns := s.fetchCommit(ctx, base, owner, repo, sha, logger)
		warnings = append(warnings, warns...)
		if part == nil {
			continue
		}
		parts = append(parts, part)
		inv = append(inv, cinv)
	}

	return parts, inv, warnings, nil
}

// fetchCommit fetches one commit's metadata + patch and renders the XML
// fragment. Returns a nil part plus warnings when any step fails.
func (s *GeminiServer) fetchCommit(
	ctx context.Context, base, owner, repo, sha string, logger Logger,
) (*genai.Part, commitInventory, []string) {
	commitURL := fmt.Sprintf("%s/repos/%s/%s/commits/%s", base, owner, repo, encodeRefForURL(sha))

	metaBytes, err := githubAPIGet(ctx, s, commitURL, "application/vnd.github+json", 1<<20)
	if err != nil {
		logger.Warn("Failed to fetch commit %s metadata: %v", sha, err)
		return nil, commitInventory{}, []string{fmt.Sprintf("commit %s: %v", sha, err)}
	}
	var meta githubCommitMeta
	if uerr := json.Unmarshal(metaBytes, &meta); uerr != nil {
		logger.Warn("Failed to parse commit %s metadata: %v", sha, uerr)
		return nil, commitInventory{}, []string{fmt.Sprintf("commit %s: parse error", sha)}
	}

	patchBytes, err := githubAPIGet(ctx, s, commitURL, "application/vnd.github.v3.diff", s.config.MaxGitHubDiffBytes+1)
	if err != nil {
		logger.Warn("Failed to fetch commit %s diff: %v", sha, err)
		return nil, commitInventory{}, []string{fmt.Sprintf("commit %s diff: %v", sha, err)}
	}
	patch, truncated := truncateDiff(string(patchBytes), s.config.MaxGitHubDiffBytes)

	subject, messageBody := splitCommitMessage(meta.Commit.Message)
	authorAttr := meta.Commit.Author.Name
	if meta.Author.Login != "" {
		authorAttr = "@" + meta.Author.Login
	}

	part := genai.NewPartFromText(fmt.Sprintf(
		"  <commit sha=\"%s\" author=\"%s\" date=\"%s\" subject=\"%s\" truncated=\"%s\">\n"+
			"    <message>%s</message>\n"+
			"    <patch>%s</patch>\n"+
			"  </commit>\n",
		xmlAttr(shortSHA(meta.SHA)),
		xmlAttr(authorAttr),
		xmlAttr(meta.Commit.Author.Date),
		xmlAttr(subject),
		boolStr(truncated),
		cdataWrap(messageBody),
		cdataWrap(patch),
	))
	return part, commitInventory{
		SHA:       shortSHA(meta.SHA),
		Subject:   subject,
		Truncated: truncated,
	}, nil
}

// splitCommitMessage separates a git commit message into its subject line and
// optional message body.
func splitCommitMessage(msg string) (subject, body string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "(no message)", ""
	}
	if before, after, ok := strings.Cut(msg, "\n"); ok {
		return strings.TrimSpace(before), strings.TrimSpace(after)
	}
	return msg, ""
}
