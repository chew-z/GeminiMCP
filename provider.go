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

// prequalifyModelForVendor maps each vendor to the cheap, stable model used
// for prequalification. Server-owned policy, not user configuration: the
// classification call is a ~2s JSON single-token task that needs neither a
// max-tier model nor thinking. Confirmed 2026-07-19: running the prequalify
// call on the same qwen3.8-max-preview instance wedged the follow-up
// generation in production (>300s, 8/8), while prequalifying on qwen3.7-plus
// before a 3.8 generation works.
var prequalifyModelForVendor = map[string]string{
	"deepseek": "deepseek-v4-flash",
	"qwen":     "qwen3.7-plus",
}

// NewPrequalifyProvider creates the lightweight provider used only for
// query prequalification. It shares the vendor, credentials, and endpoint of
// the main provider but pins the vendor's cheap model. The returned dialect
// is never thinking-forced, so prequalify requests keep their fast
// effort=none / non-thinking shape.
func NewPrequalifyProvider(cfg *Config, logger Logger) (Provider, error) {
	if cfg == nil {
		return nil, errors.New("config cannot be nil")
	}
	model, ok := prequalifyModelForVendor[cfg.Provider.Vendor]
	if !ok {
		return nil, fmt.Errorf("no prequalify model defined for vendor %q", cfg.Provider.Vendor)
	}
	pcfg := cfg.Provider
	pcfg.Model = model
	switch pcfg.Vendor {
	case "deepseek":
		return newOpenAIProvider(pcfg, deepseekDialect{}, logger), nil
	case "qwen":
		return newResponsesProvider(pcfg, qwenResponsesDialect{}, logger), nil
	default:
		return nil, fmt.Errorf("unsupported provider vendor %q; valid values: deepseek, qwen", pcfg.Vendor)
	}
}

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
