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

func TestBuildCORSOptions(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		wantNil     bool
		wantOrigins []string
		wantCreds   bool
		wantWarnFor []string // raw entries that should appear in a Warn log
	}{
		{
			name:    "disabled returns nil",
			config:  &Config{HTTPCORSEnabled: false, HTTPCORSOrigins: []string{"https://allowed.example"}},
			wantNil: true,
		},
		{
			name:    "empty origin list returns nil",
			config:  &Config{HTTPCORSEnabled: true},
			wantNil: true,
		},
		{
			name: "exact origin kept and trimmed",
			config: &Config{
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{"  https://allowed.example  "},
			},
			wantOrigins: []string{"https://allowed.example"},
		},
		{
			name: "wildcard kept when auth disabled",
			config: &Config{
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{"*"},
			},
			wantOrigins: []string{"*"},
		},
		{
			name: "wildcard dropped when auth enabled",
			config: &Config{
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{"*"},
				AuthEnabled:     true,
				AuthSecretKey:   "12345678901234567890123456789012",
			},
			wantNil:     true,
			wantWarnFor: []string{`"*"`},
		},
		{
			name: "wildcard subdomain forms all dropped",
			config: &Config{
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{
					"*.example.com",
					"https://*.example.com",
					"https://*.example.com:8080",
					"  *.example.com",
				},
			},
			wantNil: true,
			wantWarnFor: []string{
				`"*.example.com"`,
				`"https://*.example.com"`,
				`"https://*.example.com:8080"`,
			},
		},
		{
			name: "scheme-less host dropped",
			config: &Config{
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{"example.com"},
			},
			wantNil:     true,
			wantWarnFor: []string{`"example.com"`},
		},
		{
			name: "empty and whitespace entries dropped",
			config: &Config{
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{"", "   ", "https://allowed.example"},
			},
			wantOrigins: []string{"https://allowed.example"},
			wantWarnFor: []string{`""`, `"   "`},
		},
		{
			name: "auth enabled adds AllowCredentials",
			config: &Config{
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{"https://allowed.example"},
				AuthEnabled:     true,
				AuthSecretKey:   "12345678901234567890123456789012",
			},
			wantOrigins: []string{"https://allowed.example"},
			wantCreds:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cl := &captureLogger{}
			opts := buildCORSOptions(tc.config, cl)

			if tc.wantNil {
				assert.Nil(t, opts)
			} else {
				require.NotNil(t, opts)
			}

			cfg := &server.CORSConfig{}
			for _, o := range opts {
				o(cfg)
			}
			if !tc.wantNil {
				assert.Equal(t, tc.wantOrigins, cfg.AllowedOrigins)
				assert.Equal(t, 600, cfg.MaxAge)
				assert.Equal(t, tc.wantCreds, cfg.AllowCredentials)
				// Library defaults must remain unset so MCP-aware defaults apply.
				assert.Empty(t, cfg.AllowedHeaders, "AllowedHeaders should rely on library default")
				assert.Empty(t, cfg.ExposedHeaders, "ExposedHeaders should rely on library default")
				assert.Empty(t, cfg.AllowedMethods, "AllowedMethods should rely on library default")
			}

			for _, want := range tc.wantWarnFor {
				found := false
				for _, e := range cl.snapshot() {
					if e.level == "WARN" && strings.Contains(e.message, want) {
						found = true
						break
					}
				}
				assert.Truef(t, found, "expected WARN log mentioning %s", want)
			}
		})
	}
}

