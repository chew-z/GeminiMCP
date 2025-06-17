package main

// fallbackGeminiModels provides the 3 actual Gemini 2.5 models as documented by Google
func fallbackGeminiModels() []GeminiModelInfo {
	return []GeminiModelInfo{
		// Gemini 2.5 Pro - Production model
		{
			FamilyID:             "gemini-2.5-pro",
			Name:                 "Gemini 2.5 Pro",
			Description:          "Our most powerful thinking model with maximum response accuracy and state-of-the-art performance",
			SupportsThinking:     true,
			ContextWindowSize:    1048576,
			PreferredForThinking: true,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions:             []ModelVersion{}, // Production model uses family ID directly
		},

		// Gemini 2.5 Flash - Production model
		{
			FamilyID:             "gemini-2.5-flash",
			Name:                 "Gemini 2.5 Flash",
			Description:          "Best model in terms of price-performance, offering well-rounded capabilities",
			SupportsThinking:     true,
			ContextWindowSize:    32768,
			PreferredForThinking: false,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions:             []ModelVersion{}, // Production model uses family ID directly
		},

		// Gemini 2.5 Flash Lite - Preview model
		{
			FamilyID:             "gemini-2.5-flash-lite",
			Name:                 "Gemini 2.5 Flash Lite",
			Description:          "Optimized for cost efficiency and low latency",
			SupportsThinking:     true,
			ContextWindowSize:    32768,
			PreferredForThinking: false,
			PreferredForCaching:  false,
			PreferredForSearch:   true,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.5-flash-lite-preview-06-17",
					Name:            "Gemini 2.5 Flash Lite Preview 06 17",
					SupportsCaching: false,
					IsPreferred:     true,
				},
			},
		},
	}
}
