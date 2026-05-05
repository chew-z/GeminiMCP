package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsOriginAllowed(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		allowedOrigins []string
		authEnabled    bool
		expected       bool
	}{
		{
			name:           "wildcard allowed when auth disabled",
			origin:         "https://app.example.com",
			allowedOrigins: []string{"*"},
			authEnabled:    false,
			expected:       true,
		},
		{
			name:           "wildcard rejected when auth enabled",
			origin:         "https://app.example.com",
			allowedOrigins: []string{"*"},
			authEnabled:    true,
			expected:       false,
		},
		{
			name:           "exact origin match",
			origin:         "https://app.example.com",
			allowedOrigins: []string{"https://app.example.com"},
			authEnabled:    true,
			expected:       true,
		},
		{
			name:           "wildcard subdomain match",
			origin:         "https://api.example.com",
			allowedOrigins: []string{"*.example.com"},
			authEnabled:    false,
			expected:       true,
		},
		{
			name:           "wildcard matches nested subdomain",
			origin:         "https://a.api.example.com",
			allowedOrigins: []string{"*.example.com"},
			authEnabled:    false,
			expected:       true,
		},
		{
			name:           "wildcard does not match suffix boundary",
			origin:         "https://evil-example.com",
			allowedOrigins: []string{"*.example.com"},
			authEnabled:    false,
			expected:       false,
		},
		{
			name:           "wildcard does not match extension with extra suffix",
			origin:         "https://verybadexample.com",
			allowedOrigins: []string{"*.example.com"},
			authEnabled:    false,
			expected:       false,
		},
		{
			name:           "wildcard does not match host with suffix-like tail",
			origin:         "https://evil.example.com.evil",
			allowedOrigins: []string{"*.example.com"},
			authEnabled:    false,
			expected:       false,
		},
		{
			name:           "wildcard requires one subdomain label",
			origin:         "https://example.com",
			allowedOrigins: []string{"*.example.com"},
			authEnabled:    false,
			expected:       false,
		},
		{
			name:           "origin not allowed",
			origin:         "https://app.example.org",
			allowedOrigins: []string{"https://app.example.com", "*.example.net"},
			authEnabled:    false,
			expected:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isOriginAllowed(tc.origin, tc.allowedOrigins, tc.authEnabled))
		})
	}
}

func TestCreateHTTPMiddleware(t *testing.T) {
	logger := NewLogger(LevelError)
	secret := "12345678901234567890123456789012"
	auth := NewAuthMiddleware(secret, true, logger)
	token, err := auth.GenerateToken("u-1", "alice", "admin", 1)
	require.NoError(t, err)

	tests := []struct {
		name          string
		config        *Config
		authHeader    string
		origin        string
		expectedAuth  bool
		expectedError string
	}{
		{
			name: "auth enabled with valid token",
			config: &Config{
				AuthEnabled:     true,
				AuthSecretKey:   secret,
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{"https://allowed.example"},
			},
			authHeader:    "Bearer " + token,
			origin:        "https://allowed.example",
			expectedAuth:  true,
			expectedError: "",
		},
		{
			name: "auth enabled with missing token",
			config: &Config{
				AuthEnabled:     true,
				AuthSecretKey:   secret,
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{"https://allowed.example"},
			},
			origin:        "https://allowed.example",
			expectedAuth:  false,
			expectedError: "missing_token",
		},
		{
			name: "auth disabled",
			config: &Config{
				AuthEnabled:     false,
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{"*"},
			},
			expectedAuth:  false,
			expectedError: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://mcp.local/mcp", nil)
			req.RemoteAddr = "203.0.113.10:4242"
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}

			middleware := createHTTPMiddleware(tc.config, logger)
			ctx := middleware(context.Background(), req)

			assert.Equal(t, http.MethodPost, ctx.Value(httpMethodKey))
			assert.Equal(t, "/mcp", ctx.Value(httpPathKey))
			assert.Equal(t, "203.0.113.10:4242", ctx.Value(httpRemoteAddrKey))
			assert.Equal(t, tc.expectedAuth, isAuthenticated(ctx))
			assert.Equal(t, tc.expectedError, getAuthError(ctx))

			if tc.expectedAuth {
				userID, username, role := getUserInfo(ctx)
				assert.Equal(t, "u-1", userID)
				assert.Equal(t, "alice", username)
				assert.Equal(t, "admin", role)
			}
		})
	}
}

func TestCreateCustomHTTPHandler(t *testing.T) {
	t.Run("non-well-known routes are delegated to MCP handler", func(t *testing.T) {
		mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/mcp", r.URL.Path)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("mcp-ok"))
		})
		handler := createCustomHTTPHandler(mcpHandler, &Config{}, NewLogger(LevelError))

		req := httptest.NewRequest(http.MethodPost, "http://mcp.local/mcp", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusAccepted, rec.Code)
		assert.Equal(t, "mcp-ok", rec.Body.String())
	})
}

