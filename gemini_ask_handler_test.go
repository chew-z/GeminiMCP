package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestParseAskRequest(t *testing.T) {
	tests := []struct {
		name          string
		req           mcp.CallToolRequest
		wantQuery     string
		wantModelName string
		wantErr       bool
	}{
		{
			name: "valid request",
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "gemini_ask",
					Arguments: map[string]interface{}{
						"query": "test query",
					},
				},
			},
			wantQuery:     "test query",
			wantModelName: "gemini-2.5-pro", // Assuming default
			wantErr:       false,
		},
		{
			name: "missing query",
			req: mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "gemini_ask",
					Arguments: map[string]interface{}{
						"query": "",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &GeminiServer{
				config: &Config{GeminiModel: "gemini-2.5-pro"},
			}
			query, _, modelName, err := s.parseAskRequest(context.Background(), tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseAskRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if query != tt.wantQuery {
				t.Errorf("parseAskRequest() query = %v, want %v", query, tt.wantQuery)
			}
			if modelName != tt.wantModelName {
				t.Errorf("parseAskRequest() modelName = %v, want %v", modelName, tt.wantModelName)
			}
		})
	}
}
