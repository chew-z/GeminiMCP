package main

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// FetchGeminiModels queries the Gemini API for available models and populates the model store.
// Only models whose name contains "gemini" are included. Each API model becomes a single-version
// family in our catalog — the model's own ID is both the family ID and the version ID.
func FetchGeminiModels(ctx context.Context, client *genai.Client) error {
	logger := getLoggerFromContext(ctx)

	page, err := client.Models.List(ctx, &genai.ListModelsConfig{PageSize: 100})
	if err != nil {
		return err
	}

	var models []GeminiModelInfo
	for _, m := range page.Items {
		name := strings.TrimPrefix(m.Name, "models/")

		// Only include Gemini Pro, Flash, and Flash Lite text generation models.
		// Skip embedding, imagen, experimental, and other non-text models.
		if !isRelevantModel(name) {
			continue
		}
		if !supportsGenerateContent(m.SupportedActions) {
			continue
		}

		info := GeminiModelInfo{
			FamilyID:          name,
			Name:              m.DisplayName,
			Description:       m.Description,
			SupportsThinking:  m.Thinking,
			ContextWindowSize: int(m.InputTokenLimit),
			Versions: []ModelVersion{
				{ID: name, IsPreferred: true},
			},
		}

		models = append(models, info)
		logger.Debug("Discovered model: %s (%s) context=%d thinking=%t",
			name, m.DisplayName, m.InputTokenLimit, m.Thinking)
	}

	if len(models) == 0 {
		return fmt.Errorf("no supported Gemini models found via API")
	}

	SetModels(models)
	logger.Info("Loaded %d Gemini models from API", len(models))
	return nil
}

// isRelevantModel returns true for Gemini Pro, Flash, and Flash Lite models only.
func isRelevantModel(name string) bool {
	if !strings.Contains(name, "gemini") {
		return false
	}
	// Must be one of the three model tiers we support
	return strings.Contains(name, "pro") ||
		strings.Contains(name, "flash")
}

// supportsGenerateContent checks if a model supports text generation.
func supportsGenerateContent(actions []string) bool {
	for _, a := range actions {
		if a == "generateContent" {
			return true
		}
	}
	return false
}
