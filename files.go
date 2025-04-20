package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

// FileUploadRequest struct definition moved to structs.go

// Validate ensures the file upload request contains valid data
func (r *FileUploadRequest) Validate() error {
	if r.FileName == "" {
		return errors.New("filename is required")
	}
	if r.MimeType == "" {
		return errors.New("mime type is required")
	}
	if len(r.Content) == 0 {
		return errors.New("content is required")
	}
	return nil
}

// FileInfo struct definition moved to structs.go
// FileStore struct definition moved to structs.go

// NewFileStore creates a new file store
func NewFileStore(client *genai.Client, config *Config) *FileStore {
	return &FileStore{
		client:   client,
		config:   config,
		fileInfo: make(map[string]*FileInfo),
	}
}

// UploadFile uploads a file to the Gemini API
func (fs *FileStore) UploadFile(ctx context.Context, req *FileUploadRequest) (*FileInfo, error) {
	// Get logger from context
	logger := getLoggerFromContext(ctx)

	// Input validation
	if req.FileName == "" {
		return nil, errors.New("filename is required")
	}
	if req.MimeType == "" {
		return nil, errors.New("mime type is required")
	}
	if len(req.Content) == 0 {
		return nil, errors.New("content is required")
	}

	// Validate file size
	if int64(len(req.Content)) > fs.config.MaxFileSize {
		return nil, fmt.Errorf("file size exceeds maximum allowed (%d bytes)", fs.config.MaxFileSize)
	}

	// Validate mime type
	mimeTypeAllowed := false
	for _, allowedType := range fs.config.AllowedFileTypes {
		if req.MimeType == allowedType {
			mimeTypeAllowed = true
			break
		}
	}
	if !mimeTypeAllowed {
		return nil, fmt.Errorf("mime type %s is not allowed", req.MimeType)
	}

	// Check if client is properly initialized
	if fs.client == nil || fs.client.Files == nil {
		logger.Error("Gemini client or Files service not properly initialized")
		return nil, errors.New("internal error: Gemini client not properly initialized")
	}

	// Create options with display name if provided
	opts := &genai.UploadFileConfig{
		MIMEType: req.MimeType,
	}
	if req.DisplayName != "" {
		opts.DisplayName = req.DisplayName
	} else {
		// Use filename as display name if not provided
		opts.DisplayName = req.FileName
	}

	// Upload file to Gemini API
	logger.Info("Uploading file %s with MIME type %s", req.FileName, req.MimeType)
	file, err := fs.client.Files.Upload(ctx, bytes.NewReader(req.Content), opts)
	if err != nil {
		logger.Error("Failed to upload file: %v", err)
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	// Extract ID from name (format: "files/abc123")
	id := file.Name
	if strings.HasPrefix(file.Name, "files/") {
		id = strings.TrimPrefix(file.Name, "files/")
	}

	// Create file info
	fileInfo := &FileInfo{
		ID:          id,
		Name:        file.Name,
		URI:         file.URI,
		DisplayName: file.DisplayName,
		MimeType:    file.MIMEType,
		Size:        0, // SizeBytes is now a pointer in the new API
		UploadedAt:  file.CreateTime,
	}

	// Set size if available
	if file.SizeBytes != nil {
		fileInfo.Size = *file.SizeBytes
	} else {
		// If not available, use the content length
		fileInfo.Size = int64(len(req.Content))
	}

	// Set expiration if provided
	if !file.ExpirationTime.IsZero() {
		fileInfo.ExpiresAt = file.ExpirationTime
	} else {
		// Default expiration if not provided by API
		fileInfo.ExpiresAt = time.Now().Add(24 * time.Hour)
	}

	// Store file info
	// Validate URI before storing
	if fileInfo.URI == "" {
		logger.Error("Invalid URI for uploaded file: empty URI")
		return nil, errors.New("Invalid URI for uploaded file")
	}

	logger.Debug("Storing file info with URI: %s", fileInfo.URI)
	fs.mu.Lock()
	fs.fileInfo[id] = fileInfo
	fs.mu.Unlock()

	logger.Info("File uploaded successfully with ID: %s", id)
	return fileInfo, nil
}

// GetFile gets file information by ID
func (fs *FileStore) GetFile(ctx context.Context, id string) (*FileInfo, error) {
	logger := getLoggerFromContext(ctx)

	// Validate client first
	if fs.client == nil {
		return nil, errors.New("file store client is nil")
	}

	// Check cache first
	fs.mu.RLock()
	info, ok := fs.fileInfo[id]
	fs.mu.RUnlock()

	if ok {
		logger.Debug("File info for %s found in cache", id)
		return info, nil
	}

	// If not in cache, try to get from API
	name := id
	if !strings.HasPrefix(id, "files/") {
		name = "files/" + id
	}

	// Check if client is properly initialized
	if fs.client == nil || fs.client.Files == nil {
		logger.Error("Gemini client or Files service not properly initialized")
		return nil, errors.New("internal error: Gemini client not properly initialized")
	}

	logger.Info("Fetching file info for %s from API", name)
	file, err := fs.client.Files.Get(ctx, name, nil)
	if err != nil {
		logger.Error("Failed to get file from API: %v", err)
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	// Extract ID from name
	fileID := file.Name
	if strings.HasPrefix(file.Name, "files/") {
		fileID = strings.TrimPrefix(file.Name, "files/")
	}

	// Create file info
	fileInfo := &FileInfo{
		ID:          fileID,
		Name:        file.Name,
		URI:         file.URI,
		DisplayName: file.DisplayName,
		MimeType:    file.MIMEType,
		Size:        0, // SizeBytes is now a pointer in the new API
		UploadedAt:  file.CreateTime,
	}

	// Set size if available
	if file.SizeBytes != nil {
		fileInfo.Size = *file.SizeBytes
	}

	// Set expiration if provided
	if !file.ExpirationTime.IsZero() {
		fileInfo.ExpiresAt = file.ExpirationTime
	}

	// Store in cache
	fs.mu.Lock()
	fs.fileInfo[fileID] = fileInfo
	fs.mu.Unlock()

	logger.Debug("Added file info for %s to cache", fileID)
	return fileInfo, nil
}

// DeleteFile deletes a file by ID
func (fs *FileStore) DeleteFile(ctx context.Context, id string) error {
	logger := getLoggerFromContext(ctx)

	// Get the file info first to get the full name
	fileInfo, err := fs.GetFile(ctx, id)
	if err != nil {
		return err
	}

	// Check if client is properly initialized
	if fs.client == nil || fs.client.Files == nil {
		logger.Error("Gemini client or Files service not properly initialized")
		return errors.New("internal error: Gemini client not properly initialized")
	}

	// Delete from API
	logger.Info("Deleting file %s", fileInfo.Name)
	if _, err := fs.client.Files.Delete(ctx, fileInfo.Name, &genai.DeleteFileConfig{}); err != nil {
		logger.Error("Failed to delete file: %v", err)
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Remove from cache
	fs.mu.Lock()
	delete(fs.fileInfo, id)
	fs.mu.Unlock()

	logger.Info("File deleted successfully: %s", id)
	return nil
}

// ListFiles returns all files
func (fs *FileStore) ListFiles(ctx context.Context) ([]*FileInfo, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Listing all files")

	// Check if client is properly initialized
	if fs.client == nil || fs.client.Files == nil {
		logger.Error("Gemini client or Files service not properly initialized")
		return nil, errors.New("internal error: Gemini client not properly initialized")
	}

	// Get files from API
	page, err := fs.client.Files.List(ctx, nil)
	if err != nil {
		logger.Error("Failed to list files: %v", err)
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	files := []*FileInfo{}
	fileMap := make(map[string]*FileInfo)

	// Process all files in the page
	for _, file := range page.Items {

		// Extract ID from name
		id := file.Name
		if strings.HasPrefix(file.Name, "files/") {
			id = strings.TrimPrefix(file.Name, "files/")
		}

		// Create file info
		fileInfo := &FileInfo{
			ID:          id,
			Name:        file.Name,
			URI:         file.URI,
			DisplayName: file.DisplayName,
			MimeType:    file.MIMEType,
			Size:        0, // SizeBytes is now a pointer in the new API
			UploadedAt:  file.CreateTime,
		}

		// Set size if available
		if file.SizeBytes != nil {
			fileInfo.Size = *file.SizeBytes
		}

		// Set expiration if provided
		if !file.ExpirationTime.IsZero() {
			fileInfo.ExpiresAt = file.ExpirationTime
		}

		files = append(files, fileInfo)
		fileMap[id] = fileInfo
	}

	// Update cache
	fs.mu.Lock()
	for id, info := range fileMap {
		fs.fileInfo[id] = info
	}
	fs.mu.Unlock()

	logger.Info("Found %d files", len(files))
	return files, nil
}

// Helper function to format file sizes in a human-readable way
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

// Helper function to get MIME type from file path
// Moved to gemini_utils.go
