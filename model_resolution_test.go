package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestResolveAndValidateModel(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())
	addDynamicAlias("gemini-legacy-pro-preview", "gemini-3.1-pro-preview")

	tests := []struct {
		name        string
		input       string
		wantModelID string
		wantErr     string
	}{
		{
			name:        "known version id stays unchanged",
			input:       "gemini-3.1-pro-preview-0507",
			wantModelID: "gemini-3.1-pro-preview-0507",
		},
		{
			name:        "known family id resolves to preferred version",
			input:       "gemini-3.1-pro-preview",
			wantModelID: "gemini-3.1-pro-preview-0507",
		},
		{
			name:        "runtime alias resolves to preferred version",
			input:       "gemini-legacy-pro-preview",
			wantModelID: "gemini-3.1-pro-preview-0507",
		},
		{
			name:        "unknown gemini model redirects by tier",
			input:       "gemini-99-pro-preview",
			wantModelID: "gemini-3.1-pro-preview",
		},
		{
			name:    "non-gemini model is rejected",
			input:   "gpt-4.1",
			wantErr: "not a recognized Gemini model",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveAndValidateModel(context.Background(), tc.input)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantModelID, got)
		})
	}
}

func TestCreateModelConfig(t *testing.T) {
	seedModelStateForTest(t, testModelCatalog())

	baseConfig := &Config{
		GeminiModel:        "gemini-3.1-pro-preview",
		GeminiSystemPrompt: "system prompt",
		GeminiTemperature:  0.2,
		EnableThinking:     true,
		ThinkingLevel:      "high",
		ServiceTier:        "standard",
	}

	tests := []struct {
		name          string
		args          map[string]interface{}
		defaultModel  string
		wantModelName string
		wantMaxTokens int32
		wantThinking  bool
		wantLevel     string
		wantErr       string
	}{
		{
			name:          "uses model max output token limit by default",
			args:          map[string]interface{}{},
			defaultModel:  "gemini-3.1-pro-preview",
			wantModelName: "gemini-3.1-pro-preview-0507",
			wantMaxTokens: 8192,
			wantThinking:  true,
			wantLevel:     "high",
		},
		{
			name: "max_tokens argument overrides model limit",
			args: map[string]interface{}{
				"max_tokens": float64(1234),
			},
			defaultModel:  "gemini-3.1-pro-preview",
			wantModelName: "gemini-3.1-pro-preview-0507",
			wantMaxTokens: 1234,
			wantThinking:  true,
			wantLevel:     "high",
		},
		{
			name: "unknown gemini model redirects by tier",
			args: map[string]interface{}{
				"model": "gemini-9-flash-preview",
			},
			defaultModel:  "gemini-3.1-pro-preview",
			wantModelName: "gemini-3-flash-preview",
			wantMaxTokens: 4096,
			wantThinking:  true,
			wantLevel:     "high",
		},
		{
			name: "invalid thinking_level falls back to default",
			args: map[string]interface{}{
				"thinking_level": "ultra",
			},
			defaultModel:  "gemini-3.1-pro-preview",
			wantModelName: "gemini-3.1-pro-preview-0507",
			wantMaxTokens: 8192,
			wantThinking:  true,
			wantLevel:     "high",
		},
		{
			name: "model without thinking support skips thinking config",
			args: map[string]interface{}{
				"model": "gemini-3.1-flash-lite-preview",
			},
			defaultModel:  "gemini-3.1-pro-preview",
			wantModelName: "gemini-3.1-flash-lite-preview-0417",
			wantMaxTokens: 2048,
			wantThinking:  false,
		},
		{
			name: "non-gemini model is rejected",
			args: map[string]interface{}{
				"model": "claude-3.7-sonnet",
			},
			defaultModel: "gemini-3.1-pro-preview",
			wantErr:      "not a recognized Gemini model",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "gemini_ask",
					Arguments: tc.args,
				},
			}

			cfg, modelName, err := createModelConfig(context.Background(), req, baseConfig, tc.defaultModel)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
			assert.Equal(t, tc.wantModelName, modelName)
			assert.Equal(t, tc.wantMaxTokens, cfg.MaxOutputTokens)

			if tc.wantThinking {
				require.NotNil(t, cfg.ThinkingConfig)
				assert.Equal(t, tc.wantLevel, string(cfg.ThinkingConfig.ThinkingLevel))
			} else {
				assert.Nil(t, cfg.ThinkingConfig)
			}
		})
	}
}

func TestCheckModelStatus(t *testing.T) {
	t.Run("adds dynamic alias when stage is not preview or stable", func(t *testing.T) {
		seedModelStateForTest(t, testModelCatalog())

		checkModelStatus(context.Background(), &genai.GenerateContentResponse{
			ModelStatus: &genai.ModelStatus{
				ModelStage: genai.ModelStageDeprecated,
			},
		}, "gemini-9-pro-preview")

		// Alias points to a pro family and ResolveModelID upgrades it to the preferred version.
		assert.Equal(t, "gemini-3.1-pro-preview-0507", ResolveModelID("gemini-9-pro-preview"))
	})

	t.Run("does not add alias for preview model stage", func(t *testing.T) {
		seedModelStateForTest(t, testModelCatalog())

		checkModelStatus(context.Background(), &genai.GenerateContentResponse{
			ModelStatus: &genai.ModelStatus{
				ModelStage: genai.ModelStagePreview,
			},
		}, "gemini-9-pro-preview")

		assert.Equal(t, "gemini-9-pro-preview", ResolveModelID("gemini-9-pro-preview"))
	})
}
