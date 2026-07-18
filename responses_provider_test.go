package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestResponsesProvider(t *testing.T, handler http.HandlerFunc) (*responsesProvider, *httptest.Server) {
	return newTestResponsesProviderWithDialect(t, qwenResponsesDialect{}, "qwen3.7-max", handler)
}

func newTestResponsesProviderWithDialect(t *testing.T, dialect responsesDialect, model string, handler http.HandlerFunc) (*responsesProvider, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	p := &responsesProvider{client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL), option.WithMaxRetries(0)), model: model, dialect: dialect, logger: NewLogger(LevelError)}
	return p, server
}

func writeResponse(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(body))
	require.NoError(t, err)
}

func completedResponse(text string) string {
	return `{"object":"response","status":"completed","model":"served","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + text + `"}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":3}}}`
}

func TestResponsesProviderRequestShape(t *testing.T) {
	p, server := newTestResponsesProvider(t, func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "qwen3.7-max", body["model"])
		assert.Equal(t, "system", body["instructions"])
		assert.Equal(t, "firstsecond", body["input"])
		assert.Equal(t, 0.3, body["temperature"])
		assert.Equal(t, float64(100), body["max_output_tokens"])
		assert.Equal(t, "json_object", body["text"].(map[string]any)["format"].(map[string]any)["type"])
		writeResponse(t, w, completedResponse("OK"))
	})
	defer server.Close()
	_, err := p.Generate(context.Background(), GenerationRequest{SystemPrompt: "system", Parts: []ContentPart{{Text: "first"}, {File: &FileContent{Name: "x"}}, {Text: "second"}}, Temperature: 0.3, MaxOutputTokens: 100, ResponseFormat: "json_object"})
	require.NoError(t, err)
}

func TestResponsesProviderResponseParsing(t *testing.T) {
	tests := []struct {
		name, body, finish string
		wantErr            bool
	}{
		{"completed", completedResponse("answer"), "stop", false},
		{"incomplete max", `{"object":"response","status":"incomplete","model":"served","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","content":[{"type":"output_text","text":"partial"}]}]}`, "max_output_tokens", false},
		{"incomplete filter", `{"object":"response","status":"incomplete","model":"served","incomplete_details":{"reason":"content_filter"},"output":[{"type":"message","content":[{"type":"output_text","text":"partial"}]}]}`, "content_filter", false},
		{"failed", `{"object":"response","status":"failed","error":{"code":"server_error","message":"broken"}}`, "", true},
		{"cancelled", `{"object":"response","status":"cancelled"}`, "", true},
		{"empty output", `{"object":"response","status":"completed","output":[]}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, server := newTestResponsesProvider(t, func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, tt.body) })
			defer server.Close()
			resp, err := p.Generate(context.Background(), GenerationRequest{})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.finish, resp.FinishReason)
			if tt.name == "completed" {
				assert.Equal(t, "answer", resp.Text)
				assert.Equal(t, UsageInfo{PromptTokens: 10, OutputTokens: 5, ReasoningTokens: 3, CachedTokens: 2, TotalTokens: 15}, resp.Usage)
			}
		})
	}
	_, err := convertResponse(nil)
	require.Error(t, err)
}

func TestResponsesProviderIsRetryable(t *testing.T) {
	p := &responsesProvider{}
	for _, tt := range []struct {
		err  error
		want bool
	}{{&openai.Error{StatusCode: 429}, true}, {&openai.Error{StatusCode: 500}, true}, {&openai.Error{StatusCode: 503}, true}, {&openai.Error{StatusCode: 400}, false}, {&openai.Error{StatusCode: 403}, false}, {errors.New("rate limit reached"), true}} {
		assert.Equal(t, tt.want, p.IsRetryable(tt.err))
	}
}
