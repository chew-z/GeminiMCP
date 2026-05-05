package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/genai"
)

// maxReportedWarnings is the cap for file failure warnings surfaced to the model.
const maxReportedWarnings = 10

func (s *GeminiServer) parseAskRequest(ctx context.Context, req mcp.CallToolRequest) (string, *genai.GenerateContentConfig, string, error) {
	// Extract and validate query parameter (required)
	query, err := validateRequiredString(req, "query")
	if err != nil {
		return "", nil, "", err
	}

	// Create Gemini model configuration
	config, modelName, err := createModelConfig(ctx, req, s.config, s.config.GeminiModel)
	if err != nil {
		return "", nil, "", fmt.Errorf("error creating model configuration: %v", err)
	}

	return query, config, modelName, nil
}

// GeminiAskHandler is a handler for the gemini_ask tool that uses mcp-go types directly
func (s *GeminiServer) GeminiAskHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Debug("handling gemini_ask request with direct handler")

	query, config, modelName, err := s.parseAskRequest(ctx, req)
	if err != nil {
		return createErrorResult(err.Error()), nil
	}

	// Resolve the system prompt server-side, in parallel with context gathering.
	// This is the sole assigner of SystemInstruction — createModelConfig does
	// not touch it.
	promptCh := s.resolveSystemPromptAsync(ctx, req, query, logger)

	ghContextParts, uploads, inventory, allWarnings, errResult := s.gatherAllContext(ctx, req)
	if errResult != nil {
		return errResult, nil
	}

	prompt := <-promptCh
	config.SystemInstruction = genai.NewContentFromText(prompt.SystemPrompt, "")

	// Validate client and models before proceeding
	if s.client == nil || s.client.Models == nil {
		logger.Error("Gemini client or Models service not properly initialized")
		return createErrorResult("Internal error: Gemini client not properly initialized"), nil
	}

	// If any GitHub context is attached, append a descriptive addendum to the
	// system instruction so Gemini can cite the correct <context> elements.
	applyContextInventory(config, &inventory)

	// Process with context if anything was attached
	if len(ghContextParts) > 0 || len(uploads) > 0 {
		return s.processWithFiles(ctx, req, query, ghContextParts, uploads, allWarnings, inventory.Repo, prompt.Category, modelName, config)
	}
	return s.processWithoutFiles(ctx, req, query, prompt.Category, modelName, config)
}

// gatherAllContext runs the two independent context-gathering paths (GitHub
// PR/commits/diff and files) and merges their warnings and inventory state.
// The "everything the client asked for failed" hard-fail lives here, not
// inside either sub-fetcher, so a failed PR fetch does not block a successful
// github_files fetch (and vice versa).
func (s *GeminiServer) gatherAllContext(
	ctx context.Context, req mcp.CallToolRequest,
) ([]*genai.Part, []*FileUploadRequest, contextInventory, []string, *mcp.CallToolResult) {
	logger := getLoggerFromContext(ctx)

	spec := parseGitHubContextSpec(req)

	ghContextParts, inventory, ghWarnings, errResult := s.gatherGitHubContext(ctx, req)
	if errResult != nil {
		return nil, nil, inventory, nil, errResult
	}

	logger.Debug("starting file handling logic")
	// Pass spec.any() as the "more than just files was requested" flag so a
	// file-fetch failure becomes a warning (not a hard-fail) whenever other
	// github_* sources were also requested — even if those other sources
	// themselves failed. The consolidated decision below sees both paths.
	uploads, fileWarnings, errResult := s.gatherFileUploads(ctx, req, spec.any())
	if errResult != nil {
		return nil, nil, inventory, nil, errResult
	}

	finalizeFilesInventory(req, &inventory, len(uploads))

	allWarnings := append([]string{}, ghWarnings...)
	allWarnings = append(allWarnings, fileWarnings...)

	totalContent := len(ghContextParts) + len(uploads)
	if errResult = consolidatedContextError(req, spec, totalContent, allWarnings); errResult != nil {
		return nil, nil, inventory, nil, errResult
	}

	return ghContextParts, uploads, inventory, allWarnings, nil
}

