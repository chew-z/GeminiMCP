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
	defaultGeminiModel        = "gemini-2.5-pro"
	defaultGeminiSearchModel  = "gemini-2.5-flash-lite" // Default model specifically for search
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

	// Thinking settings
	defaultEnableThinking      = true
	defaultThinkingBudgetLevel = "low" // Default thinking budget level
	thinkingBudgetNone         = 0     // None: Thinking disabled
	thinkingBudgetLow          = 4096  // Low: 4096 tokens
	thinkingBudgetMedium       = 16384 // Medium: 16384 tokens
	thinkingBudgetHigh         = 24576 // High: Maximum allowed by Gemini (24576 tokens)
)

// Config struct definition moved to structs.go

// getThinkingBudgetFromLevel converts a thinking budget level string to a token count
func getThinkingBudgetFromLevel(level string) int {
	switch strings.ToLower(level) {
	case "none":
		return thinkingBudgetNone
	case "low":
		return thinkingBudgetLow
	case "medium":
		return thinkingBudgetMedium
	case "high":
		return thinkingBudgetHigh
	default:
		return thinkingBudgetLow // Default to low if invalid level
	}
}

// Helper function to parse an integer environment variable with a default
func parseEnvVarInt(key string, defaultValue int) int {
	if str := os.Getenv(key); str != "" {
		if val, err := strconv.Atoi(str); err == nil {
			return val
		}
		// Log warning directly as logger might not be initialized yet
		fmt.Fprintf(os.Stderr, "[WARN] Invalid integer value for %s: %q. Using default: %d\n", key, str, defaultValue)
	}
	return defaultValue
}

// Helper function to parse a float64 environment variable with a default
func parseEnvVarFloat(key string, defaultValue float64) float64 {
	if str := os.Getenv(key); str != "" {
		if val, err := strconv.ParseFloat(str, 64); err == nil {
			return val
		}
		fmt.Fprintf(os.Stderr, "[WARN] Invalid float value for %s: %q. Using default: %f\n", key, str, defaultValue)
	}
	return defaultValue
}

// Helper function to parse a duration environment variable with a default
func parseEnvVarDuration(key string, defaultValue time.Duration) time.Duration {
	if str := os.Getenv(key); str != "" {
		if val, err := time.ParseDuration(str); err == nil {
			return val
		}
		fmt.Fprintf(os.Stderr, "[WARN] Invalid duration value for %s: %q. Using default: %s\n", key, str, defaultValue.String())
	}
	return defaultValue
}

// Helper function to parse a boolean environment variable with a default
func parseEnvVarBool(key string, defaultValue bool) bool {
	if str := os.Getenv(key); str != "" {
		if val, err := strconv.ParseBool(str); err == nil {
			return val
		}
		fmt.Fprintf(os.Stderr, "[WARN] Invalid boolean value for %s: %q. Using default: %t\n", key, str, defaultValue)
	}
	return defaultValue
}

