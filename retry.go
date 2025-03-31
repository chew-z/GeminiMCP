package main

import (
	"context"
	"errors"
	"strings"
	"time"
)

// RetryWithBackoff attempts to execute a function multiple times with exponential backoff
// between retries. It will retry only if the error matches the shouldRetry function.
func RetryWithBackoff(
	ctx context.Context,
	maxRetries int,
	initialBackoff time.Duration,
	maxBackoff time.Duration,
	operation func() error,
	shouldRetry func(error) bool,
	logger Logger,
) error {
	var err error
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Execute the operation
		err = operation()

		// If no error or we shouldn't retry this error, return
		if err == nil || !shouldRetry(err) {
			return err
		}

		// If this was our last attempt, return the error
		if attempt == maxRetries {
			return err
		}

		// Log retry attempt
		logger.Warn("Operation failed (attempt %d/%d): %v. Retrying in %v...",
			attempt+1, maxRetries+1, err, backoff)

		// Wait for backoff period or context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			// Continue with next attempt
		}

		// Increase backoff for next attempt, but don't exceed max
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return err
}

// IsTimeoutError returns true if the error appears to be related to a timeout
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// For errors that don't use standard Go error wrapping,
	// fall back to string checking
	errMsg := err.Error()
	return strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "deadline exceeded") ||
		strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection reset")
}