// finalizeFilesInventory records uploaded files in the inventory when they
// were sourced from github_files.
func finalizeFilesInventory(req mcp.CallToolRequest, inv *contextInventory, uploadCount int) {
	if uploadCount == 0 {
		return
	}
	if len(extractArgumentStringArray(req, "github_files")) == 0 {
		return
	}
	inv.Files.Count = uploadCount
	inv.Files.Ref = extractArgumentString(req, "github_ref", "")
	if inv.Repo == "" {
		inv.Repo = extractArgumentString(req, "github_repo", "")
	}
}

// consolidatedContextError returns a single error result enumerating every
// accumulated warning if the client requested any context source and we
// produced nothing at all. Returns nil when the request has useful content
// or the client asked for nothing.
func consolidatedContextError(
	req mcp.CallToolRequest, spec githubContextSpec, totalContent int, allWarnings []string,
) *mcp.CallToolResult {
	if totalContent > 0 {
		return nil
	}
	anyRequested := spec.any() ||
		len(extractArgumentStringArray(req, "github_files")) > 0
	if !anyRequested {
		return nil
	}
	msg := "Failed to fetch any of the requested context."
	if len(allWarnings) > 0 {
		msg += " Warnings: " + strings.Join(allWarnings, "; ")
	}
	return createErrorResult(msg)
}

// gatherGitHubContext fetches the github_pr / github_commits / github_diff
// parameters (independently and in parallel-friendly order) and returns the
// resulting genai Parts in the stable merge order:
//
//	[commits] → [diff] → [PR bundle]
//
// Files are intentionally NOT handled here — they're fetched by
// gatherFileUploads.
func (s *GeminiServer) gatherGitHubContext(
	ctx context.Context, req mcp.CallToolRequest,
) ([]*genai.Part, contextInventory, []string, *mcp.CallToolResult) {
	var inv contextInventory

	spec := parseGitHubContextSpec(req)
	if !spec.any() {
		return nil, inv, nil, nil
	}

	githubRepo := extractArgumentString(req, "github_repo", "")
	if githubRepo == "" {
		return nil, inv, nil, createErrorResult(
			"'github_repo' is required when using 'github_pr', 'github_commits', or 'github_diff_base'/'github_diff_head'.")
	}
	owner, repo, err := parseGitHubRepo(githubRepo)
	if err != nil {
		return nil, inv, nil, createErrorResult(err.Error())
	}
	inv.Repo = owner + "/" + repo

	if spec.wantsDiff && (spec.diffBase == "" || spec.diffHead == "") {
		return nil, inv, nil, createErrorResult("'github_diff_base' and 'github_diff_head' must both be provided.")
	}

	parts, warnings, errResult := s.fetchGitHubContextSources(ctx, owner, repo, spec, &inv)
	if errResult != nil {
		return nil, inv, nil, errResult
	}
	// Return whatever parts and warnings the per-source fetchers produced,
	// even if empty. The consolidated "nothing succeeded" decision lives in
	// gatherAllContext so it can see both github-context and file-upload
	// results before hard-failing.
	return parts, inv, warnings, nil
}

// githubContextSpec describes which github_* sources the client requested.
type githubContextSpec struct {
	hasPR     bool
	prNumber  int
	commits   []string
	wantsDiff bool
	diffBase  string
	diffHead  string
}

func (g githubContextSpec) any() bool {
	return g.hasPR || len(g.commits) > 0 || g.wantsDiff
}

func parseGitHubContextSpec(req mcp.CallToolRequest) githubContextSpec {
	prNumber, hasPR := extractGitHubPRNumber(req)
	diffBase := extractArgumentString(req, "github_diff_base", "")
	diffHead := extractArgumentString(req, "github_diff_head", "")
	return githubContextSpec{
		hasPR:     hasPR,
		prNumber:  prNumber,
		commits:   extractArgumentStringArray(req, "github_commits"),
		wantsDiff: diffBase != "" || diffHead != "",
		diffBase:  diffBase,
		diffHead:  diffHead,
	}
}

