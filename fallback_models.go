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
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{ID: "gemini-3.1-pro-preview", SupportsCaching: true, IsPreferred: true},
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
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{ID: "gemini-3-flash-preview", SupportsCaching: true, IsPreferred: true},
				{ID: "gemini-flash-latest", SupportsCaching: true},
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
			PreferredForCaching:  true,
			PreferredForSearch:   true,
			Versions: []ModelVersion{
				{ID: "gemini-3.1-flash-lite-preview", SupportsCaching: true, IsPreferred: true},
				{ID: "gemini-flash-lite-latest", SupportsCaching: true},
			},
		},
	}
}
