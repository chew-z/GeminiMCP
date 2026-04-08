package main

import (
	"context"
)

// FetchGeminiModels loads the predefined Gemini 3.x model families into the model store.
func FetchGeminiModels(ctx context.Context, apiKey string) {
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

	logger.Info("Setting up Gemini 3.x model families...")

	if apiKey != "" {
		logger.Debug("API key provided (masked)")
	} else {
		logger.Debug("No API key provided")
	}

	models := fallbackGeminiModels()

	// Update model store with write lock
	modelStore.Lock()
	modelStore.models = models
	modelStore.Unlock()

	logger.Info("Successfully configured %d Gemini model families", len(models))

	// Log the configured models for easier debugging
	for i, model := range models {
		logger.Debug("Model family %d: %s (%s)", i+1, model.FamilyID, model.Name)
		for j, version := range model.Versions {
			logger.Debug("  Version %d.%d: %s (%s)", i+1, j+1, version.ID, version.Name)
		}
	}

}
