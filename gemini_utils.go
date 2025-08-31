package main

import (
	"context"
	"path/filepath"
	"strings"
)

// getLoggerFromContext safely extracts a logger from the context or creates a new one
func getLoggerFromContext(ctx context.Context) Logger {
	loggerValue := ctx.Value(loggerKey)
	if loggerValue != nil {
		if l, ok := loggerValue.(Logger); ok {
			return l
		}
	}
	// Create a new logger if one isn't in the context or type assertion fails
	return NewLogger(LevelDebug)
}

// This function has been removed after refactoring to use direct MCP types

// This function has been removed after refactoring to use formatMCPResponse and direct MCP types

// Helper function to get MIME type from file path
func getMimeTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	// Treat all common code and text-based formats as plain text
	case ".txt", ".html", ".htm", ".css", ".js", ".json", ".xml", ".csv", ".sql", ".php", ".rb", ".go", ".py", ".java", ".c", ".cpp", ".h", ".hpp":
		return "text/plain"

	// Markdown has its own type
	case ".md":
		return "text/markdown"

	// Non-text file types
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	case ".wav":
		return "audio/wav"
	case ".doc", ".docx":
		return "application/msword"
	case ".xls", ".xlsx":
		return "application/vnd.ms-excel"
	case ".ppt", ".pptx":
		return "application/vnd.ms-powerpoint"
	case ".zip":
		return "application/zip"

	// Default for unknown types
	default:
		return "application/octet-stream"
	}
}
