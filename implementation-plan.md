# File Handling and Context Caching Implementation Plan

## Overview

This implementation plan outlines how to add file handling and context caching capabilities to the GeminiMCP project using the official Google Generative AI Go client SDK (`github.com/google/generative-ai-go/genai`). These features will allow users to:

1. Upload files for processing by Gemini models
2. Create cached contexts that include both text and file content
3. Perform multiple queries against the same cached context
4. Manage the lifecycle of files and caches

## Key Components from Gemini SDK

The Gemini API provides built-in support for file handling and context caching through the following components:

### File Handling
- `Client.UploadFile`: Uploads a file to the Gemini API
- `Client.DeleteFile`: Deletes a file
- `Client.GetFile`: Gets file metadata
- `Client.ListFiles`: Lists all files
- `FileData`: References a file in content via its URI

### Context Caching
- `Client.CreateCachedContent`: Creates a cached context
- `Client.DeleteCachedContent`: Deletes a cached context
- `Client.GenerativeModelFromCachedContent`: Creates a model using cached content
- `CachedContent`: Represents a cached context with files, text, and system instructions

## Implementation Components

### 1. Data Structures

```go
// In a new file called 'files.go'

// FileUploadRequest represents a request to upload a file
type FileUploadRequest struct {
    FileName    string `json:"filename"`
    MimeType    string `json:"mime_type"`
    Content     []byte `json:"content"` // Base64 encoded content
    DisplayName string `json:"display_name,omitempty"`
}

// FileInfo represents information about a stored file
type FileInfo struct {
    ID          string    `json:"id"`           // The unique ID (last part of the Name)
    Name        string    `json:"name"`         // The full resource name (e.g., "files/abc123")
    URI         string    `json:"uri"`          // The URI to use in requests
    DisplayName string    `json:"display_name"` // Human-readable name
    MimeType    string    `json:"mime_type"`
    Size        int64     `json:"size"`
    UploadedAt  time.Time `json:"uploaded_at"`
    ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

// FileStore manages file metadata
type FileStore struct {
    client   *genai.Client
    config   *Config
    mu       sync.RWMutex
    fileInfo map[string]*FileInfo // Map of ID -> FileInfo
}
```

```go
// In a new file called 'cache.go'

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
    ID          string    `json:"id"`          // The unique ID (last part of the Name)
    Name        string    `json:"name"`        // The full resource name
    DisplayName string    `json:"display_name"`
    Model       string    `json:"model"`
    CreatedAt   time.Time `json:"created_at"`
    ExpiresAt   time.Time `json:"expires_at"`
    FileIDs     []string  `json:"file_ids,omitempty"`
}

// CacheStore manages cache metadata
type CacheStore struct {
    client   *genai.Client
    config   *Config
    fileStore *FileStore
    mu       sync.RWMutex
    cacheInfo map[string]*CacheInfo // Map of ID -> CacheInfo
}
```

### 2. Configuration Updates

