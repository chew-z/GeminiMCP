package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/genai"
)

// GeminiModelInfo holds information about a Gemini model
type GeminiModelInfo struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	SupportsCaching   bool   `json:"supports_caching"`    // Whether this model supports caching
	SupportsThinking  bool   `json:"supports_thinking"`   // Whether this model supports thinking mode
	ContextWindowSize int    `json:"context_window_size"` // Maximum context window size in tokens
}

// GetModelByID returns a specific model by ID, or nil if not found
func GetModelByID(modelID string) *GeminiModelInfo {
	models := GetAvailableGeminiModels()
	for _, model := range models {
		if model.ID == modelID {
			return &model
		}
	}
	return nil
}

// ValidateModelID checks if a model ID is in the list of available models
// Returns nil if valid, error otherwise
func ValidateModelID(modelID string) error {
	// First check if it's in our known models list
	if GetModelByID(modelID) != nil {
		return nil
	}

	// Special handling for preview models or other special cases
	// Preview models often have date suffixes like "preview-04-17"
	if strings.Contains(modelID, "preview") ||
		strings.Contains(modelID, "exp") ||
		strings.HasSuffix(modelID, "-dev") {
		// Allow preview/experimental models even if not in our list
		return nil
	}

	// Model is neither in our list nor a recognized preview format
	// Return a warning, but don't block the model from being used
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Unknown model ID: %s. Known models are:", modelID))
	for _, model := range GetAvailableGeminiModels() {
		sb.WriteString(fmt.Sprintf("\n- %s: %s", model.ID, model.Name))
	}
	sb.WriteString("\n\nHowever, we will attempt to use this model anyway. It may be a new or preview model.")

	return errors.New(sb.String())
}

