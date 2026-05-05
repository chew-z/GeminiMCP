package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rpcRequest renders a JSON-RPC 2.0 request for HandleMessage.
func rpcRequest(t *testing.T, id any, method string, params any) json.RawMessage {
	t.Helper()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return raw
}

// dispatchRPC routes a JSON-RPC payload through MCPServer.HandleMessage and
// returns either a successful response or a JSON-RPC error envelope. Exactly
// one of the two return values is non-nil.
func dispatchRPC(
	t *testing.T,
	srv *server.MCPServer,
	raw json.RawMessage,
) (*mcp.JSONRPCResponse, *mcp.JSONRPCError) {
	t.Helper()
	msg := srv.HandleMessage(context.Background(), raw)
	require.NotNil(t, msg)
	encoded, err := json.Marshal(msg)
	require.NoError(t, err)

	var probe struct {
		Error *json.RawMessage `json:"error"`
	}
	require.NoError(t, json.Unmarshal(encoded, &probe))
	if probe.Error != nil {
		var rpcErr mcp.JSONRPCError
		require.NoError(t, json.Unmarshal(encoded, &rpcErr))
		return nil, &rpcErr
	}
	var rpcResp mcp.JSONRPCResponse
	require.NoError(t, json.Unmarshal(encoded, &rpcResp))
	return &rpcResp, nil
}

// decodeCallToolResult turns a JSON-RPC result payload into a CallToolResult.
func decodeCallToolResult(t *testing.T, resp *mcp.JSONRPCResponse) *mcp.CallToolResult {
	t.Helper()
	require.NotNil(t, resp)
	raw, err := json.Marshal(resp.Result)
	require.NoError(t, err)
	rawMsg := json.RawMessage(raw)
	result, err := mcp.ParseCallToolResult(&rawMsg)
	require.NoError(t, err)
	return result
}

// initializeResult drives an initialize handshake against srv and returns the
// parsed result. Used by TestServerInitializeMetadata to verify task
// capabilities and serverInfo metadata in one place.
func initializeResult(t *testing.T, srv *server.MCPServer) mcp.InitializeResult {
	t.Helper()
	raw := rpcRequest(t, 1, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "test-client",
			"version": "0.0.0",
		},
	})
	resp, rpcErr := dispatchRPC(t, srv, raw)
	require.Nil(t, rpcErr, "initialize returned a JSON-RPC error: %+v", rpcErr)

	encoded, err := json.Marshal(resp.Result)
	require.NoError(t, err)
	var initResult mcp.InitializeResult
	require.NoError(t, json.Unmarshal(encoded, &initResult))
	return initResult
}

// TestServerInitializeMetadata verifies that buildMCPServerOptions produces a
// server that advertises the documented task capabilities (Step 1) and the
// canonical website URL in serverInfo (Step 5). Assertions are pinned to the
// exact wire shape mcp-go 0.51 emits — TasksCapability uses presence-only
// pointer fields plus a nested Requests.Tools.Call marker.
func TestServerInitializeMetadata(t *testing.T) {
	logger := NewLogger(LevelError)
	cfg := &Config{MaxConcurrentTasks: 1}
	srv := server.NewMCPServer("gemini", "1.0.0", buildMCPServerOptions(cfg, logger)...)

	result := initializeResult(t, srv)

	require.NotNil(t, result.Capabilities.Tasks, "tasks capability not advertised")
	assert.NotNil(t, result.Capabilities.Tasks.List, "tasks.list flag missing")
	assert.NotNil(t, result.Capabilities.Tasks.Cancel, "tasks.cancel flag missing")
	require.NotNil(t, result.Capabilities.Tasks.Requests,
		"tasks.requests missing — toolCallTasks=true should advertise tool augmentation")
	require.NotNil(t, result.Capabilities.Tasks.Requests.Tools,
		"tasks.requests.tools missing")
	assert.NotNil(t, result.Capabilities.Tasks.Requests.Tools.Call,
		"tasks.requests.tools.call should be set when toolCallTasks=true")

	assert.Equal(t, serverWebsiteURL, result.ServerInfo.WebsiteURL,
		"serverInfo.websiteUrl should reflect WithWebsiteURL option")
}

// TestServerInitializeWithoutTasks verifies that the degraded-mode path (nil
// config / no concurrent tasks) does not advertise the tasks capability,
// confirming the conditional in buildMCPServerOptions still gates the
// WithTaskCapabilities option.
func TestServerInitializeWithoutTasks(t *testing.T) {
	logger := NewLogger(LevelError)
	srv := server.NewMCPServer("gemini", "1.0.0", buildMCPServerOptions(nil, logger)...)

	result := initializeResult(t, srv)
	assert.Nil(t, result.Capabilities.Tasks,
		"degraded mode (config==nil) must not advertise task capabilities")
	assert.Equal(t, serverWebsiteURL, result.ServerInfo.WebsiteURL)
}

