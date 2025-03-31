package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
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
)

// Config holds all configuration parameters for the application
type Config struct {
	// Gemini API settings
	GeminiAPIKey       string
	GeminiModel        string
	GeminiSystemPrompt string
	GeminiTemperature  float64

	// HTTP client settings
	HTTPTimeout time.Duration

	// Retry settings
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
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

	return &Config{
		GeminiAPIKey:       geminiAPIKey,
		GeminiModel:        geminiModel,
		GeminiSystemPrompt: geminiSystemPrompt,
		GeminiTemperature:  geminiTemperature,
		HTTPTimeout:        timeout,
		MaxRetries:         maxRetries,
		InitialBackoff:     initialBackoff,
		MaxBackoff:         maxBackoff,
	}, nil
}