// NewConfig creates a new configuration from environment variables
func NewConfig() (*Config, error) {
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
	timeout := parseEnvVarDuration("GEMINI_TIMEOUT", 90*time.Second)
	maxRetries := parseEnvVarInt("GEMINI_MAX_RETRIES", 2)
	initialBackoff := parseEnvVarDuration("GEMINI_INITIAL_BACKOFF", 1*time.Second)
	maxBackoff := parseEnvVarDuration("GEMINI_MAX_BACKOFF", 10*time.Second)

	// Set default temperature or override with environment variable
	geminiTemperature := parseEnvVarFloat("GEMINI_TEMPERATURE", defaultGeminiTemperature)
	// Specific validation for temperature range, as it's a critical parameter
	if geminiTemperature < 0.0 || geminiTemperature > 1.0 {
		return nil, fmt.Errorf("GEMINI_TEMPERATURE must be between 0.0 and 1.0, got %v", geminiTemperature)
	}

	// File handling settings
	maxFileSize := int64(parseEnvVarInt("GEMINI_MAX_FILE_SIZE", int(defaultMaxFileSize)))
	if maxFileSize <= 0 {
		fmt.Fprintf(os.Stderr, "[WARN] GEMINI_MAX_FILE_SIZE must be positive. Using default: %d\n", defaultMaxFileSize)
		maxFileSize = defaultMaxFileSize
	}

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

	// Cache settings
	enableCaching := parseEnvVarBool("GEMINI_ENABLE_CACHING", defaultEnableCaching)
	defaultCacheTTL := parseEnvVarDuration("GEMINI_DEFAULT_CACHE_TTL", defaultDefaultCacheTTL)
	if defaultCacheTTL <= 0 {
		fmt.Fprintf(os.Stderr, "[WARN] GEMINI_DEFAULT_CACHE_TTL must be positive. Using default: %s\n", defaultDefaultCacheTTL.String())
		defaultCacheTTL = defaultDefaultCacheTTL
	}

	// Thinking settings
	enableThinking := parseEnvVarBool("GEMINI_ENABLE_THINKING", defaultEnableThinking)

	// Set thinking budget level from environment variable or use default
	thinkingBudgetLevel := defaultThinkingBudgetLevel
	if levelStr := os.Getenv("GEMINI_THINKING_BUDGET_LEVEL"); levelStr != "" {
		level := strings.ToLower(levelStr)
		if level == "none" || level == "low" || level == "medium" || level == "high" {
			thinkingBudgetLevel = level
		} else {
			fmt.Fprintf(os.Stderr, "[WARN] Invalid GEMINI_THINKING_BUDGET_LEVEL value: %q. Using default: %q\n",
				levelStr, defaultThinkingBudgetLevel)
		}
	}

	// Set thinking budget from environment variable or derive from level
	thinkingBudget := getThinkingBudgetFromLevel(thinkingBudgetLevel)
	// If GEMINI_THINKING_BUDGET is set, it overrides the level-derived value.
	// The helper will use the level-derived budget as a fallback if parsing fails.
	thinkingBudget = parseEnvVarInt("GEMINI_THINKING_BUDGET", thinkingBudget)

	// HTTP transport settings
	enableHTTP := parseEnvVarBool("GEMINI_ENABLE_HTTP", defaultEnableHTTP)
	httpAddress := os.Getenv("GEMINI_HTTP_ADDRESS")
	if httpAddress == "" {
		httpAddress = defaultHTTPAddress
	}
	httpPath := os.Getenv("GEMINI_HTTP_PATH")
	if httpPath == "" {
		httpPath = defaultHTTPPath
	}
	httpStateless := parseEnvVarBool("GEMINI_HTTP_STATELESS", defaultHTTPStateless)
	httpHeartbeat := parseEnvVarDuration("GEMINI_HTTP_HEARTBEAT", defaultHTTPHeartbeat)
	if httpHeartbeat < 0 {
		fmt.Fprintf(os.Stderr, "[WARN] GEMINI_HTTP_HEARTBEAT must be non-negative. Using default: %s\n", defaultHTTPHeartbeat.String())
		httpHeartbeat = defaultHTTPHeartbeat
	}
	httpCORSEnabled := parseEnvVarBool("GEMINI_HTTP_CORS_ENABLED", defaultHTTPCORSEnabled)
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
	authEnabled := parseEnvVarBool("GEMINI_AUTH_ENABLED", defaultAuthEnabled)
	authSecretKey := os.Getenv("GEMINI_AUTH_SECRET_KEY")

	// If authentication is enabled, require secret key
	if authEnabled && authSecretKey == "" {
		return nil, fmt.Errorf("GEMINI_AUTH_SECRET_KEY is required when GEMINI_AUTH_ENABLED=true")
	}

	// Warn if secret key is too short (for security)
	if authEnabled && len(authSecretKey) < 32 {
		fmt.Fprintf(os.Stderr, "[WARN] GEMINI_AUTH_SECRET_KEY should be at least 32 characters for security\n")
	}

	// Prompt defaults
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
		EnableCaching:            enableCaching,
		DefaultCacheTTL:          defaultCacheTTL,
		EnableThinking:           enableThinking,
		ThinkingBudget:           thinkingBudget,
		ThinkingBudgetLevel:      thinkingBudgetLevel,
		PromptDefaultAudience:    promptDefaultAudience,
		PromptDefaultFocus:       promptDefaultFocus,
		PromptDefaultSeverity:    promptDefaultSeverity,
		PromptDefaultDocFormat:   promptDefaultDocFormat,
		PromptDefaultFramework:   promptDefaultFramework,
		PromptDefaultCoverage:    promptDefaultCoverage,
		PromptDefaultCompliance:  promptDefaultCompliance,
	}, nil
}
