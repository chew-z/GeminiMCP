package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Default configuration values
const (
	// Note: if this value changes, make sure to update the models.go list
	defaultGeminiModel        = "gemini-1.5-pro"
	defaultGeminiTemperature  = 0.4
	defaultGeminiSystemPrompt = `
You are a senior developer. Your job is to do a thorough code review of this code.
You should write it up and output markdown.
Include line numbers, and contextual info.
Your code review will be passed to another teammate, so be thorough.
Think deeply  before writing the code review. Review every part, and don't hallucinate.
`
	// System prompt for search-based queries
	defaultGeminiSearchSystemPrompt = `
You are a helpful search assistant. Use the Google Search results to provide accurate and up-to-date information.
Your answers should be comprehensive but concise, focusing on the most relevant information.
Cite your sources when appropriate and maintain a neutral, informative tone.
If the search results don't contain enough information to fully answer the query, acknowledge the limitations.
`
	// File handling defaults
	defaultMaxFileSize = int64(10 * 1024 * 1024) // 10MB explicitly as int64

	// Cache settings defaults
	defaultEnableCaching   = true
	defaultDefaultCacheTTL = 1 * time.Hour
)

// Config holds all configuration parameters for the application
type Config struct {
	// Gemini API settings
	GeminiAPIKey             string
	GeminiModel              string
	GeminiSystemPrompt       string
	GeminiSearchSystemPrompt string
	GeminiTemperature        float64

	// HTTP client settings
	HTTPTimeout time.Duration

	// Retry settings
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration

	// File handling settings
	MaxFileSize      int64    // Max file size in bytes
	AllowedFileTypes []string // Allowed MIME types

	// Cache settings
	EnableCaching   bool          // Enable/disable caching
	DefaultCacheTTL time.Duration // Default TTL if not specified
}

// NewConfig creates a new configuration from environment variables
func NewConfig() (*Config, error) {
	// Validate that the default model is valid
	if err := ValidateModelID(defaultGeminiModel); err != nil {
		return nil, fmt.Errorf("default model is not valid: %w", err)
	}
	// Get Gemini API key - required
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		return nil, errors.New("GEMINI_API_KEY environment variable is required")
	}

	// Get Gemini model - optional with default
	geminiModel := os.Getenv("GEMINI_MODEL")
	if geminiModel == "" {
		geminiModel = defaultGeminiModel // Default model if not specified
	} else {
		// Validate the model from environment variable
		if err := ValidateModelID(geminiModel); err != nil {
			return nil, fmt.Errorf("invalid GEMINI_MODEL environment variable: %w", err)
		}
	}

	// Get Gemini system prompt - optional with default
	geminiSystemPrompt := os.Getenv("GEMINI_SYSTEM_PROMPT")
	if geminiSystemPrompt == "" {
		geminiSystemPrompt = defaultGeminiSystemPrompt // Default system prompt if not specified
	}

	// Get Gemini search system prompt - optional with default
	geminiSearchSystemPrompt := os.Getenv("GEMINI_SEARCH_SYSTEM_PROMPT")
	if geminiSearchSystemPrompt == "" {
		geminiSearchSystemPrompt = defaultGeminiSearchSystemPrompt // Default search system prompt if not specified
	}

	// Default timeout of 90 seconds
	timeout := 90 * time.Second

	// Override timeout if provided in environment
	if timeoutStr := os.Getenv("GEMINI_TIMEOUT"); timeoutStr != "" {
		if timeoutSec, err := strconv.Atoi(timeoutStr); err == nil && timeoutSec > 0 {
			timeout = time.Duration(timeoutSec) * time.Second
		}
	}

	// Set default retry values
	maxRetries := 2
	initialBackoff := 1 * time.Second
	maxBackoff := 10 * time.Second

	// Override retry settings if provided in environment
	if retriesStr := os.Getenv("GEMINI_MAX_RETRIES"); retriesStr != "" {
		if retries, err := strconv.Atoi(retriesStr); err == nil && retries >= 0 {
			maxRetries = retries
		}
	}

	if backoffStr := os.Getenv("GEMINI_INITIAL_BACKOFF"); backoffStr != "" {
		if backoff, err := strconv.Atoi(backoffStr); err == nil && backoff > 0 {
			initialBackoff = time.Duration(backoff) * time.Second
		}
	}

	if maxBackoffStr := os.Getenv("GEMINI_MAX_BACKOFF"); maxBackoffStr != "" {
		if backoff, err := strconv.Atoi(maxBackoffStr); err == nil && backoff > 0 {
			maxBackoff = time.Duration(backoff) * time.Second
		}
	}

	// Set default temperature or override with environment variable
	geminiTemperature := defaultGeminiTemperature
	if tempStr := os.Getenv("GEMINI_TEMPERATURE"); tempStr != "" {
		if tempVal, err := strconv.ParseFloat(tempStr, 64); err == nil {
			// Validate that temperature is within valid range (0.0 to 1.0)
			if tempVal < 0.0 || tempVal > 1.0 {
				return nil, fmt.Errorf("GEMINI_TEMPERATURE must be between 0.0 and 1.0, got %v", tempVal)
			}
			geminiTemperature = tempVal
		} else {
			return nil, fmt.Errorf("invalid GEMINI_TEMPERATURE value: %v", err)
		}
	}

	// File handling settings
	maxFileSize := defaultMaxFileSize
	if sizeStr := os.Getenv("GEMINI_MAX_FILE_SIZE"); sizeStr != "" {
		if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil && size > 0 {
			maxFileSize = size
		}
	}

	allowedFileTypes := []string{
		"text/x-go", "text/x-python", "text/javascript", "text/typescript",
		"text/x-java", "text/x-c", "text/x-c++", "text/markdown", "text/html",
		"text/css", "application/json", "text/yaml", "text/plain",
	}
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
		GeminiAPIKey:             geminiAPIKey,
		GeminiModel:              geminiModel,
		GeminiSystemPrompt:       geminiSystemPrompt,
		GeminiSearchSystemPrompt: geminiSearchSystemPrompt,
		GeminiTemperature:        geminiTemperature,
		HTTPTimeout:        timeout,
		MaxRetries:         maxRetries,
		InitialBackoff:     initialBackoff,
		MaxBackoff:         maxBackoff,
		MaxFileSize:        maxFileSize,
		AllowedFileTypes:   allowedFileTypes,
		EnableCaching:      enableCaching,
		DefaultCacheTTL:    defaultCacheTTL,
	}, nil
}