// fetchGitHubContextSources runs the actual fetches in stable merge order
// (commits → diff → PR) and accumulates parts, warnings, and inventory state.
func (s *GeminiServer) fetchGitHubContextSources(
	ctx context.Context, owner, repo string, spec githubContextSpec, inv *contextInventory,
) ([]*genai.Part, []string, *mcp.CallToolResult) {
	logger := getLoggerFromContext(ctx)
	var parts []*genai.Part
	var warnings []string

	if len(spec.commits) > 0 {
		commitParts, commitInv, commitWarnings, err := s.gatherCommits(ctx, owner, repo, spec.commits)
		if err != nil {
			logger.Error("Commits fetch failed: %v", err)
			return nil, nil, createErrorResult(err.Error())
		}
		parts = append(parts, commitParts...)
		inv.Commits = commitInv
		warnings = append(warnings, commitWarnings...)
	}

	if spec.wantsDiff {
		diffParts, diffInv, err := s.gatherCompareDiff(ctx, owner, repo, spec.diffBase, spec.diffHead)
		if err != nil {
			logger.Error("Compare diff fetch failed: %v", err)
			warnings = append(warnings, fmt.Sprintf("github_diff %s..%s: %v", spec.diffBase, spec.diffHead, err))
		} else {
			parts = append(parts, diffParts...)
			inv.Diff = diffInv
		}
	}

	if spec.hasPR {
		prParts, prInv, prWarnings, err := s.gatherPullRequest(ctx, owner, repo, spec.prNumber)
		if err != nil {
			logger.Error("PR fetch failed: %v", err)
			warnings = append(warnings, fmt.Sprintf("github_pr #%d: %v", spec.prNumber, err))
		} else {
			parts = append(parts, prParts...)
			inv.PR = prInv
			warnings = append(warnings, prWarnings...)
		}
	}

	return parts, warnings, nil
}

// buildContextInventoryAddendum renders a deterministic, descriptive (not
// instructional) summary of every attached context block, suitable for
// appending to the system prompt. The text references the XML tag names the
// server emits inside <context> so Gemini can cite them correctly.
func buildContextInventoryAddendum(inv *contextInventory) string {
	if inv == nil || !inv.HasAny() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nYou have been provided with the following context blocks")
	if inv.Repo != "" {
		fmt.Fprintf(&b, " from github.com/%s", inv.Repo)
	}
	b.WriteString(", inside a <context> element (your instructions are inside a <task> element):\n")

	writeFilesInventory(&b, inv.Files)
	writeCommitsInventory(&b, inv.Commits)
	writeDiffInventory(&b, inv.Diff)
	writePRInventory(&b, inv.PR)

	b.WriteString("\nUse these blocks to answer the user's task.")
	return b.String()
}

func writeFilesInventory(b *strings.Builder, f fileInventory) {
	if f.Count == 0 {
		return
	}
	if f.Ref != "" {
		fmt.Fprintf(b, "- %d <file> element(s) at ref %s\n", f.Count, f.Ref)
		return
	}
	fmt.Fprintf(b, "- %d <file> element(s)\n", f.Count)
}

func writeCommitsInventory(b *strings.Builder, commits []commitInventory) {
	if len(commits) == 0 {
		return
	}
	fmt.Fprintf(b, "- %d <commit> element(s)\n", len(commits))
}

func writeDiffInventory(b *strings.Builder, d *diffInventory) {
	if d == nil {
		return
	}
	suffix := ""
	if d.Truncated {
		suffix = " (diff was truncated at size limit)"
	}
	fmt.Fprintf(b, "- A <diff> element (%s..%s)%s\n", d.Base, d.Head, suffix)
}

func writePRInventory(b *strings.Builder, pr *prInventory) {
	if pr == nil {
		return
	}
	suffix := ""
	if pr.DiffTruncated {
		suffix = " (diff was truncated at size limit)"
	}
	fmt.Fprintf(b, "- A <pull_request> element #%d (%q), with <description>, <patch>, and %d <review>(s)%s\n",
		pr.Number, pr.Title, pr.ReviewCount, suffix)
}

