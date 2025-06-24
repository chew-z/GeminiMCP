package main

import (
	"fmt"
	"strings"
	"sync"
)

// modelStore handles storing and retrieving models
var modelStore struct {
	sync.RWMutex
	models []GeminiModelInfo
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

// GetModelByID returns model info for either a family ID or a version ID
func GetModelByID(modelID string) *GeminiModelInfo {
	models := GetAvailableGeminiModels()

	// Check if it's a family ID
	for _, model := range models {
		if model.FamilyID == modelID {
			return &model
		}

		// Check if it's a version ID
		for _, version := range model.Versions {
			if version.ID == modelID {
				return &model // Return the family for this version
			}
		}
	}
	return nil
}

// GetModelVersion returns the specific version info for a model ID
func GetModelVersion(modelID string) *ModelVersion {
	for _, model := range GetAvailableGeminiModels() {
		for i, version := range model.Versions {
			if version.ID == modelID {
				return &model.Versions[i]
			}
		}
	}
	return nil
}

// ResolveModelID converts a model family ID or version ID to an actual API-usable version ID
// If the provided ID is already a version ID, it returns it unchanged
// If it's a family ID, it returns the ID of the preferred or first version
func ResolveModelID(modelID string) string {
	// First check if this is already a specific version ID
	if GetModelVersion(modelID) != nil {
		return modelID // Already a valid version ID
	}

	// It might be a family ID, try to find the best version
	model := GetModelByID(modelID)
	if model != nil {
		// Find preferred version first
		for _, version := range model.Versions {
			if version.IsPreferred {
				return version.ID
			}
		}

		// Otherwise return the first version
		if len(model.Versions) > 0 {
			return model.Versions[0].ID
		}
	}

	// If we get here, it's an unknown ID, return it unchanged
	return modelID
}

// ValidateModelID checks if a model ID is in the list of available models
// Returns nil if valid, error otherwise
func ValidateModelID(modelID string) error {
	// First check if it's a known version ID or family ID
	if GetModelVersion(modelID) != nil || GetModelByID(modelID) != nil {
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
		sb.WriteString(fmt.Sprintf("\n- %s: %s", model.FamilyID, model.Name))
		for _, version := range model.Versions {
			sb.WriteString(fmt.Sprintf("\n  - %s: %s", version.ID, version.Name))
		}
	}
	sb.WriteString("\n\nHowever, we will attempt to use this model anyway. It may be a new or preview model.")

	return fmt.Errorf("%s", sb.String())
}

// GetPreferredModelForTask returns the best version ID for a specific task
// taskType can be "thinking", "caching", or "search"
// If no preferred model is found, returns an empty string
func GetPreferredModelForTask(taskType string) string {
	models := GetAvailableGeminiModels()
	for _, model := range models {
		switch taskType {
		case "thinking":
			if model.PreferredForThinking {
				// Use the ResolveModelID function to get a specific version ID
				return ResolveModelID(model.FamilyID)
			}
		case "caching":
			if model.PreferredForCaching {
				// For caching tasks, we need a version that supports caching
				for _, version := range model.Versions {
					if version.SupportsCaching {
						return version.ID
					}
				}
				// If no version supports caching, use the default version
				return ResolveModelID(model.FamilyID)
			}
		case "search":
			if model.PreferredForSearch {
				// For search tasks, use the preferred version
				return ResolveModelID(model.FamilyID)
			}
		}
	}
	return ""
}
