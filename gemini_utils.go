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

var mimeTypes = map[string]string{
	".txt":  "text/plain",
	".html": "text/plain",
	".htm":  "text/plain",
	".css":  "text/plain",
	".js":   "text/plain",
	".json": "text/plain",
	".xml":  "text/plain",
	".csv":  "text/plain",
	".sql":  "text/plain",
	".php":  "text/plain",
	".rb":   "text/plain",
	".go":   "text/plain",
	".py":   "text/plain",
	".java": "text/plain",
	".c":    "text/plain",
	".cpp":  "text/plain",
	".h":    "text/plain",
	".hpp":  "text/plain",
	".md":   "text/plain",
	".pdf":  "application/pdf",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".mp3":  "audio/mpeg",
	".mp4":  "video/mp4",
	".wav":  "audio/wav",
	".doc":  "application/msword",
	".docx": "application/msword",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.ms-excel",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.ms-powerpoint",
	".zip":  "application/zip",
}

// Helper function to get MIME type from file path
func getMimeTypeFromPath(path string) string {
	// First, check for specific filenames that don't have extensions
	// but should be treated as text.
	switch filepath.Base(path) {
	case "go.mod", "go.sum", "Makefile", "Dockerfile", ".gitignore":
		return "text/plain"
	}

	// If it's not a special filename, check the extension
	ext := strings.ToLower(filepath.Ext(path))
	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}

	// Default for unknown types
	return "application/octet-stream"
}
