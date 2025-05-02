package main

// fallbackGeminiModels provides a list of default Gemini models to use if API fetching fails
func fallbackGeminiModels() []GeminiModelInfo {
	return []GeminiModelInfo{
		// Gemini 2.5 Pro Models (Preview/Experimental)
		{
			FamilyID:             "gemini-2.5-pro",
			Name:                 "Gemini 2.5 Pro",
			Description:          "Preview/Experimental Pro model with advanced reasoning capabilities",
			SupportsThinking:     true, // Confirmed to work with thinking mode
			ContextWindowSize:    1048576,
			PreferredForThinking: true,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.5-pro-exp-03-25",
					Name:            "Gemini 2.5 Pro Exp 03 25",
					SupportsCaching: true,
					IsPreferred:     true,
				},
				{
					ID:              "gemini-2.5-pro-preview-03-25",
					Name:            "Gemini 2.5 Pro Preview 03 25",
					SupportsCaching: true,
					IsPreferred:     false,
				},
			},
		},

		// Gemini 2.5 Flash Model
		{
			FamilyID:             "gemini-2.5-flash",
			Name:                 "Gemini 2.5 Flash",
			Description:          "Preview/Experimental Flash model optimized for efficiency and speed",
			SupportsThinking:     false,
			ContextWindowSize:    32768,
			PreferredForThinking: false,
			PreferredForCaching:  false,
			PreferredForSearch:   true,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.5-flash-preview-04-17",
					Name:            "Gemini 2.5 Flash Preview 04 17",
					SupportsCaching: true,
					IsPreferred:     true,
				},
			},
		},

		// Gemini 2.0 Flash Models
		{
			FamilyID:             "gemini-2.0-flash",
			Name:                 "Gemini 2.0 Flash",
			Description:          "Flash model optimized for efficiency and speed",
			SupportsThinking:     false,
			ContextWindowSize:    32768,
			PreferredForThinking: false,
			PreferredForCaching:  true,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.0-flash-001",
					Name:            "Gemini 2.0 Flash 001",
					SupportsCaching: true,
					IsPreferred:     true,
				},
				{
					ID:              "gemini-2.0-flash-exp",
					Name:            "Gemini 2.0 Flash Exp",
					SupportsCaching: false,
					IsPreferred:     false,
				},
			},
		},

		// Gemini 2.0 Flash Lite Model
		{
			FamilyID:             "gemini-2.0-flash-lite",
			Name:                 "Gemini 2.0 Flash Lite",
			Description:          "Flash lite model optimized for efficiency and speed",
			SupportsThinking:     false,
			ContextWindowSize:    32768,
			PreferredForThinking: false,
			PreferredForCaching:  false,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.0-flash-lite-001",
					Name:            "Gemini 2.0 Flash Lite 001",
					SupportsCaching: true,
					IsPreferred:     true,
				},
			},
		},

		// Gemini 2.0 Pro Models
		{
			FamilyID:             "gemini-2.0-pro",
			Name:                 "Gemini 2.0 Pro",
			Description:          "Pro model with advanced reasoning capabilities",
			SupportsThinking:     true,
			ContextWindowSize:    1048576,
			PreferredForThinking: false,
			PreferredForCaching:  false,
			PreferredForSearch:   false,
			Versions: []ModelVersion{
				{
					ID:              "gemini-2.0-pro-exp",
					Name:            "Gemini 2.0 Pro Exp",
					SupportsCaching: false,
					IsPreferred:     true,
				},
			},
		},
	}
}
