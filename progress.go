package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// progressNotificationMethod is the MCP-spec method name for progress
// notifications. mcp-go does not export a constant for it, so we use the
// literal string — same pattern mcp-go itself uses for list_changed events.
const progressNotificationMethod = "notifications/progress"

// progressEmitter is the minimal surface startProgressReporter needs from
// *server.MCPServer. Declared as an interface so tests can pass a recording
// stub; *server.MCPServer satisfies it structurally.
type progressEmitter interface {
	SendNotificationToClient(ctx context.Context, method string, params map[string]any) error
}

// startProgressReporter begins emitting spec-compliant notifications/progress
// at `interval` until the returned stop() is called or ctx is cancelled.
// No-op (returns a no-op stop) when the client did not request progress via
// _meta.progressToken, when the server is not retrievable from ctx, or when
// interval <= 0.
func startProgressReporter(
	ctx context.Context,
	req mcp.CallToolRequest,
	interval time.Duration,
	total float64,
	label string,
	logger Logger,
) (stop func()) {
	noop := func() {}

	if interval <= 0 {
		return noop
	}
	if req.Params.Meta == nil || req.Params.Meta.ProgressToken == nil {
		return noop
	}
	srv := server.ServerFromContext(ctx)
	if srv == nil {
		return noop
	}
	return startProgressReporterWithEmitter(ctx, srv, req.Params.Meta.ProgressToken, interval, total, label, logger)
}

// startProgressReporterWithEmitter is the testable core: it takes an explicit
// progressEmitter instead of reading *server.MCPServer from ctx. Callers in
// production go through startProgressReporter; tests pass a recording stub.
func startProgressReporterWithEmitter(
	ctx context.Context,
	emitter progressEmitter,
	token mcp.ProgressToken,
	interval time.Duration,
	total float64,
	label string,
	logger Logger,
) (stop func()) {
	done := make(chan struct{})
	var once sync.Once
	stop = func() {
		once.Do(func() { close(done) })
	}

	start := time.Now()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				elapsed := time.Since(start).Seconds()
				params := map[string]any{
					"progressToken": token,
					"progress":      elapsed,
					"total":         total,
					"message":       fmt.Sprintf("%s — %.0fs elapsed", label, elapsed),
				}
				if err := emitter.SendNotificationToClient(ctx, progressNotificationMethod, params); err != nil {
					if logger != nil {
						logger.Debug("progress notification dropped: %v", err)
					}
				}
			}
		}
	}()

	return stop
}