// fallbackGeminiModels provides a list of default Gemini models to use if API fetching fails
func fallbackGeminiModels() []GeminiModelInfo {
	return []GeminiModelInfo{
		// Gemini 1.5 Pro Models (core models)
		{
			ID:                "gemini-1.5-pro-latest",
			Name:              "Gemini 1.5 Pro Latest",
			Description:       "Pro model with advanced reasoning capabilities",
			SupportsCaching:   false,
			SupportsThinking:  false, // Not reliably working with thinking mode
			ContextWindowSize: 1048576,
		},
		{
			ID:                "gemini-1.5-pro-001",
			Name:              "Gemini 1.5 Pro 001",
			Description:       "Pro model with advanced reasoning capabilities",
			SupportsCaching:   true,
			SupportsThinking:  false, // Doesn't work with thinking mode per testing
			ContextWindowSize: 1048576,
		},
		{
			ID:                "gemini-1.5-pro",
			Name:              "Gemini 1.5 Pro",
			Description:       "Pro model with advanced reasoning capabilities",
			SupportsCaching:   false,
			SupportsThinking:  false, // Doesn't work with thinking mode per testing
			ContextWindowSize: 1048576,
		},

		// Gemini 1.5 Flash Models (core models)
		{
			ID:                "gemini-1.5-flash-latest",
			Name:              "Gemini 1.5 Flash Latest",
			Description:       "Flash model optimized for efficiency and speed",
			SupportsCaching:   false,
			SupportsThinking:  false,
			ContextWindowSize: 32768,
		},
		{
			ID:                "gemini-1.5-flash-001",
			Name:              "Gemini 1.5 Flash 001",
			Description:       "Flash model optimized for efficiency and speed",
			SupportsCaching:   true,
			SupportsThinking:  false,
			ContextWindowSize: 32768,
		},
		{
			ID:                "gemini-1.5-flash",
			Name:              "Gemini 1.5 Flash",
			Description:       "Flash model optimized for efficiency and speed",
			SupportsCaching:   false,
			SupportsThinking:  false,
			ContextWindowSize: 32768,
		},

		// Gemini 2.5 Models (Preview/Experimental)
		{
			ID:                "gemini-2.5-pro-exp-03-25",
			Name:              "Gemini 2.5 Pro Exp 03 25",
			Description:       "Preview/Experimental Pro model with advanced reasoning capabilities",
			SupportsCaching:   false,
			SupportsThinking:  true, // Confirmed to work with thinking mode
			ContextWindowSize: 1048576,
		},
		{
			ID:                "gemini-2.5-pro-preview-03-25",
			Name:              "Gemini 2.5 Pro Preview 03 25",
			Description:       "Preview/Experimental Pro model with advanced reasoning capabilities (best thinking mode support)",
			SupportsCaching:   false,
			SupportsThinking:  true, // Confirmed to work with thinking mode
			ContextWindowSize: 1048576,
		},
		{
			ID:                "gemini-2.5-flash-preview-04-17",
			Name:              "Gemini 2.5 Flash Preview 04 17",
			Description:       "Preview/Experimental Flash model optimized for efficiency and speed",
			SupportsCaching:   false,
			SupportsThinking:  false,
			ContextWindowSize: 32768,
		},

		// Gemini 2.0 Models (core models)
		{
			ID:                "gemini-2.0-flash",
			Name:              "Gemini 2.0 Flash",
			Description:       "Flash model optimized for efficiency and speed",
			SupportsCaching:   false,
			SupportsThinking:  false,
			ContextWindowSize: 32768,
		},
		{
			ID:                "gemini-2.0-flash-001",
			Name:              "Gemini 2.0 Flash 001",
			Description:       "Flash model optimized for efficiency and speed",
			SupportsCaching:   true,
			SupportsThinking:  false,
			ContextWindowSize: 32768,
		},
		{
			ID:                "gemini-2.0-flash-lite",
			Name:              "Gemini 2.0 Flash Lite",
			Description:       "Flash model optimized for efficiency and speed",
			SupportsCaching:   false,
			SupportsThinking:  false,
			ContextWindowSize: 32768,
		},

		// Latest Preview/Experimental models
		{
			ID:                "gemini-2.0-flash-exp",
			Name:              "Gemini 2.0 Flash Exp",
			Description:       "Preview/Experimental Flash model optimized for efficiency and speed",
			SupportsCaching:   false,
			SupportsThinking:  false,
			ContextWindowSize: 32768,
		},
		{
			ID:                "gemini-2.0-pro-exp",
			Name:              "Gemini 2.0 Pro Exp",
			Description:       "Preview/Experimental Pro model with advanced reasoning capabilities",
			SupportsCaching:   false,
			SupportsThinking:  true,
			ContextWindowSize: 1048576,
		},
	}
}

// modelStore handles storing and retrieving models
var modelStore struct {
	sync.RWMutex
	models []GeminiModelInfo
}

