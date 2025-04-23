package main

import "github.com/mark3labs/mcp-go/mcp"

// DEPRECATED: These internal types are legacy and will be removed once the transition to direct handlers is complete

// internalCallToolRequest is the internal representation of a tool request
// DEPRECATED: Will be removed once all handlers use mcp.CallToolRequest directly
type internalCallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// internalCallToolResponse is the internal representation of a tool response
// DEPRECATED: Will be removed once all handlers use mcp.CallToolResult directly
type internalCallToolResponse struct {
	IsError bool                  `json:"isError"`
	Content []internalToolContent `json:"content,omitempty"`
}

// internalToolContent is a content item in an internal tool response
// DEPRECATED: Will be removed once all handlers use mcp.Content directly
type internalToolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// convertToInternalRequest converts an mcp.CallToolRequest to our internal request type
// DEPRECATED: Will be removed once all handlers use mcp.CallToolRequest directly
func convertToInternalRequest(mcpReq *mcp.CallToolRequest) *internalCallToolRequest {
	return &internalCallToolRequest{
		Name:      mcpReq.Params.Name,
		Arguments: mcpReq.Params.Arguments,
	}
}

// convertToMCPResult converts our internal response to mcp.CallToolResult
// DEPRECATED: Will be removed once all handlers use mcp.CallToolResult directly
func convertToMCPResult(protoResp *internalCallToolResponse) *mcp.CallToolResult {
	result := &mcp.CallToolResult{
		IsError: protoResp.IsError,
	}

	// Convert content items
	for _, content := range protoResp.Content {
		switch content.Type {
		case "text":
			result.Content = append(result.Content, mcp.NewTextContent(content.Text))
			// Note: We only handle text content for now
		}
	}

	return result
}
