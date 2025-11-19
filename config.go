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
	defaultGeminiModel        = "gemini-3-pro-preview"
	defaultGeminiSearchModel  = "gemini-flash-lite-latest" // Default model specifically for search
	defaultGeminiTemperature  = 1.0                        // Gemini 3 default temperature
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

	// GitHub settings defaults
	defaultGitHubAPIBaseURL  = "https://api.github.com"
	defaultMaxGitHubFiles    = 20
	defaultMaxGitHubFileSize = int64(1 * 1024 * 1024) // 1MB

	// HTTP transport defaults
	defaultEnableHTTP      = false
	defaultHTTPAddress     = ":8080"
	defaultHTTPPath        = "/mcp"
	defaultHTTPStateless   = false
	defaultHTTPHeartbeat   = 0 * time.Second // No heartbeat by default
	defaultHTTPCORSEnabled = true

	// Authentication defaults
	defaultAuthEnabled = false // Authentication disabled by default

	// Cache settings defaults
	defaultEnableCaching   = true
	defaultDefaultCacheTTL = 1 * time.Hour

	// Thinking settings for Gemini 3
	defaultEnableThinking = true
	defaultThinkingLevel  = "high" // Default thinking level for Gemini 3 (low, medium [coming soon], high [default])
)

// Config struct definition moved to structs.go

// validateThinkingLevel validates the thinking level string for Gemini 3
func validateThinkingLevel(level string) bool {
	normalizedLevel := strings.ToLower(level)
	return normalizedLevel == "low" || normalizedLevel == "high"
	// Note: "medium" is coming soon and not supported at launch
}

// Helper function to parse an integer environment variable with a default
func parseEnvVarInt(key string, defaultValue int, logger Logger) int {
	if str := os.Getenv(key); str != "" {
		if val, err := strconv.Atoi(str); err == nil {
			return val
		}
		logger.Warnf("Invalid integer value for %s: %q. Using default: %d", key, str, defaultValue)
	}
	return defaultValue
}

// Helper function to parse a float64 environment variable with a default
func parseEnvVarFloat(key string, defaultValue float64, logger Logger) float64 {
	if str := os.Getenv(key); str != "" {
		if val, err := strconv.ParseFloat(str, 64); err == nil {
			return val
		}
		logger.Warnf("Invalid float value for %s: %q. Using default: %f", key, str, defaultValue)
	}
	return defaultValue
}

// Helper function to parse a duration environment variable with a default
func parseEnvVarDuration(key string, defaultValue time.Duration, logger Logger) time.Duration {
	if str := os.Getenv(key); str != "" {
		if val, err := time.ParseDuration(str); err == nil {
			return val
		}
		logger.Warnf("Invalid duration value for %s: %q. Using default: %s", key, str, defaultValue.String())
	}
	return defaultValue
}

// Helper function to parse a boolean environment variable with a default
func parseEnvVarBool(key string, defaultValue bool, logger Logger) bool {
	if str := os.Getenv(key); str != "" {
		if val, err := strconv.ParseBool(str); err == nil {
			return val
		}
		logger.Warnf("Invalid boolean value for %s: %q. Using default: %t", key, str, defaultValue)
	}
	return defaultValue
}

