package main

// fallbackGeminiModels provides the Gemini models supported by this server.
// Only Gemini 3+ models are supported — all use the thinkingLevel parameter.
func fallbackGeminiModels() []GeminiModelInfo {
	return []GeminiModelInfo{
		// Gemini 3.1 Pro — advanced reasoning and agentic workflows
		{
			FamilyID: "gemini-3.1-pro-preview",
			Name:     "Gemini 3.1 Pro",
			Description: "Advanced intelligence, complex problem-solving, and powerful agentic " +
				"and vibe coding capabilities",
			SupportsThinking:     true,
			ContextWindowSize:    1048576,
			PreferredForThinking: true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{ID: "gemini-3.1-pro-preview", IsPreferred: true},
			},
		},

		// Gemini 3 Flash — frontier-class performance at a fraction of the cost
		{
			FamilyID:             "gemini-3-flash-preview",
			Name:                 "Gemini 3 Flash",
			Description:          "Frontier-class performance rivaling larger models at a fraction of the cost",
			SupportsThinking:     true,
			ContextWindowSize:    1048576,
			PreferredForThinking: false,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{ID: "gemini-3-flash-preview", IsPreferred: true},
				{ID: "gemini-flash-latest"},
			},
		},

		// Gemini 3.1 Flash Lite — fastest and most cost-efficient
		{
			FamilyID:             "gemini-3.1-flash-lite-preview",
			Name:                 "Gemini 3.1 Flash Lite",
			Description:          "Fastest and most cost-efficient model for high-volume, lightweight tasks",
			SupportsThinking:     true,
			ContextWindowSize:    1048576,
			PreferredForThinking: false,
			PreferredForSearch:   true,
			Versions: []ModelVersion{
				{ID: "gemini-3.1-flash-lite-preview", IsPreferred: true},
				{ID: "gemini-flash-lite-latest"},
			},
		},
	}
}
