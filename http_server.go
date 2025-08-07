package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
		Addr:    config.HTTPAddress,
		Handler: createCustomHTTPHandler(httpServer, config, logger),
	}

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	wg.Add(1)

	// Start server in goroutine
	go func() {
		defer wg.Done()
		if err := customServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed to start: %v", err)
			cancel()
		}
	}()

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
		// Log HTTP request
		logger.Info("HTTP %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

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
			if origin != "" && isOriginAllowed(origin, config.HTTPCORSOrigins) {
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

// isOriginAllowed checks if the origin is in the allowed list
func isOriginAllowed(origin string, allowedOrigins []string) bool {
	for _, allowed := range allowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
		// Support wildcard subdomains (e.g., "*.example.com")
		if strings.HasPrefix(allowed, "*.") {
			domain := strings.TrimPrefix(allowed, "*.")
			if strings.HasSuffix(origin, domain) {
				return true
			}
		}
	}
	return false
}

// createCustomHTTPHandler creates a custom HTTP handler that includes OAuth well-known endpoint
func createCustomHTTPHandler(mcpHandler http.Handler, config *Config, logger Logger) http.Handler {
	mux := http.NewServeMux()

	// Add OAuth well-known endpoint
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("OAuth well-known endpoint accessed from %s", r.RemoteAddr)

		// Create OAuth authorization server metadata
		metadata := map[string]interface{}{
			"issuer":                           fmt.Sprintf("http://%s", r.Host),
			"authorization_endpoint":           fmt.Sprintf("http://%s/oauth/authorize", r.Host),
			"token_endpoint":                   fmt.Sprintf("http://%s/oauth/token", r.Host),
			"response_types_supported":         []string{"code"},
			"grant_types_supported":            []string{"authorization_code"},
			"code_challenge_methods_supported": []string{"S256"},
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")

		// Add CORS headers if enabled
		if config.HTTPCORSEnabled {
			origin := r.Header.Get("Origin")
			if origin != "" && isOriginAllowed(origin, config.HTTPCORSOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
		}

		if err := json.NewEncoder(w).Encode(metadata); err != nil {
			logger.Error("Failed to encode OAuth metadata: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Handle all other requests with the MCP handler
	mux.Handle("/", mcpHandler)

	return mux
}