// NewConfig creates a new configuration from environment variables
func NewConfig(logger Logger) (*Config, error) {
	// No longer validating default model at startup - will be checked when needed
	// This allows for new models not in our hardcoded list
	// Get Gemini API key - required
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		return nil, errors.New("GEMINI_API_KEY environment variable is required")
	}

	// Get Gemini model - optional with default
	geminiModel := os.Getenv("GEMINI_MODEL")
	if geminiModel == "" {
		geminiModel = defaultGeminiModel // Default model if not specified
	}
	// Note: We no longer validate the model here to allow for new models
	// and preview versions not in our hardcoded list

	// Get Gemini search model - optional with default
	geminiSearchModel := os.Getenv("GEMINI_SEARCH_MODEL")
	if geminiSearchModel == "" {
		geminiSearchModel = defaultGeminiSearchModel // Default search model if not specified
	}
	// Note: We also don't validate the search model here

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

	// Use helper functions to parse environment variables
	timeout := parseEnvVarDuration("GEMINI_TIMEOUT", 90*time.Second, logger)
	maxRetries := parseEnvVarInt("GEMINI_MAX_RETRIES", 2, logger)
	initialBackoff := parseEnvVarDuration("GEMINI_INITIAL_BACKOFF", 1*time.Second, logger)
	maxBackoff := parseEnvVarDuration("GEMINI_MAX_BACKOFF", 10*time.Second, logger)

	// Set default temperature or override with environment variable
	geminiTemperature := parseEnvVarFloat("GEMINI_TEMPERATURE", defaultGeminiTemperature, logger)
	// Specific validation for temperature range, as it's a critical parameter
	if geminiTemperature < 0.0 || geminiTemperature > 1.0 {
		return nil, fmt.Errorf("GEMINI_TEMPERATURE must be between 0.0 and 1.0, got %v", geminiTemperature)
	}

	// File handling settings
	maxFileSize := int64(parseEnvVarInt("GEMINI_MAX_FILE_SIZE", int(defaultMaxFileSize), logger))
	if maxFileSize <= 0 {
		logger.Warnf("GEMINI_MAX_FILE_SIZE must be positive. Using default: %d", defaultMaxFileSize)
		maxFileSize = defaultMaxFileSize
	}

	fileReadBaseDir := os.Getenv("GEMINI_FILE_READ_BASE_DIR")

	var allowedFileTypes []string
	if typesStr := os.Getenv("GEMINI_ALLOWED_FILE_TYPES"); typesStr != "" {
		parts := strings.Split(typesStr, ",")
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				allowedFileTypes = append(allowedFileTypes, trimmed)
			}
		}
	}
	if len(allowedFileTypes) == 0 {
		allowedFileTypes = []string{
			"text/plain", "text/javascript", "text/typescript",
			"text/markdown", "text/html", "text/css",
			"application/json", "text/yaml", "application/octet-stream",
		}
	}

	// GitHub settings
	githubToken := os.Getenv("GEMINI_GITHUB_TOKEN")
	githubAPIBaseURL := os.Getenv("GEMINI_GITHUB_API_BASE_URL")
	if githubAPIBaseURL == "" {
		githubAPIBaseURL = defaultGitHubAPIBaseURL
	}
	maxGitHubFiles := parseEnvVarInt("GEMINI_MAX_GITHUB_FILES", defaultMaxGitHubFiles, logger)
	if maxGitHubFiles <= 0 {
		logger.Warnf("GEMINI_MAX_GITHUB_FILES must be positive. Using default: %d", defaultMaxGitHubFiles)
		maxGitHubFiles = defaultMaxGitHubFiles
	}
	maxGitHubFileSize := int64(parseEnvVarInt("GEMINI_MAX_GITHUB_FILE_SIZE", int(defaultMaxGitHubFileSize), logger))
	if maxGitHubFileSize <= 0 {
		logger.Warnf("GEMINI_MAX_GITHUB_FILE_SIZE must be positive. Using default: %d", defaultMaxGitHubFileSize)
		maxGitHubFileSize = defaultMaxGitHubFileSize
	}

	// Cache settings
	enableCaching := parseEnvVarBool("GEMINI_ENABLE_CACHING", defaultEnableCaching, logger)
	defaultCacheTTL := parseEnvVarDuration("GEMINI_DEFAULT_CACHE_TTL", defaultDefaultCacheTTL, logger)
	if defaultCacheTTL <= 0 {
		logger.Warnf("GEMINI_DEFAULT_CACHE_TTL must be positive. Using default: %s", defaultDefaultCacheTTL.String())
		defaultCacheTTL = defaultDefaultCacheTTL
	}

	// Thinking settings for Gemini 3
	enableThinking := parseEnvVarBool("GEMINI_ENABLE_THINKING", defaultEnableThinking, logger)

	// Set thinking level from environment variable or use default
	thinkingLevel := defaultThinkingLevel
	if levelStr := os.Getenv("GEMINI_THINKING_LEVEL"); levelStr != "" {
		level := strings.ToLower(levelStr)
		if validateThinkingLevel(level) {
			thinkingLevel = level
		} else {
			logger.Warnf("Invalid GEMINI_THINKING_LEVEL value: %q (valid values: 'low', 'high'). Using default: %q",
				levelStr, defaultThinkingLevel)
		}
	}

	// HTTP transport settings
	enableHTTP := parseEnvVarBool("GEMINI_ENABLE_HTTP", defaultEnableHTTP, logger)
	httpAddress := os.Getenv("GEMINI_HTTP_ADDRESS")
	if httpAddress == "" {
		httpAddress = defaultHTTPAddress
	}
	httpPath := os.Getenv("GEMINI_HTTP_PATH")
	if httpPath == "" {
		httpPath = defaultHTTPPath
	}
	httpStateless := parseEnvVarBool("GEMINI_HTTP_STATELESS", defaultHTTPStateless, logger)
	httpHeartbeat := parseEnvVarDuration("GEMINI_HTTP_HEARTBEAT", defaultHTTPHeartbeat, logger)
	if httpHeartbeat < 0 {
		logger.Warnf("GEMINI_HTTP_HEARTBEAT must be non-negative. Using default: %s", defaultHTTPHeartbeat.String())
		httpHeartbeat = defaultHTTPHeartbeat
	}
	httpCORSEnabled := parseEnvVarBool("GEMINI_HTTP_CORS_ENABLED", defaultHTTPCORSEnabled, logger)
	var httpCORSOrigins []string
	if originsStr := os.Getenv("GEMINI_HTTP_CORS_ORIGINS"); originsStr != "" {
		parts := strings.Split(originsStr, ",")
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				httpCORSOrigins = append(httpCORSOrigins, trimmed)
			}
		}
	}
	if len(httpCORSOrigins) == 0 {
		httpCORSOrigins = []string{"*"} // Default allow all origins
	}

	// Authentication settings
	authEnabled := parseEnvVarBool("GEMINI_AUTH_ENABLED", defaultAuthEnabled, logger)
	authSecretKey := os.Getenv("GEMINI_AUTH_SECRET_KEY")

	// If authentication is enabled, require secret key
	if authEnabled && authSecretKey == "" {
		return nil, fmt.Errorf("GEMINI_AUTH_SECRET_KEY is required when GEMINI_AUTH_ENABLED=true")
	}

	// Warn if secret key is too short (for security)
	if authEnabled && len(authSecretKey) < 32 {
		logger.Warnf("GEMINI_AUTH_SECRET_KEY should be at least 32 characters for security")
	}

	// Prompt defaults
	projectLanguage := os.Getenv("GEMINI_PROJECT_LANGUAGE")
	if projectLanguage == "" {
		projectLanguage = "go"
	}

	promptDefaultAudience := os.Getenv("GEMINI_PROMPT_DEFAULT_AUDIENCE")
	if promptDefaultAudience == "" {
		promptDefaultAudience = "intermediate"
	}

	promptDefaultFocus := os.Getenv("GEMINI_PROMPT_DEFAULT_FOCUS")
	if promptDefaultFocus == "" {
		promptDefaultFocus = "general"
	}

	promptDefaultSeverity := os.Getenv("GEMINI_PROMPT_DEFAULT_SEVERITY")
	if promptDefaultSeverity == "" {
		promptDefaultSeverity = "warning"
	}

	promptDefaultDocFormat := os.Getenv("GEMINI_PROMPT_DEFAULT_DOC_FORMAT")
	if promptDefaultDocFormat == "" {
		promptDefaultDocFormat = "markdown"
	}

	promptDefaultFramework := os.Getenv("GEMINI_PROMPT_DEFAULT_FRAMEWORK")
	if promptDefaultFramework == "" {
		promptDefaultFramework = "standard"
	}

	promptDefaultCoverage := os.Getenv("GEMINI_PROMPT_DEFAULT_COVERAGE")
	if promptDefaultCoverage == "" {
		promptDefaultCoverage = "comprehensive"
	}

	promptDefaultCompliance := os.Getenv("GEMINI_PROMPT_DEFAULT_COMPLIANCE")
	if promptDefaultCompliance == "" {
		promptDefaultCompliance = "OWASP"
	}

	return &Config{
			GeminiAPIKey:             geminiAPIKey,
			GeminiModel:              geminiModel,
			GeminiSearchModel:        geminiSearchModel, // Assign the read value
			GeminiSystemPrompt:       geminiSystemPrompt,
			GeminiSearchSystemPrompt: geminiSearchSystemPrompt,
			GeminiTemperature:        geminiTemperature,
			HTTPTimeout:              timeout,
			EnableHTTP:               enableHTTP,
			HTTPAddress:              httpAddress,
			HTTPPath:                 httpPath,
			HTTPStateless:            httpStateless,
			HTTPHeartbeat:            httpHeartbeat,
			HTTPCORSEnabled:          httpCORSEnabled,
			HTTPCORSOrigins:          httpCORSOrigins,
			AuthEnabled:              authEnabled,
			AuthSecretKey:            authSecretKey,
			MaxRetries:               maxRetries,
			InitialBackoff:           initialBackoff,
			MaxBackoff:               maxBackoff,
			MaxFileSize:              maxFileSize,
			AllowedFileTypes:         allowedFileTypes,
			FileReadBaseDir:          fileReadBaseDir,

			// GitHub settings
			GitHubToken:       githubToken,
			GitHubAPIBaseURL:  githubAPIBaseURL,
			MaxGitHubFiles:    maxGitHubFiles,
			MaxGitHubFileSize: maxGitHubFileSize,

			// Cache settings
			EnableCaching:           enableCaching,
			DefaultCacheTTL:         defaultCacheTTL,
			EnableThinking:          enableThinking,
			ThinkingLevel:           thinkingLevel,
			ProjectLanguage:         projectLanguage,
			PromptDefaultAudience:   promptDefaultAudience,
			PromptDefaultFocus:      promptDefaultFocus,
			PromptDefaultSeverity:   promptDefaultSeverity,
			PromptDefaultDocFormat:  promptDefaultDocFormat,
			PromptDefaultFramework:  promptDefaultFramework,
			PromptDefaultCoverage:   promptDefaultCoverage,
			PromptDefaultCompliance: promptDefaultCompliance,
		},
		nil
}
