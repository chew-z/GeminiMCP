package main

import (
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

// bestModelForTier returns the FamilyID of the best available model in the same
// tier as the given model name. Falls back to the first model in the catalog.
func bestModelForTier(modelName string) string {
	tier, ok := classifyModel(modelName)
	if ok {
		for _, m := range GetAvailableGeminiModels() {
			if t, match := classifyModel(m.FamilyID); match && t == tier {
				return m.FamilyID
			}
		}
	}
	// Fallback: first model in catalog (pro, the best)
	if models := GetAvailableGeminiModels(); len(models) > 0 {
		return models[0].FamilyID
	}
	return modelName // catalog empty, pass through
}

// addDynamicAlias registers a runtime alias so future requests for
// deprecatedID are silently redirected to replacementID via ResolveModelID.
func addDynamicAlias(deprecatedID, replacementID string) {
	modelAliasesMu.Lock()
	defer modelAliasesMu.Unlock()
	modelAliases[deprecatedID] = replacementID
}

// ValidateModelID checks if a model ID is in the list of available models.
// If unknown, it returns the best available replacement (quiet redirect).
// The returned string is the validated (or replaced) model ID.
// The boolean indicates whether a redirect occurred.
func ValidateModelID(modelID string) (string, bool) {
	if GetModelVersion(modelID) != nil || GetModelByID(modelID) != nil {
		return modelID, false
	}
	// Unknown model — redirect to best in same tier
	replacement := bestModelForTier(modelID)
	return replacement, true
}