// applyContextInventory appends the inventory addendum to the existing system
// instruction on the config. It never rewrites the client-supplied system
// prompt — it only adds descriptive trailing text.
func applyContextInventory(config *genai.GenerateContentConfig, inv *contextInventory) {
	if config == nil || !inv.HasAny() {
		return
	}
	addendum := buildContextInventoryAddendum(inv)
	if addendum == "" {
		return
	}
	var existing strings.Builder
	if config.SystemInstruction != nil {
		for _, part := range config.SystemInstruction.Parts {
			if part != nil && part.Text != "" {
				existing.WriteString(part.Text)
			}
		}
	}
	existing.WriteString(addendum)
	config.SystemInstruction = genai.NewContentFromText(existing.String(), "")
}

// gatherFileUploads fetches any github_files attached to the request. The
// github-family parameters (github_pr / github_commits / github_diff_*) are
// orthogonal peers handled separately by gatherGitHubContext; this function
// only deals with file attachments.
//
// The otherGitHubContextPresent flag relaxes the "files requested but none
// gathered" hard-fail when another github-sourced context block was already
// successfully attached — in that case the request has useful content and
// the failed files become warnings.
func (s *GeminiServer) gatherFileUploads(
	ctx context.Context, req mcp.CallToolRequest, otherGitHubContextPresent bool,
) ([]*FileUploadRequest, []string, *mcp.CallToolResult) {
	logger := getLoggerFromContext(ctx)

	githubFiles := extractArgumentStringArray(req, "github_files")
	logger.Debug("github_files parameter: %d entries", len(githubFiles))

	if len(githubFiles) == 0 {
		return nil, nil, nil
	}

	uploads, warnings, errResult := s.gatherGitHubFiles(ctx, req, githubFiles)
	if errResult != nil {
		return s.handleFileUploadError(errResult, warnings, githubFiles, otherGitHubContextPresent)
	}

	// Guard: if files were explicitly requested but none were gathered,
	// return an error instead of silently falling through to processWithoutFiles —
	// unless other github context was attached successfully, in which case
	// this is a partial failure that should surface as warnings.
	if len(uploads) == 0 {
		if otherGitHubContextPresent {
			logger.Warn("files requested but none gathered; other GitHub context present, continuing")
			return nil, warnings, nil
		}
		logger.Error("Files were requested but none could be gathered")
		return nil, nil, createErrorResult("Failed to retrieve any of the requested files. Cannot proceed without file context.")
	}

	return uploads, warnings, nil
}

// handleFileUploadError softens a hard-fail from the github_files fetcher into
// warnings when other github context was already successfully attached,
// allowing the caller to proceed with partial context.
func (s *GeminiServer) handleFileUploadError(
	errResult *mcp.CallToolResult, warnings, githubFiles []string, otherGitHubContextPresent bool,
) ([]*FileUploadRequest, []string, *mcp.CallToolResult) {
	if otherGitHubContextPresent && len(githubFiles) > 0 {
		for _, f := range githubFiles {
			warnings = append(warnings, fmt.Sprintf("%s: could not be fetched from GitHub", f))
		}
		return nil, warnings, nil
	}
	return nil, nil, errResult
}

// gatherGitHubFiles fetches files from a GitHub repository.
// Returns uploads, warning messages for failed files, and an optional error result.
func (s *GeminiServer) gatherGitHubFiles(
	ctx context.Context, req mcp.CallToolRequest, githubFiles []string,
) ([]*FileUploadRequest, []string, *mcp.CallToolResult) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Processing GitHub files request")

	githubRepo := extractArgumentString(req, "github_repo", "")
	if githubRepo == "" {
		logger.Error("GitHub repository parameter missing")
		return nil, nil, createErrorResult("'github_repo' is required when using 'github_files'.")
	}

	githubRef := extractArgumentString(req, "github_ref", "")

	// Validate and fetch
	if err := validateFilePathArray(githubFiles); err != nil {
		logger.Error("GitHub file path validation failed: %v", err)
		return nil, nil, createErrorResult(err.Error())
	}

	fetchedUploads, fileErrs := fetchFromGitHub(ctx, s, githubRepo, githubRef, githubFiles)
	var warnings []string
	if len(fileErrs) > 0 {
		// Build a set of successfully fetched filenames to identify which ones failed
		fetched := make(map[string]bool, len(fetchedUploads))
		for _, u := range fetchedUploads {
			fetched[u.FileName] = true
		}
		for _, file := range githubFiles {
			if !fetched[file] {
				warnings = append(warnings, fmt.Sprintf("%s: could not be fetched from GitHub", file))
			}
		}
		for _, err := range fileErrs {
			logger.Error("Error processing github file: %v", err)
		}
		if len(fetchedUploads) == 0 {
			return nil, nil, createErrorResult(fmt.Sprintf("Error processing github files: %v", fileErrs))
		}
		// Partial failure: some files succeeded, some failed
		logger.Warn("Partial GitHub fetch: %d/%d files succeeded, %d failed",
			len(fetchedUploads), len(githubFiles), len(fileErrs))
	}
	return fetchedUploads, warnings, nil
}

