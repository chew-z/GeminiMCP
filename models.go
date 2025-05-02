package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/genai"
)

// GeminiModelInfo struct definition moved to structs.go

// GetModelByID returns a specific model by ID, or nil if not found
// It handles both base model IDs and version IDs
func GetModelByID(modelID string) *GeminiModelInfo {
	models := GetAvailableGeminiModels()
	
	// First check exact match with base model ID
	for _, model := range models {
		if model.ID == modelID {
			return &model
		}
		
		// Check if it matches any version ID
		for _, version := range model.Versions {
			if version.ID == modelID {
				// Create a copy of the model
				modelCopy := model
				// Override ID with version ID
				modelCopy.ID = version.ID
				// Override SupportsCaching from the version's value
				modelCopy.SupportsCaching = version.SupportsCaching
				return &modelCopy
			}
		}
	}
	return nil
}

// GetPreferredModelForTask returns the first model marked as preferred for a specific task
// taskType can be "thinking", "caching", or "search"
// If no preferred model is found, returns nil
// For caching tasks, it returns a model with appropriate version that supports caching
func GetPreferredModelForTask(taskType string) *GeminiModelInfo {
	models := GetAvailableGeminiModels()
	for _, model := range models {
		switch taskType {
		case "thinking":
			if model.PreferredForThinking {
				return &model
			}
		case "caching":
			if model.PreferredForCaching {
				// For caching tasks, we need a version that supports caching
				for _, version := range model.Versions {
					if version.SupportsCaching {
						// Return a model copy with the version ID
						modelCopy := model
						modelCopy.ID = version.ID
						modelCopy.SupportsCaching = true
						return &modelCopy
					}
				}
				// If we reach here, this model is preferred for caching but has no caching versions
				return &model
			}
		case "search":
			if model.PreferredForSearch {
				// For search tasks, prefer the first version if available (typically the newest)
				if len(model.Versions) > 0 {
					modelCopy := model
					modelCopy.ID = model.Versions[0].ID
					return &modelCopy
				}
				return &model
			}
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

		// Gemini 2.5 Pro Models (Preview/Experimental)
		{
			ID:                   "gemini-2.5-pro",
			Name:                 "Gemini 2.5 Pro",
			Description:          "Preview/Experimental Pro model with advanced reasoning capabilities",
			SupportsCaching:      false, // Base model doesn't support caching, versions do
			SupportsThinking:     true, // Confirmed to work with thinking mode
			ContextWindowSize:    1048576,
			PreferredForThinking: true,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.5-pro-exp-03-25",
					Name:            "Gemini 2.5 Pro Exp 03 25",
					SupportsCaching: true,
				},
				{
					ID:              "gemini-2.5-pro-preview-03-25",
					Name:            "Gemini 2.5 Pro Preview 03 25",
					SupportsCaching: true,
				},
			},
		},

		// Gemini 2.5 Flash Model
		{
			ID:                   "gemini-2.5-flash",
			Name:                 "Gemini 2.5 Flash",
			Description:          "Preview/Experimental Flash model optimized for efficiency and speed",
			SupportsCaching:      false, // Base model doesn't support caching, versions do
			SupportsThinking:     false,
			ContextWindowSize:    32768,
			PreferredForThinking: false,
			PreferredForCaching:  false,
			PreferredForSearch:   true,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.5-flash-preview-04-17",
					Name:            "Gemini 2.5 Flash Preview 04 17",
					SupportsCaching: true,
				},
			},
		},

		// Gemini 2.0 Flash Models
		{
			ID:                   "gemini-2.0-flash",
			Name:                 "Gemini 2.0 Flash",
			Description:          "Flash model optimized for efficiency and speed",
			SupportsCaching:      false, // Base model doesn't directly support caching
			SupportsThinking:     false,
			ContextWindowSize:    32768,
			PreferredForThinking: false,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.0-flash-001",
					Name:            "Gemini 2.0 Flash 001",
					SupportsCaching: true,
				},
				{
					ID:              "gemini-2.0-flash-exp",
					Name:            "Gemini 2.0 Flash Exp",
					SupportsCaching: false,
				},
			},
		},

		// Gemini 2.0 Flash Lite Model
		{
			ID:                   "gemini-2.0-flash-lite",
			Name:                 "Gemini 2.0 Flash Lite",
			Description:          "Flash lite model optimized for efficiency and speed",
			SupportsCaching:      false, // Base model doesn't directly support caching
			SupportsThinking:     false,
			ContextWindowSize:    32768,
			PreferredForThinking: false,
			PreferredForCaching:  false,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.0-flash-lite-001",
					Name:            "Gemini 2.0 Flash Lite 001",
					SupportsCaching: true,
				},
			},
		},

		// Gemini 2.0 Pro Models
		{
			ID:                   "gemini-2.0-pro",
			Name:                 "Gemini 2.0 Pro",
			Description:          "Pro model with advanced reasoning capabilities",
			SupportsCaching:      false, // Base model doesn't support caching
			SupportsThinking:     true,
			ContextWindowSize:    1048576,
			PreferredForThinking: false,
			PreferredForCaching:  false,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.0-pro-exp",
					Name:            "Gemini 2.0 Pro Exp",
					SupportsCaching: false,
				},
			},
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

	// Get predefined models to preserve descriptions and preferences
	predefinedModels := fallbackGeminiModels()
	// Create a map of predefined model IDs to their indices
	predefinedModelMap := make(map[string]int)
	for i, model := range predefinedModels {
		predefinedModelMap[model.ID] = i
	}

	// Create a slice to store fetched models
	var mergedModels []GeminiModelInfo

	// Track total models found for better diagnostics
	modelCount := 0

	// Map to store base models and their versions
	baseModels := make(map[string]*GeminiModelInfo)

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

			// Skip embedding models and visual models
			idLower := strings.ToLower(id)
			if strings.Contains(idLower, "embedding") ||
				strings.Contains(idLower, "vision") ||
				strings.Contains(idLower, "visual") ||
				strings.Contains(idLower, "image") {
				logger.Debug("Skipping embedding/visual model: %s", id)
				continue
			}

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

			// Determine if this is a base model or a version
			isVersionModel := strings.HasSuffix(id, "-001") || 
				strings.Contains(id, "preview") || 
				strings.Contains(id, "exp") ||
				strings.Contains(id, "stable")

			// Extract base model ID
			baseModelID := id
			if isVersionModel {
				// Remove version-specific suffixes to get the base model ID
				baseModelID = strings.TrimSuffix(baseModelID, "-001")
				
				// Handle special cases for preview/experimental models
				if strings.Contains(baseModelID, "-preview-") {
					parts := strings.Split(baseModelID, "-preview-")
					if len(parts) > 0 {
						baseModelID = parts[0]
					}
				}
				if strings.Contains(baseModelID, "-exp-") {
					parts := strings.Split(baseModelID, "-exp-")
					if len(parts) > 0 {
						baseModelID = parts[0]
					}
				}
			}

			// Create description and capability properties based on model type
			description := "Google Gemini model"
			supportsThinking := false
			contextWindowSize := 32768 // Default for Flash models

			// Determine model capabilities based on name patterns
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

			// For version models, create a ModelVersion
			if isVersionModel {
				// Create model version
				modelVersion := ModelVersion{
					ID:              id,
					Name:            name,
					SupportsCaching: supportsCaching,
				}

				// Add or update the base model
				baseModel, exists := baseModels[baseModelID]
				if !exists {
					// Create base model
					baseName := "Gemini " + strings.TrimPrefix(baseModelID, "gemini-")
					baseName = strings.ReplaceAll(baseName, "-", " ")
					baseName = strings.Title(baseName)

					baseModel = &GeminiModelInfo{
						ID:                   baseModelID,
						Name:                 baseName,
						Description:          description,
						SupportsCaching:      false, // Base model doesn't directly support caching
						SupportsThinking:     supportsThinking,
						ContextWindowSize:    contextWindowSize,
						PreferredForThinking: false,
						PreferredForCaching:  false,
						PreferredForSearch:   false,
						Versions:             []ModelVersion{modelVersion},
					}
					baseModels[baseModelID] = baseModel
				} else {
					// Add version to existing base model
					baseModel.Versions = append(baseModel.Versions, modelVersion)
				}
			} else {
				// This is a base model without a version specified
				// Check if we already have it
				baseModel, exists := baseModels[id]
				if !exists {
					// Create base model
					baseModel = &GeminiModelInfo{
						ID:                   id,
						Name:                 name,
						Description:          description,
						SupportsCaching:      supportsCaching,
						SupportsThinking:     supportsThinking,
						ContextWindowSize:    contextWindowSize,
						PreferredForThinking: false,
						PreferredForCaching:  false,
						PreferredForSearch:   false,
						Versions:             []ModelVersion{},
					}
					baseModels[id] = baseModel
				}
			}
		}  
	}

	// Convert the map of base models to a slice
	for _, baseModel := range baseModels {
		// Check if this is a predefined model
		if idx, exists := predefinedModelMap[baseModel.ID]; exists {
			// Keep the predefined description
			logger.Debug("Using predefined description for model: %s", baseModel.ID)
			baseModel.Description = predefinedModels[idx].Description

			// Keep predefined capabilities if they differ
			if predefinedModels[idx].SupportsCaching != baseModel.SupportsCaching {
				baseModel.SupportsCaching = predefinedModels[idx].SupportsCaching
				logger.Debug("Using predefined caching capability for model: %s", baseModel.ID)
			}
			if predefinedModels[idx].SupportsThinking != baseModel.SupportsThinking {
				baseModel.SupportsThinking = predefinedModels[idx].SupportsThinking
				logger.Debug("Using predefined thinking capability for model: %s", baseModel.ID)
			}
			if predefinedModels[idx].ContextWindowSize != baseModel.ContextWindowSize {
				baseModel.ContextWindowSize = predefinedModels[idx].ContextWindowSize
				logger.Debug("Using predefined context window size for model: %s", baseModel.ID)
			}

			// Keep predefined preferences
			baseModel.PreferredForThinking = predefinedModels[idx].PreferredForThinking
			baseModel.PreferredForCaching = predefinedModels[idx].PreferredForCaching
			baseModel.PreferredForSearch = predefinedModels[idx].PreferredForSearch
			logger.Debug("Using predefined task preferences for model: %s", baseModel.ID)

			// Merge versions from predefined model if applicable
			if len(predefinedModels[idx].Versions) > 0 {
				// Create a map of existing versions
				existingVersions := make(map[string]bool)
				for _, v := range baseModel.Versions {
					existingVersions[v.ID] = true
				}

				// Add missing versions from predefined model
				for _, v := range predefinedModels[idx].Versions {
					if !existingVersions[v.ID] {
						baseModel.Versions = append(baseModel.Versions, v)
						logger.Debug("Added predefined version %s to model %s", v.ID, baseModel.ID)
					}
				}
			}
		} else {
			// For non-predefined models, set preferences based on model characteristics
			if baseModel.ID == "gemini-2.5-pro" {
				baseModel.PreferredForThinking = true
				logger.Debug("Marking model %s as preferred for thinking tasks", baseModel.ID)
			} else if baseModel.ID == "gemini-2.0-flash" {
				baseModel.PreferredForCaching = true
				logger.Debug("Marking model %s as preferred for caching", baseModel.ID)
			} else if baseModel.ID == "gemini-2.5-flash" {
				// Mark newer flash models as preferred for search
				baseModel.PreferredForSearch = true
				logger.Debug("Marking model %s as preferred for search", baseModel.ID)
			}
		}

		// Add model to merged models list
		mergedModels = append(mergedModels, *baseModel)
		logger.Debug("Added model to merged list: %s", baseModel.ID)
	}

	// If we got models, update the store
	if len(mergedModels) > 0 {
		logger.Info("Successfully fetched and merged %d Gemini models from %d total models", len(mergedModels), modelCount)

		// Update model store with write lock
		modelStore.Lock()
		modelStore.models = mergedModels
		modelStore.Unlock()

		// Log the found models for easier debugging
		for i, model := range mergedModels {
			logger.Debug("Merged model %d: %s (%s)", i+1, model.ID, model.Name)
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
