package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestOpenAIProvider creates a provider and server whose handler receives
// the raw chat-completions request body.
func newTestOpenAIProvider(t *testing.T, handler http.HandlerFunc) (*openaiProvider, *httptest.Server) {
	return newTestOpenAIProviderWithDialect(t, deepseekDialect{}, "deepseek-v4-pro", handler)
}

// newTestOpenAIProviderWithDialect creates a provider with the given dialect.
func newTestOpenAIProviderWithDialect(
	t *testing.T, dialect vendorDialect, model string, handler http.HandlerFunc,
) (*openaiProvider, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	p := &openaiProvider{
		client:  openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL), option.WithMaxRetries(0)),
		model:   model,
		dialect: dialect,
		logger:  NewLogger(LevelError),
	}
	return p, server
}

func TestQwenDialectRequestShape(t *testing.T) {
	tests := []struct {
		name  string
		req   GenerationRequest
		check func(t *testing.T, body map[string]any)
	}{
		{
			"thinking on with budget",
			GenerationRequest{Parts: []ContentPart{{Text: "user"}}, Thinking: ThinkingSpec{Enabled: true, Budget: 4096}},
			func(t *testing.T, body map[string]any) {
				assert.Equal(t, true, body["enable_thinking"])
				assert.Equal(t, float64(4096), body["thinking_budget"])
				_, exists := body["reasoning_effort"]
				assert.False(t, exists)
				_, exists = body["thinking"]
				assert.False(t, exists)
			},
		},
		{
			"thinking on without budget",
			GenerationRequest{Parts: []ContentPart{{Text: "user"}}, Thinking: ThinkingSpec{Enabled: true}},
			func(t *testing.T, body map[string]any) {
				assert.Equal(t, true, body["enable_thinking"])
				_, exists := body["thinking_budget"]
				assert.False(t, exists)
			},
		},
		{
			"thinking off",
			GenerationRequest{Parts: []ContentPart{{Text: "user"}}},
			func(t *testing.T, body map[string]any) { assert.Equal(t, false, body["enable_thinking"]) },
		},
		{
			"json mode disables thinking",
			GenerationRequest{Parts: []ContentPart{{Text: "user"}}, Thinking: ThinkingSpec{Enabled: true}, ResponseFormat: "json_object"},
			func(t *testing.T, body map[string]any) {
				assert.Equal(t, false, body["enable_thinking"])
				assert.Equal(t, "json_object", body["response_format"].(map[string]any)["type"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, server := newTestOpenAIProviderWithDialect(t, qwenDialect{}, "qwen3.7-max", func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				tt.check(t, body)
				writeCompletion(t, w, `{"model":"served","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"OK"}}]}`)
			})
			defer server.Close()
			_, err := p.Generate(context.Background(), tt.req)
			require.NoError(t, err)
		})
	}
}

func TestQwenThinkingEnabled(t *testing.T) {
	for _, tt := range []struct {
		name string
		req  GenerationRequest
		want bool
	}{
		{"disabled", GenerationRequest{}, false},
		{"enabled", GenerationRequest{Thinking: ThinkingSpec{Enabled: true}}, true},
		{"json mode guard", GenerationRequest{Thinking: ThinkingSpec{Enabled: true}, ResponseFormat: "json_object"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.want, qwenThinkingEnabled(tt.req)) })
	}
}

func writeCompletion(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(body))
	require.NoError(t, err)
}

