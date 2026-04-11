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

// minGeminiGeneration is the lowest Gemini major version the server accepts.
// Gemini 2.x and earlier are rejected even if still advertised by the API.
const minGeminiGeneration = 3

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

// inferModelTier maps a client-provided model name to one of the three
// logical tiers (pro, flash, flash-lite) by substring match. The input is
// lowercased and trimmed, so case and whitespace don't matter. Clients
// express intent; the server honors it even for deprecated or non-canonical
// names. See CLAUDE.md principles #1 and #3.
//
// Rejects non-Gemini names and non-text-generation variants (TTS, image,
// native-audio, customtools) — those cannot be served by gemini_ask /
// gemini_search regardless of expressed tier.
//
// For catalog selection where the generation floor matters, use classifyModel.
func inferModelTier(name string) (modelTier, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))

	if !strings.Contains(lower, "gemini") {
		return 0, false
	}
	for _, suffix := range []string{"-tts", "-image", "-customtools", "-native-audio", "-image-generation"} {
		if strings.Contains(lower, suffix) {
			return 0, false
		}
	}

	hasFlash := strings.Contains(lower, "flash")
	hasLite := strings.Contains(lower, "lite")
	switch {
	case hasFlash && hasLite:
		return tierFlashLite, true
	case hasFlash:
		return tierFlash, true
	case strings.Contains(lower, "pro"):
		return tierPro, true
	}
	return 0, false
}

// classifyModel determines which tier a model belongs to for catalog selection,
// or returns false if the model should be excluded (non-text-generation
// variants, non-Gemini names, or versions below the generation floor).
// Used by the fetch pipeline to pick tier winners from the live API listing.
//
// For resolving client-provided model IDs to a tier (where a sub-floor name
// should be redirected forward rather than rejected), use inferModelTier.
func classifyModel(name string) (modelTier, bool) {
	tier, ok := inferModelTier(name)
	if !ok {
		return 0, false
	}

	// Enforce generation floor for catalog selection: we never want to pick a
	// Gemini 2.x or earlier model as a tier winner. Names whose version can't
	// be parsed (e.g. "gemini-flash-latest") pass through — the -latest
	// deprioritization in isNewerModel keeps them from beating concrete
	// Gemini-3 picks, while still letting them appear in the catalog if the
	// API surfaces them as the only option.
	if v, ok := parseModelVersion(name); ok && v.major < minGeminiGeneration {
		return 0, false
	}
	return tier, true
}

// modelVersion holds the parsed numeric version from a Gemini model name.
type modelVersion struct {
	major  int
	minor  int
	suffix string // everything after the tier identifier (e.g. "-preview", "-exp-03-25")
}

// parseModelVersion extracts major, minor, and suffix from a Gemini model name.
// Returns ok=false if the name doesn't match the expected pattern.
// Case-insensitive: the input is lowercased before matching so defensive
// callers passing uppercase names still hit the version extraction.
func parseModelVersion(name string) (modelVersion, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
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
	// Find the tier portion and take what follows it. Gemini model IDs use
	// hyphens exclusively (see inferModelTier), so we only match hyphenated
	// tier tokens. Name is already lowercased at function entry.
	suffix := ""
	for _, tier := range []string{"flash-lite", "flash", "pro"} {
		if _, after, found := strings.Cut(name, tier); found {
			suffix = after
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
