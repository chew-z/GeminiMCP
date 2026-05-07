package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestCompleteModel(t *testing.T) {
	// Seed model store for tests (normally populated from API at startup).
	seedModelStateForTest(t, []GeminiModelInfo{
		{FamilyID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro", SupportsThinking: true, ContextWindowSize: 1048576,
			Versions: []ModelVersion{{ID: "gemini-3.1-pro-preview", IsPreferred: true}}},
		{FamilyID: "gemini-3-flash-preview", Name: "Gemini 3 Flash", SupportsThinking: true, ContextWindowSize: 1048576,
			Versions: []ModelVersion{{ID: "gemini-3-flash-preview", IsPreferred: true}, {ID: "gemini-flash-latest"}}},
		{FamilyID: "gemini-3.1-flash-lite", Name: "Gemini 3.1 Flash Lite", SupportsThinking: true, ContextWindowSize: 1048576,
			Versions: []ModelVersion{{ID: "gemini-3.1-flash-lite", IsPreferred: true}, {ID: "gemini-flash-lite-latest"}}},
	})

	tests := []struct {
		name      string
		prefix    string
		wantMin   int
		wantMatch string
	}{
		{
			name:      "empty prefix returns all models",
			prefix:    "",
			wantMin:   3,
			wantMatch: "",
		},
		{
			name:      "gemini-3 prefix",
			prefix:    "gemini-3",
			wantMin:   1,
			wantMatch: "gemini-3.1-pro-preview",
		},
		{
			name:      "case insensitive",
			prefix:    "GEMINI",
			wantMin:   1,
			wantMatch: "",
		},
		{
			name:      "no match",
			prefix:    "nonexistent-model",
			wantMin:   0,
			wantMatch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := completeModel(tt.prefix)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if len(result.Values) < tt.wantMin {
				t.Errorf("expected at least %d values, got %d: %v", tt.wantMin, len(result.Values), result.Values)
			}
			if tt.wantMatch != "" {
				found := false
				for _, v := range result.Values {
					if v == tt.wantMatch {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected %q in results, got %v", tt.wantMatch, result.Values)
				}
			}
		})
	}
}

func TestCompleteThinkingLevel(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		wantVals []string
	}{
		{
			name:     "empty prefix returns all",
			prefix:   "",
			wantVals: []string{"minimal", "low", "medium", "high"},
		},
		{
			name:     "m prefix",
			prefix:   "m",
			wantVals: []string{"minimal", "medium"},
		},
		{
			name:     "exact match",
			prefix:   "high",
			wantVals: []string{"high"},
		},
		{
			name:     "no match",
			prefix:   "x",
			wantVals: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := completeThinkingLevel(tt.prefix)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if len(result.Values) != len(tt.wantVals) {
				t.Errorf("expected %d values, got %d: %v", len(tt.wantVals), len(result.Values), result.Values)
			}
			for _, want := range tt.wantVals {
				found := false
				for _, got := range result.Values {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected %q in results %v", want, result.Values)
				}
			}
		})
	}
}

func TestGeminiCompletionProvider(t *testing.T) {
	seedModelStateForTest(t, []GeminiModelInfo{
		{FamilyID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro", SupportsThinking: true, ContextWindowSize: 1048576,
			Versions: []ModelVersion{{ID: "gemini-3.1-pro-preview", IsPreferred: true}}},
		{FamilyID: "gemini-3-flash-preview", Name: "Gemini 3 Flash", SupportsThinking: true, ContextWindowSize: 1048576,
			Versions: []ModelVersion{{ID: "gemini-3-flash-preview", IsPreferred: true}}},
	})

	provider := &GeminiCompletionProvider{}
	ctx := context.Background()

	t.Run("model argument", func(t *testing.T) {
		result, err := provider.CompletePromptArgument(ctx, "code_review", mcp.CompleteArgument{
			Name:  "model",
			Value: "gemini-3",
		}, mcp.CompleteContext{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Values) == 0 {
			t.Error("expected at least one model completion")
		}
	})

	t.Run("thinking_level argument", func(t *testing.T) {
		result, err := provider.CompletePromptArgument(ctx, "debug_help", mcp.CompleteArgument{
			Name:  "thinking_level",
			Value: "h",
		}, mcp.CompleteContext{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Values) != 1 || result.Values[0] != "high" {
			t.Errorf("expected [high], got %v", result.Values)
		}
	})

	t.Run("unknown argument returns empty", func(t *testing.T) {
		result, err := provider.CompletePromptArgument(ctx, "code_review", mcp.CompleteArgument{
			Name:  "problem_statement",
			Value: "some text",
		}, mcp.CompleteContext{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Values) != 0 {
			t.Errorf("expected empty values for unknown argument, got %v", result.Values)
		}
	})
}
