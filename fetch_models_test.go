package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

func TestClassifyModel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTier modelTier
		wantOK   bool
	}{
		{"pro model", "gemini-3.1-pro-preview", tierPro, true},
		{"flash model", "gemini-3-flash-preview", tierFlash, true},
		{"flash-lite model", "gemini-3.1-flash-lite-preview", tierFlashLite, true},
		{"flash_lite model", "gemini-3_flash_lite", tierFlashLite, true},
		{"flash-latest alias", "gemini-flash-latest", tierFlash, true},
		{"flash-lite-latest alias", "gemini-flash-lite-latest", tierFlashLite, true},
		{"non-gemini model", "palm-2-pro", 0, false},
		{"tts variant", "gemini-3-pro-tts", 0, false},
		{"image variant", "gemini-3-flash-image", 0, false},
		{"customtools variant", "gemini-3-pro-customtools", 0, false},
		{"below generation floor - 2.5 flash", "gemini-2.5-flash", 0, false},
		{"below generation floor - 2.5 pro", "gemini-2.5-pro", 0, false},
		{"below generation floor - 2.5 flash preview", "gemini-2.5-flash-preview-09-2025", 0, false},
		{"native-audio variant", "gemini-2.5-flash-native-audio-preview-12-2025", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tier, ok := classifyModel(tc.input)
			assert.Equal(t, tc.wantOK, ok)
			if ok {
				assert.Equal(t, tc.wantTier, tier)
			}
		})
	}
}

func TestInferModelTier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTier modelTier
		wantOK   bool
	}{
		// Current Gemini 3+ models resolve normally.
		{"3.1 pro", "gemini-3.1-pro-preview", tierPro, true},
		{"3 flash", "gemini-3-flash-preview", tierFlash, true},
		{"3.1 flash-lite", "gemini-3.1-flash-lite-preview", tierFlashLite, true},
		// Tier aliases without an explicit version also resolve (no floor to enforce).
		{"pro-latest alias", "gemini-pro-latest", tierPro, true},
		{"flash-latest alias", "gemini-flash-latest", tierFlash, true},
		{"flash-lite-latest alias", "gemini-flash-lite-latest", tierFlashLite, true},
		// Sub-floor names MUST still yield a tier — this is what lets
		// bestModelForTier redirect client input forward instead of rejecting it.
		{"2.5 flash resolves to flash tier", "gemini-2.5-flash", tierFlash, true},
		{"2.5 pro resolves to pro tier", "gemini-2.5-pro", tierPro, true},
		{"2.5 flash-lite resolves to flash-lite tier", "gemini-2.5-flash-lite", tierFlashLite, true},
		{"1.5 pro resolves to pro tier", "gemini-1.5-pro", tierPro, true},
		{"2.5 flash dated preview", "gemini-2.5-flash-preview-09-2025", tierFlash, true},
		// Non-text-generation variants are still rejected (we can't serve them).
		{"tts variant rejected", "gemini-2.5-flash-tts", 0, false},
		{"native-audio variant rejected", "gemini-2.5-flash-native-audio-preview-12-2025", 0, false},
		{"image variant rejected", "gemini-3-flash-image", 0, false},
		// Non-Gemini names are still rejected outright.
		{"non-gemini rejected", "palm-2-pro", 0, false},
		{"gpt rejected", "gpt-4.1", 0, false},
		{"claude rejected", "claude-3.7-sonnet", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tier, ok := inferModelTier(tc.input)
			assert.Equal(t, tc.wantOK, ok)
			if ok {
				assert.Equal(t, tc.wantTier, tier)
			}
		})
	}
}

func TestIsNewerModel(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		current   string
		want      bool
	}{
		{"3.1 > 3", "gemini-3.1-pro-preview", "gemini-3-pro-preview", true},
		{"3 < 3.1", "gemini-3-pro-preview", "gemini-3.1-pro-preview", false},
		{"3.10 > 3.2 (numeric)", "gemini-3.10-pro-preview", "gemini-3.2-pro-preview", true},
		{"3.2 < 3.10 (numeric)", "gemini-3.2-pro-preview", "gemini-3.10-pro-preview", false},
		{"latest loses to concrete", "gemini-3-pro-latest", "gemini-3-pro-preview", false},
		{"concrete beats latest", "gemini-3-pro-preview", "gemini-3-pro-latest", true},
		{"equal names", "gemini-3-pro-preview", "gemini-3-pro-preview", false},
		{"higher major wins", "gemini-4-pro-preview", "gemini-3.9-pro-preview", true},
		{"suffix comparison at equal version", "gemini-3-pro-preview", "gemini-3-pro-exp", true},
		{"preview beats stable at equal version", "gemini-3-flash-preview", "gemini-3-flash", true},
		{"flash-latest loses to concrete 3.x preview", "gemini-flash-latest", "gemini-3-flash-preview", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isNewerModel(tc.candidate, tc.current))
		})
	}
}

func TestParseModelVersion(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantMajor  int
		wantMinor  int
		wantSuffix string
		wantOK     bool
	}{
		{"major only", "gemini-3-pro-preview", 3, 0, "-preview", true},
		{"major.minor", "gemini-3.1-pro-preview", 3, 1, "-preview", true},
		{"double digit minor", "gemini-3.10-flash-preview", 3, 10, "-preview", true},
		{"flash-lite", "gemini-3.1-flash-lite-preview", 3, 1, "-preview", true},
		{"2.5 flash dated preview", "gemini-2.5-flash-preview-09-2025", 2, 5, "-preview-09-2025", true},
		{"latest alias has no version", "gemini-flash-latest", 0, 0, "", false},
		{"no match", "palm-2-pro", 0, 0, "", false},
		{"no match - no dash after version", "gemini-3pro", 0, 0, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, ok := parseModelVersion(tc.input)
			assert.Equal(t, tc.wantOK, ok)
			if ok {
				assert.Equal(t, tc.wantMajor, v.major)
				assert.Equal(t, tc.wantMinor, v.minor)
				assert.Equal(t, tc.wantSuffix, v.suffix)
			}
		})
	}
}

func TestTierName(t *testing.T) {
	assert.Equal(t, "pro", tierName(tierPro))
	assert.Equal(t, "flash", tierName(tierFlash))
	assert.Equal(t, "flash-lite", tierName(tierFlashLite))
	assert.Equal(t, "unknown", tierName(modelTier(99)))
}

func TestSupportsGenerateContent(t *testing.T) {
	assert.True(t, supportsGenerateContent([]string{"generateContent", "countTokens"}))
	assert.False(t, supportsGenerateContent([]string{"embedContent"}))
	assert.False(t, supportsGenerateContent(nil))
}

func TestToModelInfo(t *testing.T) {
	c := &modelCandidate{
		name: "gemini-3.1-pro-preview",
		model: &genai.Model{
			DisplayName:      "Gemini 3.1 Pro",
			Description:      "Pro model",
			Thinking:         true,
			InputTokenLimit:  1048576,
			OutputTokenLimit: 8192,
		},
	}

	info := toModelInfo(c)
	assert.Equal(t, "gemini-3.1-pro-preview", info.FamilyID)
	assert.Equal(t, "Gemini 3.1 Pro", info.Name)
	assert.Equal(t, "Pro model", info.Description)
	assert.True(t, info.SupportsThinking)
	assert.Equal(t, 1048576, info.ContextWindowSize)
	assert.Equal(t, 8192, info.MaxOutputTokens)
	if assert.Len(t, info.Versions, 1) {
		assert.Equal(t, "gemini-3.1-pro-preview", info.Versions[0].ID)
		assert.True(t, info.Versions[0].IsPreferred)
	}
}
