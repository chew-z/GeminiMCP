package main

import (
	"bytes"
	"github.com/google/generative-ai-go/genai"
)

// newTestLogger creates a logger that writes to the provided buffer
func newTestLogger(buf *bytes.Buffer) Logger {
	return &StandardLogger{
		level:  LevelDebug,
		writer: buf,
	}
}

// MockGeminiResponse creates a mock Gemini API response for testing
func MockGeminiResponse(content string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text(content),
					},
					Role: "model",
				},
			},
		},
	}
}
