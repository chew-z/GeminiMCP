package main

import (
	"context"
	"errors"
	"fmt"
)

// NewGeminiServer creates a new GeminiServer with the provided configuration
func NewGeminiServer(ctx context.Context, config *Config) (*GeminiServer, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	provider, err := NewProvider(config, getLoggerFromContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	return &GeminiServer{
		config:   config,
		provider: provider,
	}, nil
}