func TestProtectedResourceMetadata(t *testing.T) {
	t.Run("returns expected fields with HTTPPublicURL configured", func(t *testing.T) {
		config := &Config{
			AuthEnabled:   true,
			HTTPPublicURL: "https://example.test/gemini",
		}
		handler := createCustomHTTPHandler(http.NotFoundHandler(), config, NewLogger(LevelError))

		req := httptest.NewRequest(http.MethodGet, "http://mcp.local/.well-known/oauth-protected-resource", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))

		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "https://example.test/gemini", body["resource"])
		assert.Equal(t, []any{"header"}, body["bearer_methods_supported"])
	})

	t.Run("reachable at the path-suffixed canonical URL", func(t *testing.T) {
		config := &Config{
			AuthEnabled:   true,
			HTTPPublicURL: "https://example.test/gemini",
		}
		handler := createCustomHTTPHandler(http.NotFoundHandler(), config, NewLogger(LevelError))

		req := httptest.NewRequest(http.MethodGet, "http://mcp.local/.well-known/oauth-protected-resource/gemini", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "https://example.test/gemini", body["resource"])
	})

	t.Run("uses https when r.TLS is set", func(t *testing.T) {
		config := &Config{AuthEnabled: true}
		handler := createCustomHTTPHandler(http.NotFoundHandler(), config, NewLogger(LevelError))

		ts := httptest.NewTLSServer(handler)
		defer ts.Close()

		resp, err := ts.Client().Get(ts.URL + "/.well-known/oauth-protected-resource")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		resource, _ := body["resource"].(string)
		assert.True(t, strings.HasPrefix(resource, "https://"), "expected https:// scheme, got %q", resource)
	})

	t.Run("refuses non-loopback http requests", func(t *testing.T) {
		config := &Config{AuthEnabled: true, HTTPTrustForwardedProto: false}
		handler := createCustomHTTPHandler(http.NotFoundHandler(), config, NewLogger(LevelError))

		req := httptest.NewRequest(http.MethodGet, "http://remote.example/.well-known/oauth-protected-resource", nil)
		req.Host = "remote.example"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("permits loopback http for local dev", func(t *testing.T) {
		config := &Config{AuthEnabled: true}
		handler := createCustomHTTPHandler(http.NotFoundHandler(), config, NewLogger(LevelError))

		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/.well-known/oauth-protected-resource", nil)
		req.Host = "127.0.0.1:8080"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "http://127.0.0.1:8080", body["resource"])
	})

	t.Run("ignores X-Forwarded-Proto when trust is off", func(t *testing.T) {
		config := &Config{AuthEnabled: true, HTTPTrustForwardedProto: false}
		handler := createCustomHTTPHandler(http.NotFoundHandler(), config, NewLogger(LevelError))

		req := httptest.NewRequest(http.MethodGet, "http://remote.example/.well-known/oauth-protected-resource", nil)
		req.Host = "remote.example"
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("honours X-Forwarded-Proto when trust is on", func(t *testing.T) {
		config := &Config{AuthEnabled: true, HTTPTrustForwardedProto: true}
		handler := createCustomHTTPHandler(http.NotFoundHandler(), config, NewLogger(LevelError))

		req := httptest.NewRequest(http.MethodGet, "http://remote.example/.well-known/oauth-protected-resource", nil)
		req.Host = "remote.example"
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "https://remote.example", body["resource"])
	})

	t.Run("returns 404 when AuthEnabled is false", func(t *testing.T) {
		config := &Config{AuthEnabled: false, HTTPPublicURL: "https://example.test/gemini"}
		handler := createCustomHTTPHandler(http.NotFoundHandler(), config, NewLogger(LevelError))

		for _, p := range []string{
			"/.well-known/oauth-protected-resource",
			"/.well-known/oauth-protected-resource/gemini",
		} {
			req := httptest.NewRequest(http.MethodGet, "http://mcp.local"+p, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusNotFound, rec.Code, "path %s", p)
		}
	})

	t.Run("sets CORS headers for allowed origin", func(t *testing.T) {
		config := &Config{
			AuthEnabled:     true,
			HTTPPublicURL:   "https://example.test/gemini",
			HTTPCORSEnabled: true,
			HTTPCORSOrigins: []string{"https://app.example.com"},
		}
		handler := createCustomHTTPHandler(http.NotFoundHandler(), config, NewLogger(LevelError))

		req := httptest.NewRequest(http.MethodGet, "http://mcp.local/.well-known/oauth-protected-resource", nil)
		req.Header.Set("Origin", "https://app.example.com")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "GET, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "Content-Type, Authorization", rec.Header().Get("Access-Control-Allow-Headers"))
	})
}

func TestStartHTTPServer(t *testing.T) {
	mcpServer := server.NewMCPServer("gemini", "1.0.0")
	config := &Config{
		HTTPAddress:     "127.0.0.1:0",
		HTTPPath:        "/mcp",
		HTTPTimeout:     200 * time.Millisecond,
		HTTPCORSEnabled: true,
		HTTPCORSOrigins: []string{"*"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := startHTTPServer(ctx, mcpServer, config, NewLogger(LevelError))
	require.NoError(t, err)
}
