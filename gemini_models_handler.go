package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// GeminiModelsHandler is a handler for the gemini_models tool.
// It generates model documentation dynamically from the model catalog.
func (s *GeminiServer) GeminiModelsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Handling gemini_models request with direct handler")

	writer := NewSafeWriter(logger)

	writer.Write("# Available Gemini Models\n\n")
	writer.Write("Tools: `gemini_ask` (code analysis, general queries) | `gemini_search` (search-grounded queries)\n\n")

	// Generate model sections from the catalog
	models := GetAvailableGeminiModels()
	for _, model := range models {
		writer.Write("## %s\n", model.Name)
		writer.Write("- **Model ID**: `%s`", model.FamilyID)

		// Show default usage
		if model.FamilyID == s.config.GeminiModel {
			writer.Write(" (default for gemini_ask)")
		}
		if model.FamilyID == s.config.GeminiSearchModel {
			writer.Write(" (default for gemini_search)")
		}
		writer.Write("\n")

		// Show aliases (non-preferred versions)
		for _, v := range model.Versions {
			if !v.IsPreferred && v.ID != model.FamilyID {
				writer.Write("- **Alias**: `%s`\n", v.ID)
			}
		}

		writer.Write("- **Description**: %s\n", model.Description)
		writer.Write("- **Context Window**: %s tokens\n", formatTokenCount(model.ContextWindowSize))

		if model.SupportsThinking {
			writer.Write("- **Thinking Mode**: Yes (`thinking_level`: minimal, low, medium, high)\n")
		}

		writer.Write("- **Caching**: Automatic implicit caching (no configuration needed)\n\n")
	}

	// Feature docs — compact, generated once
	writeFeatureDocs(writer, models)

	if writer.Failed() {
		return createErrorResult("Error generating model list"), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(writer.String()),
		},
	}, nil
}

// writeFeatureDocs appends compact feature documentation sections.
func writeFeatureDocs(w *SafeWriter, models []GeminiModelInfo) {
	if len(models) == 0 {
		return
	}

	// Pick a representative model for examples
	proModel := models[0].FamilyID
	flashModel := proModel
	if len(models) > 1 {
		flashModel = models[1].FamilyID
	}

	// Thinking Mode
	w.Write("## Thinking Mode\n\n")
	w.Write("Control reasoning depth with `thinking_level`:\n")
	w.Write("- `minimal` — near-zero overhead, model may still think on complex tasks\n")
	w.Write("- `low` — fast, low cost\n")
	w.Write("- `medium` — balanced\n")
	w.Write("- `high` (default) — deepest reasoning\n\n")

	// Implicit Caching
	w.Write("## Implicit Caching\n\n")
	w.Write("Automatic ~90%% token discount on repeated prefixes. Files are placed before the query to maximize hits. No configuration needed.\n\n")

	// File Attachments
	w.Write("## File Attachments (gemini_ask)\n\n")
	w.Write("Use `github_files` + `github_repo` + `github_ref` for code context:\n")
	w.Write("```json\n")
	w.Write("{\n")
	w.Write("  \"query\": \"Review this code for issues\",\n")
	w.Write("  \"model\": \"%s\",\n", proModel)
	w.Write("  \"github_files\": [\"main.go\", \"config.go\"],\n")
	w.Write("  \"github_repo\": \"owner/repo\",\n")
	w.Write("  \"github_ref\": \"main\",\n")
	w.Write("  \"enable_thinking\": true\n")
	w.Write("}\n")
	w.Write("```\n\n")

	// Search with Time Filtering
	w.Write("## Time Filtering (gemini_search)\n\n")
	w.Write("Filter by publication date (RFC3339): `start_time` + `end_time`\n")
	w.Write("```json\n")
	w.Write("{\n")
	w.Write("  \"query\": \"Recent Go best practices\",\n")
	w.Write("  \"model\": \"%s\",\n", flashModel)
	w.Write("  \"start_time\": \"2025-01-01T00:00:00Z\"\n")
	w.Write("}\n")
	w.Write("```\n\n")

	// System Prompts
	w.Write("## System Prompts\n\n")
	w.Write("Override via `systemPrompt` parameter, `GEMINI_SYSTEM_PROMPT` env var, or `--gemini-system-prompt` flag.\n")
}

// formatTokenCount formats a token count for display (e.g., 1048576 → "1M").
func formatTokenCount(tokens int) string {
	if tokens >= 1_000_000 && tokens%1_000_000 == 0 {
		return fmt.Sprintf("%dM", tokens/1_000_000)
	}
	if tokens >= 1_000 && tokens%1_000 == 0 {
		return fmt.Sprintf("%dK", tokens/1_000)
	}
	return fmt.Sprintf("%d", tokens)
}
