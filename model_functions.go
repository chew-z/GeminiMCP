package main

import (
	"fmt"
	"strings"
	"sync"
)

// modelStore holds the active model catalog, populated at startup from the API.
var modelStore struct {
	sync.RWMutex
	models []GeminiModelInfo
}

// modelAliases maps deprecated or old model IDs silently to their current replacements.
// Used by ResolveModelID so callers don't need to know about renamed preview models.
// Protected by modelAliasesMu for concurrent read/write access.
var (
	modelAliasesMu sync.RWMutex
	modelAliases   = map[string]string{
		"gemini-3-pro-preview":  "gemini-3.1-pro-preview",
		"gemini-pro-latest":     "gemini-3.1-pro-preview",
		"gemini-2.5-pro":        "gemini-3.1-pro-preview",
		"gemini-2.5-flash":      "gemini-3-flash-preview",
		"gemini-2.5-flash-lite": "gemini-3.1-flash-lite-preview",
	}
)

// SetModels replaces the model catalog (called at startup after API fetch).
func SetModels(models []GeminiModelInfo) {
	modelStore.Lock()
	modelStore.models = models
	modelStore.Unlock()
}

// GetAvailableGeminiModels returns the active model catalog.
func GetAvailableGeminiModels() []GeminiModelInfo {
	modelStore.RLock()
	defer modelStore.RUnlock()
	return modelStore.models
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
	// Silently redirect deprecated/renamed model IDs to their current equivalents
	modelAliasesMu.RLock()
	canonical, ok := modelAliases[modelID]
	modelAliasesMu.RUnlock()
	if ok {
		modelID = canonical
	}

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

// ValidateModelID checks if a model ID is in the list of available models.
// Returns nil if valid, error if the model is unknown and should be rejected.
func ValidateModelID(modelID string) error {
	if GetModelVersion(modelID) != nil || GetModelByID(modelID) != nil {
		return nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "unknown model: %s. Available models:", modelID)
	for _, model := range GetAvailableGeminiModels() {
		fmt.Fprintf(&sb, "\n- %s (%s)", model.FamilyID, model.Name)
	}

	return fmt.Errorf("%s", sb.String())
}

// AddDynamicAlias registers a runtime alias so that future calls using
// deprecatedID are silently redirected to replacementID via ResolveModelID.
func AddDynamicAlias(deprecatedID, replacementID string) {
	modelAliasesMu.Lock()
	defer modelAliasesMu.Unlock()
	modelAliases[deprecatedID] = replacementID
}
