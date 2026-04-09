package main

import (
	"context"
	"fmt"
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
	// Text / markup
	".txt":   "text/plain",
	".html":  "text/plain",
	".htm":   "text/plain",
	".css":   "text/plain",
	".xml":   "text/plain",
	".csv":   "text/plain",
	".md":    "text/plain",
	".json":  "text/plain",
	".log":   "text/plain",
	".diff":  "text/plain",
	".patch": "text/plain",

	// Shell
	".sh":   "text/plain",
	".bash": "text/plain",
	".zsh":  "text/plain",
	".fish": "text/plain",

	// JavaScript / TypeScript
	".js":  "text/plain",
	".jsx": "text/plain",
	".mjs": "text/plain",
	".cjs": "text/plain",
	".ts":  "text/plain",
	".tsx": "text/plain",

	// Systems languages
	".go":    "text/plain",
	".c":     "text/plain",
	".cpp":   "text/plain",
	".h":     "text/plain",
	".hpp":   "text/plain",
	".rs":    "text/plain",
	".swift": "text/plain",

	// JVM
	".java":   "text/plain",
	".kt":     "text/plain",
	".kts":    "text/plain",
	".scala":  "text/plain",
	".gradle": "text/plain",

	// Scripting
	".py":  "text/plain",
	".rb":  "text/plain",
	".php": "text/plain",
	".pl":  "text/plain",
	".pm":  "text/plain",
	".lua": "text/plain",
	".r":   "text/plain",

	// Functional / other
	".ex":   "text/plain",
	".exs":  "text/plain",
	".hs":   "text/plain",
	".clj":  "text/plain",
	".dart": "text/plain",

	// Frontend frameworks
	".vue":    "text/plain",
	".svelte": "text/plain",

	// SQL
	".sql": "text/plain",

	// Config
	".yaml": "text/plain",
	".yml":  "text/plain",
	".toml": "text/plain",
	".cfg":  "text/plain",
	".ini":  "text/plain",
	".env":  "text/plain",

	// Infrastructure
	".tf":  "text/plain",
	".hcl": "text/plain",

	// Schema / IDL
	".proto":   "text/plain",
	".graphql": "text/plain",
	".gql":     "text/plain",

	// Build
	".cmake": "text/plain",

	// Binary / media
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

// isTextMimeType returns true for MIME types whose content can be safely injected
// as inline text rather than uploaded via the Files API.
func isTextMimeType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "text/") ||
		mimeType == "application/json"
}

// Helper function to get MIME type from file path
func getMimeTypeFromPath(path string) string {
	// First, check for specific filenames that don't have extensions
	// but should be treated as text.
	switch filepath.Base(path) {
	case "go.mod", "go.sum",
		"Makefile", "Dockerfile", "Rakefile", "Gemfile", "Brewfile",
		"Procfile", "Vagrantfile", "Justfile", "Taskfile", "Caddyfile",
		".gitignore", ".dockerignore", ".editorconfig",
		".prettierrc", ".eslintrc", ".eslintignore", ".prettierignore",
		"CMakeLists.txt", "OWNERS", "CODEOWNERS":
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

// humanReadableSize formats file sizes in a human-readable way
func humanReadableSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
