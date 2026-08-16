package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestParseProviderRetryDelay(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    time.Duration
		ok      bool
	}{
		{"retry-after seconds", map[string]string{"Retry-After": "3"}, 3 * time.Second, true},
		{"retry-after capped", map[string]string{"Retry-After": "900"}, maxRetryHint, true},
		{"retry-after zero unusable", map[string]string{"Retry-After": "0"}, 0, false},
		{"retry-after date in past", map[string]string{"Retry-After": "Sun, 06 Nov 1994 08:49:37 GMT"}, 0, false},
		{"reset requests duration", map[string]string{"X-Ratelimit-Reset-Requests": "6m5s"}, maxRetryHint, true},
		{"reset tokens duration", map[string]string{"X-Ratelimit-Reset-Tokens": "1s"}, time.Second, true},
		{"max of both resets", map[string]string{"X-Ratelimit-Reset-Requests": "1s", "X-Ratelimit-Reset-Tokens": "2s"}, 2 * time.Second, true},
		{"retry-after wins over resets", map[string]string{"Retry-After": "1", "X-Ratelimit-Reset-Tokens": "9s"}, time.Second, true},
		{"garbage unusable", map[string]string{"Retry-After": "soon", "X-Ratelimit-Reset-Tokens": "later"}, 0, false},
		{"no headers", map[string]string{}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			for key, value := range tt.headers {
				header.Set(key, value)
			}
			got, ok := parseProviderRetryDelay(header)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestWithRetryHint(t *testing.T) {
	t.Run("wraps api error with hint", func(t *testing.T) {
		apiErr := &openai.Error{StatusCode: 429, Response: &http.Response{Header: http.Header{"Retry-After": []string{"5"}}}}
		wrapped := withRetryHint(apiErr)
		rae, ok := errors.AsType[RetryAfterError](wrapped)
		require.True(t, ok)
		assert.Equal(t, 5*time.Second, rae.RetryAfter())
		// Unwrap chain keeps the typed API error visible to IsRetryable.
		unwrapped, ok := errors.AsType[*openai.Error](wrapped)
		require.True(t, ok)
		assert.Equal(t, 429, unwrapped.StatusCode)
	})
	t.Run("no hint leaves error unchanged", func(t *testing.T) {
		apiErr := &openai.Error{StatusCode: 429, Response: &http.Response{Header: http.Header{}}}
		assert.Same(t, apiErr, withRetryHint(apiErr))
	})
	t.Run("non-api error unchanged", func(t *testing.T) {
		plain := errors.New("boom")
		assert.Same(t, plain, withRetryHint(plain))
	})
}

// TestWithRetryClassifiedHonorsHint proves a RetryAfterError hint replaces
// the jitter backoff between attempts.
func TestWithRetryClassifiedHonorsHint(t *testing.T) {
	cfg := &Config{MaxRetries: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond}
	attempts := 0
	start := time.Now()
	_, err := withRetryClassified(context.Background(), cfg, NewLogger(LevelError), "hint", func(error) bool { return true },
		func(context.Context) (int, error) {
			attempts++
			if attempts == 1 {
				return 0, &retryHintError{err: errors.New("429"), delay: 50 * time.Millisecond}
			}
			return 1, nil
		})
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
	assert.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond)
}
