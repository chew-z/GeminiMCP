package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"google.golang.org/genai"
)

// versionRe matches "gemini-{major}[.{minor}]-..." and captures major/minor.
var versionRe = regexp.MustCompile(`gemini-(\d+)(?:\.(\d+))?-`)

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
		MaxOutputTokens:   int(c.model.OutputTokenLimit),
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

// modelVersion holds the parsed numeric version from a Gemini model name.
type modelVersion struct {
	major  int
	minor  int
	suffix string // everything after the tier identifier (e.g. "-preview", "-exp-03-25")
}

// parseModelVersion extracts major, minor, and suffix from a Gemini model name.
// Returns ok=false if the name doesn't match the expected pattern.
func parseModelVersion(name string) (modelVersion, bool) {
	m := versionRe.FindStringSubmatch(name)
	if m == nil {
		return modelVersion{}, false
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return modelVersion{}, false
	}
	minor := 0
	if m[2] != "" {
		minor, err = strconv.Atoi(m[2])
		if err != nil {
			return modelVersion{}, false
		}
	}

	// Extract suffix: everything after the tier keyword (pro/flash-lite/flash).
	// Find the tier portion and take what follows it.
	suffix := ""
	lower := strings.ToLower(name)
	for _, tier := range []string{"flash-lite", "flash_lite", "flash", "pro"} {
		idx := strings.Index(lower, tier)
		if idx >= 0 {
			suffix = name[idx+len(tier):]
			break
		}
	}

	return modelVersion{major: major, minor: minor, suffix: suffix}, true
}

// isNewerModel returns true if candidate is a newer version than current.
// Uses numeric comparison of major.minor versions, falling back to lexicographic
// for unparseable names.
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

	cv, cOK := parseModelVersion(candidate)
	rv, rOK := parseModelVersion(current)

	if cOK && rOK {
		if cv.major != rv.major {
			return cv.major > rv.major
		}
		if cv.minor != rv.minor {
			return cv.minor > rv.minor
		}
		// Same major.minor — compare suffix lexicographically
		return cv.suffix > rv.suffix
	}

	// Fallback: lexicographic for unparseable names
	return candidate > current
}

// supportsGenerateContent checks if a model supports text generation.
func supportsGenerateContent(actions []string) bool {
	return slices.Contains(actions, "generateContent")
}
