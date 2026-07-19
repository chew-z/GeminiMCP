package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// ProviderConfig contains credentials and endpoint settings for a selected
// model provider.
type ProviderConfig struct {
	Vendor  string
	APIKey  string
	BaseURL string
	Model   string
}

// deepseekModels is the static allowlist of supported DeepSeek models.
var deepseekModels = []string{"deepseek-v4-pro"}

// qwenModels is the Qwen model allowlist.
var qwenModels = []string{"qwen3.7-max", "qwen3.7-plus", "qwen3.8-max-preview"}

// thinkingForcedQwenModels are thinking-only: DashScope rejects
// enable_thinking=false (reasoning effort none) with a 400 — confirmed
// empirically in production 2026-07-19. Per Alibaba's deep-thinking guide,
// qwen3.8-max-preview accepts only low/high/xhigh (aliases minimal/medium/max),
// defaults to xhigh with a 131072-token thinking budget, and long generations
// at top effort are expected behavior. The dialect serves these models at low
// effort (measured: review-class work in ~45s; high overruns multi-minute
// budgets on the same task) — see qwen_responses_dialect.go.
var thinkingForcedQwenModels = []string{"qwen3.8-max-preview"}

// NewProvider creates the configured model provider.
func NewProvider(cfg *Config, logger Logger) (Provider, error) {
	if cfg == nil {
		return nil, errors.New("config cannot be nil")
	}

	switch cfg.Provider.Vendor {
	case "deepseek":
		return newOpenAIProvider(cfg.Provider, deepseekDialect{}, logger), nil
	case "qwen":
		dialect := qwenResponsesDialect{thinkingForced: slices.Contains(thinkingForcedQwenModels, cfg.Provider.Model)}
		return newResponsesProvider(cfg.Provider, dialect, logger), nil
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
