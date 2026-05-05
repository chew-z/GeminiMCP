package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/mark3labs/mcp-go/server"
)

// startHTTPServer starts the HTTP transport server
func startHTTPServer(ctx context.Context, mcpServer *server.MCPServer, config *Config, logger Logger) error {
	// Create HTTP server options
	var opts []server.StreamableHTTPOption

	// Configure heartbeat if enabled
	if config.HTTPHeartbeat > 0 {
		opts = append(opts, server.WithHeartbeatInterval(config.HTTPHeartbeat))
	}

	// Configure stateless mode
	if config.HTTPStateless {
		opts = append(opts, server.WithStateLess(true))
	}

	// Configure endpoint path
	opts = append(opts, server.WithEndpointPath(config.HTTPPath))

	// Add HTTP context function for CORS, logging, and authentication
	if config.HTTPCORSEnabled || config.AuthEnabled {
		opts = append(opts, server.WithHTTPContextFunc(createHTTPMiddleware(config, logger)))
	}

	// Create streamable HTTP server
	httpServer := server.NewStreamableHTTPServer(mcpServer, opts...)

	// Create custom HTTP server with OAuth well-known endpoint
	customServer := &http.Server{
		Addr:         config.HTTPAddress,
		Handler:      createCustomHTTPHandler(httpServer, config, logger),
		ReadTimeout:  config.HTTPTimeout,
		WriteTimeout: config.HTTPWriteTimeout,
		IdleTimeout:  config.HTTPTimeout * 2, // Typically longer
	}

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	// Start server in goroutine
	wg.Go(func() {
		if err := customServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed to start: %v", err)
			cancel()
		}
	})

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		logger.Info("Received signal %v, shutting down HTTP server...", sig)
	case <-ctx.Done():
		logger.Info("Context cancelled, shutting down HTTP server...")
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), config.HTTPTimeout)
	defer shutdownCancel()

	if err := customServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error: %v", err)
		return err
	}

	wg.Wait()
	logger.Info("HTTP server stopped")
	return nil
}

// createHTTPMiddleware creates an HTTP context function with CORS, logging, and authentication
func createHTTPMiddleware(config *Config, logger Logger) server.HTTPContextFunc {
	// Create authentication middleware
	var authMiddleware *AuthMiddleware
	if config.AuthEnabled {
		authMiddleware = NewAuthMiddleware(config.AuthSecretKey, config.AuthEnabled, logger)
		logger.Info("HTTP authentication enabled")
	}

	return func(ctx context.Context, r *http.Request) context.Context {
		// Per-request INFO line was noisy (twice per MCP call). We log at
		// DEBUG; the tool-entry line in wrapHandlerWithLogger captures the
		// authenticated user + request ID that operators actually need.
		logger.Debug("HTTP %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Apply authentication middleware if enabled
		if authMiddleware != nil {
			// Create a wrapper function for the next middleware step
			nextFunc := func(ctx context.Context, r *http.Request) context.Context {
				return ctx
			}
			// Apply authentication middleware
			ctx = authMiddleware.HTTPContextFunc(nextFunc)(ctx, r)
		}

		// Add CORS headers if enabled
		if config.HTTPCORSEnabled {
			// Check if request origin is allowed
			origin := r.Header.Get("Origin")
			if origin != "" && isOriginAllowed(origin, config.HTTPCORSOrigins, config.AuthEnabled) {
				// Note: We can't set response headers directly here as this is a context function
				// CORS headers would need to be handled at the HTTP server level
				logger.Info("CORS: Origin %s is allowed", origin)
			}
		}

		// Add request info to context
		ctx = context.WithValue(ctx, httpMethodKey, r.Method)
		ctx = context.WithValue(ctx, httpPathKey, r.URL.Path)
		ctx = context.WithValue(ctx, httpRemoteAddrKey, r.RemoteAddr)

		return ctx
	}
}

// isOriginAllowed checks if the origin is in the allowed list, with special handling for auth
func isOriginAllowed(origin string, allowedOrigins []string, authEnabled bool) bool {
	originHost, err := normalizeOriginHost(origin)
	if err != nil {
		return false
	}

	for _, allowed := range allowedOrigins {
		if allowed == "*" {
			if authEnabled {
				continue // Do not allow wildcard origin if auth is enabled
			}
			return true
		}
		if allowed == origin {
			return true
		}
		// Support wildcard subdomains (e.g., "*.example.com")
		if domain, ok := strings.CutPrefix(allowed, "*."); ok {
			patternHost := extractHostname(domain)
			if patternHost == "" {
				continue
			}
			if isSubdomainMatch(originHost, patternHost) {
				return true
			}
		}
	}
	return false
}

func normalizeOriginHost(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname()), nil
	}

	// Be tolerant of host-only values and treat them like regular hostnames.
	u, err = url.Parse("//" + strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid origin: %q", raw)
	}

	return strings.ToLower(u.Hostname()), nil
}