// TestRecoveryShieldsPanickingHandler registers a deliberately-panicking tool
// against a server built from buildMCPServerOptions and asserts (Step 2) that
// (a) the call returns rather than crashing the process,
// (b) the response signals a non-success outcome through *either* IsError on
//
//	the CallToolResult *or* a JSON-RPC error envelope, and
//
// (c) a follow-up tools/list call on the same server still succeeds.
func TestRecoveryShieldsPanickingHandler(t *testing.T) {
	logger := NewLogger(LevelError)
	cfg := &Config{MaxConcurrentTasks: 1}
	srv := server.NewMCPServer("gemini", "1.0.0", buildMCPServerOptions(cfg, logger)...)

	panicTool := mcp.NewTool(
		"panic_tool",
		mcp.WithDescription("test-only tool that always panics"),
		mcp.WithString("query", mcp.Required()),
	)
	srv.AddTool(panicTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		panic("intentional test panic")
	})

	// Initialize first so the server enters a serving state for tools/call.
	_ = initializeResult(t, srv)

	callRaw := rpcRequest(t, 2, "tools/call", map[string]any{
		"name":      "panic_tool",
		"arguments": map[string]any{"query": "boom"},
	})
	resp, rpcErr := dispatchRPC(t, srv, callRaw)

	switch {
	case rpcErr != nil:
		// JSON-RPC envelope error path — also a valid recovery signal.
		assert.NotEmpty(t, rpcErr.Error.Message, "JSON-RPC error must carry a message")
	case resp != nil:
		toolResult := decodeCallToolResult(t, resp)
		assert.True(t, toolResult.IsError, "panic must surface as tool execution error")
	default:
		t.Fatalf("expected either a JSON-RPC error or a tool result, got neither")
	}

	// Session must remain usable after the recovered panic.
	listRaw := rpcRequest(t, 3, "tools/list", map[string]any{})
	listResp, listErr := dispatchRPC(t, srv, listRaw)
	require.Nil(t, listErr, "tools/list should succeed after recovered panic")
	require.NotNil(t, listResp)
}

// declaredHandlerParams maps each registered tool name to the parameter keys
// the handler code actually reads from request arguments. Mirrors the audit
// table in docs/reports/2026-05-05_dependency-bump-mcp-go-genai.md so that
// any future handler reading a new key without declaring it in tools.go
// fails this test rather than silently skipping schema validation.
var declaredHandlerParams = map[string][]string{
	"gemini_ask": {
		"query", "model", "github_repo", "github_ref", "github_files",
		"github_pr", "github_commits", "github_diff_base", "github_diff_head",
		"thinking_level",
	},
	"gemini_search": {
		"query", "model", "thinking_level", "start_time", "end_time",
	},
}

// schemaProperties extracts the declared parameter names from a Tool's
// inputSchema by marshalling it to JSON and re-decoding the relevant slice
// of fields. Avoids depending on any unexported mcp-go types.
func schemaProperties(t *testing.T, tool mcp.Tool) (map[string]any, any) {
	t.Helper()
	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)
	var decoded struct {
		Properties           map[string]any `json:"properties"`
		AdditionalProperties any            `json:"additionalProperties"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return decoded.Properties, decoded.AdditionalProperties
}

// TestToolSchemasCoverHandlerParams locks in the audit underlying Step 3:
// every key the handlers extract from request arguments must be declared in
// the tool's inputSchema, otherwise enabling
// WithInputSchemaValidation+additionalProperties=false would reject
// legitimate calls that the handlers silently consume.
func TestToolSchemasCoverHandlerParams(t *testing.T) {
	tools := map[string]mcp.Tool{
		"gemini_ask":    GeminiAskTool,
		"gemini_search": GeminiSearchTool,
	}

	for name, expected := range declaredHandlerParams {
		t.Run(name, func(t *testing.T) {
			tool, ok := tools[name]
			require.True(t, ok, "tool %q not found", name)
			props, additional := schemaProperties(t, tool)

			for _, key := range expected {
				assert.Contains(t, props, key,
					"handler reads %q for tool %q but inputSchema does not declare it",
					key, name)
			}
			// additionalProperties=false is required so undeclared keys are
			// rejected. mcp.WithSchemaAdditionalProperties(false) marshals
			// to a literal JSON false.
			assert.Equal(t, false, additional,
				"tool %q must set additionalProperties:false to lock down input shape", name)
		})
	}
}

// TestInputSchemaValidationRejectsBadCalls exercises WithInputSchemaValidation
// (Step 3) end-to-end: invalid arguments must produce a non-success response,
// while a syntactically valid call (registered against a no-op handler so we
// don't hit the real Gemini API) must succeed.
func TestInputSchemaValidationRejectsBadCalls(t *testing.T) {
	logger := NewLogger(LevelError)
	cfg := &Config{MaxConcurrentTasks: 1}
	srv := server.NewMCPServer("gemini", "1.0.0", buildMCPServerOptions(cfg, logger)...)

	// Register a stub handler against the real GeminiAskTool schema; the
	// handler is only reached on the accept-valid case below.
	stubHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("stub ok"), nil
	}
	srv.AddTool(GeminiAskTool, stubHandler)

	_ = initializeResult(t, srv)

	cases := []struct {
		name      string
		arguments map[string]any
		wantFail  bool
	}{
		{
			name: "rejects undeclared key",
			arguments: map[string]any{
				"query":     "hello",
				"not_a_key": "uh-oh",
			},
			wantFail: true,
		},
		{
			name: "rejects wrong-type github_pr",
			arguments: map[string]any{
				"query":       "hello",
				"github_pr":   "not-a-number",
				"github_repo": "owner/repo",
			},
			wantFail: true,
		},
		{
			name: "accepts a valid minimal payload",
			arguments: map[string]any{
				"query": "hello",
			},
			wantFail: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := rpcRequest(t, 10, "tools/call", map[string]any{
				"name":      "gemini_ask",
				"arguments": tc.arguments,
			})
			resp, rpcErr := dispatchRPC(t, srv, raw)

			if tc.wantFail {
				failed := false
				if rpcErr != nil {
					failed = true
				} else if resp != nil {
					toolResult := decodeCallToolResult(t, resp)
					failed = toolResult.IsError
				}
				assert.True(t, failed, "expected tool call to fail (IsError or JSON-RPC error)")
				return
			}

			require.Nil(t, rpcErr, "valid call must not return JSON-RPC error: %+v", rpcErr)
			require.NotNil(t, resp)
			toolResult := decodeCallToolResult(t, resp)
			assert.False(t, toolResult.IsError, "valid call should not surface IsError")
		})
	}
}
