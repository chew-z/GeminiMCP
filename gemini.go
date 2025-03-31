package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gomcpgo/mcp/pkg/protocol"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// GeminiServer implements the ToolHandler interface for Gemini API interactions
type GeminiServer struct {
	config *Config
	client *genai.Client
}

// NewGeminiServer creates a new GeminiServer with the provided configuration
func NewGeminiServer(ctx context.Context, config *Config) (*GeminiServer, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	if config.GeminiAPIKey == "" {
		return nil, errors.New("Gemini API key is required")
	}

	// Initialize the Gemini client
	client, err := genai.NewClient(ctx, option.WithAPIKey(config.GeminiAPIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &GeminiServer{
		config: config,
		client: client,
	}, nil
}

// Close closes the Gemini client connection
func (s *GeminiServer) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

// ListTools implements the ToolHandler interface for GeminiServer
func (s *GeminiServer) ListTools(ctx context.Context) (*protocol.ListToolsResponse, error) {
	return &protocol.ListToolsResponse{
		Tools: []protocol.Tool{
			{
				Name:        "ask_gemini",
				Description: "Use Google's Gemini AI model to ask about complex coding problems",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"query": {
							"type": "string",
							"description": "The coding problem that we are asking Gemini AI to work on [question + code]"
						},
						"model": {
							"type": "string",
							"description": "Optional: Specific Gemini model to use (overrides default configuration)"
						},
						"systemPrompt": {
							"type": "string",
							"description": "Optional: Custom system prompt to use for this request (overrides default configuration)"
						}
					},
					"required": ["query"]
				}`),
			},
			{
				Name:        "gemini_models",
				Description: "List available Gemini models with descriptions",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {},
					"required": []
				}`),
			},
		},
	}, nil
}

// getLoggerFromContext safely extracts a logger from the context or creates a new one
func getLoggerFromContext(ctx context.Context) Logger {
	loggerValue := ctx.Value(loggerKey)
	if loggerValue != nil {
		if l, ok := loggerValue.(Logger); ok {
			return l
		}
	}
	// Create a new logger if one isn't in the context or type assertion fails
	return NewLogger(LevelInfo)
}

// createErrorResponse creates a standardized error response
func createErrorResponse(message string) *protocol.CallToolResponse {
	return &protocol.CallToolResponse{
		IsError: true,
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: message,
			},
		},
	}
}

// CallTool implements the ToolHandler interface for GeminiServer
func (s *GeminiServer) CallTool(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	// No need to get logger here as it's not used in this method
	switch req.Name {
	case "ask_gemini":
		return s.handleAskGemini(ctx, req)
	case "gemini_models":
		return s.handleGeminiModels(ctx)
	default:
		return createErrorResponse(fmt.Sprintf("unknown tool: %s", req.Name)), nil
	}
}

// handleAskGemini handles requests to the ask_gemini tool
func (s *GeminiServer) handleAskGemini(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)

	// Extract and validate query parameter (required)
	query, ok := req.Arguments["query"].(string)
	if !ok {
		return createErrorResponse("query must be a string"), nil
	}

	// Extract optional model parameter
	modelName := s.config.GeminiModel
	if customModel, ok := req.Arguments["model"].(string); ok && customModel != "" {
		// Validate the custom model
		if err := ValidateModelID(customModel); err != nil {
			logger.Error("Invalid model requested: %v", err)
			return createErrorResponse(fmt.Sprintf("Invalid model specified: %v", err)), nil
		}
		logger.Info("Using request-specific model: %s", customModel)
		modelName = customModel
	}

	// Extract optional systemPrompt parameter
	systemPrompt := s.config.GeminiSystemPrompt
	if customPrompt, ok := req.Arguments["systemPrompt"].(string); ok && customPrompt != "" {
		logger.Info("Using request-specific system prompt")
		systemPrompt = customPrompt
	}

	// Create Gemini model with configuration
	model := s.client.GenerativeModel(modelName)
	model.SystemInstruction = genai.NewUserContent(genai.Text(systemPrompt))

	// Use the configured temperature
	model.SetTemperature(float32(s.config.GeminiTemperature))
	logger.Debug("Using temperature: %v for model %s", s.config.GeminiTemperature, modelName)

	// Send request to Gemini API
	response, err := s.executeGeminiRequest(ctx, model, query)
	if err != nil {
		logger.Error("Gemini API error: %v", err)
		return createErrorResponse(fmt.Sprintf("error from Gemini API: %v", err)), nil
	}

	return s.formatResponse(response), nil
}