func extractHostname(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	trimmed = strings.TrimPrefix(trimmed, "//")
	host, _, err := net.SplitHostPort(trimmed)
	if err == nil {
		trimmed = host
	}

	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(trimmed)), ".")
}

func isSubdomainMatch(originHost, patternHost string) bool {
	if originHost == "" || patternHost == "" {
		return false
	}
	if originHost == patternHost {
		return false
	}
	return strings.HasSuffix(originHost, "."+patternHost)
}

// resolvePublicURL returns the canonical resource identifier for the RFC 9728
// metadata document. Returns ("", false) when the request cannot produce a
// spec-compliant identifier — caller must respond 503 in that case.
//
// HTTPPublicURL was already validated at startup (https any host, or http only
// for loopback) and is returned verbatim. The fallback branch enforces the same
// rule at request time using r.TLS and (when trust is enabled)
// X-Forwarded-Proto.
func resolvePublicURL(config *Config, r *http.Request) (string, bool) {
	if config.HTTPPublicURL != "" {
		return config.HTTPPublicURL, true
	}
	scheme := "http"
	switch {
	case r.TLS != nil:
		scheme = "https"
	case config.HTTPTrustForwardedProto:
		if r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
	}
	if scheme == "http" && !isLoopbackHost(r.Host) {
		return "", false
	}
	return scheme + "://" + r.Host, true
}

// createCustomHTTPHandler creates a custom HTTP handler that includes the
// RFC 9728 protected-resource metadata endpoint.
func createCustomHTTPHandler(mcpHandler http.Handler, config *Config, logger Logger) http.Handler {
	mux := http.NewServeMux()

	// RFC 9728 OAuth Protected Resource Metadata. Mounted only when JWT auth
	// is enabled — otherwise the resource has nothing meaningful to advertise.
	if config.AuthEnabled {
		registerProtectedResourceMetadata(mux, config, logger)
	}

	// Handle all other requests with the MCP handler
	mux.Handle("/", mcpHandler)

	return mux
}

// registerProtectedResourceMetadata wires the RFC 9728 endpoint at the
// host-rooted path and (when HTTPPublicURL has a non-empty path) at the
// spec-canonical path-suffixed form via mcp-go's helper.
func registerProtectedResourceMetadata(mux *http.ServeMux, config *Config, logger Logger) {
	handle := func(w http.ResponseWriter, r *http.Request) {
		resource, ok := resolvePublicURL(config, r)
		if !ok {
			logger.Warn("9728 metadata refused: cannot derive HTTPS resource for host %q "+
				"(set GEMINI_HTTP_PUBLIC_URL or GEMINI_HTTP_TRUST_FORWARDED_PROTO=true)", r.Host)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		metadata := map[string]any{
			"resource":                 resource,
			"bearer_methods_supported": []string{"header"},
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if config.HTTPCORSEnabled {
			origin := r.Header.Get("Origin")
			if origin != "" && isOriginAllowed(origin, config.HTTPCORSOrigins, config.AuthEnabled) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
		}
		if err := json.NewEncoder(w).Encode(metadata); err != nil {
			logger.Error("Failed to encode 9728 metadata: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}
	mux.HandleFunc("/.well-known/oauth-protected-resource", handle)
	if config.HTTPPublicURL == "" {
		return
	}
	u, err := url.Parse(config.HTTPPublicURL)
	if err != nil {
		logger.Error("invalid GEMINI_HTTP_PUBLIC_URL %q: %v — only host-rooted 9728 path will be served",
			config.HTTPPublicURL, err)
		return
	}
	if u.Path != "" && u.Path != "/" {
		mux.HandleFunc(server.ProtectedResourceMetadataPath(config.HTTPPublicURL), handle)
	}
}
