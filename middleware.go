package main

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewLoggerMiddleware creates a middleware function that ensures a logger is always
// present in the context passed to the handler function.
func NewLoggerMiddleware(handler server.ToolHandlerFunc, logger Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Add logger to context if not already present
		if ctx.Value(loggerKey) == nil {
			ctx = context.WithValue(ctx, loggerKey, logger)
		}

		return handler(ctx, req)
	}
}
