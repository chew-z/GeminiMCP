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
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	SupportsCaching bool   `json:"supports_caching"` // Whether this model supports caching
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
	if GetModelByID(modelID) != nil {
		return nil
	}

	// Model not found, return error with available models
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Invalid model ID: %s. Available models are:", modelID))
	for _, model := range GetAvailableGeminiModels() {
		sb.WriteString(fmt.Sprintf("\n- %s: %s", model.ID, model.Name))
	}

	return errors.New(sb.String())
}

// fallbackGeminiModels provides a list of default Gemini models to use if API fetching fails
func fallbackGeminiModels() []GeminiModelInfo {
	return []GeminiModelInfo{
		{
			ID:              "gemini-2.0-flash-lite",
			Name:            "Gemini 2.0 Flash Lite",
			Description:     "Optimized for speed, scale, and cost efficiency",
			SupportsCaching: false,
		},
		{
			ID:              "gemini-2.5-pro-exp-03-25",
			Name:            "Gemini 2.5 Pro",
			Description:     "State-of-the-art thinking model, capable of reasoning over complex problems in code, math, and STEM, as well as analyzing large datasets, codebases, and documents using long context",
			SupportsCaching: false,
		},
		{
			ID:              "gemini-2.0-flash-001",
			Name:            "Gemini 2.0 Flash",
			Description:     "Version of Gemini 2.0 Flash that supports text-only output",
			SupportsCaching: true, // Has version suffix
		},
		{
			ID:              "gemini-1.5-pro",
			Name:            "Gemini 1.5 Pro",
			Description:     "Previous generation pro model with strong reasoning capabilities and long context support",
			SupportsCaching: false,
		},
		{
			ID:              "gemini-1.5-flash",
			Name:            "Gemini 1.5 Flash",
			Description:     "Previous generation flash model optimized for efficiency and speed",
			SupportsCaching: false,
		},
		{
			ID:              "gemini-1.5-pro-001",
			Name:            "Gemini 1.5 Pro (Stable)",
			Description:     "Stable version of Gemini 1.5 Pro with version suffix",
			SupportsCaching: true, // Has version suffix
		},
		{
			ID:              "gemini-1.5-flash-001",
			Name:            "Gemini 1.5 Flash (Stable)",
			Description:     "Stable version of Gemini 1.5 Flash with version suffix",
			SupportsCaching: true, // Has version suffix
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
	
	// Fetch models from API
	for model, err := range client.Models.All(ctx) {
		if err != nil {
			logger.Error("Error fetching models: %v", err)
			return fmt.Errorf("error fetching models: %w", err)
		}
		
		// Only include Gemini models
		if strings.HasPrefix(model.Name, "models/gemini") {
			// Extract ID from model name
			id := strings.TrimPrefix(model.Name, "models/")
			
			// Check if model has version suffix for caching support
			supportsCaching := strings.HasSuffix(id, "-001")
			
			// Create a more user-friendly name
			name := strings.TrimPrefix(id, "gemini-")
			name = strings.ReplaceAll(name, "-", " ")
			name = strings.Title(name)
			if supportsCaching {
				name += " (Stable)"
			}
			name = "Gemini " + name
			
			// Create description based on model capabilities
			description := "Google Gemini model"
			if strings.Contains(id, "pro") {
				description = "Pro model with strong reasoning capabilities and long context support"
			} else if strings.Contains(id, "flash") {
				description = "Flash model optimized for efficiency and speed"
			}
			
			// Add model to list
			fetchedModels = append(fetchedModels, GeminiModelInfo{
				ID:              id,
				Name:            name,
				Description:     description,
				SupportsCaching: supportsCaching,
			})
			
			logger.Debug("Found Gemini model: %s", id)
		}
	}
	
	// If we got models, update the store
	if len(fetchedModels) > 0 {
		logger.Info("Successfully fetched %d Gemini models", len(fetchedModels))
		
		// Update model store with write lock
		modelStore.Lock()
		modelStore.models = fetchedModels
		modelStore.Unlock()
		
		return nil
	}
	
	logger.Warn("No Gemini models found via API, using fallback models")
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