// handleGeminiModels handles requests to the gemini_models tool
func (s *GeminiServer) handleGeminiModels(ctx context.Context) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Listing available Gemini models")

	// Get available models
	models := GetAvailableGeminiModels()

	// Create a formatted response using strings.Builder with error handling
	var formattedContent strings.Builder

	// Define a helper function to write with error checking
	writeStringf := func(format string, args ...interface{}) error {
		_, err := formattedContent.WriteString(fmt.Sprintf(format, args...))
		return err
	}

	// Write the header
	if err := writeStringf("# Available Gemini Models\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	// Write each model's information
	for _, model := range models {
		if err := writeStringf("## %s\n", model.Name); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		if err := writeStringf("- ID: `%s`\n", model.ID); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		if err := writeStringf("- Description: %s\n\n", model.Description); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}
	}

	// Add usage hint
	if err := writeStringf("## Usage\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("You can specify a model ID in the `model` parameter when using the `ask_gemini` tool:\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("```json\n{\n  \"query\": \"Your question here\",\n  \"model\": \"gemini-2.5-pro-exp-03-25\"\n}\n```\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: formattedContent.String(),
			},
		},
	}, nil
}

// executeGeminiRequest makes the request to the Gemini API with retry capability
func (s *GeminiServer) executeGeminiRequest(ctx context.Context, model *genai.GenerativeModel, query string) (*genai.GenerateContentResponse, error) {
	logger := getLoggerFromContext(ctx)

	var response *genai.GenerateContentResponse

	// Define the operation to retry
	operation := func() error {
		var err error
		// Set timeout context for the API call
		timeoutCtx, cancel := context.WithTimeout(ctx, s.config.HTTPTimeout)
		defer cancel()

		response, err = model.GenerateContent(timeoutCtx, genai.Text(query))
		if err != nil {
			// Check specifically for timeout errors
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("request timed out after %v: consider increasing GEMINI_TIMEOUT: %w", s.config.HTTPTimeout, err)
			}

			// Handle other types of errors
			return fmt.Errorf("failed to generate content: %w", err)
		}

		// Check for empty response
		if response == nil || len(response.Candidates) == 0 {
			return errors.New("no response candidates returned from Gemini API")
		}

		return nil
	}

	// Execute the operation with retry logic
	err := RetryWithBackoff(
		ctx,
		s.config.MaxRetries,
		s.config.InitialBackoff,
		s.config.MaxBackoff,
		operation,
		IsTimeoutError, // Using the IsTimeoutError from retry.go
		logger,
	)

	if err != nil {
		return nil, err
	}

	return response, nil
}

// formatResponse formats the Gemini API response
func (s *GeminiServer) formatResponse(resp *genai.GenerateContentResponse) *protocol.CallToolResponse {
	var content string

	// Extract text from the response
	for _, candidate := range resp.Candidates {
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				// Use type assertion for text parts
				if textPart, ok := part.(genai.Text); ok {
					content += string(textPart)
				}
			}
		}
	}

	// Check for empty content and provide a fallback message
	if content == "" {
		content = "The Gemini model returned an empty response. This might indicate that the model couldn't generate an appropriate response for your query. Please try rephrasing your question or providing more context."
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: content,
			},
		},
	}
}
