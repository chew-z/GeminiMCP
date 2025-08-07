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

// readLocalFiles reads multiple files from local filesystem and combines their content
func readLocalFiles(filePaths []string, maxSize int64) (content string, detectedLang string, error error) {
	var contentParts []string
	var languages []string

	for _, path := range filePaths {
		// Read file content
		fileContent, err := os.ReadFile(path)
		if err != nil {
			return "", "", fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Validate file size
		if int64(len(fileContent)) > maxSize {
			return "", "", fmt.Errorf("file %s size (%d bytes) exceeds maximum allowed (%d bytes)",
				path, len(fileContent), maxSize)
		}

		// Detect language from file extension
		lang := detectLanguageFromPath(path)
		if lang != "auto-detect" {
			languages = append(languages, lang)
		}

		// Format content with file separator
		contentParts = append(contentParts, fmt.Sprintf("=== File: %s ===\n%s\n", path, string(fileContent)))
	}

	// Combine all file contents
	content = strings.Join(contentParts, "\n")

	// Determine primary language
	detectedLang = detectPrimaryLanguage(languages)

	return content, detectedLang, nil
}

// detectLanguageFromPath detects programming language from file extension
func detectLanguageFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".java":
		return "java"
	case ".cpp", ".cc", ".cxx":
		return "c++"
	case ".c":
		return "c"
	case ".h", ".hpp":
		return "c++"
	case ".rs":
		return "rust"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt", ".kts":
		return "kotlin"
	case ".cs":
		return "csharp"
	case ".sh", ".bash":
		return "bash"
	case ".ps1":
		return "powershell"
	case ".sql":
		return "sql"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".xml":
		return "xml"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".md":
		return "markdown"
	case ".txt":
		return "text"
	default:
		return "auto-detect"
	}
}

// detectPrimaryLanguage determines the most common language from a list
func detectPrimaryLanguage(languages []string) string {
	if len(languages) == 0 {
		return "auto-detect"
	}

	// Count occurrences of each language
	langCount := make(map[string]int)
	for _, lang := range languages {
		langCount[lang]++
	}

	// Find the most frequent language
	maxCount := 0
	primaryLang := languages[0]
	for lang, count := range langCount {
		if count > maxCount {
			maxCount = count
			primaryLang = lang
		}
	}

	return primaryLang
}

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

	// Define code file extensions to look for
	codeExtensions := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".java": true,
		".cpp": true, ".cc": true, ".cxx": true, ".c": true, ".h": true, ".hpp": true,
		".rs": true, ".rb": true, ".php": true, ".swift": true, ".kt": true, ".kts": true,
		".cs": true, ".sh": true, ".bash": true, ".ps1": true, ".sql": true,
		".html": true, ".htm": true, ".css": true, ".xml": true, ".json": true,
		".yaml": true, ".yml": true, ".md": true, ".txt": true,
	}

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

		// Check if file has a code extension
		ext := strings.ToLower(filepath.Ext(path))
		if codeExtensions[ext] {
			codeFiles = append(codeFiles, path)
		}

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