```go
// In config.go

// Add to Config struct
type Config struct {
    // Existing fields...
    
    // File handling settings
    MaxFileSize      int64    `json:"max_file_size"`       // Max file size in bytes
    AllowedFileTypes []string `json:"allowed_file_types"`  // Allowed MIME types
    
    // Cache settings
    EnableCaching   bool          `json:"enable_caching"`     // Enable/disable caching
    DefaultCacheTTL time.Duration `json:"default_cache_ttl"`  // Default TTL if not specified
}

// Add to default constants
const (
    // Existing constants...
    
    defaultMaxFileSize     = 10 * 1024 * 1024 // 10MB
    defaultEnableCaching   = true
    defaultDefaultCacheTTL = 1 * time.Hour
)

// Update NewConfig function to load these settings from environment variables
func NewConfig() (*Config, error) {
    // Existing code...
    
    // File handling settings
    maxFileSize := defaultMaxFileSize
    if sizeStr := os.Getenv("GEMINI_MAX_FILE_SIZE"); sizeStr != "" {
        if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil && size > 0 {
            maxFileSize = size
        }
    }
    
    allowedFileTypes := []string{"text/plain", "text/markdown", "application/pdf", "image/png", "image/jpeg"}
    if typesStr := os.Getenv("GEMINI_ALLOWED_FILE_TYPES"); typesStr != "" {
        allowedFileTypes = strings.Split(typesStr, ",")
    }
    
    // Cache settings
    enableCaching := defaultEnableCaching
    if cacheStr := os.Getenv("GEMINI_ENABLE_CACHING"); cacheStr != "" {
        enableCaching = strings.ToLower(cacheStr) == "true"
    }
    
    defaultCacheTTL := defaultDefaultCacheTTL
    if ttlStr := os.Getenv("GEMINI_DEFAULT_CACHE_TTL"); ttlStr != "" {
        if ttl, err := time.ParseDuration(ttlStr); err == nil && ttl > 0 {
            defaultCacheTTL = ttl
        }
    }
    
    return &Config{
        // Existing fields...
        MaxFileSize:      maxFileSize,
        AllowedFileTypes: allowedFileTypes,
        EnableCaching:    enableCaching,
        DefaultCacheTTL:  defaultCacheTTL,
    }, nil
}
```

### 3. File Store Implementation

```go
// In files.go

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
    
    // Create options with display name if provided
    opts := &genai.UploadFileOptions{}
    if req.DisplayName != "" {
        opts.DisplayName = req.DisplayName
    }
    opts.MIMEType = req.MimeType
    
    // Upload file to Gemini API
    file, err := fs.client.UploadFile(ctx, req.FileName, bytes.NewReader(req.Content), opts)
    if err != nil {
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
        Size:        file.SizeBytes,
        UploadedAt:  file.CreateTime,
    }
    
    // Set expiration if provided
    if !file.ExpirationTime.IsZero() {
        fileInfo.ExpiresAt = file.ExpirationTime
    }
    
    // Store file info
    fs.mu.Lock()
    fs.fileInfo[id] = fileInfo
    fs.mu.Unlock()
    
    return fileInfo, nil
}

// GetFile gets file information by ID
func (fs *FileStore) GetFile(ctx context.Context, id string) (*FileInfo, error) {
    // Check cache first
    fs.mu.RLock()
    info, ok := fs.fileInfo[id]
    fs.mu.RUnlock()
    
    if ok {
        return info, nil
    }
    
    // If not in cache, try to get from API
    name := id
    if !strings.HasPrefix(id, "files/") {
        name = "files/" + id
    }
    
    file, err := fs.client.GetFile(ctx, name)
    if err != nil {
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
        Size:        file.SizeBytes,
        UploadedAt:  file.CreateTime,
    }
    
    // Set expiration if provided
    if !file.ExpirationTime.IsZero() {
        fileInfo.ExpiresAt = file.ExpirationTime
    }
    
    // Store in cache
    fs.mu.Lock()
    fs.fileInfo[fileID] = fileInfo
    fs.mu.Unlock()
    
    return fileInfo, nil
}

// DeleteFile deletes a file by ID
func (fs *FileStore) DeleteFile(ctx context.Context, id string) error {
    // Get the file info first to get the full name
    fileInfo, err := fs.GetFile(ctx, id)
    if err != nil {
        return err
    }
    
    // Delete from API
    if err := fs.client.DeleteFile(ctx, fileInfo.Name); err != nil {
        return fmt.Errorf("failed to delete file: %w", err)
    }
    
    // Remove from cache
    fs.mu.Lock()
    delete(fs.fileInfo, id)
    fs.mu.Unlock()
    
    return nil
}

// ListFiles returns all files
func (fs *FileStore) ListFiles(ctx context.Context) ([]*FileInfo, error) {
    // Get files from API
    iter := fs.client.ListFiles(ctx)
    
    files := []*FileInfo{}
    fileMap := make(map[string]*FileInfo)
    
    // Iterate through all files
    for {
        file, err := iter.Next()
        if err == iterator.Done {
            break
        }
        if err != nil {
            return nil, fmt.Errorf("failed to list files: %w", err)
        }
        
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
            Size:        file.SizeBytes,
            UploadedAt:  file.CreateTime,
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
    
    return files, nil
}
```

