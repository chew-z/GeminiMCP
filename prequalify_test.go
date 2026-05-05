package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// TestBuildPrequalifyConfigUsesResponseJsonSchema locks in Step 4: the
// classifier config must populate ResponseJsonSchema (the modern declarative
// JSON-Schema map) and leave the legacy ResponseSchema field nil. The schema
// shape — top-level "type":"string" plus the six-category enum — is the
// public contract this server depends on, so the test pins it explicitly.
func TestBuildPrequalifyConfigUsesResponseJsonSchema(t *testing.T) {
	cfg := buildPrequalifyConfig("gemini-3-flash-preview", "low")
	require.NotNil(t, cfg)
	assert.Nil(t, cfg.ResponseSchema, "ResponseSchema must be unset; ResponseJsonSchema replaces it")
	assert.Equal(t, "application/json", cfg.ResponseMIMEType)

	schema, ok := cfg.ResponseJsonSchema.(map[string]any)
	require.True(t, ok, "ResponseJsonSchema must be a map[string]any literal")
	assert.Equal(t, "string", schema["type"])

	enum, ok := schema["enum"].([]string)
	require.True(t, ok, "enum must be []string")
	assert.ElementsMatch(t,
		[]string{"general", "analyze", "review", "security", "debug", "tests"},
		enum,
	)
}

// TestParsePrequalifyResponseAcceptsAllCategories asserts that every category
// emitted under the new ResponseJsonSchema contract round-trips through
// parsePrequalifyResponse to the matching queryCategory. Mocking happens at
// the response-shape level — no live API call — so this test is independent
// of the Gemini client wiring.
func TestParsePrequalifyResponseAcceptsAllCategories(t *testing.T) {
	cases := []struct {
		// rawText is what the model returns; structured-output wraps the
		// enum value in JSON quotes.
		rawText string
		want    queryCategory
	}{
		{`"general"`, categoryGeneral},
		{`"analyze"`, categoryAnalyze},
		{`"review"`, categoryReview},
		{`"security"`, categorySecurity},
		{`"debug"`, categoryDebug},
		{`"tests"`, categoryTests},
	}

	for _, tc := range cases {
		t.Run(string(tc.want), func(t *testing.T) {
			resp := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Parts: []*genai.Part{
								{Text: tc.rawText},
							},
						},
					},
				},
			}
			cat, raw, err := parsePrequalifyResponse(resp)
			require.NoError(t, err, "raw=%q", raw)
			assert.Equal(t, tc.want, cat)
		})
	}
}

// TestParsePrequalifyResponseRejectsUnknown guards the rejection branch so
// fallback selection in resolveSystemPromptAsync continues to fire on
// off-contract classifier output.
func TestParsePrequalifyResponseRejectsUnknown(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: `"not-a-category"`},
					},
				},
			},
		},
	}
	_, raw, err := parsePrequalifyResponse(resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown category")
	assert.Equal(t, "not-a-category", raw)
}
