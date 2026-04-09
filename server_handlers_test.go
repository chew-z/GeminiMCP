package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnforceHTTPAuth(t *testing.T) {
	logger := NewLogger(LevelError)

	t.Run("non-http context skips auth", func(t *testing.T) {
		err := enforceHTTPAuth(context.Background(), "tool", "gemini_ask", logger)
		require.NoError(t, err)
	})

	t.Run("http context with auth error returns error", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), httpMethodKey, "POST")
		ctx = context.WithValue(ctx, authErrorKey, "missing_token")

		err := enforceHTTPAuth(ctx, "tool", "gemini_ask", logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication required: missing_token")
	})

	t.Run("authenticated http context succeeds", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), httpMethodKey, "POST")
		ctx = context.WithValue(ctx, authenticatedKey, true)
		ctx = context.WithValue(ctx, userIDKey, "u-1")
		ctx = context.WithValue(ctx, usernameKey, "alice")
		ctx = context.WithValue(ctx, userRoleKey, "admin")

		err := enforceHTTPAuth(ctx, "tool", "gemini_ask", logger)
		require.NoError(t, err)
	})
}

func TestWrapHandlerWithLogger(t *testing.T) {
	logger := NewLogger(LevelError)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "gemini_ask",
			Arguments: map[string]interface{}{"query": "test"},
		},
	}

	t.Run("auth failure short-circuits handler", func(t *testing.T) {
		called := false
		handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			called = true
			return &mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent("ok")}}, nil
		}
		wrapped := wrapHandlerWithLogger(handler, "gemini_ask", logger)

		ctx := context.WithValue(context.Background(), httpMethodKey, "POST")
		ctx = context.WithValue(ctx, authErrorKey, "missing_token")
		result, err := wrapped(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, toolResultText(t, result), "authentication required: missing_token")
		assert.False(t, called)
	})

	t.Run("successful handler is passed through", func(t *testing.T) {
		expected := &mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent("ok")}}
		handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return expected, nil
		}
		wrapped := wrapHandlerWithLogger(handler, "gemini_ask", logger)

		result, err := wrapped(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("handler error is passed through", func(t *testing.T) {
		expected := &mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent("partial")}}
		expectedErr := errors.New("handler failed")
		handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return expected, expectedErr
		}
		wrapped := wrapHandlerWithLogger(handler, "gemini_ask", logger)

		result, err := wrapped(context.Background(), req)

		assert.Equal(t, expected, result)
		assert.Equal(t, expectedErr, err)
	})
}

func TestWrapPromptHandlerWithLogger(t *testing.T) {
	logger := NewLogger(LevelError)
	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name:      "code-review",
			Arguments: map[string]string{"problem_statement": "check this"},
		},
	}

	t.Run("auth failure returns empty prompt result", func(t *testing.T) {
		called := false
		handler := func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			called = true
			return &mcp.GetPromptResult{Description: "ok"}, nil
		}
		wrapped := wrapPromptHandlerWithLogger(handler, "code-review", logger)

		ctx := context.WithValue(context.Background(), httpMethodKey, "POST")
		ctx = context.WithValue(ctx, authErrorKey, "missing_token")
		result, err := wrapped(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Contains(t, result.Description, "authentication required: missing_token")
		assert.Empty(t, result.Messages)
		assert.False(t, called)
	})

	t.Run("successful prompt handler is passed through", func(t *testing.T) {
		expected := &mcp.GetPromptResult{
			Description: "ok",
			Messages:    []mcp.PromptMessage{},
		}
		handler := func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return expected, nil
		}
		wrapped := wrapPromptHandlerWithLogger(handler, "code-review", logger)

		result, err := wrapped(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("prompt handler error is passed through", func(t *testing.T) {
		expected := &mcp.GetPromptResult{Description: "partial"}
		expectedErr := errors.New("prompt failed")
		handler := func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return expected, expectedErr
		}
		wrapped := wrapPromptHandlerWithLogger(handler, "code-review", logger)

		result, err := wrapped(context.Background(), req)

		assert.Equal(t, expected, result)
		assert.Equal(t, expectedErr, err)
	})
}

func TestRegisterErrorTools(t *testing.T) {
	mcpServer := server.NewMCPServer("gemini", "1.0.0")
	errorServer := &ErrorGeminiServer{errorMessage: "initialization failed"}
	registerErrorTools(mcpServer, errorServer, NewLogger(LevelError))

	tools := mcpServer.ListTools()
	require.Len(t, tools, 3)

	for _, toolName := range []string{"gemini_ask", "gemini_search", "gemini_models"} {
		t.Run(toolName, func(t *testing.T) {
			tool := mcpServer.GetTool(toolName)
			require.NotNil(t, tool)

			ctx := context.WithValue(context.Background(), loggerKey, NewLogger(LevelError))
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      toolName,
					Arguments: map[string]interface{}{},
				},
			}
			result, err := tool.Handler(ctx, req)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError)
			assert.Contains(t, toolResultText(t, result), "Error in tool '"+toolName+"': initialization failed")
		})
	}
}

func TestSetupGeminiServerFailures(t *testing.T) {
	t.Run("fails when logger is missing in context", func(t *testing.T) {
		mcpServer := server.NewMCPServer("gemini", "1.0.0")

		err := setupGeminiServer(context.Background(), mcpServer, &Config{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "logger not found in context")
	})

	t.Run("fails when config is nil", func(t *testing.T) {
		mcpServer := server.NewMCPServer("gemini", "1.0.0")
		ctx := context.WithValue(context.Background(), loggerKey, NewLogger(LevelError))

		err := setupGeminiServer(ctx, mcpServer, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create Gemini service: config cannot be nil")
	})

	t.Run("fails when Gemini API key is missing", func(t *testing.T) {
		mcpServer := server.NewMCPServer("gemini", "1.0.0")
		ctx := context.WithValue(context.Background(), loggerKey, NewLogger(LevelError))

		err := setupGeminiServer(ctx, mcpServer, &Config{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create Gemini service: gemini API key is required")
	})
}
