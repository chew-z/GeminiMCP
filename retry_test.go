package main

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestWithRetry(t *testing.T) {
	cfg := &Config{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
	}
	logger := NewLogger(LevelDebug)

	tests := []struct {
		name          string
		fn            func(ctx context.Context) (int, error)
		wantVal       int
		wantErr       bool
		cancelContext bool
	}{
		{
			name: "success on first try",
			fn: func(ctx context.Context) (int, error) {
				return 1, nil
			},
			wantVal: 1,
			wantErr: false,
		},
		{
			name: "success after 2 retries",
			fn: func() func(context.Context) (int, error) {
				attempts := 0
				return func(ctx context.Context) (int, error) {
					attempts++
					if attempts <= 2 {
						return 0, errors.New("retryable error: unavailable")
					}
					return 1, nil
				}
			}(),
			wantVal: 1,
			wantErr: false,
		},
		{
			name: "failure after all retries",
			fn: func(ctx context.Context) (int, error) {
				return 0, errors.New("retryable error: unavailable")
			},
			wantErr: true,
		},
		{
			name: "non-retryable error",
			fn: func(ctx context.Context) (int, error) {
				return 0, errors.New("non-retryable error")
			},
			wantErr: true,
		},
		{
			name: "context canceled",
			fn: func(ctx context.Context) (int, error) {
				return 0, errors.New("retryable error: unavailable")
			},
			wantErr:       true,
			cancelContext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.cancelContext {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			val, err := withRetry(ctx, cfg, logger, "testOp", tt.fn)

			if (err != nil) != tt.wantErr {
				t.Errorf("withRetry() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if val != tt.wantVal {
				t.Errorf("withRetry() val = %v, want %v", val, tt.wantVal)
			}
		})
	}
}

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

var _ net.Error = timeoutNetError{}

func TestWithRetryClassified(t *testing.T) {
	cfg := &Config{MaxRetries: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond}
	logger := NewLogger(LevelError)
	tests := []struct {
		name         string
		err          error
		classifier   func(error) bool
		wantAttempts int
	}{
		{"custom 429 retries", errors.New("429"), func(error) bool { return true }, 2},
		{"custom 400 terminal", errors.New("400"), func(error) bool { return false }, 1},
		{"transport retries nil classifier", timeoutNetError{}, nil, 2},
		{"transport retries classifier", timeoutNetError{}, func(error) bool { return false }, 2},
		{"message fallback retries", errors.New("rate limit"), nil, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			_, err := withRetryClassified(context.Background(), cfg, logger, "test", tt.classifier, func(context.Context) (int, error) { attempts++; return 0, tt.err })
			if err == nil {
				t.Fatal("expected error")
			}
			if attempts != tt.wantAttempts {
				t.Fatalf("attempts=%d want=%d", attempts, tt.wantAttempts)
			}
		})
	}
	t.Run("context cancellation is terminal", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		attempts := 0
		_, err := withRetryClassified(ctx, cfg, logger, "test", func(error) bool { return true }, func(context.Context) (int, error) { attempts++; return 0, errors.New("429") })
		if !errors.Is(err, context.Canceled) || attempts != 0 {
			t.Fatalf("err=%v attempts=%d", err, attempts)
		}
	})
}
