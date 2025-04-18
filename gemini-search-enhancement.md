# GeminiMCP: gemini_search Enhancement Proposal

## Project Context
GeminiMCP is a Model Control Protocol (MCP) server integrating with Google's Gemini API. It provides three main tools:
- `gemini_ask`: For code analysis and general queries
- `gemini_search`: For grounded answers using Google Search
- `gemini_models`: Lists available Gemini models with capabilities

Recent enhancements added advanced model configuration options for `gemini_ask` that aren't yet available in `gemini_search`.

## Current Implementation Status

### gemini_ask Tool
- Supports advanced model configuration options:
  - `thinking`: Enables advanced reasoning capabilities with Gemini 2.5 models
  - `context_window_size`: Controls maximum token context size
- Implementation correctly:
  - Validates model capability for thinking support
  - Applies ThinkingConfig with IncludeThoughts: true
  - Caps context window size at model limits

### gemini_search Tool
- Currently only supports basic parameters:
  - `query`: The search query
  - `systemPrompt`: Custom system prompt
- Uses fixed model "gemini-2.0-flash" without access to advanced features
- Doesn't support thinking or context window size parameters

## Enhancement Proposal

Add support for `thinking` and `context_window_size` parameters to `gemini_search` for consistency and advanced capabilities:

1. Update the tool's input schema to include new parameters
2. Make model selection dynamic based on requested capabilities
3. Add parameter validation and proper config application

## Technical Implementation

### 1. Update Input Schema
```json
{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "The question to ask Gemini using Google Search for grounding"
    },
    "systemPrompt": {
      "type": "string",
      "description": "Optional: Custom system prompt to use for this request"
    },
    "thinking": {
      "type": "boolean",
      "description": "Optional: Enable advanced reasoning capabilities (requires thinking-capable models)"
    },
    "context_window_size": {
      "type": "integer",
      "description": "Optional: Maximum context window size in tokens (model-specific limits apply)"
    },
    "model": {
      "type": "string",
      "description": "Optional: Override default search model (supports thinking-capable models)"
    }
  },
  "required": ["query"]
}
```

### 2. Update Model Selection Logic
```go
// Default model selection for search
modelName := "gemini-2.0-flash"

// Extract thinking parameter if provided
useThinking := false
if thinkingParam, ok := req.Arguments["thinking"].(bool); ok && thinkingParam {
    useThinking = true
}

// If thinking is requested, use a thinking-capable model
if useThinking {
    // Use Gemini 2.5 Pro for thinking capabilities
    modelName = "gemini-2.5-pro-exp-03-25"
    logger.Info("Switching to %s to support thinking capability", modelName)
}

// Allow model override if specified
if customModel, ok := req.Arguments["model"].(string); ok && customModel != "" {
    // Validate the custom model
    if err := ValidateModelID(customModel); err != nil {
        logger.Error("Invalid model requested: %v", err)
        return createErrorResponse(fmt.Sprintf("Invalid model specified: %v", err)), nil
    }
    
    // Check thinking compatibility if requested
    if useThinking {
        model := GetModelByID(customModel)
        if model == nil || !model.ThinkingCapable {
            return createErrorResponse(fmt.Sprintf("Model %s doesn't support thinking capability", customModel)), nil
        }
    }
    
    logger.Info("Using request-specific model: %s", customModel)
    modelName = customModel
}
```

### 3. Apply Configuration Parameters
```go
// Create the generate content configuration
config := &genai.GenerateContentConfig{
    SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
    Temperature:       genai.Ptr(float32(s.config.GeminiTemperature)),
    Tools: []*genai.Tool{
        {
            GoogleSearch: &genai.GoogleSearch{},
        },
    },
}

// Apply thinking capability if requested
if useThinking {
    model := GetModelByID(modelName)
    if model != nil && model.ThinkingCapable {
        config.ThinkingConfig = &genai.ThinkingConfig{
            IncludeThoughts: true,
        }
        logger.Debug("Enabled thinking capability for model %s", modelName)
    }
}

// Extract and apply context window size if provided
contextWindowSize := 0
if windowSizeRaw, ok := req.Arguments["context_window_size"].(float64); ok {
    contextWindowSize = int(windowSizeRaw)
    
    // Validate against model limits
    model := GetModelByID(modelName)
    if model != nil && contextWindowSize > model.ContextWindowSize {
        logger.Warn("Requested context window size %d exceeds model limit of %d tokens, capping at model limit",
            contextWindowSize, model.ContextWindowSize)
        contextWindowSize = model.ContextWindowSize
    }
    
    if contextWindowSize > 0 {
        config.MaxOutputTokens = int32(contextWindowSize)
        logger.Debug("Set context window size to %d tokens for model %s", contextWindowSize, modelName)
    }
}
```

## Next Steps

1. Implement the proposed changes to the `gemini_search` tool
2. Add relevant test cases to ensure proper functionality
3. Update documentation to reflect new capabilities
4. Consider potential limitations when using thinking capability with search functionality
5. Evaluate whether any modifications are needed to handle the returned structured data

This enhancement will provide a more consistent API across the MCP tools and allow users to leverage advanced reasoning capabilities for search-based queries.
