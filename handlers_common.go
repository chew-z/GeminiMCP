package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// extractArgumentString extracts a string argument from the request parameters
func extractArgumentString(req mcp.CallToolRequest, name string) string {
	args := req.GetArguments()
	if val, ok := args[name].(string); ok && val != "" {
		return val
	}
	return ""
}

// extractGitHubPRNumber extracts the github_pr integer argument from the request.
// MCP clients typically send numeric fields as JSON numbers (float64) or strings
// depending on the transport; we accept both forms. Returns (value, ok) where
// ok is false if the parameter is missing, empty, or not parseable.
func extractGitHubPRNumber(req mcp.CallToolRequest) (int, bool) {
	args := req.GetArguments()
	switch v := args["github_pr"].(type) {
	case float64:
		if v == 0 {
			return 0, false
		}
		return int(v), true
	case int:
		if v == 0 {
			return 0, false
		}
		return v, true
	case int64:
		if v == 0 {
			return 0, false
		}
		return int(v), true
	case string:
		if v == "" {
			return 0, false
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// extractArgumentStringArray extracts a string array argument from the request parameters.
// It handles three input forms:
//   - []any (JSON array from proper MCP clients)
//   - string containing a JSON array (e.g., '["file1.go", "file2.go"]' from some clients)
//   - plain string (single value, e.g., "config.py")
func extractArgumentStringArray(req mcp.CallToolRequest, name string) []string {
	var result []string
	args := req.GetArguments()
	switch v := args[name].(type) {
	case []any:
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
	case string:
		if v == "" {
			return result
		}
		// Some MCP clients pass arrays as JSON strings — try to parse
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "[") {
			var parsed []string
			if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
				return parsed
			}
		}
		result = append(result, v)
	}
	return result
}

// progressLabel returns the configured provider model name for progress messages.
func progressLabel(modelName string) string {
	return modelName
}

// createErrorResult creates a standardized error result for mcp.CallToolResult
func createErrorResult(message string) *mcp.CallToolResult {
	return mcp.NewToolResultError(message)
}

// logAPIError logs a failure from the provider API. The wrapped error is
// the authoritative signal of how the call ended; ctx.Err() is supplementary
// and used to disambiguate context.Canceled (which can be either a client
// disconnect or a propagated server-side deadline). Caller-initiated cancels
// and deadline expiries are logged at Info to keep the error channel
// signal-heavy; everything else is Error.
//
// ctx may be nil — defensive callers pass it to disambiguate but the function
// must not panic if it is missing.
func logAPIError(ctx context.Context, logger Logger, prefix string, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		logger.Info("%s deadline exceeded: %v", prefix, err)
	case errors.Is(err, context.Canceled):
		if ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			logger.Info("%s canceled by server deadline: %v", prefix, err)
			return
		}
		logger.Info("%s canceled by caller (client disconnect or upstream cancel): %v", prefix, err)
	default:
		logger.Error("%s: %v", prefix, err)
	}
}

// convertResponseToMCPResult converts a provider response to an MCP text result.
// It surfaces abnormal finish reasons as a visible prefix and logs provider
// token usage so operators can detect truncation and monitor consumption.
func convertResponseToMCPResult(resp *GenerationResponse, logger Logger) *mcp.CallToolResult {
	if resp == nil {
		return mcp.NewToolResultError("provider returned an empty response")
	}
	text := resp.Text
	if text == "" {
		text = "The model returned an empty response. This might indicate that the model " +
			"couldn't generate an appropriate response for your query. Please try rephrasing your question or providing more context."
	}
	if !finishReasonNormal(resp.FinishReason) {
		text = fmt.Sprintf("[WARN finish_reason=%s]\n", resp.FinishReason) + text
	}
	if logger != nil {
		u := resp.Usage
		logger.Info(
			"provider response: model=%s finish=%s prompt_tokens=%d output_tokens=%d "+
				"cached_tokens=%d reasoning_tokens=%d total_tokens=%d",
			resp.Model, resp.FinishReason, u.PromptTokens, u.OutputTokens,
			u.CachedTokens, u.ReasoningTokens, u.TotalTokens,
		)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(text),
		},
	}
}

// SafeWriter provides error-safe writing to strings.Builder for handlers
type SafeWriter struct {
	builder *strings.Builder
	logger  Logger
	failed  bool
}

// NewSafeWriter creates a new SafeWriter instance
func NewSafeWriter(logger Logger) *SafeWriter {
	return &SafeWriter{
		builder: &strings.Builder{},
		logger:  logger,
		failed:  false,
	}
}

// Write adds formatted content to the builder, logging errors but continuing
func (sw *SafeWriter) Write(format string, args ...any) {
	if sw.failed {
		return // Don't write if we've already failed
	}
	_, err := sw.builder.WriteString(fmt.Sprintf(format, args...))
	if err != nil {
		sw.logger.Error("Error writing to response: %v", err)
		sw.failed = true
	}
}

// Failed returns true if any write operations have failed
func (sw *SafeWriter) Failed() bool {
	return sw.failed
}

// String returns the built string
func (sw *SafeWriter) String() string {
	return sw.builder.String()
}

// Validation helper functions

// validateRequiredString validates that a required string parameter is not empty
func validateRequiredString(req mcp.CallToolRequest, paramName string) (string, error) {
	value, ok := req.GetArguments()[paramName].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s must be a string and cannot be empty", paramName)
	}
	return value, nil
}

// validateFilePathArray validates an array of GitHub file paths.
func validateFilePathArray(filePaths []string) error {
	for _, filePath := range filePaths {
		if strings.Contains(filePath, "..") || strings.HasPrefix(filePath, "/") {
			return fmt.Errorf("invalid file path: %s. Path must be relative and within the repository", filePath)
		}
	}
	return nil
}