func TestStreamableHTTPCORS_Headers(t *testing.T) {
	const allowed = "https://allowed.example"

	newTestServer := func(t *testing.T, config *Config) *httptest.Server {
		t.Helper()
		mcpServer := server.NewMCPServer("gemini-test", "0.0.0")
		var opts []server.StreamableHTTPOption
		if corsOpts := buildCORSOptions(config, NewLogger(LevelError)); corsOpts != nil {
			opts = append(opts, server.WithStreamableHTTPCORS(corsOpts...))
		}
		if config.HTTPCORSEnabled || config.AuthEnabled {
			opts = append(opts, server.WithHTTPContextFunc(createHTTPMiddleware(config, NewLogger(LevelError))))
		}
		ts := server.NewTestStreamableHTTPServer(mcpServer, opts...)
		t.Cleanup(ts.Close)
		return ts
	}

	doPreflight := func(t *testing.T, ts *httptest.Server, origin string, requestHeaders string, authHeader string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodOptions, ts.URL+"/mcp", nil)
		require.NoError(t, err)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		if requestHeaders != "" {
			req.Header.Set("Access-Control-Request-Headers", requestHeaders)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	t.Run("realistic MCP preflight", func(t *testing.T) {
		ts := newTestServer(t, &Config{
			HTTPCORSEnabled: true,
			HTTPCORSOrigins: []string{allowed},
		})
		resp := doPreflight(t, ts, allowed,
			"Content-Type, Authorization, Mcp-Session-Id, Last-Event-ID, MCP-Protocol-Version", "")

		assert.Less(t, resp.StatusCode, 300, "preflight should be 2xx, got %d", resp.StatusCode)
		assert.Equal(t, allowed, resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "POST")
		assert.Equal(t, "600", resp.Header.Get("Access-Control-Max-Age"))

		allowHeaders := strings.ToLower(resp.Header.Get("Access-Control-Allow-Headers"))
		for _, h := range []string{"content-type", "authorization", "mcp-session-id", "last-event-id"} {
			assert.Containsf(t, allowHeaders, h, "Allow-Headers must advertise %q", h)
		}
		// Note: mcp-go v0.51 default Allow-Headers does not yet include
		// MCP-Protocol-Version; revisit on next mcp-go bump.
	})

	t.Run("simple cross-origin response", func(t *testing.T) {
		ts := newTestServer(t, &Config{
			HTTPCORSEnabled: true,
			HTTPCORSOrigins: []string{allowed},
		})
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"ping","id":1}`))
		require.NoError(t, err)
		req.Header.Set("Origin", allowed)
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, allowed, resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Contains(t, strings.ToLower(resp.Header.Get("Access-Control-Expose-Headers")), "mcp-session-id")
	})

	t.Run("disallowed origin gets no Allow-Origin", func(t *testing.T) {
		ts := newTestServer(t, &Config{
			HTTPCORSEnabled: true,
			HTTPCORSOrigins: []string{allowed},
		})
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"ping","id":1}`))
		require.NoError(t, err)
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
	})

	t.Run("wildcard subdomain dropped", func(t *testing.T) {
		ts := newTestServer(t, &Config{
			HTTPCORSEnabled: true,
			HTTPCORSOrigins: []string{"*.example.com"},
		})
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"ping","id":1}`))
		require.NoError(t, err)
		req.Header.Set("Origin", "https://api.example.com")
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
	})

	t.Run("auth enabled wildcard rejected", func(t *testing.T) {
		ts := newTestServer(t, &Config{
			HTTPCORSEnabled: true,
			HTTPCORSOrigins: []string{"*"},
			AuthEnabled:     true,
			AuthSecretKey:   "12345678901234567890123456789012",
		})
		resp := doPreflight(t, ts, "https://anywhere.example", "Content-Type", "")
		assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
	})

	t.Run("credentials flag follows AuthEnabled", func(t *testing.T) {
		t.Run("auth enabled emits Allow-Credentials", func(t *testing.T) {
			ts := newTestServer(t, &Config{
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{allowed},
				AuthEnabled:     true,
				AuthSecretKey:   "12345678901234567890123456789012",
			})
			resp := doPreflight(t, ts, allowed, "Content-Type", "")
			assert.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
		})
		t.Run("auth disabled omits Allow-Credentials", func(t *testing.T) {
			ts := newTestServer(t, &Config{
				HTTPCORSEnabled: true,
				HTTPCORSOrigins: []string{allowed},
			})
			resp := doPreflight(t, ts, allowed, "Content-Type", "")
			assert.Empty(t, resp.Header.Get("Access-Control-Allow-Credentials"))
		})
	})

	t.Run("preflight succeeds without Authorization even when auth is enabled", func(t *testing.T) {
		ts := newTestServer(t, &Config{
			HTTPCORSEnabled: true,
			HTTPCORSOrigins: []string{allowed},
			AuthEnabled:     true,
			AuthSecretKey:   "12345678901234567890123456789012",
		})
		resp := doPreflight(t, ts, allowed, "Content-Type, Authorization, Mcp-Session-Id", "")
		assert.Less(t, resp.StatusCode, 300, "preflight must succeed without Authorization, got %d", resp.StatusCode)
		assert.Equal(t, allowed, resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
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
