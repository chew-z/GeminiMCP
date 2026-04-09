package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/genai"
)

// modelTier represents one of the three supported model tiers.
type modelTier int

const (
	tierPro modelTier = iota
	tierFlash
	tierFlashLite
)

// tierName returns a human-readable label for a tier.
func tierName(t modelTier) string {
	switch t {
	case tierPro:
		return "pro"
	case tierFlash:
		return "flash"
	case tierFlashLite:
		return "flash-lite"
	default:
		return "unknown"
	}
}

// modelCandidate holds a potential winner for a tier.
type modelCandidate struct {
	model *genai.Model
	name  string // "models/" prefix stripped
}

// FetchGeminiModels queries the Gemini API for available models and populates
// the model store with exactly three models: the latest Pro, Flash, and Flash Lite.
// Uses Models.All() which handles pagination automatically.
func FetchGeminiModels(ctx context.Context, client *genai.Client) error {
	logger := getLoggerFromContext(ctx)

	best, err := selectBestModels(ctx, client, logger)
	if err != nil {
		return err
	}

	// Build the catalog from the winners — one per tier.
	var models []GeminiModelInfo
	for _, tier := range []modelTier{tierPro, tierFlash, tierFlashLite} {
		c := best[tier]
		if c == nil {
			logger.Warn("No model found for tier %s", tierName(tier))
			continue
		}

		models = append(models, toModelInfo(c))
		logger.Info("Selected %s model: %s (%s) context=%d thinking=%t",
			tierName(tier), c.name, c.model.DisplayName,
			c.model.InputTokenLimit, c.model.Thinking)
	}

	if len(models) == 0 {
		return fmt.Errorf("no supported Gemini models found via API")
	}

	SetModels(models)
	logger.Info("Loaded %d Gemini models from API", len(models))
	return nil
}

// selectBestModels iterates all API models and picks the latest per tier.
func selectBestModels(ctx context.Context, client *genai.Client, logger Logger) (map[modelTier]*modelCandidate, error) {
	best := map[modelTier]*modelCandidate{}

	for m, err := range client.Models.All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("error iterating models: %w", err)
		}

		name := strings.TrimPrefix(m.Name, "models/")

		if !supportsGenerateContent(m.SupportedActions) {
			continue
		}

		tier, ok := classifyModel(name)
		if !ok {
			continue
		}

		prev := best[tier]
		if prev == nil || isNewerModel(name, prev.name) {
			prevLabel := "<none>"
			if prev != nil {
				prevLabel = prev.name
			}
			best[tier] = &modelCandidate{model: m, name: name}
			logger.Debug("Model candidate for %s: %s (replaced %s)",
				tierName(tier), name, prevLabel)
		}
	}
	return best, nil
}

// toModelInfo converts a winning candidate to a GeminiModelInfo.
func toModelInfo(c *modelCandidate) GeminiModelInfo {
	return GeminiModelInfo{
		FamilyID:          c.name,
		Name:              c.model.DisplayName,
		Description:       c.model.Description,
		SupportsThinking:  c.model.Thinking,
		ContextWindowSize: int(c.model.InputTokenLimit),
		Versions: []ModelVersion{
			{ID: c.name, IsPreferred: true},
		},
	}
}

// classifyModel determines which tier a model belongs to, or returns false
// if the model should be excluded (e.g., TTS, image, customtools, non-gemini).
func classifyModel(name string) (modelTier, bool) {
	if !strings.Contains(name, "gemini") {
		return 0, false
	}

	// Exclude non-text-generation variants
	for _, suffix := range []string{"-tts", "-image", "-customtools"} {
		if strings.Contains(name, suffix) {
			return 0, false
		}
	}

	// Classification order matters: check "flash-lite" / "flash_lite" before "flash",
	// and "flash" before "pro" (to avoid matching "pro" in "flash-pro" if that ever exists).
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "flash-lite") || strings.Contains(lower, "flash_lite"):
		return tierFlashLite, true
	case strings.Contains(lower, "flash"):
		return tierFlash, true
	case strings.Contains(lower, "pro"):
		return tierPro, true
	default:
		return 0, false
	}
}

// isNewerModel returns true if candidate is a newer version than current.
// Strategy: higher version numbers win; preview/exp beats stable at equal version;
// simple lexicographic comparison on the normalized name works because Gemini
// naming follows "gemini-{major}.{minor}-{tier}[-preview|-exp-{date}]" consistently.
func isNewerModel(candidate, current string) bool {
	// "latest" aliases always lose to concrete versions — we want real model IDs.
	candidateIsLatest := strings.Contains(candidate, "-latest")
	currentIsLatest := strings.Contains(current, "-latest")
	if candidateIsLatest && !currentIsLatest {
		return false
	}
	if !candidateIsLatest && currentIsLatest {
		return true
	}

	// Both are concrete or both are "latest": compare lexicographically.
	// Gemini naming (gemini-3.1-pro-preview > gemini-3-pro-preview > gemini-2.5-pro)
	// sorts correctly because major.minor increments are reflected in the string.
	return candidate > current
}

// supportsGenerateContent checks if a model supports text generation.
func supportsGenerateContent(actions []string) bool {
	return slices.Contains(actions, "generateContent")
}
