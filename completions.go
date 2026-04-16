package main

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// validThinkingLevels are the allowed thinking level values for auto-completion.
var validThinkingLevels = []string{"minimal", "low", "medium", "high"}

// GeminiCompletionProvider provides auto-completion for prompt arguments.
type GeminiCompletionProvider struct{}

// CompletePromptArgument returns completion suggestions for the given prompt argument.
func (p *GeminiCompletionProvider) CompletePromptArgument(
	_ context.Context,
	_ string,
	argument mcp.CompleteArgument,
	_ mcp.CompleteContext,
) (*mcp.Completion, error) {
	switch argument.Name {
	case "model":
		return completeModel(argument.Value), nil
	case "thinking_level":
		return completeThinkingLevel(argument.Value), nil
	default:
		return &mcp.Completion{}, nil
	}
}

// completeModel returns model IDs matching the given prefix.
func completeModel(prefix string) *mcp.Completion {
	prefix = strings.ToLower(prefix)
	var matches []string

	// Collect model family IDs and version IDs
	models := GetAvailableGeminiModels()
	for _, m := range models {
		if strings.HasPrefix(strings.ToLower(m.FamilyID), prefix) {
			matches = append(matches, m.FamilyID)
		}
		for _, v := range m.Versions {
			if v.ID != m.FamilyID && strings.HasPrefix(strings.ToLower(v.ID), prefix) {
				matches = append(matches, v.ID)
			}
		}
	}

	// Include known aliases
	modelAliasesMu.RLock()
	for alias := range modelAliases {
		if strings.HasPrefix(strings.ToLower(alias), prefix) {
			matches = append(matches, alias)
		}
	}
	modelAliasesMu.RUnlock()

	total := len(matches)
	if len(matches) > 100 {
		matches = matches[:100]
	}

	return &mcp.Completion{
		Values:  matches,
		Total:   total,
		HasMore: total > 100,
	}
}

// completeThinkingLevel returns thinking levels matching the given prefix.
func completeThinkingLevel(prefix string) *mcp.Completion {
	prefix = strings.ToLower(prefix)
	var matches []string
	for _, level := range validThinkingLevels {
		if strings.HasPrefix(level, prefix) {
			matches = append(matches, level)
		}
	}
	return &mcp.Completion{
		Values: matches,
		Total:  len(matches),
	}
}
