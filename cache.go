package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

// CacheRequest struct definition moved to structs.go
// CacheInfo struct definition moved to structs.go
// CacheStore struct definition moved to structs.go

// NewCacheStore creates a new cache store
func NewCacheStore(client *genai.Client, config *Config, fileStore *FileStore) *CacheStore {
	return &CacheStore{
		client:    client,
		config:    config,
		fileStore: fileStore,
		cacheInfo: make(map[string]*CacheInfo),
	}
}

// CreateCache creates a cached context
func (cs *CacheStore) CreateCache(ctx context.Context, req *CacheRequest) (*CacheInfo, error) {
	logger := getLoggerFromContext(ctx)

	// Check if caching is enabled
	if !cs.config.EnableCaching {
		return nil, errors.New("caching is disabled")
	}

	// Input validation
	if req.Model == "" {
		return nil, errors.New("model is required")
	}

	// Validate the model
	if err := ValidateModelID(req.Model); err != nil {
		return nil, fmt.Errorf("invalid model: %w", err)
	}

	// Parse TTL
	var ttl time.Duration
	if req.TTL == "" {
		ttl = cs.config.DefaultCacheTTL
	} else {
		var err error
		ttl, err = time.ParseDuration(req.TTL)
		if err != nil {
			return nil, fmt.Errorf("invalid TTL format: %w", err)
		}
	}

	// Create config with TTL
	config := &genai.CreateCachedContentConfig{
		TTL: ttl,
	}

	// Set up display name if provided
	if req.DisplayName != "" {
		config.DisplayName = req.DisplayName
	}

	// Set up system instruction if provided
	if req.SystemPrompt != "" {
		config.SystemInstruction = genai.NewContentFromText(req.SystemPrompt, "")
	}

	// Build contents with files and text
	contents := []*genai.Content{}

	// Add files if provided
	if len(req.FileIDs) > 0 {
		logger.Info("Adding %d files to cache context", len(req.FileIDs))
		for _, fileID := range req.FileIDs {
			// Get file info
			fileInfo, err := cs.fileStore.GetFile(ctx, fileID)
			if err != nil {
				logger.Error("Failed to get file with ID %s: %v", fileID, err)
				return nil, fmt.Errorf("failed to get file with ID %s: %w", fileID, err)
			}

			// Add file to contents
			logger.Info("Adding file %s with URI %s to cache context", fileID, fileInfo.URI)
			logger.Debug("File details: Name=%s, MimeType=%s, Size=%d", fileInfo.DisplayName, fileInfo.MimeType, fileInfo.Size)
			contents = append(contents, genai.NewContentFromURI(fileInfo.URI, fileInfo.MimeType, genai.RoleUser))
		}
	}

	// Add text content if provided
	if req.Content != "" {
		logger.Debug("Adding text content to cache context")
		contents = append(contents, genai.NewContentFromText(req.Content, genai.RoleUser))
	}

	// Add contents to config if we have any
	if len(contents) > 0 {
		config.Contents = contents
	}

	// Create the cached content
    logger.Info("Creating cached content with model %s", req.Model)
    cc, err := withRetry(ctx, cs.config, logger, "gemini.caches.create", func(ctx context.Context) (*genai.CachedContent, error) {
        return cs.client.Caches.Create(ctx, req.Model, config)
    })
    if err != nil {
        logger.Error("Failed to create cached content: %v", err)
        return nil, fmt.Errorf("failed to create cached content: %w", err)
    }

	// Extract ID from name (format: "cachedContents/abc123")
	id := cc.Name
	if strings.HasPrefix(cc.Name, "cachedContents/") {
		id = strings.TrimPrefix(cc.Name, "cachedContents/")
	}

	// Calculate expiration time
	expiresAt := cc.ExpireTime

	// Create cache info
	cacheInfo := &CacheInfo{
		ID:          id,
		Name:        cc.Name,
		DisplayName: cc.DisplayName,
		Model:       cc.Model,
		CreatedAt:   cc.CreateTime,
		ExpiresAt:   expiresAt,
		FileIDs:     req.FileIDs,
	}

	// Store cache info
	cs.mu.Lock()
	cs.cacheInfo[id] = cacheInfo
	cs.mu.Unlock()

	logger.Info("Cache created successfully with ID: %s", id)
	return cacheInfo, nil
}

// GetCache gets cache information by ID
func (cs *CacheStore) GetCache(ctx context.Context, id string) (*CacheInfo, error) {
	logger := getLoggerFromContext(ctx)

	// Check cache first
	cs.mu.RLock()
	info, ok := cs.cacheInfo[id]
	cs.mu.RUnlock()

	if ok {
		logger.Debug("Cache info for %s found in local cache", id)
		return info, nil
	}

	// If not in cache, try to get from API
	name := id
	if !strings.HasPrefix(id, "cachedContents/") {
		name = "cachedContents/" + id
	}

    logger.Info("Fetching cache info for %s from API", name)
    cc, err := withRetry(ctx, cs.config, logger, "gemini.caches.get", func(ctx context.Context) (*genai.CachedContent, error) {
        return cs.client.Caches.Get(ctx, name, nil)
    })
    if err != nil {
        logger.Error("Failed to get cached content: %v", err)
        return nil, fmt.Errorf("failed to get cached content: %w", err)
    }

	// Extract ID from name
	cacheID := cc.Name
	if strings.HasPrefix(cc.Name, "cachedContents/") {
		cacheID = strings.TrimPrefix(cc.Name, "cachedContents/")
	}

	// Get expiration time
	expiresAt := cc.ExpireTime

	// Create cache info
	cacheInfo := &CacheInfo{
		ID:          cacheID,
		Name:        cc.Name,
		DisplayName: cc.DisplayName,
		Model:       cc.Model,
		CreatedAt:   cc.CreateTime,
		ExpiresAt:   expiresAt,
		// Note: We can't get file IDs from the API, so this will be empty
	}

	// Store in cache
	cs.mu.Lock()
	cs.cacheInfo[cacheID] = cacheInfo
	cs.mu.Unlock()

	logger.Debug("Added cache info for %s to local cache", cacheID)
	return cacheInfo, nil
}
