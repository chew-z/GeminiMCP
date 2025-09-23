package main

import (
	"context"
	"errors"
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
			name:          "context canceled",
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
