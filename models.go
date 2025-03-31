package main

import "fmt"

// GeminiModelInfo holds information about a Gemini model
type GeminiModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ValidateModelID checks if a model ID is in the list of available models
// Returns nil if valid, error otherwise
func ValidateModelID(modelID string) error {
	models := GetAvailableGeminiModels()
	for _, model := range models {
		if model.ID == modelID {
			return nil
		}
	}
	
	// Model not found, return error with available models
	errMsg := fmt.Sprintf("Invalid model ID: %s. Available models are:", modelID)
	for _, model := range models {
		errMsg += fmt.Sprintf("\n- %s: %s", model.ID, model.Name)
	}
	
	return fmt.Errorf(errMsg)
}

// GetAvailableGeminiModels returns a list of available Gemini models
func GetAvailableGeminiModels() []GeminiModelInfo {
	return []GeminiModelInfo{
		{
			ID:          "gemini-2.0-flash-lite",
			Name:        "Gemini 2.0 Flash Lite",
			Description: "Optimized for speed, scale, and cost efficiency",
		},
		{
			ID:          "gemini-2.5-pro-exp-03-25",
			Name:        "Gemini 2.5 Pro",
			Description: "State-of-the-art thinking model, capable of reasoning over complex problems in code, math, and STEM, as well as analyzing large datasets, codebases, and documents using long context",
		},
		{
			ID:          "gemini-2.0-flash-001",
			Name:        "Gemini 2.0 Flash",
			Description: "Version of Gemini 2.0 Flash that supports text-only output",
		},
		{
			ID:          "gemini-1.5-pro",
			Name:        "Gemini 1.5 Pro",
			Description: "Previous generation pro model with strong reasoning capabilities and long context support",
		},
		{
			ID:          "gemini-1.5-flash",
			Name:        "Gemini 1.5 Flash",
			Description: "Previous generation flash model optimized for efficiency and speed",
		},
	}
}
