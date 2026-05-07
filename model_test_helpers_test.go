package main

import "testing"

func testModelCatalog() []GeminiModelInfo {
	return []GeminiModelInfo{
		{
			FamilyID:          "gemini-3.1-pro-preview",
			Name:              "Gemini 3.1 Pro",
			Description:       "Pro tier model for complex tasks.",
			SupportsThinking:  true,
			ContextWindowSize: 1048576,
			MaxOutputTokens:   8192,
			Versions: []ModelVersion{
				{ID: "gemini-3.1-pro-preview-0507", IsPreferred: true},
				{ID: "gemini-3.1-pro-preview-0401"},
			},
		},
		{
			FamilyID:          "gemini-3-flash-preview",
			Name:              "Gemini 3 Flash",
			Description:       "Fast general-purpose model.",
			SupportsThinking:  true,
			ContextWindowSize: 1048576,
			MaxOutputTokens:   4096,
			Versions: []ModelVersion{
				{ID: "gemini-3-flash-preview-0502", IsPreferred: true},
			},
		},
		{
			FamilyID:          "gemini-3.1-flash-lite",
			Name:              "Gemini 3.1 Flash Lite",
			Description:       "Low-latency lightweight model.",
			SupportsThinking:  false,
			ContextWindowSize: 524288,
			MaxOutputTokens:   2048,
			Versions: []ModelVersion{
				{ID: "gemini-3.1-flash-lite", IsPreferred: true},
			},
		},
	}
}

func cloneModelCatalog(models []GeminiModelInfo) []GeminiModelInfo {
	cloned := make([]GeminiModelInfo, len(models))
	for i, m := range models {
		cloned[i] = m
		cloned[i].Versions = append([]ModelVersion(nil), m.Versions...)
	}
	return cloned
}

func seedModelStateForTest(t *testing.T, models []GeminiModelInfo) {
	t.Helper()

	originalModels := cloneModelCatalog(GetAvailableGeminiModels())

	modelAliasesMu.Lock()
	originalAliases := make(map[string]string, len(modelAliases))
	for from, to := range modelAliases {
		originalAliases[from] = to
	}
	modelAliases = map[string]string{}
	modelAliasesMu.Unlock()

	SetModels(cloneModelCatalog(models))

	t.Cleanup(func() {
		SetModels(originalModels)
		modelAliasesMu.Lock()
		modelAliases = originalAliases
		modelAliasesMu.Unlock()
	})
}
