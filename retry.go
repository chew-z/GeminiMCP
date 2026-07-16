package main

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"net"
	"strings"
	"time"
)

// withRetry executes fn with configurable retries and exponential backoff with
// jitter, using the default transient-error classification.
// It returns the value from fn on success, or the last error if all retries fail.
func withRetry[T any](ctx context.Context, cfg *Config, logger Logger, opName string, fn func(context.Context) (T, error)) (T, error) {
	return withRetryClassified(ctx, cfg, logger, opName, nil, fn)
}

// withRetryClassified is withRetry with a pluggable retryability classifier.
// Universal rules always apply first: context.Canceled/DeadlineExceeded are
// terminal. Timed-out or temporary transport (net) errors are retryable; other
// transport errors fall through to the classifier. For everything else the
// classifier decides when non-nil (Provider.IsRetryable); a nil classifier
// falls back to the default message heuristics.
func withRetryClassified[T any](ctx context.Context, cfg *Config, logger Logger, opName string,
	isRetryable func(error) bool, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	maxAttempts := cfg.MaxRetries + 1
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	for attempt := range maxAttempts {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		logger.Debug("%s: attempt %d/%d", opName, attempt+1, maxAttempts)
		val, err := fn(ctx)
		if err == nil {
			logger.Debug("%s: attempt %d/%d succeeded", opName, attempt+1, maxAttempts)
			if attempt > 0 {
				logger.Info("%s succeeded after %d attempt(s)", opName, attempt+1)
			}
			return val, nil
		}

		// Do not retry non-retryable errors or on last attempt
		retryable := classifyRetryable(err, isRetryable)
		if !retryable || attempt == maxAttempts-1 {
			logger.Debug("%s: attempt %d/%d failed terminally (retryable=%v): %v",
				opName, attempt+1, maxAttempts, retryable, err)
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
	d := min(time.Duration(float64(base)*mult), maxBackoff)
	// Full jitter in [0.5, 1.5)x
	jitter := 0.5 + rand.Float64()
	return time.Duration(float64(d) * jitter)
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

// classifyRetryable applies the universal rules (context errors terminal,
// timed-out or temporary transport errors are retryable), then defers to the
// optional classifier. Other transport errors reach that classifier. A nil
// classifier keeps today's default: string-heuristic message matching.
func classifyRetryable(err error, isRetryable func(error) bool) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if nerr, ok := errors.AsType[net.Error](err); ok {
		if nerr.Timeout() {
			return true
		}
		type temporary interface{ Temporary() bool }
		if t, ok := any(nerr).(temporary); ok && t.Temporary() {
			return true
		}
	}

	if isRetryable != nil {
		return isRetryable(err)
	}
	return isRetryableByMessage(err)
}
