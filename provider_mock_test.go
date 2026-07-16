package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	generateFn  func(context.Context, GenerationRequest) (*GenerationResponse, error)
	retryableFn func(error) bool
	mu          sync.Mutex
	calls       []GenerationRequest
}

type capturedLogEntry struct{ level, message string }
type captureLogger struct {
	mu      sync.Mutex
	entries []capturedLogEntry
}

func (c *captureLogger) record(level, format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, capturedLogEntry{level, fmt.Sprintf(format, args...)})
}
func (c *captureLogger) Debug(format string, args ...any) { c.record("DEBUG", format, args...) }
func (c *captureLogger) Info(format string, args ...any)  { c.record("INFO", format, args...) }
func (c *captureLogger) Warn(format string, args ...any)  { c.record("WARN", format, args...) }
func (c *captureLogger) Error(format string, args ...any) { c.record("ERROR", format, args...) }
func (c *captureLogger) snapshot() []capturedLogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]capturedLogEntry, len(c.entries))
	copy(out, c.entries)
	return out
}

func toolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	return text.Text
}

func setupEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for key, value := range env {
		t.Setenv(key, value)
	}
}

func withCleanEnv(t *testing.T) {
	t.Helper()
	original := append([]string(nil), os.Environ()...)
	os.Clearenv()
	t.Cleanup(func() {
		os.Clearenv()
		for _, item := range original {
			key, value, ok := strings.Cut(item, "=")
			if ok {
				_ = os.Setenv(key, value)
			}
		}
	})
}

func (p *mockProvider) Generate(ctx context.Context, req GenerationRequest) (*GenerationResponse, error) {
	p.mu.Lock()
	p.calls = append(p.calls, req)
	p.mu.Unlock()
	if p.generateFn != nil {
		return p.generateFn(ctx, req)
	}
	return &GenerationResponse{Text: "ok", FinishReason: "STOP"}, nil
}

func (p *mockProvider) IsRetryable(err error) bool {
	if p.retryableFn != nil {
		return p.retryableFn(err)
	}
	return false
}

func (p *mockProvider) requests() []GenerationRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	requests := make([]GenerationRequest, len(p.calls))
	copy(requests, p.calls)
	return requests
}
