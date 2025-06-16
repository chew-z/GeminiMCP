package main

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

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
	// Create a map of predefined model family IDs to their indices
	predefinedModelMap := make(map[string]int)
	for i, model := range predefinedModels {
		predefinedModelMap[model.FamilyID] = i
	}

	// Map to store models by family ID
	modelFamilies := make(map[string]*GeminiModelInfo)

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
				// Use helper function to extract base model ID
				baseModelID = extractBaseModelID(id)
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

			// Check if this is a preferred version based on specific model patterns
			isPreferred := false

			// Define preferred versions for major model families
			preferredVersions := map[string]bool{
				"gemini-2.5-pro-preview-06-05":        true,
				"gemini-2.5-flash-preview-05-20":      true,
				"gemini-2.0-flash-001":                true,
				"gemini-2.0-flash-lite-001":           true,
				"gemini-2.0-pro-exp-02-05":            true,
				"gemini-2.0-flash-thinking-exp-01-21": true,
				"gemini-2.0-flash-thinking-exp-1219":  true,
				"gemini-2.0-flash-thinking-exp":       true,
				"gemini-2.0-flash-live-001":           true,
				"gemini-2.0-flash-exp":                true,
				"gemini-2.0-flash-lite-preview":       true,
				"gemini-1.5-flash-8b-001":             true,
				"gemini-exp-1206":                     true,
			}

			// Check if this is a specifically preferred version
			if preferredVersions[id] {
				isPreferred = true
				logger.Debug("Marking model %s as preferred based on predefined list", id)
			} else if strings.Contains(id, "exp") && !strings.Contains(idLower, "preview") {
				// Fallback: mark non-specialized experimental models as preferred if not preview
				isSpecialized := strings.Contains(idLower, "audio") ||
					strings.Contains(idLower, "dialog") ||
					strings.Contains(idLower, "tts") ||
					strings.Contains(idLower, "vision") ||
					strings.Contains(idLower, "visual") ||
					strings.Contains(idLower, "image")

				if !isSpecialized {
					isPreferred = true
					logger.Debug("Marking experimental model %s as preferred (fallback)", id)
				} else {
					logger.Debug("Skipping specialized model %s from being marked as preferred", id)
				}
			}

			// For version models, create a ModelVersion
			if isVersionModel {
				// Create model version
				modelVersion := ModelVersion{
					ID:              id,
					Name:            name,
					SupportsCaching: supportsCaching,
					IsPreferred:     isPreferred,
				}

				// Get or create the family model
				familyModel, exists := modelFamilies[baseModelID]
				if !exists {
					// Create base model name
					baseName := "Gemini " + strings.TrimPrefix(baseModelID, "gemini-")
					baseName = strings.ReplaceAll(baseName, "-", " ")
					baseName = strings.Title(baseName)

					// Create new family model
					familyModel = &GeminiModelInfo{
						FamilyID:             baseModelID,
						Name:                 baseName,
						Description:          description,
						SupportsThinking:     supportsThinking,
						ContextWindowSize:    contextWindowSize,
						PreferredForThinking: false,
						PreferredForCaching:  false,
						PreferredForSearch:   false,
						Versions:             []ModelVersion{modelVersion},
					}
					modelFamilies[baseModelID] = familyModel
				} else {
					// Add version to existing family model
					familyModel.Versions = append(familyModel.Versions, modelVersion)
				}
			} else {
				// This is a base model without a version specified
				// Check if we already have this family
				if _, exists := modelFamilies[id]; !exists {
					// Create a new family model
					familyModel := &GeminiModelInfo{
						FamilyID:             id,
						Name:                 name,
						Description:          description,
						SupportsThinking:     supportsThinking,
						ContextWindowSize:    contextWindowSize,
						PreferredForThinking: false,
						PreferredForCaching:  false,
						PreferredForSearch:   false,
						Versions:             []ModelVersion{},
					}
					modelFamilies[id] = familyModel
				}
			}
		}
	}

	// Convert the map of model families to a slice
	var mergedModels []GeminiModelInfo

	// Apply preferences from predefined models
	for familyID, familyModel := range modelFamilies {
		// Check if this is a predefined model family
		if idx, exists := predefinedModelMap[familyID]; exists {
			predefinedModel := predefinedModels[idx]

			// Keep the predefined description
			logger.Debug("Using predefined description for model family: %s", familyID)
			familyModel.Description = predefinedModel.Description

			// Keep predefined capabilities if they differ
			if predefinedModel.SupportsThinking != familyModel.SupportsThinking {
				familyModel.SupportsThinking = predefinedModel.SupportsThinking
				logger.Debug("Using predefined thinking capability for model family: %s", familyID)
			}
			if predefinedModel.ContextWindowSize != familyModel.ContextWindowSize {
				familyModel.ContextWindowSize = predefinedModel.ContextWindowSize
				logger.Debug("Using predefined context window size for model family: %s", familyID)
			}

			// Keep predefined preferences
			familyModel.PreferredForThinking = predefinedModel.PreferredForThinking
			familyModel.PreferredForCaching = predefinedModel.PreferredForCaching
			familyModel.PreferredForSearch = predefinedModel.PreferredForSearch
			logger.Debug("Using predefined task preferences for model family: %s", familyID)

			// Merge versions from predefined model if applicable
			if len(predefinedModel.Versions) > 0 {
				// Create a map of existing versions and predefined preferences
				existingVersions := make(map[string]*ModelVersion)
				predefinedPreferences := make(map[string]bool)

				for i := range familyModel.Versions {
					existingVersions[familyModel.Versions[i].ID] = &familyModel.Versions[i]
				}

				for _, v := range predefinedModel.Versions {
					predefinedPreferences[v.ID] = v.IsPreferred
				}

				// First, update preferences for existing versions based on predefined model
				for id, pref := range predefinedPreferences {
					if existingVersion, exists := existingVersions[id]; exists {
						if existingVersion.IsPreferred != pref {
							logger.Debug("Updating preferred status for version %s in family %s from %v to %v",
								id, familyID, existingVersion.IsPreferred, pref)
							existingVersion.IsPreferred = pref
						}
					}
				}

				// Add missing versions from predefined model
				for _, v := range predefinedModel.Versions {
					if _, exists := existingVersions[v.ID]; !exists {
						familyModel.Versions = append(familyModel.Versions, v)
						logger.Debug("Added predefined version %s to model family %s", v.ID, familyID)
					}
				}
			}
		} else {
			// For non-predefined model families, set preferences based on characteristics
			if strings.Contains(strings.ToLower(familyID), "2.5-pro") {
				familyModel.PreferredForThinking = true
				logger.Debug("Marking model family %s as preferred for thinking tasks", familyID)
			} else if strings.Contains(strings.ToLower(familyID), "2.0-flash") {
				familyModel.PreferredForCaching = true
				logger.Debug("Marking model family %s as preferred for caching", familyID)
			} else if strings.Contains(strings.ToLower(familyID), "2.5-flash") {
				familyModel.PreferredForSearch = true
				logger.Debug("Marking model family %s as preferred for search", familyID)
			}
		}

		// Ensure at least one version is marked as preferred if there are versions
		if len(familyModel.Versions) > 0 {
			// Check if any version is marked as preferred
			hasPreferred := false
			for _, v := range familyModel.Versions {
				if v.IsPreferred {
					hasPreferred = true
					break
				}
			}

			// If no version is preferred, mark the first one as preferred
			if !hasPreferred {
				familyModel.Versions[0].IsPreferred = true
				logger.Debug("Marking version %s as preferred for model family %s",
					familyModel.Versions[0].ID, familyID)
			}
		}

		// Add the family model to the result list
		mergedModels = append(mergedModels, *familyModel)
		logger.Debug("Added model family to merged list: %s", familyID)
	}

	// Add missing predefined models that weren't found in the API
	for _, predefinedModel := range predefinedModels {
		if _, exists := modelFamilies[predefinedModel.FamilyID]; !exists {
			// This predefined model wasn't found in the API
			logger.Debug("Adding predefined model family not found in API: %s", predefinedModel.FamilyID)
			mergedModels = append(mergedModels, predefinedModel)
		}
	}

	// If we got models, update the store
	if len(mergedModels) > 0 {
		logger.Info("Successfully fetched and merged %d Gemini model families", len(mergedModels))

		// Update model store with write lock
		modelStore.Lock()
		modelStore.models = mergedModels
		modelStore.Unlock()

		// Log the found models for easier debugging
		for i, model := range mergedModels {
			logger.Debug("Merged model family %d: %s (%s)", i+1, model.FamilyID, model.Name)
			for j, version := range model.Versions {
				logger.Debug("  Version %d.%d: %s (%s)", i+1, j+1, version.ID, version.Name)
			}
		}

		return nil
	}

	logger.Warn("No Gemini models found via API (from %d total models), using fallback models", modelCount)
	logger.Debug("API may have returned models in an unexpected format or access restrictions may be in place")

	// Use fallback models
	modelStore.Lock()
	modelStore.models = fallbackGeminiModels()
	modelStore.Unlock()

	return fmt.Errorf("no Gemini models found via API")
}
