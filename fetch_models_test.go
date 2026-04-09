package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		{"non-gemini model", "palm-2-pro", 0, false},
		{"tts variant", "gemini-3-pro-tts", 0, false},
		{"image variant", "gemini-3-flash-image", 0, false},
		{"customtools variant", "gemini-3-pro-customtools", 0, false},
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isNewerModel(tc.candidate, tc.current))
		})
	}
}

func TestParseModelVersion(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMajor int
		wantMinor int
		wantOK    bool
	}{
		{"major only", "gemini-3-pro-preview", 3, 0, true},
		{"major.minor", "gemini-3.1-pro-preview", 3, 1, true},
		{"double digit minor", "gemini-3.10-flash-preview", 3, 10, true},
		{"flash-lite", "gemini-3.1-flash-lite-preview", 3, 1, true},
		{"no match", "palm-2-pro", 0, 0, false},
		{"no match - no dash after version", "gemini-3pro", 0, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, ok := parseModelVersion(tc.input)
			assert.Equal(t, tc.wantOK, ok)
			if ok {
				assert.Equal(t, tc.wantMajor, v.major)
				assert.Equal(t, tc.wantMinor, v.minor)
			}
		})
	}
}
