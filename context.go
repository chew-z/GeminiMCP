package main

// contextKey is a type for context keys to prevent collisions
type contextKey string

// Context keys
const (
	// loggerKey is the context key for the logger
	loggerKey contextKey = "logger"
	// configKey is the context key for the configuration
	configKey contextKey = "config"
	// authErrorKey is the context key for authentication errors
	authErrorKey contextKey = "auth_error"
	// authenticatedKey is the context key for authentication status
	authenticatedKey contextKey = "authenticated"
	// userIDKey is the context key for user ID
	userIDKey contextKey = "user_id"
	// usernameKey is the context key for username
	usernameKey contextKey = "username"
	// userRoleKey is the context key for user role
	userRoleKey contextKey = "user_role"
	// httpMethodKey is the context key for HTTP method
	httpMethodKey contextKey = "http_method"
	// httpPathKey is the context key for HTTP path
	httpPathKey contextKey = "http_path"
	// httpRemoteAddrKey is the context key for HTTP remote address
	httpRemoteAddrKey contextKey = "http_remote_addr"
)
