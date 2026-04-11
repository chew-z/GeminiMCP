package main

import (
	"fmt"
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
	modelAliases   = map[string]string{}
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
// tier as the given model name. Returns "" if the model name is not a
// recognizable Gemini model — callers should treat this as a rejection signal.
//
// Uses inferModelTier (not classifyModel) for the input so that deprecated
// sub-floor names like "gemini-2.5-flash" still resolve: the client's intent
// is "flash tier", and the server honors that by returning the current flash
// winner rather than bouncing the call with an error. See CLAUDE.md first
// principle #1 — the server decides the concrete model, the client expresses
// intent.
func bestModelForTier(modelName string) string {
	tier, ok := inferModelTier(modelName)
	if !ok {
		return "" // unclassifiable (non-Gemini or non-text-generation) — reject
	}
	for _, m := range GetAvailableGeminiModels() {
		if t, match := classifyModel(m.FamilyID); match && t == tier {
			return m.FamilyID
		}
	}
	return "" // catalog empty for this tier
}

// addDynamicAlias registers a runtime alias so future requests for
// deprecatedID are silently redirected to replacementID via ResolveModelID.
func addDynamicAlias(deprecatedID, replacementID string) {
	modelAliasesMu.Lock()
	defer modelAliasesMu.Unlock()
	modelAliases[deprecatedID] = replacementID
}

// ValidateModelID checks if a model ID is known. Returns:
//   - (modelID, false, nil)        — exact catalog match
//   - (redirectedID, true, nil)    — unknown Gemini model, redirected to same tier
//   - ("", false, error)           — non-Gemini model, rejected
func ValidateModelID(modelID string) (string, bool, error) {
	if GetModelVersion(modelID) != nil || GetModelByID(modelID) != nil {
		return modelID, false, nil
	}
	replacement := bestModelForTier(modelID)
	if replacement == "" {
		return "", false, fmt.Errorf("unknown model %q: not a recognized Gemini model", modelID)
	}
	return replacement, true, nil
}
