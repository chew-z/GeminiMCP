package main

import (
	"context"

	"github.com/gomcpgo/mcp/pkg/handler"
	"github.com/gomcpgo/mcp/pkg/protocol"
)

// LoggerMiddleware wraps a ToolHandler and ensures that a logger is always
// present in the context passed to its methods.
type LoggerMiddleware struct {
	handler handler.ToolHandler
	logger  Logger
}

// NewLoggerMiddleware creates a new LoggerMiddleware that wraps the provided handler.
func NewLoggerMiddleware(handler handler.ToolHandler, logger Logger) *LoggerMiddleware {
	return &LoggerMiddleware{
		handler: handler,
		logger:  logger,
	}
}

// ListTools implements the ToolHandler interface by ensuring a logger is in the context.
func (m *LoggerMiddleware) ListTools(ctx context.Context) (*protocol.ListToolsResponse, error) {
	// Add logger to context if not already present
	if ctx.Value(loggerKey) == nil {
		ctx = context.WithValue(ctx, loggerKey, m.logger)
	}

	return m.handler.ListTools(ctx)
}

// CallTool implements the ToolHandler interface by ensuring a logger is in the context.
func (m *LoggerMiddleware) CallTool(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	// Add logger to context if not already present
	if ctx.Value(loggerKey) == nil {
		ctx = context.WithValue(ctx, loggerKey, m.logger)
	}

	return m.handler.CallTool(ctx, req)
}
