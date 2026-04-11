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

		commitURL := fmt.Sprintf("%s/repos/%s/%s/commits/%s", base, owner, repo, encodeRefForURL(sha))

		// Metadata — for the rich header.
		metaBytes, err := githubAPIGet(ctx, s, commitURL, "application/vnd.github+json", 1<<20)
		if err != nil {
			logger.Warn("Failed to fetch commit %s metadata: %v", sha, err)
			warnings = append(warnings, fmt.Sprintf("commit %s: %v", sha, err))
			continue
		}
		var meta githubCommitMeta
		if uerr := json.Unmarshal(metaBytes, &meta); uerr != nil {
			logger.Warn("Failed to parse commit %s metadata: %v", sha, uerr)
			warnings = append(warnings, fmt.Sprintf("commit %s: parse error", sha))
			continue
		}

		// Patch.
		patchBytes, err := githubAPIGet(ctx, s, commitURL, "application/vnd.github.v3.diff", s.config.MaxGitHubDiffBytes+1)
		if err != nil {
			logger.Warn("Failed to fetch commit %s diff: %v", sha, err)
			warnings = append(warnings, fmt.Sprintf("commit %s diff: %v", sha, err))
			continue
		}
		patch, truncated := truncateDiff(string(patchBytes), s.config.MaxGitHubDiffBytes)

		subject, messageBody := splitCommitMessage(meta.Commit.Message)
		// Author is either a GitHub Login (alphanumeric/dash, safe) or the
		// free-form Commit.Author.Name (attacker-controlled). Quote the
		// free-form form so a name containing "---" cannot spoof a header.
		var author string
		if meta.Author.Login != "" {
			author = "@" + meta.Author.Login
		} else {
			author = fmt.Sprintf("%q", meta.Commit.Author.Name)
		}

		// subject is attacker-controlled (commit message first line); %q
		// quotes it so embedded "---" cannot impersonate a block header.
		header := fmt.Sprintf("--- Commit %s by %s (%s): %q ---",
			shortSHA(meta.SHA), author, meta.Commit.Author.Date, subject)
		var body strings.Builder
		if messageBody != "" {
			body.WriteString(sanitizeUntrustedBlockContent(messageBody))
			body.WriteString("\n\n")
		}
		body.WriteString(patch)

		parts = append(parts, makeTextPart(header, body.String()))
		inv = append(inv, commitInventory{
			SHA:       shortSHA(meta.SHA),
			Subject:   subject,
			Truncated: truncated,
		})
	}

	return parts, inv, warnings, nil
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