### 4. Cache Store Implementation

```go
// In cache.go

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
    
    // Create expiration
    expiration := genai.ExpireTimeOrTTL{
        TTL: ttl,
    }
    
    // Set up cached content
    cachedContent := &genai.CachedContent{
        Model:      req.Model,
        Expiration: expiration,
    }
    
    // Add display name if provided
    if req.DisplayName != "" {
        cachedContent.DisplayName = req.DisplayName
    }
    
    // Set up system instruction if provided
    if req.SystemPrompt != "" {
        cachedContent.SystemInstruction = genai.NewUserContent(genai.Text(req.SystemPrompt))
    }
    
    // Build contents with files and text
    contents := []*genai.Content{}
    
    // Add files if provided
    if len(req.FileIDs) > 0 {
        for _, fileID := range req.FileIDs {
            // Get file info
            fileInfo, err := cs.fileStore.GetFile(ctx, fileID)
            if err != nil {
                return nil, fmt.Errorf("failed to get file with ID %s: %w", fileID, err)
            }
            
            // Add file to contents
            contents = append(contents, genai.NewUserContent(genai.FileData{URI: fileInfo.URI}))
        }
    }
    
    // Add text content if provided
    if req.Content != "" {
        contents = append(contents, genai.NewUserContent(genai.Text(req.Content)))
    }
    
    // Set contents if we have any
    if len(contents) > 0 {
        cachedContent.Contents = contents
    }
    
    // Create the cached content
    cc, err := cs.client.CreateCachedContent(ctx, cachedContent)
    if err != nil {
        return nil, fmt.Errorf("failed to create cached content: %w", err)
    }
    
    // Extract ID from name (format: "cachedContents/abc123")
    id := cc.Name
    if strings.HasPrefix(cc.Name, "cachedContents/") {
        id = strings.TrimPrefix(cc.Name, "cachedContents/")
    }
    
    // Calculate expiration time
    expiresAt := time.Time{}
    if cc.Expiration.ExpireTime.IsZero() && cc.Expiration.TTL > 0 {
        expiresAt = cc.CreateTime.Add(cc.Expiration.TTL)
    } else if !cc.Expiration.ExpireTime.IsZero() {
        expiresAt = cc.Expiration.ExpireTime
    }
    
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
    
    return cacheInfo, nil
}

// GetCache gets cache information by ID
func (cs *CacheStore) GetCache(ctx context.Context, id string) (*CacheInfo, error) {
    // Check cache first
    cs.mu.RLock()
    info, ok := cs.cacheInfo[id]
    cs.mu.RUnlock()
    
    if ok {
        return info, nil
    }
    
    // If not in cache, try to get from API
    name := id
    if !strings.HasPrefix(id, "cachedContents/") {
        name = "cachedContents/" + id
    }
    
    cc, err := cs.client.GetCachedContent(ctx, name)
    if err != nil {
        return nil, fmt.Errorf("failed to get cached content: %w", err)
    }
    
    // Extract ID from name
    cacheID := cc.Name
    if strings.HasPrefix(cc.Name, "cachedContents/") {
        cacheID = strings.TrimPrefix(cc.Name, "cachedContents/")
    }
    
    // Calculate expiration time
    expiresAt := time.Time{}
    if cc.Expiration.ExpireTime.IsZero() && cc.Expiration.TTL > 0 {
        expiresAt = cc.CreateTime.Add(cc.Expiration.TTL)
    } else if !cc.Expiration.ExpireTime.IsZero() {
        expiresAt = cc.Expiration.ExpireTime
    }
    
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
    
    return cacheInfo, nil
}

// DeleteCache deletes a cache by ID
func (cs *CacheStore) DeleteCache(ctx context.Context, id string) error {
    // Get the cache info first to get the full name
    cacheInfo, err := cs.GetCache(ctx, id)
    if err != nil {
        return err
    }
    
    // Delete from API
    if err := cs.client.DeleteCachedContent(ctx, cacheInfo.Name); err != nil {
        return fmt.Errorf("failed to delete cached content: %w", err)
    }
    
    // Remove from cache
    cs.mu.Lock()
    delete(cs.cacheInfo, id)
    cs.mu.Unlock()
    
    return nil
}

// ListCaches returns all caches
func (cs *CacheStore) ListCaches(ctx context.Context) ([]*CacheInfo, error) {
    // Get caches from API
    iter := cs.client.ListCachedContents(ctx)
    
    caches := []*CacheInfo{}
    cacheMap := make(map[string]*CacheInfo)
    
    // Iterate through all caches
    for {
        cc, err := iter.Next()
        if err == iterator.Done {
            break
        }
        if err != nil {
            return nil, fmt.Errorf("failed to list cached contents: %w", err)
        }
        
        // Extract ID from name
        id := cc.Name
        if strings.HasPrefix(cc.Name, "cachedContents/") {
            id = strings.TrimPrefix(cc.Name, "cachedContents/")
        }
        
        // Calculate expiration time
        expiresAt := time.Time{}
        if cc.Expiration.ExpireTime.IsZero() && cc.Expiration.TTL > 0 {
            expiresAt = cc.CreateTime.Add(cc.Expiration.TTL)
        } else if !cc.Expiration.ExpireTime.IsZero() {
            expiresAt = cc.Expiration.ExpireTime
        }
        
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
    
    return caches, nil
}
```

