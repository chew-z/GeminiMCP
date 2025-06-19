package main

import (
	"context"
)

// InitializeModelStore simply uses the predefined fallback models since we only support
// the 3 specific Gemini 2.5 models: Pro, Flash, and Flash Lite
func InitializeModelStore(ctx context.Context) error {
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

	logger.Info("Initializing Gemini 2.5 model families from fallback data...")

	// Use the 3 predefined Gemini 2.5 models
	models := fallbackGeminiModels()

	// Update model store with write lock
	modelStore.Lock()
	modelStore.models = models
	modelStore.Unlock()

	logger.Info("Successfully initialized %d Gemini 2.5 model families in model store", len(models))

	// Log the configured models for easier debugging
	for i, model := range models {
		logger.Debug("Model family %d: %s (%s)", i+1, model.FamilyID, model.Name)
		for j, version := range model.Versions {
			logger.Debug("  Version %d.%d: %s (%s)", i+1, j+1, version.ID, version.Name)
		}
	}

	return nil
}
