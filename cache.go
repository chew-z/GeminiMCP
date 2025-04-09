package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

// CacheRequest represents a request to create a cached context
type CacheRequest struct {
	Model        string   `json:"model"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	FileIDs      []string `json:"file_ids,omitempty"`
	Content      string   `json:"content,omitempty"`
	TTL          string   `json:"ttl,omitempty"` // Duration like "1h", "24h", etc.
	DisplayName  string   `json:"display_name,omitempty"`
}

// CacheInfo represents information about a cached context
type CacheInfo struct {
	ID          string    `json:"id"`   // The unique ID (last part of the Name)
	Name        string    `json:"name"` // The full resource name
	DisplayName string    `json:"display_name"`
	Model       string    `json:"model"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	FileIDs     []string  `json:"file_ids,omitempty"`
}

// CacheStore manages cache metadata
type CacheStore struct {
	client    *genai.Client
	config    *Config
	fileStore *FileStore
	mu        sync.RWMutex
	cacheInfo map[string]*CacheInfo // Map of ID -> CacheInfo
}

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
				logger.Debug("Adding file %s with URI %s to cache context", fileID, fileInfo.URI)
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
	cc, err := cs.client.Caches.Create(ctx, req.Model, config)
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
	cc, err := cs.client.Caches.Get(ctx, name, nil)
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

// DeleteCache deletes a cache by ID
func (cs *CacheStore) DeleteCache(ctx context.Context, id string) error {
	logger := getLoggerFromContext(ctx)

	// Get the cache info first to get the full name
	cacheInfo, err := cs.GetCache(ctx, id)
	if err != nil {
		return err
	}

	// Delete from API
	logger.Info("Deleting cache %s", cacheInfo.Name)
	if _, err := cs.client.Caches.Delete(ctx, cacheInfo.Name, nil); err != nil {
		logger.Error("Failed to delete cached content: %v", err)
		return fmt.Errorf("failed to delete cached content: %w", err)
	}

	// Remove from cache
	cs.mu.Lock()
	delete(cs.cacheInfo, id)
	cs.mu.Unlock()

	logger.Info("Cache deleted successfully: %s", id)
	return nil
}

// ListCaches returns all caches
func (cs *CacheStore) ListCaches(ctx context.Context) ([]*CacheInfo, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Listing all cached contents")

	// Get caches from API
	page, err := cs.client.Caches.List(ctx, nil)
	if err != nil {
		logger.Error("Failed to list cached contents: %v", err)
		return nil, fmt.Errorf("failed to list cached contents: %w", err)
	}

	caches := []*CacheInfo{}
	cacheMap := make(map[string]*CacheInfo)

	// Process the page
	for _, cc := range page.Items {

// Extract ID from name
		id := cc.Name
		if strings.HasPrefix(cc.Name, "cachedContents/") {
			id = strings.TrimPrefix(cc.Name, "cachedContents/")
		}

		// Get expiration time
		expiresAt := cc.ExpireTime

		// Create cache info
		cacheInfo := &CacheInfo{
			ID:          id,
			Name:        cc.Name,
			DisplayName: cc.DisplayName,
			Model:       cc.Model,
			CreatedAt:   cc.CreateTime,
			ExpiresAt:   expiresAt,
			// Note: We can't get file IDs from the API, so this will be empty
		}

		caches = append(caches, cacheInfo)
		cacheMap[id] = cacheInfo
	}

	// Update cache
	cs.mu.Lock()
	for id, info := range cacheMap {
		cs.cacheInfo[id] = info
	}
	cs.mu.Unlock()

	logger.Info("Found %d cached contents", len(caches))
	return caches, nil
}