### 5. Updating Gemini Server

```go
// In gemini.go

// Add these fields to GeminiServer
type GeminiServer struct {
    config     *Config
    client     *genai.Client
    fileStore  *FileStore
    cacheStore *CacheStore
}

// Update NewGeminiServer to initialize the stores
func NewGeminiServer(ctx context.Context, config *Config) (*GeminiServer, error) {
    // Existing code...
    
    // Create the file and cache stores
    fileStore := NewFileStore(client, config)
    cacheStore := NewCacheStore(client, config, fileStore)
    
    return &GeminiServer{
        config:     config,
        client:     client,
        fileStore:  fileStore,
        cacheStore: cacheStore,
    }, nil
}

// Update ListTools to include the new tools
func (s *GeminiServer) ListTools(ctx context.Context) (*protocol.ListToolsResponse, error) {
    tools := []protocol.Tool{
        // Existing tools...
        
        {
            Name:        "upload_file",
            Description: "Upload a file to Gemini for processing",
            InputSchema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "filename": {"type": "string", "description": "Name of the file"},
                    "mime_type": {"type": "string", "description": "MIME type of the file"},
                    "content": {"type": "string", "description": "Base64-encoded file content"},
                    "display_name": {"type": "string", "description": "Optional human-readable name for the file"}
                },
                "required": ["filename", "mime_type", "content"]
            }`),
        },
        {
            Name:        "list_files",
            Description: "List all uploaded files",
            InputSchema: json.RawMessage(`{
                "type": "object",
                "properties": {},
                "required": []
            }`),
        },
        {
            Name:        "delete_file",
            Description: "Delete an uploaded file",
            InputSchema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "file_id": {"type": "string", "description": "ID of the file to delete"}
                },
                "required": ["file_id"]
            }`),
        },
    }
    
    // Add cache tools if caching is enabled
    if s.config.EnableCaching {
        tools = append(tools, []protocol.Tool{
            {
                Name:        "create_cache",
                Description: "Create a cached context for repeated queries",
                InputSchema: json.RawMessage(`{
                    "type": "object",
                    "properties": {
                        "model": {"type": "string", "description": "Gemini model to use"},
                        "system_prompt": {"type": "string", "description": "Optional system prompt for the context"},
                        "file_ids": {"type": "array", "items": {"type": "string"}, "description": "Optional IDs of files to include in the context"},
                        "content": {"type": "string", "description": "Optional text content to include in the context"},
                        "ttl": {"type": "string", "description": "Optional time-to-live for the cache (e.g. '1h', '24h')"},
                        "display_name": {"type": "string", "description": "Optional human-readable name for the cache"}
                    },
                    "required": ["model"]
                }`),
            },
            {
                Name:        "query_with_cache",
                Description: "Query Gemini using a cached context",
                InputSchema: json.RawMessage(`{
                    "type": "object",
                    "properties": {
                        "cache_id": {"type": "string", "description": "ID of the cache to use"},
                        "query": {"type": "string", "description": "Query to send to Gemini"}
                    },
                    "required": ["cache_id", "query"]
                }`),
            },
            {
                Name:        "list_caches",
                Description: "List all cached contexts",
                InputSchema: json.RawMessage(`{
                    "type": "object",
                    "properties": {},
                    "required": []
                }`),
            },
            {
                Name:        "delete_cache",
                Description: "Delete a cached context",
                InputSchema: json.RawMessage(`{
                    "type": "object",
                    "properties": {
                        "cache_id": {"type": "string", "description": "ID of the cache to delete"}
                    },
                    "required": ["cache_id"]
                }`),
            },
        }...)
    }
    
    return &protocol.ListToolsResponse{
        Tools: tools,
    }, nil
}

// Update CallTool to handle the new tools
func (s *GeminiServer) CallTool(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
    switch req.Name {
    // Existing cases...
    
    case "upload_file":
        return s.handleUploadFile(ctx, req)
    case "list_files":
        return s.handleListFiles(ctx)
    case "delete_file":
        return s.handleDeleteFile(ctx, req)
    case "create_cache":
        return s.handleCreateCache(ctx, req)
    case "query_with_cache":
        return s.handleQueryWithCache(ctx, req)
    case "list_caches":
        return s.handleListCaches(ctx)
    case "delete_cache":
        return s.handleDeleteCache(ctx, req)
    default:
        return createErrorResponse(fmt.Sprintf("unknown tool: %s", req.Name)), nil
    }
}

// Handler implementations for each tool
func (s *GeminiServer) handleUploadFile(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
    logger := getLoggerFromContext(ctx)
    logger.Info("Handling file upload request")

    // Extract and validate required parameters
    filename, ok := req.Arguments["filename"].(string)
    if !ok || filename == "" {
        return createErrorResponse("filename must be a non-empty string"), nil
    }

    mimeType, ok := req.Arguments["mime_type"].(string)
    if !ok || mimeType == "" {
        return createErrorResponse("mime_type must be a non-empty string"), nil
    }

    contentBase64, ok := req.Arguments["content"].(string)
    if !ok || contentBase64 == "" {
        return createErrorResponse("content must be a non-empty base64-encoded string"), nil
    }

    // Get optional display name
    displayName, _ := req.Arguments["display_name"].(string)

    // Decode base64 content
    content, err := base64.StdEncoding.DecodeString(contentBase64)
    if err != nil {
        logger.Error("Failed to decode base64 content: %v", err)
        return createErrorResponse("invalid base64 encoding for content"), nil
    }

    // Create upload request
    uploadReq := &FileUploadRequest{
        FileName:    filename,
        MimeType:    mimeType,
        Content:     content,
        DisplayName: displayName,
    }

    // Upload the file
    fileInfo, err := s.fileStore.UploadFile(ctx, uploadReq)
    if err != nil {
        logger.Error("Failed to upload file: %v", err)
        return createErrorResponse(fmt.Sprintf("failed to upload file: %v", err)), nil
    }

    // Format the response
    return &protocol.CallToolResponse{
        Content: []protocol.ToolContent{
            {
                Type: "text",
                Text: fmt.Sprintf("File uploaded successfully:\n\n- File ID: `%s`\n- Name: %s\n- Size: %d bytes\n- MIME Type: %s\n\nUse this File ID when creating a cache context.",
                    fileInfo.ID, fileInfo.DisplayName, fileInfo.Size, fileInfo.MimeType),
            },
        },
    }, nil
}

func (s *GeminiServer) handleListFiles(ctx context.Context) (*protocol.CallToolResponse, error) {
    logger := getLoggerFromContext(ctx)
    logger.Info("Handling list files request")

    // Get files
    files, err := s.fileStore.ListFiles(ctx)
    if err != nil {
        logger.Error("Failed to list files: %v", err)
        return createErrorResponse(fmt.Sprintf("failed to list files: %v", err)), nil
    }

    // Format the response
    var sb strings.Builder
    sb.WriteString("# Uploaded Files\n\n")

    if len(files) == 0 {
        sb.WriteString("No files found.")
    } else {
        sb.WriteString("| ID | Name | MIME Type | Size | Upload Time |\n")
        sb.WriteString("|-----|-------|-----------|------|-------------|\n")

        for _, file := range files {
            displayName := file.DisplayName
            if displayName == "" {
                displayName = file.Name
            }
            
            sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
                file.ID,
                displayName,
                file.MimeType,
                humanReadableSize(file.Size),
                file.UploadedAt.Format(time.RFC3339),
            ))
        }
    }

    return &protocol.CallToolResponse{
        Content: []protocol.ToolContent{
            {
                Type: "text",
                Text: sb.String(),
            },
        },
    }, nil
}

func (s *GeminiServer) handleDeleteFile(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
    logger := getLoggerFromContext(ctx)
    logger.Info("Handling delete file request")

    // Extract and validate required parameters
    fileID, ok := req.Arguments["file_id"].(string)
    if !ok || fileID == "" {
        return createErrorResponse("file_id must be a non-empty string"), nil
    }

    // Delete the file
    if err := s.fileStore.DeleteFile(ctx, fileID); err != nil {
        logger.Error("Failed to delete file: %v", err)
        return createErrorResponse(fmt.Sprintf("failed to delete file: %v", err)), nil
    }

    // Format the response
    return &protocol.CallToolResponse{
        Content: []protocol.ToolContent{
            {
                Type: "text",
                Text: fmt.Sprintf("File with ID `%s` was successfully deleted.", fileID),
            },
        },
    }, nil
}

func (s *GeminiServer) handleCreateCache(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
    logger := getLoggerFromContext(ctx)
    logger.Info("Handling create cache request")

    // Check if caching is enabled
    if !s.config.EnableCaching {
        return createErrorResponse("caching is disabled"), nil
    }

    // Extract and validate required parameters
    model, ok := req.Arguments["model"].(string)
    if !ok || model == "" {
        return createErrorResponse("model must be a non-empty string"), nil
    }

    // Extract optional parameters
    systemPrompt, _ := req.Arguments["system_prompt"].(string)
    content, _ := req.Arguments["content"].(string)
    ttl, _ := req.Arguments["ttl"].(string)
    displayName, _ := req.Arguments["display_name"].(string)

    // Extract file IDs if provided
    var fileIDs []string
    if fileIDsRaw, ok := req.Arguments["file_ids"]; ok {
        if fileIDsList, ok := fileIDsRaw.([]interface{}); ok {
            for _, fileIDRaw := range fileIDsList {
                if fileID, ok := fileIDRaw.(string); ok {
                    fileIDs = append(fileIDs, fileID)
                }
            }
        }
    }

    // Validate that either content or file IDs are provided
    if content == "" && len(fileIDs) == 0 {
        return createErrorResponse("either content or file_ids must be provided"), nil
    }

    // Create cache request
    cacheReq := &CacheRequest{
        Model:        model,
        SystemPrompt: systemPrompt,
        FileIDs:      fileIDs,
        Content:      content,
        TTL:          ttl,
        DisplayName:  displayName,
    }

    // Create the cache
    cacheInfo, err := s.cacheStore.CreateCache(ctx, cacheReq)
    if err != nil {
        logger.Error("Failed to create cache: %v", err)
        return createErrorResponse(fmt.Sprintf("failed to create cache: %v", err)), nil
    }

    // Format the response
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("Cache created successfully:\n\n"))
    sb.WriteString(fmt.Sprintf("- Cache ID: `%s`\n", cacheInfo.ID))
    
    if cacheInfo.DisplayName != "" {
        sb.WriteString(fmt.Sprintf("- Name: %s\n", cacheInfo.DisplayName))
    }
    
    sb.WriteString(fmt.Sprintf("- Model: %s\n", cacheInfo.Model))
    sb.WriteString(fmt.Sprintf("- Created: %s\n", cacheInfo.CreatedAt.Format(time.RFC3339)))
    
    if !cacheInfo.ExpiresAt.IsZero() {
        sb.WriteString(fmt.Sprintf("- Expires: %s\n", cacheInfo.ExpiresAt.Format(time.RFC3339)))
    }
    
    if len(fileIDs) > 0 {
        sb.WriteString("\nIncluded files:\n")
        for _, fileID := range fileIDs {
            sb.WriteString(fmt.Sprintf("- `%s`\n", fileID))
        }
    }
    
    sb.WriteString("\nUse this Cache ID with the `query_with_cache` tool to perform queries using this cached context.")

    return &protocol.CallToolResponse{
        Content: []protocol.ToolContent{
            {
                Type: "text",
                Text: sb.String(),
            },
        },
    }, nil
}

func (s *GeminiServer) handleQueryWithCache(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
    logger := getLoggerFromContext(ctx)
    logger.Info("Handling query with cache request")

    // Check if caching is enabled
    if !s.config.EnableCaching {
        return createErrorResponse("caching is disabled"), nil
    }

    // Extract and validate required parameters
    cacheID, ok := req.Arguments["cache_id"].(string)
    if !ok || cacheID == "" {
        return createErrorResponse("cache_id must be a non-empty string"), nil
    }

    query, ok := req.Arguments["query"].(string)
    if !ok || query == "" {
        return createErrorResponse("query must be a non-empty string"), nil
    }

    // Get cache info
    cacheInfo, err := s.cacheStore.GetCache(ctx, cacheID)
    if err != nil {
        logger.Error("Failed to get cache info: %v", err)
        return createErrorResponse(fmt.Sprintf("failed to get cache: %v", err)), nil
    }

    // Create Gemini model with the cached content
    name := cacheInfo.Name
    if !strings.HasPrefix(name, "cachedContents/") {
        name = "cachedContents/" + name
    }
    
    model := s.client.GenerativeModel(cacheInfo.Model)
    model.CachedContentName = name
    
    // Set the temperature from config
    model.SetTemperature(float32(s.config.GeminiTemperature))
    
    // Send the query
    response, err := model.GenerateContent(ctx, genai.Text(query))
    if err != nil {
        logger.Error("Gemini API error: %v", err)
        return createErrorResponse(fmt.Sprintf("error from Gemini API: %v", err)), nil
    }

    return s.formatResponse(response), nil
}

func (s *GeminiServer) handleListCaches(ctx context.Context) (*protocol.CallToolResponse, error) {
    logger := getLoggerFromContext(ctx)
    logger.Info("Handling list caches request")

    // Check if caching is enabled
    if !s.config.EnableCaching {
        return createErrorResponse("caching is disabled"), nil
    }

    // Get caches
    caches, err := s.cacheStore.ListCaches(ctx)
    if err != nil {
        logger.Error("Failed to list caches: %v", err)
        return createErrorResponse(fmt.Sprintf("failed to list caches: %v", err)), nil
    }

    // Format the response
    var sb strings.Builder
    sb.WriteString("# Cached Contexts\n\n")

    if len(caches) == 0 {
        sb.WriteString("No cached contexts found.")
    } else {
        sb.WriteString("| ID | Name | Model | Created | Expires |\n")
        sb.WriteString("|-----|------|-------|---------|----------|\n")

        for _, cache := range caches {
            displayName := cache.DisplayName
            if displayName == "" {
                displayName = cache.ID
            }
            
            expires := "Never"
            if !cache.ExpiresAt.IsZero() {
                expires = cache.ExpiresAt.Format(time.RFC3339)
            }
            
            sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
                cache.ID,
                displayName,
                cache.Model,
                cache.CreatedAt.Format(time.RFC3339),
                expires,
            ))
        }
    }

    return &protocol.CallToolResponse{
        Content: []protocol.ToolContent{
            {
                Type: "text",
                Text: sb.String(),
            },
        },
    }, nil
}

func (s *GeminiServer) handleDeleteCache(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
    logger := getLoggerFromContext(ctx)
    logger.Info("Handling delete cache request")

    // Check if caching is enabled
    if !s.config.EnableCaching {
        return createErrorResponse("caching is disabled"), nil
    }

    // Extract and validate required parameters
    cacheID, ok := req.Arguments["cache_id"].(string)
    if !ok || cacheID == "" {
        return createErrorResponse("cache_id must be a non-empty string"), nil
    }

    // Delete the cache
    if err := s.cacheStore.DeleteCache(ctx, cacheID); err != nil {
        logger.Error("Failed to delete cache: %v", err)
        return createErrorResponse(fmt.Sprintf("failed to delete cache: %v", err)), nil
    }

    // Format the response
    return &protocol.CallToolResponse{
        Content: []protocol.ToolContent{
            {
                Type: "text",
                Text: fmt.Sprintf("Cache with ID `%s` was successfully deleted.", cacheID),
            },
        },
    }, nil
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

## Implementation Roadmap

1. **Phase 1: Core Implementation**
   - Update the Config struct to include file and cache settings
   - Create the FileStore implementation in files.go
   - Create the CacheStore implementation in cache.go
   - Update the main GeminiServer to include the new stores and tools
   - Implement all handler methods

2. **Phase 2: Testing**
   - Add unit tests for file operations (mock API responses)
   - Add unit tests for cache operations
   - Add end-to-end tests with actual Gemini API calls
   - Test failure modes and error handling

3. **Phase 3: Documentation**
   - Update API documentation to describe the new tools
   - Add examples of how to use file handling and context caching
   - Document best practices and limitations

## Security Considerations

1. **File Validation**
   - The implementation validates MIME types against an allowed list
   - File sizes are checked against configured maximums
   - Base64 decoding helps prevent injection of binary content

2. **Authorization**
   - All operations use the same API key, so no additional authorization is needed
   - In a multi-user environment, additional authorization would be required

3. **Resource Management**
   - Files and caches are tracked to prevent resource leaks
   - The Gemini API manages actual storage and enforces its own limits

## Limitations and Considerations

1. **File Types**
   - The Gemini API supports specific file types (text, PDF, images)
   - File support varies by model version
   - Check the [Gemini API documentation](https://ai.google.dev/gemini-api/docs/prompting_with_media) for current limitations

2. **Cache Expiration**
   - Caches can expire, and requests will fail if using an expired cache
   - TTL should be set appropriately for the use case
   - Consider shorter TTLs for development/testing and longer for production

3. **State Management**
   - The implementation maintains in-memory state for files and caches
   - This state will be lost on server restart
   - Consider adding persistence for production deployments

## Conclusion

Adding file handling and context caching to the GeminiMCP project enhances its capabilities by allowing users to work with documents and maintain context across multiple queries. This implementation follows the patterns established in the existing codebase while adding new functionality in a clean, modular way.

The approach leverages the native capabilities of the Gemini API rather than reimplementing them, which ensures compatibility with future API updates and reduces maintenance overhead. The new tools are fully integrated with the existing MCP protocol framework, providing a seamless experience for users.