package main

import (
	"context"
	"errors"
	"fmt"
)

// ProviderConfig contains credentials and endpoint settings for a selected
// model provider.
type ProviderConfig struct {
	Vendor  string
	APIKey  string
	BaseURL string
	Model   string
}

// deepseekModels is the temporary DeepSeek model allowlist for Phase 2.
var deepseekModels = []string{"deepseek-v4-pro"}

// qwenModels is the Qwen model allowlist for Phase 3.
var qwenModels = []string{"qwen3.7-max", "qwen3.7-plus"}

// NewProvider creates the configured model provider.
func NewProvider(cfg *Config, logger Logger) (Provider, error) {
	if cfg == nil {
		return nil, errors.New("config cannot be nil")
	}

	switch cfg.Provider.Vendor {
	case "deepseek":
		return newOpenAIProvider(cfg.Provider, deepseekDialect{}, logger), nil
	case "qwen":
		return newOpenAIProvider(cfg.Provider, qwenDialect{}, logger), nil
	default:
		return nil, fmt.Errorf("unsupported provider vendor %q; valid values: deepseek, qwen", cfg.Provider.Vendor)
	}
}

// Provider is the minimal LLM-backend seam. Implementations own all
// vendor-specific request shaping (thinking, JSON mode, temperature) and
// error classification; the rest of the server speaks only these types.
type Provider interface {
	Generate(ctx context.Context, req GenerationRequest) (*GenerationResponse, error)
	// IsRetryable classifies a Generate error as transient. It is consulted
	// by withRetryClassified after the universal rules (context errors are
	// terminal, transport errors are retryable) have been applied.
	IsRetryable(err error) bool
}

// ThinkingSpec controls model reasoning for a single request.
type ThinkingSpec struct {
	Enabled bool
	Budget  int32 // optional token budget; 0 = provider default
}

// ContentPart is one element of the user-turn envelope. Exactly one of Text
// or File is set.
type ContentPart struct {
	Text string
	File *FileContent
}

// FileContent carries a binary attachment for providers that support one.
type FileContent struct {
	Name string
	MIME string
	Data []byte
}

// GenerationRequest is a provider-agnostic generation call.
type GenerationRequest struct {
	SystemPrompt    string
	Parts           []ContentPart
	Thinking        ThinkingSpec
	ResponseFormat  string // "" (plain text) or "json_object"
	Temperature     float64
	MaxOutputTokens int32 // 0 = omit; API default applies
}

// UsageInfo carries token accounting from a generation response.
type UsageInfo struct {
	PromptTokens    int32
	OutputTokens    int32
	ReasoningTokens int32
	CachedTokens    int32
	TotalTokens     int32
}

// GenerationResponse is the provider-agnostic result of a generation call.
type GenerationResponse struct {
	Text         string
	FinishReason string // vendor finish reason; "" or "STOP"/"stop" means normal
	Model        string // concrete model version that served the request
	Usage        UsageInfo
}

// finishReasonNormal reports whether a finish reason indicates a normal,
// complete generation (as opposed to truncation, safety filtering, etc.).
func finishReasonNormal(reason string) bool {
	switch reason {
	case "", "STOP", "stop":
		return true
	}
	return false
}
