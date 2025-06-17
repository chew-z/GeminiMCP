package main

// fallbackGeminiModels provides a list of default Gemini 2.5 models to use if API fetching fails
// Note: Only Gemini 2.5 family models are supported for optimal thinking and caching capabilities
func fallbackGeminiModels() []GeminiModelInfo {
	return []GeminiModelInfo{
		// Gemini 2.5 Pro Models
		{
			FamilyID:             "gemini-2.5-pro",
			Name:                 "Gemini 2.5 Pro",
			Description:          "Pro model with advanced reasoning capabilities and thinking mode support",
			SupportsThinking:     true, // All 2.5 models support thinking
			ContextWindowSize:    1048576,
			PreferredForThinking: true,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.5-pro-preview-06-05",
					Name:            "Gemini 2.5 Pro Preview 06 05",
					SupportsCaching: true, // All 2.5 models support implicit caching
					IsPreferred:     true,
				},
				{
					ID:              "gemini-2.5-pro-exp-03-25",
					Name:            "Gemini 2.5 Pro Exp 03 25",
					SupportsCaching: true,
					IsPreferred:     false,
				},
			},
		},

		// Gemini 2.5 Flash Model
		{
			FamilyID:             "gemini-2.5-flash",
			Name:                 "Gemini 2.5 Flash",
			Description:          "Flash model optimized for efficiency and speed with thinking mode support",
			SupportsThinking:     true, // All 2.5 models support thinking
			ContextWindowSize:    32768,
			PreferredForThinking: true,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.5-flash-preview-05-20",
					Name:            "Gemini 2.5 Flash Preview 05 20",
					SupportsCaching: true, // All 2.5 models support implicit caching
					IsPreferred:     true,
				},
				{
					ID:              "gemini-2.5-flash-preview-04-17",
					Name:            "Gemini 2.5 Flash Preview 04 17",
					SupportsCaching: true,
					IsPreferred:     false,
				},
			},
		},

		// Gemini 2.5 Flash Lite Model
		{
			FamilyID:             "gemini-2.5-flash-lite",
			Name:                 "Gemini 2.5 Flash Lite",
			Description:          "Flash lite model optimized for low-cost, low-latency with optional thinking mode",
			SupportsThinking:     true, // All 2.5 models support thinking (off by default for Lite)
			ContextWindowSize:    32768,
			PreferredForThinking: true,
			PreferredForCaching:  false, // Flash Lite does not support implicit caching yet
			PreferredForSearch:   true,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.5-flash-lite-preview-06-17",
					Name:            "Gemini 2.5 Flash Lite Preview 06 17",
					SupportsCaching: false, // Flash Lite does not support implicit caching yet
					IsPreferred:     true,
				},
			},
		},
	}
}
