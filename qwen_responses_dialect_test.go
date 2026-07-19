package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQwenResponsesDialectThinkingForced verifies that thinking-forced models
// (e.g. qwen3.8-max-preview) never emit effort=none — DashScope rejects
// enable_thinking=false for them — and get tiered effort: high for real
// generations, low for utility calls like prequalification.
func TestQwenResponsesDialectThinkingForced(t *testing.T) {
	for _, tt := range []struct {
		name   string
		req    GenerationRequest
		effort string
	}{
		{"thinking enabled", GenerationRequest{Thinking: ThinkingSpec{Enabled: true}}, "high"},
		{"thinking disabled", GenerationRequest{}, "low"},
		{"json object with thinking", GenerationRequest{Thinking: ThinkingSpec{Enabled: true}, ResponseFormat: "json_object"}, "high"},
		{"json object without thinking", GenerationRequest{ResponseFormat: "json_object"}, "low"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, server := newTestResponsesProviderWithDialect(t, qwenResponsesDialect{thinkingForced: true}, "qwen3.8-max-preview", func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, tt.effort, body["reasoning"].(map[string]any)["effort"])
				assert.Equal(t, "enable", r.Header.Get("x-dashscope-session-cache"))
				writeResponse(t, w, completedResponse("OK"))
			})
			defer server.Close()
			_, err := p.Generate(context.Background(), tt.req)
			require.NoError(t, err)
		})
	}
}

func TestQwenResponsesDialectRequestShape(t *testing.T) {
	for _, tt := range []struct {
		name   string
		req    GenerationRequest
		effort string
	}{
		{"thinking enabled", GenerationRequest{Thinking: ThinkingSpec{Enabled: true}}, "max"},
		{"thinking disabled", GenerationRequest{}, "none"},
		{"json wins", GenerationRequest{Thinking: ThinkingSpec{Enabled: true}, ResponseFormat: "json_object"}, "none"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, server := newTestResponsesProvider(t, func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, tt.effort, body["reasoning"].(map[string]any)["effort"])
				assert.Equal(t, "enable", r.Header.Get("x-dashscope-session-cache"))
				writeResponse(t, w, completedResponse("OK"))
			})
			defer server.Close()
			_, err := p.Generate(context.Background(), tt.req)
			require.NoError(t, err)
		})
	}
}
