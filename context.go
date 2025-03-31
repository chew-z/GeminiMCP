package main

// contextKey is a type for context keys to prevent collisions
type contextKey string

// Context keys
const (
	// loggerKey is the context key for the logger
	loggerKey contextKey = "logger"
	// configKey is the context key for the configuration
	configKey contextKey = "config"
)
