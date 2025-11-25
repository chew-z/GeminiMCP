package main

// fallbackGeminiModels provides the Gemini models supported by this server
func fallbackGeminiModels() []GeminiModelInfo {
	return []GeminiModelInfo{
		// Gemini 3 Pro - Latest model with advanced reasoning
		{
			FamilyID: "gemini-3-pro-preview",
			Name:     "Gemini 3 Pro",
			Description: "First model in the Gemini 3 series. Best for complex tasks requiring " +
				"broad world knowledge and advanced reasoning across modalities",
			SupportsThinking:     true,
			ContextWindowSize:    1048576,
			PreferredForThinking: true,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{ID: "gemini-3-pro-preview", SupportsCaching: true, IsPreferred: true},
			},
		},

		// Gemini 2.5 Pro - Previous generation (still supported)
		{
			FamilyID:             "gemini-2.5-pro",
			Name:                 "Gemini 2.5 Pro",
			Description:          "Previous generation thinking model with maximum response accuracy",
			SupportsThinking:     true,
			ContextWindowSize:    1048576,
			PreferredForThinking: false,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{ID: "gemini-2.5-pro", SupportsCaching: true},
			},
		},

		// Gemini 2.5 Flash - Production model
		{
			FamilyID:             "gemini-flash-latest",
			Name:                 "Gemini 2.5 Flash",
			Description:          "Best model in terms of price-performance, offering well-rounded capabilities",
			SupportsThinking:     true,
			ContextWindowSize:    32768,
			PreferredForThinking: false,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{ID: "gemini-flash-latest", SupportsCaching: true},
			},
		},

		// Gemini 2.5 Flash Lite - GA model
		{
			FamilyID:             "gemini-flash-lite-latest",
			Name:                 "Gemini 2.5 Flash Lite",
			Description:          "Optimized for cost efficiency and low latency",
			SupportsThinking:     true,
			ContextWindowSize:    32768,
			PreferredForThinking: false,
			PreferredForCaching:  false,
			PreferredForSearch:   true,
			Versions: []ModelVersion{
				{ID: "gemini-flash-lite-latest", SupportsCaching: false},
			},
		},
	}
}