// processWithFiles handles a Gemini API request with any combination of
// pre-built github-context XML parts (commits / diff / PR bundle) and file
// attachments. Everything is placed BEFORE the query to maximise implicit
// caching — Gemini caches the shared prefix of repeated requests, so stable
// content at the front gets cached automatically across calls.
//
// The stable merge order is:
//
//	<context> [commits] → [diff] → [PR bundle] → [files] </context> → <task><query>…</query></task> → <final_instruction>
//
// contextParts MUST already be in the above order when passed in.
func (s *GeminiServer) processWithFiles(ctx context.Context, req mcp.CallToolRequest, query string,
	contextParts []*genai.Part, uploads []*FileUploadRequest,
	warnings []string, repo string, category queryCategory,
	modelName string, config *genai.GenerateContentConfig) (*mcp.CallToolResult, error) {

	logger := getLoggerFromContext(ctx)

	if len(contextParts) > 0 {
		logger.Info("Processing %d github-context part(s) for inline injection", len(contextParts))
	}

	logger.Info("Processing %d file(s) for inline injection", len(uploads))
	githubRef := extractArgumentString(req, "github_ref", "")
	fileParts := s.buildFileParts(ctx, uploads, githubRef, logger)

	parts := wrapUserTurnWithContext(repo, contextParts, fileParts, query, warnings, finalInstructionFor(category))

	logger.Debug("request shape: model=%s category=%s thinking=%v max_tokens=%d context_parts=%d file_parts=%d warnings=%d",
		modelName, category, config.ThinkingConfig != nil, config.MaxOutputTokens,
		len(contextParts), len(fileParts), len(warnings))
	if loggerDebugEnabled(logger) {
		logger.Debug("envelope (with context): repo=%q parts=%d bytes=%d\n%s",
			repo, len(parts), totalPartBytes(parts), renderPartsForDebug(parts))
	}

	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	// Generate content with files
	stop := startProgressReporter(ctx, req,
		s.config.ProgressInterval,
		s.config.HTTPTimeout.Seconds(),
		progressLabel(modelName, config),
		logger)
	defer stop()
	response, err := withRetry(ctx, s.config, logger, "gemini.models.generate_content", func(ctx context.Context) (*genai.GenerateContentResponse, error) {
		return s.client.Models.GenerateContent(ctx, modelName, contents, config)
	})
	if err != nil {
		logGeminiAPIError(ctx, logger, "Gemini API error", err)
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	checkModelStatus(ctx, response, modelName)
	return convertGenaiResponseToMCPResult(response, logger), nil
}

// buildFileParts converts file uploads to the XML <file> fragments emitted
// inside the <context> envelope. Text files embed their content as raw text in
// a single text Part. Binary files upload via the Files API and get rendered
// as a three-Part sequence (opener text, URI, closer text) so the Files-API
// Part sits inside its own <file> element.
func (s *GeminiServer) buildFileParts(
	ctx context.Context, uploads []*FileUploadRequest, githubRef string, logger Logger,
) []*genai.Part {
	fileParts := make([]*genai.Part, 0, len(uploads))
	for _, upload := range uploads {
		if isTextMimeType(upload.MimeType) {
			logger.Info("Injecting %s (%d bytes) as inline text", upload.FileName, len(upload.Content))
			fileParts = append(fileParts, renderTextFilePart(upload, githubRef))
			continue
		}
		fileParts = append(fileParts, s.renderBinaryFileParts(ctx, upload, githubRef, logger)...)
	}
	return fileParts
}

func renderTextFilePart(upload *FileUploadRequest, githubRef string) *genai.Part {
	return genai.NewPartFromText(fmt.Sprintf(
		"  <file path=\"%s\" ref=\"%s\" kind=\"text\" mime=\"%s\">%s</file>\n",
		xmlAttr(upload.FileName),
		xmlAttr(githubRef),
		xmlAttr(upload.MimeType),
		string(upload.Content),
	))
}

func (s *GeminiServer) renderBinaryFileParts(
	ctx context.Context, upload *FileUploadRequest, githubRef string, logger Logger,
) []*genai.Part {
	logger.Info("Uploading binary file %s (%s) via Files API", upload.FileName, upload.MimeType)
	uploadConfig := &genai.UploadFileConfig{
		MIMEType:    upload.MimeType,
		DisplayName: upload.FileName,
	}
	file, err := withRetry(ctx, s.config, logger, "gemini.files.upload", func(ctx context.Context) (*genai.File, error) {
		return s.client.Files.Upload(ctx, bytes.NewReader(upload.Content), uploadConfig)
	})
	if err != nil || file.URI == "" {
		if err != nil {
			logger.Error("Failed to upload file %s: %v - skipping binary file", upload.FileName, err)
		} else {
			logger.Error("File %s uploaded but URI is empty - skipping binary file", upload.FileName)
		}
		return []*genai.Part{genai.NewPartFromText(fmt.Sprintf(
			"  <file path=\"%s\" ref=\"%s\" kind=\"binary\" mime=\"%s\">%s</file>\n",
			xmlAttr(upload.FileName),
			xmlAttr(githubRef),
			xmlAttr(upload.MimeType),
			"[Error: This binary file could not be uploaded and cannot be displayed inline.]",
		))}
	}
	return []*genai.Part{
		genai.NewPartFromText(fmt.Sprintf(
			"  <file path=\"%s\" ref=\"%s\" kind=\"binary\" mime=\"%s\">",
			xmlAttr(upload.FileName),
			xmlAttr(githubRef),
			xmlAttr(upload.MimeType),
		)),
		genai.NewPartFromURI(file.URI, upload.MimeType),
		genai.NewPartFromText("</file>\n"),
	}
}

// processWithoutFiles handles a Gemini API request without file attachments
func (s *GeminiServer) processWithoutFiles(ctx context.Context, req mcp.CallToolRequest, query string,
	category queryCategory,
	modelName string, config *genai.GenerateContentConfig) (*mcp.CallToolResult, error) {

	logger := getLoggerFromContext(ctx)

	parts := wrapUserTurnQueryOnly(query, finalInstructionFor(category))

	logger.Debug("request shape: model=%s category=%s thinking=%v max_tokens=%d context_parts=0 file_parts=0 warnings=0",
		modelName, category, config.ThinkingConfig != nil, config.MaxOutputTokens)
	if loggerDebugEnabled(logger) {
		logger.Debug("envelope (query-only): parts=%d bytes=%d\n%s",
			len(parts), totalPartBytes(parts), renderPartsForDebug(parts))
	}

	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	// Generate content
	stop := startProgressReporter(ctx, req,
		s.config.ProgressInterval,
		s.config.HTTPTimeout.Seconds(),
		progressLabel(modelName, config),
		logger)
	defer stop()
	response, err := withRetry(ctx, s.config, logger, "gemini.models.generate_content", func(ctx context.Context) (*genai.GenerateContentResponse, error) {
		return s.client.Models.GenerateContent(ctx, modelName, contents, config)
	})
	if err != nil {
		logGeminiAPIError(ctx, logger, "Gemini API error", err)
		return createErrorResult(fmt.Sprintf("Error from Gemini API: %v", err)), nil
	}

	checkModelStatus(ctx, response, modelName)
	return convertGenaiResponseToMCPResult(response, logger), nil
}

func loggerDebugEnabled(logger Logger) bool {
	if logger == nil {
		return false
	}

	type debugLogger interface {
		IsDebugEnabled() bool
	}
	if l, ok := logger.(debugLogger); ok {
		return l.IsDebugEnabled()
	}

	switch l := logger.(type) {
	case *scopedLogger:
		return loggerDebugEnabled(l.inner)
	case *StandardLogger:
		return l.level <= LevelDebug
	default:
		return false
	}
}