func TestOpenAIProviderRequestShape(t *testing.T) {
	tests := []struct {
		name  string
		req   GenerationRequest
		check func(t *testing.T, body map[string]any)
	}{
		{
			name: "thinking enabled",
			req:  GenerationRequest{SystemPrompt: "system", Parts: []ContentPart{{Text: "user"}}, Thinking: ThinkingSpec{Enabled: true}, Temperature: 0.3},
			check: func(t *testing.T, body map[string]any) {
				assert.Equal(t, "max", body["reasoning_effort"])
				assert.Equal(t, "enabled", body["thinking"].(map[string]any)["type"])
				assert.Equal(t, "deepseek-v4-pro", body["model"])
				assert.Equal(t, 0.3, body["temperature"])
				messages := body["messages"].([]any)
				assert.Equal(t, "system", messages[0].(map[string]any)["role"])
				assert.Equal(t, "user", messages[1].(map[string]any)["role"])
				_, exists := body["max_completion_tokens"]
				assert.False(t, exists)
				_, exists = body["response_format"]
				assert.False(t, exists)
			},
		},
		{
			name: "thinking disabled json object and max tokens",
			req:  GenerationRequest{Parts: []ContentPart{{Text: "user"}}, ResponseFormat: "json_object", Temperature: 1, MaxOutputTokens: 1000},
			check: func(t *testing.T, body map[string]any) {
				assert.Equal(t, "disabled", body["thinking"].(map[string]any)["type"])
				_, exists := body["reasoning_effort"]
				assert.False(t, exists)
				assert.Equal(t, "json_object", body["response_format"].(map[string]any)["type"])
				assert.Equal(t, float64(1000), body["max_completion_tokens"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, server := newTestOpenAIProvider(t, func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				tt.check(t, body)
				writeCompletion(t, w, `{"model":"served","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"OK"}}]}`)
			})
			defer server.Close()
			_, err := p.Generate(context.Background(), tt.req)
			require.NoError(t, err)
		})
	}
}

func TestOpenAIProviderResponseParsing(t *testing.T) {
	tests := []struct {
		name, response, wantText, wantFinish string
		wantErr                              bool
	}{
		{
			"usage and reasoning",
			`{"model":"served",
"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,
"prompt_tokens_details":{"cached_tokens":2},"completion_tokens_details":{"reasoning_tokens":3}},
"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"answer","reasoning_content":"hidden"}}]}`,
			"answer", "stop", false,
		},
		{"zero choices", `{"model":"served","choices":[]}`, "", "", true},
		{"empty content", `{"model":"served","choices":[{"finish_reason":"length","message":{"role":"assistant","content":""}}]}`, "", "length", false},
		{
			"content filter first choice wins",
			`{"model":"served","choices":[{"finish_reason":"content_filter",
"message":{"role":"assistant","content":"first"}},
{"finish_reason":"stop","message":{"role":"assistant","content":"second"}}]}`,
			"first", "content_filter", false,
		},
		{
			"missing usage and invalid reasoning",
			`{"model":"served","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"answer","reasoning_content":7}}]}`,
			"answer", "stop", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, server := newTestOpenAIProvider(t, func(w http.ResponseWriter, _ *http.Request) { writeCompletion(t, w, tt.response) })
			defer server.Close()
			resp, err := p.Generate(context.Background(), GenerationRequest{Parts: []ContentPart{{Text: "user"}}})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantText, resp.Text)
			assert.Equal(t, tt.wantFinish, resp.FinishReason)
			if tt.name == "usage and reasoning" {
				assert.Equal(t, UsageInfo{PromptTokens: 10, OutputTokens: 5, ReasoningTokens: 3, CachedTokens: 2, TotalTokens: 15}, resp.Usage)
			}
		})
	}
}

func TestOpenAIProviderIsRetryable(t *testing.T) {
	p := &openaiProvider{}
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{"429", &openai.Error{StatusCode: 429}, true}, {"500", &openai.Error{StatusCode: 500}, true},
		{"503", &openai.Error{StatusCode: 503}, true}, {"400", &openai.Error{StatusCode: 400}, false},
		{"403", &openai.Error{StatusCode: 403}, false}, {"message fallback", errors.New("rate limit reached"), true},
	} {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.want, p.IsRetryable(tt.err)) })
	}
}

// TestDeepSeekLive exercises the configured DeepSeek backend when explicitly enabled.
func TestDeepSeekLive(t *testing.T) {
	if os.Getenv("PROVIDER") != "deepseek" || os.Getenv("PROVIDER_API_KEY") == "" {
		t.Skip("set PROVIDER=deepseek and PROVIDER_API_KEY to run the DeepSeek live test")
	}
	cfg, err := NewConfig(NewLogger(LevelError))
	require.NoError(t, err)
	p := newOpenAIProvider(cfg.Provider, deepseekDialect{}, NewLogger(LevelError))
	resp, err := p.Generate(context.Background(), GenerationRequest{Parts: []ContentPart{{Text: "Say OK"}}})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Text)
}

// TestQwenLive exercises the configured Qwen backend when explicitly enabled.
func TestQwenLive(t *testing.T) {
	if os.Getenv("PROVIDER") != "qwen" || os.Getenv("PROVIDER_API_KEY") == "" {
		t.Skip("set PROVIDER=qwen and PROVIDER_API_KEY to run the Qwen live test")
	}
	cfg, err := NewConfig(NewLogger(LevelError))
	require.NoError(t, err)
	p := newOpenAIProvider(cfg.Provider, qwenDialect{}, NewLogger(LevelError))
	resp, err := p.Generate(context.Background(), GenerationRequest{Parts: []ContentPart{{Text: "Say OK"}}})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Text)
}
