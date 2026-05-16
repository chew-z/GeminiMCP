package main

import (
	"context"
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
	httpServer := server.NewStreamableHTTPServer(mcpServer, buildStreamableHTTPOptions(config, logger)...)

	customServer := &http.Server{
		Addr:         config.HTTPAddress,
		Handler:      httpServer,
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

// buildStreamableHTTPOptions assembles the mcp-go StreamableHTTPOption set
// from the GeminiMCP Config. Extracted to keep startHTTPServer focused on
// lifecycle management.
func buildStreamableHTTPOptions(config *Config, logger Logger) []server.StreamableHTTPOption {
	var opts []server.StreamableHTTPOption

	if config.HTTPHeartbeat > 0 {
		opts = append(opts, server.WithHeartbeatInterval(config.HTTPHeartbeat))
	}
	if config.HTTPStateless {
		opts = append(opts, server.WithStateLess(true))
	}
	opts = append(opts, server.WithEndpointPath(config.HTTPPath))

	// CORS must be registered before the context function so preflight
	// (OPTIONS) short-circuits inside the CORS layer — the CORS spec
	// forbids browsers from sending Authorization on preflight.
	if corsOpts := buildCORSOptions(config, logger); corsOpts != nil {
		opts = append(opts, server.WithStreamableHTTPCORS(corsOpts...))
	}
	if config.HTTPCORSEnabled || config.AuthEnabled {
		opts = append(opts, server.WithHTTPContextFunc(createHTTPMiddleware(config, logger)))
	}

	// RFC 9728 OAuth Protected Resource Metadata. The library handler emits
	// Cache-Control: no-store (spec-aligned) and permissive CORS on the
	// .well-known endpoint. HTTPPublicURL is required at startup when auth is
	// on (Config validation + runMain guard); this branch defends against
	// hand-rolled Config values reaching the builder.
	if config.AuthEnabled {
		if config.HTTPPublicURL == "" {
			logger.Warn("RFC 9728 metadata endpoint skipped: AuthEnabled=true but HTTPPublicURL is empty — " +
				"this is a config bug; runtime metadata discovery will fail")
		} else {
			opts = append(opts, server.WithProtectedResourceMetadata(server.ProtectedResourceMetadataConfig{
				Resource:               config.HTTPPublicURL,
				BearerMethodsSupported: []string{"header"},
			}))
		}
	}
	return opts
}

// createHTTPMiddleware creates an HTTP context function with logging and authentication.
// CORS is handled separately via server.WithStreamableHTTPCORS.
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

		// Add request info to context
		ctx = context.WithValue(ctx, httpMethodKey, r.Method)
		ctx = context.WithValue(ctx, httpPathKey, r.URL.Path)
		ctx = context.WithValue(ctx, httpRemoteAddrKey, r.RemoteAddr)

		return ctx
	}
}

// buildCORSOptions translates the GeminiMCP CORS config into mcp-go's
// first-class CORS options. Returns nil when CORS is disabled or no usable
// origins remain after filtering — callers must skip WithStreamableHTTPCORS
// in that case so the library emits no Access-Control-* headers at all.
//
// Library defaults (from mcp-go v0.51) cover the headers/methods MCP browser
// clients need (Content-Type, Mcp-Session-Id, Last-Event-ID, Authorization;
// GET/POST/DELETE/OPTIONS; Mcp-Session-Id exposed). We deliberately do not
// override them so future protocol additions land automatically.
func buildCORSOptions(config *Config, logger Logger) []server.CORSOption {
	if !config.HTTPCORSEnabled || len(config.HTTPCORSOrigins) == 0 {
		return nil
	}

	filtered := make([]string, 0, len(config.HTTPCORSOrigins))
	dropped := 0
	for _, raw := range config.HTTPCORSOrigins {
		if entry, ok := acceptCORSOrigin(raw, config.AuthEnabled, logger); ok {
			filtered = append(filtered, entry)
		} else {
			dropped++
		}
	}

	if len(filtered) == 0 {
		logger.Info("CORS: no origins remain after filtering (%d dropped); CORS will not emit any Access-Control-* headers", dropped)
		return nil
	}

	logger.Info("CORS: %d origin(s) accepted, %d dropped", len(filtered), dropped)

	opts := []server.CORSOption{
		server.WithCORSAllowedOrigins(filtered...),
		server.WithCORSMaxAge(600),
	}
	// Auth uses the Authorization header, not cookies, but credentialed CORS
	// keeps the door open for future per-session bearer cookies. Reversible.
	if config.AuthEnabled {
		opts = append(opts, server.WithCORSAllowCredentials())
	}
	return opts
}

// acceptCORSOrigin trims and validates a single configured origin entry,
// returning (normalized, true) when the entry should be passed to mcp-go,
// or ("", false) with a Warn log when it should be dropped.
func acceptCORSOrigin(raw string, authEnabled bool, logger Logger) (string, bool) {
	entry := strings.TrimSpace(raw)
	if entry == "" {
		logger.Warn("CORS: dropping empty origin entry %q", raw)
		return "", false
	}
	if entry == "*" {
		if authEnabled {
			logger.Warn("CORS: dropping wildcard origin %q because authentication is enabled", entry)
			return "", false
		}
		return entry, true
	}
	if strings.Contains(corsOriginHost(entry), "*") {
		logger.Warn("CORS: dropping wildcard-subdomain origin %q "+
			"(mcp-go requires exact origin match — list specific origins or front them with a reverse proxy)", entry)
		return "", false
	}
	if !strings.Contains(entry, "://") {
		logger.Warn("CORS: dropping scheme-less origin %q "+
			"(browsers always send scheme in the Origin header — this entry would never match)", entry)
		return "", false
	}
	return entry, true
}

// corsOriginHost returns the host portion of an origin entry for wildcard
// detection. Falls back to manual scheme-stripping when url.Parse cannot
// extract a non-empty Host (e.g. "*.example.com" parses as a path-only URL).
func corsOriginHost(entry string) string {
	if u, err := url.Parse(entry); err == nil && u.Host != "" {
		return u.Host
	}
	rest := entry
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}
