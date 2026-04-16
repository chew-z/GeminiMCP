package main

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net"
	"strings"
	"time"

	"google.golang.org/api/googleapi"
)

// withRetry executes fn with configurable retries and exponential backoff with jitter.
// It returns the value from fn on success, or the last error if all retries fail.
func withRetry[T any](ctx context.Context, cfg *Config, logger Logger, opName string, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	maxAttempts := cfg.MaxRetries + 1
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	for attempt := range maxAttempts {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		val, err := fn(ctx)
		if err == nil {
			if attempt > 0 {
				logger.Info("%s succeeded after %d attempt(s)", opName, attempt+1)
			}
			return val, nil
		}

		// Do not retry non-retryable errors or on last attempt
		if !isRetryableError(err) || attempt == maxAttempts-1 {
			return zero, err
		}

		// Backoff with jitter
		delay := computeBackoff(cfg, attempt)
		logger.Warn("%s failed (attempt %d/%d): %v; retrying in %s", opName, attempt+1, maxAttempts, err, delay)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}

	// Unreachable
	return zero, errors.New("withRetry: exhausted attempts")
}

// computeBackoff calculates exponential backoff with full jitter.
func computeBackoff(cfg *Config, attempt int) time.Duration {
	// exp backoff: initial * 2^attempt, capped at MaxBackoff
	base := cfg.InitialBackoff
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	maxBackoff := cfg.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 10 * time.Second
	}
	// Growth
	mult := math.Pow(2, float64(attempt))
	d := time.Duration(float64(base) * mult)
	if d > maxBackoff {
		d = maxBackoff
	}
	// Full jitter in [0.5, 1.5]x
	jitter := 0.5 + rand.Float64()
	return time.Duration(float64(d) * jitter)
}

// isRetryableGoogleAPIError reports whether err is a *googleapi.Error with a
// retryable status code. The second return value (ok) is true when the error
// was a Google API error, signalling the caller to stop checking other
// heuristics.
func isRetryableGoogleAPIError(err error) (retryable, ok bool) {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return false, false
	}
	if gerr.Code == 429 || (gerr.Code >= 500 && gerr.Code <= 599) {
		return true, true
	}
	return false, true
}

// isRetryableByMessage applies best-effort string heuristics to detect
// transient failures that aren't surfaced through typed errors.
func isRetryableByMessage(err error) bool {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "429"),
		strings.Contains(msg, "rate limit"),
		strings.Contains(msg, "resource exhausted"),
		strings.Contains(msg, "unavailable"),
		strings.Contains(msg, "temporarily"),
		strings.Contains(msg, "timeout"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "eof"):
		return true
	default:
		return false
	}
}

// isRetryableError determines whether an error is transient.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var nerr net.Error
	if errors.As(err, &nerr) {
		if nerr.Timeout() {
			return true
		}
		type temporary interface{ Temporary() bool }
		if t, ok := any(nerr).(temporary); ok && t.Temporary() {
			return true
		}
	}

	if retryable, ok := isRetryableGoogleAPIError(err); ok {
		return retryable
	}

	return isRetryableByMessage(err)
}
