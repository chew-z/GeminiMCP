package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	t.Run("oauth metadata endpoint returns metadata and CORS headers for allowed origin", func(t *testing.T) {
		config := &Config{
			HTTPCORSEnabled: true,
			HTTPCORSOrigins: []string{"https://app.example.com"},
		}
		mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		handler := createCustomHTTPHandler(mcpHandler, config, NewLogger(LevelError))

		req := httptest.NewRequest(http.MethodGet, "http://mcp.local/.well-known/oauth-authorization-server", nil)
		req.Header.Set("Origin", "https://app.example.com")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))
		assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "GET, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "Content-Type, Authorization", rec.Header().Get("Access-Control-Allow-Headers"))

		var metadata map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &metadata))
		assert.Equal(t, "http://mcp.local", metadata["issuer"])
		assert.Equal(t, "http://mcp.local/oauth/authorize", metadata["authorization_endpoint"])
		assert.Equal(t, "http://mcp.local/oauth/token", metadata["token_endpoint"])
	})

	t.Run("wildcard origin is blocked when auth is enabled", func(t *testing.T) {
		config := &Config{
			HTTPCORSEnabled: true,
			HTTPCORSOrigins: []string{"*"},
			AuthEnabled:     true,
		}
		handler := createCustomHTTPHandler(http.NotFoundHandler(), config, NewLogger(LevelError))

		req := httptest.NewRequest(http.MethodGet, "http://mcp.local/.well-known/oauth-authorization-server", nil)
		req.Header.Set("Origin", "https://app.example.com")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("non-oauth routes are delegated to MCP handler", func(t *testing.T) {
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
