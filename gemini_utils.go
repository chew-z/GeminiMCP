package main

import (
	"context"
	"fmt"
	"os"
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
	return NewLogger(LevelInfo)
}

// This function has been removed after refactoring to use direct MCP types

// This function has been removed after refactoring to use formatMCPResponse and direct MCP types

// Helper function to get MIME type from file path
func getMimeTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
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
	case ".csv":
		return "text/csv"
	case ".go":
		return "text/plain" // Changed from "text/x-go" to "text/plain"
	case ".py":
		return "text/plain" // Changed from "text/x-python" to "text/plain"
	case ".java":
		return "text/plain" // Changed from "text/x-java" to "text/plain"
	case ".c", ".cpp", ".h", ".hpp":
		return "text/plain" // Changed from "text/x-c" to "text/plain"
	case ".rb":
		return "text/plain"
	case ".php":
		return "text/plain"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

// This function has been removed as it was unused after refactoring to use convertGenaiResponseToMCPResult

// expandFilePaths expands file paths to handle both individual files and directories
func expandFilePaths(paths []string) ([]string, error) {
	var expandedPaths []string

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("failed to access path %s: %w", path, err)
		}

		if info.IsDir() {
			// Handle directory - find code files
			files, err := findCodeFilesInDir(path)
			if err != nil {
				return nil, fmt.Errorf("failed to find code files in directory %s: %w", path, err)
			}
			expandedPaths = append(expandedPaths, files...)
		} else {
			// Handle single file
			expandedPaths = append(expandedPaths, path)
		}
	}

	return expandedPaths, nil
}

// findCodeFilesInDir recursively finds code files in a directory
func findCodeFilesInDir(dirPath string) ([]string, error) {
	var codeFiles []string

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden files and directories
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip common non-code directories
		if info.IsDir() {
			dirName := strings.ToLower(info.Name())
			skipDirs := []string{"node_modules", "vendor", "build", "dist", "target", "__pycache__", ".git", ".svn", ".hg"}
			for _, skip := range skipDirs {
				if dirName == skip {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Add file to the list
		codeFiles = append(codeFiles, path)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return codeFiles, nil
}

// parseFilePaths parses a comma-separated string or single path into a slice
func parseFilePaths(filesArg string) []string {
	// Handle comma-separated paths
	if strings.Contains(filesArg, ",") {
		paths := strings.Split(filesArg, ",")
		var trimmedPaths []string
		for _, path := range paths {
			if trimmed := strings.TrimSpace(path); trimmed != "" {
				trimmedPaths = append(trimmedPaths, trimmed)
			}
		}
		return trimmedPaths
	}

	// Handle single path
	return []string{strings.TrimSpace(filesArg)}
}
