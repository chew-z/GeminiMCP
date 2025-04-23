package main

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// handleGeminiModels handles requests to the gemini_models tool
func (s *GeminiServer) handleGeminiModels(ctx context.Context) (*internalCallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Listing available Gemini models")

	// Direct API access to get the most up-to-date model list
	var models []GeminiModelInfo

	if s.config.GeminiAPIKey != "" {
		// We'll try to fetch models directly here for the most accurate list
		logger.Info("Fetching models directly from API for most current list...")

		// Create a new client specifically for this request
		clientConfig := &genai.ClientConfig{
			APIKey: s.config.GeminiAPIKey,
		}

		tempClient, err := genai.NewClient(ctx, clientConfig)
		if err != nil {
			logger.Warn("Could not create temporary client for model listing: %v. Will use cached list.", err)
		} else {
			// Try to fetch models directly
			var fetchedModels []GeminiModelInfo
			modelCount := 0

			// Iterate through all available models
			for model, err := range tempClient.Models.All(ctx) {
				modelCount++
				if err != nil {
					logger.Warn("Error while fetching model: %v", err)
					continue
				}

				// Look for Gemini models
				modelName := strings.ToLower(model.Name)
				if strings.Contains(modelName, "gemini") {
					id := model.Name
					if strings.HasPrefix(id, "models/") {
						id = strings.TrimPrefix(id, "models/")
					}

					logger.Debug("Found model: %s", id)

					// Determine capabilities based on model type
					supportsCaching := strings.HasSuffix(id, "-001")
					supportsThinking := strings.Contains(strings.ToLower(id), "pro")
					contextWindowSize := 32768 // Default for Flash models

					if supportsThinking {
						contextWindowSize = 1048576 // Pro models have 1M context
					}

					// Create user-friendly name
					name := strings.TrimPrefix(id, "gemini-")
					name = strings.ReplaceAll(name, "-", " ")
					name = strings.Title(name)
					name = "Gemini " + name

					// Create appropriate description
					description := "Google Gemini model"
					if strings.Contains(strings.ToLower(id), "pro") {
						description = "Pro model with advanced reasoning capabilities"
					} else if strings.Contains(strings.ToLower(id), "flash") {
						description = "Flash model optimized for efficiency and speed"
					}

					// Add preview designation if applicable
					if strings.Contains(strings.ToLower(id), "preview") || strings.Contains(strings.ToLower(id), "exp") {
						description = "Preview/Experimental " + description
					}

					// Add the model to our list
					fetchedModels = append(fetchedModels, GeminiModelInfo{
						ID:                id,
						Name:              name,
						Description:       description,
						SupportsCaching:   supportsCaching,
						SupportsThinking:  supportsThinking,
						ContextWindowSize: contextWindowSize,
					})
				}
			}

			if len(fetchedModels) > 0 {
				// Use the directly fetched models for this response
				models = fetchedModels
				logger.Info("Successfully fetched %d models directly from API for display", len(models))

				// Also update the store for future use
				modelStore.Lock()
				modelStore.models = fetchedModels
				modelStore.Unlock()
			} else {
				logger.Warn("No models found from direct API call (from %d total). Using cached list.", modelCount)
				models = GetAvailableGeminiModels()
			}
		}
	}

	// Fallback if we couldn't get models directly
	if len(models) == 0 {
		models = GetAvailableGeminiModels()
		logger.Info("Using cached model list with %d models", len(models))
	}

	// Create a formatted response using strings.Builder with error handling
	var formattedContent strings.Builder

	// Define a helper function to write with error checking
	writeStringf := func(format string, args ...interface{}) error {
		_, err := formattedContent.WriteString(fmt.Sprintf(format, args...))
		return err
	}

	// Write the header
	if err := writeStringf("# Available Gemini Models\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	// Write each model's information
	for _, model := range models {
		if err := writeStringf("## %s\n", model.Name); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		if err := writeStringf("- ID: `%s`\n", model.ID); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		if err := writeStringf("- Description: %s\n", model.Description); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		// Add caching support info
		if err := writeStringf("- Supports Caching: %v\n", model.SupportsCaching); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		// Add thinking support info
		if err := writeStringf("- Supports Thinking: %v\n", model.SupportsThinking); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		// Add context window size
		if err := writeStringf("- Context Window Size: %d tokens\n\n", model.ContextWindowSize); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}
	}

	// Add usage hint
	if err := writeStringf("## Usage\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("You can specify a model ID in the `model` parameter when using the `gemini_ask` or `gemini_search` tools:\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("```json\n// For gemini_ask\n{\n  \"query\": \"Your question here\",\n  \"model\": \"gemini-1.5-pro-001\",\n  \"use_cache\": true\n}\n\n// For gemini_search\n{\n  \"query\": \"Your search question here\",\n  \"model\": \"gemini-2.5-pro-exp-03-25\",\n  \"enable_thinking\": true\n}\n```\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	// Add info about caching
	if err := writeStringf("\n## Caching\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("Only models with version suffixes (e.g., ending with `-001`) support caching.\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("When using a cacheable model, you can enable caching with the `use_cache` parameter. This will create a temporary cache that automatically expires after 10 minutes by default. You can specify a custom TTL with the `cache_ttl` parameter.\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	// Add info about thinking mode
	if err := writeStringf("\n## Thinking Mode\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("Pro models support thinking mode, which shows the model's detailed reasoning process.\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("You can control thinking mode using these parameters:\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("* `enable_thinking`: Enables or disables thinking mode (boolean)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("* `thinking_budget_level`: Sets predefined token budgets (\"none\", \"low\", \"medium\", \"high\")\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("  - none: 0 tokens (disabled)\n  - low: 4096 tokens\n  - medium: 16384 tokens\n  - high: 24576 tokens (maximum)\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("* `thinking_budget`: Sets a specific token count (0-24576)\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("Example:\n\n```json\n{\n  \"query\": \"Your complex question here\",\n  \"model\": \"gemini-1.5-pro\",\n  \"enable_thinking\": true,\n  \"thinking_budget_level\": \"medium\"\n}\n```\n\nOr with explicit budget:\n\n```json\n{\n  \"query\": \"Your complex question here\",\n  \"model\": \"gemini-1.5-pro\",\n  \"enable_thinking\": true,\n  \"thinking_budget\": 8192\n}\n```\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	return &internalCallToolResponse{
		Content: []internalToolContent{
			{
				Type: "text",
				Text: formattedContent.String(),
			},
		},
	}, nil
}
