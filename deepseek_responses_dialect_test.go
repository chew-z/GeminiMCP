package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeepSeekResponsesDialectRequestShape verifies the reasoning-effort
// mapping sent to DeepSeek's Responses endpoint: max for thinking-enabled
// generations, low for the prequalify-style thinking-disabled calls.
func TestDeepSeekResponsesDialectRequestShape(t *testing.T) {
	for _, tt := range []struct {
		name   string
		req    GenerationRequest
		effort string
	}{
		{"thinking enabled", GenerationRequest{Thinking: ThinkingSpec{Enabled: true}}, "max"},
		{"thinking disabled", GenerationRequest{}, "low"},
		{"thinking disabled with json object", GenerationRequest{ResponseFormat: "json_object"}, "low"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, server := newTestResponsesProviderWithDialect(t, deepseekResponsesDialect{}, "deepseek-v4-pro", func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, tt.effort, body["reasoning"].(map[string]any)["effort"])
				assert.Equal(t, "deepseek-v4-pro", body["model"])
				writeResponse(t, w, completedResponse("OK"))
			})
			defer server.Close()
			_, err := p.Generate(context.Background(), tt.req)
			require.NoError(t, err)
		})
	}
}
