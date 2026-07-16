package main

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/api/googleapi"
	"google.golang.org/genai"
)

// ProviderConfig contains credentials and endpoint settings for a selected
// model provider. Gemini continues to use its dedicated configuration fields.
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
func NewProvider(ctx context.Context, cfg *Config, logger Logger) (Provider, error) {
	if cfg == nil {
		return nil, errors.New("config cannot be nil")
	}

	switch cfg.Provider.Vendor {
	case "deepseek":
		return newOpenAIProvider(cfg.Provider, deepseekDialect{}, logger), nil
	case "qwen":
		return newOpenAIProvider(cfg.Provider, qwenDialect{}, logger), nil
	case "", "gemini":
		client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: cfg.GeminiAPIKey})
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini client: %w", err)
		}
		return NewGeminiProvider(client, cfg.GeminiModel), nil
	default:
		return nil, fmt.Errorf("unsupported provider vendor %q", cfg.Provider.Vendor)
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

// GeminiProvider adapts the google genai client to the Provider interface.
// TEMPORARY: it exists only as the rollback path during the provider
// migration (Phases 1–3) and is deleted in Phase 4.
type GeminiProvider struct {
	client *genai.Client
	model  string
}

// NewGeminiProvider wraps an initialized genai client.
func NewGeminiProvider(client *genai.Client, model string) *GeminiProvider {
	return &GeminiProvider{client: client, model: model}
}

// Generate maps a GenerationRequest onto genai.GenerateContent.
func (p *GeminiProvider) Generate(ctx context.Context, req GenerationRequest) (*GenerationResponse, error) {
	if p == nil || p.client == nil || p.client.Models == nil {
		return nil, errors.New("gemini provider not initialized")
	}

	config := buildGeminiConfig(req)
	parts := makeGeminiParts(req.Parts)
	contents := []*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)}

	resp, err := p.client.Models.GenerateContent(ctx, p.model, contents, config)
	if err != nil {
		return nil, err
	}
	return convertGeminiResponse(resp)
}

// buildGeminiConfig maps provider-agnostic generation options to Gemini options.
func buildGeminiConfig(req GenerationRequest) *genai.GenerateContentConfig {
	config := &genai.GenerateContentConfig{Temperature: new(float32(req.Temperature))}
	if req.SystemPrompt != "" {
		config.SystemInstruction = genai.NewContentFromText(req.SystemPrompt, "")
	}
	if req.Thinking.Enabled {
		config.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: true,
			ThinkingLevel:   genai.ThinkingLevelHigh,
		}
	}
	if req.ResponseFormat == "json_object" {
		config.ResponseMIMEType = "application/json"
	}
	if req.MaxOutputTokens > 0 {
		config.MaxOutputTokens = req.MaxOutputTokens
	}

	return config
}

// makeGeminiParts maps provider content parts to Gemini content parts.
func makeGeminiParts(contentParts []ContentPart) []*genai.Part {
	parts := make([]*genai.Part, 0, len(contentParts))
	for _, cp := range contentParts {
		switch {
		case cp.File != nil:
			parts = append(parts, genai.NewPartFromBytes(cp.File.Data, cp.File.MIME))
		case cp.Text != "":
			parts = append(parts, genai.NewPartFromText(cp.Text))
		}
	}
	return parts
}

// convertGeminiResponse maps a Gemini response to the provider response type.
func convertGeminiResponse(resp *genai.GenerateContentResponse) (*GenerationResponse, error) {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("gemini returned an empty response")
	}

	cand := resp.Candidates[0]
	out := &GenerationResponse{
		Text:         resp.Text(),
		FinishReason: string(cand.FinishReason),
		Model:        resp.ModelVersion,
	}
	if u := resp.UsageMetadata; u != nil {
		out.Usage = UsageInfo{
			PromptTokens:    u.PromptTokenCount,
			OutputTokens:    u.CandidatesTokenCount,
			ReasoningTokens: u.ThoughtsTokenCount,
			CachedTokens:    u.CachedContentTokenCount,
			TotalTokens:     u.TotalTokenCount,
		}
	}
	return out, nil
}

// IsRetryable classifies Gemini API errors: googleapi 429/5xx are retryable,
// other googleapi codes are terminal, and everything else falls back to the
// string heuristics shared with the GitHub path.
func (p *GeminiProvider) IsRetryable(err error) bool {
	if gerr, ok := errors.AsType[*googleapi.Error](err); ok {
		return gerr.Code == 429 || (gerr.Code >= 500 && gerr.Code <= 599)
	}
	return isRetryableByMessage(err)
}