// FetchGeminiModels fetches available models from the Gemini API
// and updates the model store
func FetchGeminiModels(ctx context.Context, apiKey string) error {
	// Create a new client for fetching models
	clientConfig := &genai.ClientConfig{
		APIKey: apiKey,
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return fmt.Errorf("failed to create Gemini client for model listing: %w", err)
	}

	// Get logger from context if available
	var logger Logger
	loggerValue := ctx.Value(loggerKey)
	if loggerValue != nil {
		if l, ok := loggerValue.(Logger); ok {
			logger = l
		} else {
			logger = NewLogger(LevelInfo)
		}
	} else {
		logger = NewLogger(LevelInfo)
	}

	logger.Info("Fetching available Gemini models from API...")

	// Create a slice to store fetched models
	var fetchedModels []GeminiModelInfo

	// Track total models found for better diagnostics
	modelCount := 0

	// Fetch models from API
	logger.Debug("Starting model fetch from Gemini API...")
	for model, err := range client.Models.All(ctx) {
		modelCount++
		if err != nil {
			logger.Error("Error fetching models: %v", err)
			return fmt.Errorf("error fetching models: %w", err)
		}

		// More flexible matching for Gemini models
		// Look for common markers in model names/identifiers
		modelName := strings.ToLower(model.Name)
		if strings.Contains(modelName, "gemini") {
			// Extract ID from model name, handling different API response formats
			id := model.Name
			if strings.HasPrefix(id, "models/") {
				id = strings.TrimPrefix(id, "models/")
			}

			logger.Debug("Found Gemini model from API: %s", id)
			logger.Debug("Model details: %+v", model)

			// Check if model has version suffix for caching support
			supportsCaching := strings.HasSuffix(id, "-001") || strings.Contains(id, "stable")

			// Create a more user-friendly name
			name := strings.TrimPrefix(id, "gemini-")
			name = strings.ReplaceAll(name, "-", " ")
			name = strings.Title(name)
			if supportsCaching {
				name += " (Stable)"
			}
			name = "Gemini " + name

			// Create description and capability properties based on model type
			description := "Google Gemini model"
			supportsThinking := false
			contextWindowSize := 32768 // Default for Flash models

			// Determine model capabilities based on name patterns
			idLower := strings.ToLower(id)

			// Check for Pro models first (they have higher capabilities)
			if strings.Contains(idLower, "pro") {
				description = "Pro model with strong reasoning capabilities and long context support"

				// Only mark specific models as supporting thinking based on actual API behavior
				// Testing shows inconsistent thinking support across Pro models
				if strings.Contains(idLower, "2.5-pro") && (strings.Contains(idLower, "preview") || strings.Contains(idLower, "exp")) {
					// Only 2.5 preview/experimental models confirmed to work with thinking
					supportsThinking = true
					logger.Debug("Marking model %s as supporting thinking mode", id)
				} else {
					// Other Pro models might claim to support thinking but have API issues
					supportsThinking = false
					logger.Debug("Pro model %s may have thinking capabilities but API errors occur", id)
				}

				contextWindowSize = 1048576 // 1M tokens for Pro models
			} else if strings.Contains(idLower, "flash") {
				description = "Flash model optimized for efficiency and speed"
				// Flash models use the default values set above
			}

			// Special handling for preview/experimental models
			if strings.Contains(idLower, "preview") || strings.Contains(idLower, "exp") {
				// Add preview designation to description
				if strings.Contains(description, "Pro") {
					description = "Preview/Experimental " + description
				} else if strings.Contains(description, "Flash") {
					description = "Preview/Experimental " + description
				} else {
					description = "Preview/Experimental Gemini model"
				}
			}

			// Add model to list
			fetchedModels = append(fetchedModels, GeminiModelInfo{
				ID:                id,
				Name:              name,
				Description:       description,
				SupportsCaching:   supportsCaching,
				SupportsThinking:  supportsThinking,
				ContextWindowSize: contextWindowSize,
			})

			logger.Debug("Found Gemini model: %s", id)
		}
	}

	// If we got models, update the store
	if len(fetchedModels) > 0 {
		logger.Info("Successfully fetched %d Gemini models from %d total models", len(fetchedModels), modelCount)

		// Update model store with write lock
		modelStore.Lock()
		modelStore.models = fetchedModels
		modelStore.Unlock()

		// Log the found models for easier debugging
		for i, model := range fetchedModels {
			logger.Debug("Fetched model %d: %s (%s)", i+1, model.ID, model.Name)
		}

		return nil
	}

	logger.Warn("No Gemini models found via API (from %d total models), using fallback models", modelCount)
	logger.Debug("API may have returned models in an unexpected format or access restrictions may be in place")
	return fmt.Errorf("no Gemini models found via API")
}

// GetAvailableGeminiModels returns a list of available Gemini models
func GetAvailableGeminiModels() []GeminiModelInfo {
	// Get models with read lock
	modelStore.RLock()
	defer modelStore.RUnlock()

	// Return cached model list if available
	if len(modelStore.models) > 0 {
		return modelStore.models
	}

	// Return fallback models if nothing has been fetched yet
	return fallbackGeminiModels()
}
