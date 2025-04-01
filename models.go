package main

import (
	"errors"
	"fmt"
	"strings"
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

// GetAvailableGeminiModels returns a list of available Gemini models
func GetAvailableGeminiModels() []GeminiModelInfo {
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
