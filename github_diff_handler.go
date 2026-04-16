package main

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// gatherCompareDiff fetches a GitHub compare-refs diff for the base..head
// pair and returns a single text part plus inventory metadata.
//
// GitHub returns 422 for comparisons exceeding its 300-file / ~10k-line
// compare cap; in that case the error surfaces to the caller unchanged so
// the client is directed towards `github_commits` as a chunked alternative.
func (s *GeminiServer) gatherCompareDiff(
	ctx context.Context, owner, repo, base, head string,
) ([]*genai.Part, *diffInventory, error) {

	logger := getLoggerFromContext(ctx)
	logger.Info("Fetching GitHub diff %s/%s %s..%s", owner, repo, base, head)

	if base == "" || head == "" {
		return nil, nil, fmt.Errorf("github_diff requires both base and head")
	}

	apiBase := strings.TrimRight(s.config.GitHubAPIBaseURL, "/")
	compareURL := fmt.Sprintf("%s/repos/%s/%s/compare/%s...%s",
		apiBase, owner, repo, encodeRefForURL(base), encodeRefForURL(head))

	patchBytes, err := githubAPIGet(ctx, s, compareURL, "application/vnd.github.v3.diff", s.config.MaxGitHubDiffBytes+1)
	if err != nil {
		// Surface the 422-too-large hint in a friendlier form if we can spot it.
		msg := err.Error()
		if strings.Contains(msg, "422") {
			return nil, nil, fmt.Errorf("%s/%s compare %s..%s is too large for the GitHub compare API. "+
				"Consider passing an explicit list of commits via 'github_commits' instead", owner, repo, base, head)
		}
		return nil, nil, err
	}

	diff, truncated := truncateDiff(string(patchBytes), s.config.MaxGitHubDiffBytes)

	part := genai.NewPartFromText(fmt.Sprintf(
		"  <diff base=\"%s\" head=\"%s\" truncated=\"%s\">%s</diff>\n",
		xmlAttr(base), xmlAttr(head), boolStr(truncated), cdataWrap(diff),
	))

	inv := &diffInventory{Base: base, Head: head, Truncated: truncated}
	return []*genai.Part{part}, inv, nil
}
