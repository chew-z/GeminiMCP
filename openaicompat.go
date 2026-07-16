package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/shared"
)

// vendorDialect isolates OpenAI-compatible API differences. DeepSeek uses it
// now; Qwen will add another dialect in Phase 3 without widening Provider.
type vendorDialect interface {
	name() string
	buildRequest(params *openai.ChatCompletionNewParams, req GenerationRequest) []option.RequestOption
}

// openaiProvider adapts an OpenAI-compatible chat-completions API to Provider.
type openaiProvider struct {
	client  openai.Client
	model   string
	dialect vendorDialect
	logger  Logger
}

// newOpenAIProvider creates an OpenAI-compatible provider with SDK retries disabled.
func newOpenAIProvider(cfg ProviderConfig, dialect vendorDialect, logger Logger) *openaiProvider {
	return &openaiProvider{
		client:  openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(cfg.BaseURL), option.WithMaxRetries(0)),
		model:   cfg.Model,
		dialect: dialect,
		logger:  logger,
	}
}

// Generate sends one provider-neutral request to the compatible chat API.
func (p *openaiProvider) Generate(ctx context.Context, req GenerationRequest) (*GenerationResponse, error) {
	if p == nil || p.dialect == nil {
		return nil, errors.New("OpenAI-compatible provider not initialized")
	}

	params, requestOptions := p.buildChatParams(req)
	resp, err := p.client.Chat.Completions.New(ctx, params, requestOptions...)
	if err != nil {
		return nil, err
	}
	converted, err := convertChatCompletion(resp)
	if err != nil {
		return nil, err
	}
	p.logReasoningContent(resp.Choices[0].Message.JSON.ExtraFields)
	return converted, nil
}

// buildChatParams maps a provider-neutral request to compatible chat parameters.
func (p *openaiProvider) buildChatParams(req GenerationRequest) (openai.ChatCompletionNewParams, []option.RequestOption) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, 2)
	if req.SystemPrompt != "" {
		messages = append(messages, openai.SystemMessage(req.SystemPrompt))
	}
	var user strings.Builder
	for _, part := range req.Parts {
		if part.File != nil {
			if p.logger != nil {
				p.logger.Warn("binary file parts are not supported by this provider; skipping")
			}
			continue
		}
		user.WriteString(part.Text)
	}
	messages = append(messages, openai.UserMessage(user.String()))

	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(p.model),
		Messages:    messages,
		Temperature: openai.Float(req.Temperature),
	}
	if req.MaxOutputTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxOutputTokens))
	}
	if req.ResponseFormat == "json_object" {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}
	}

	requestOptions := p.dialect.buildRequest(&params, req)
	return params, requestOptions
}

// convertChatCompletion maps a compatible chat response to GenerationResponse.
func convertChatCompletion(resp *openai.ChatCompletion) (*GenerationResponse, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, errors.New("provider returned no choices")
	}

	choice := resp.Choices[0]
	return &GenerationResponse{
		Text:         choice.Message.Content,
		FinishReason: choice.FinishReason,
		Model:        resp.Model,
		Usage: UsageInfo{
			PromptTokens:    int32(resp.Usage.PromptTokens),
			OutputTokens:    int32(resp.Usage.CompletionTokens),
			ReasoningTokens: int32(resp.Usage.CompletionTokensDetails.ReasoningTokens),
			CachedTokens:    int32(resp.Usage.PromptTokensDetails.CachedTokens),
			TotalTokens:     int32(resp.Usage.TotalTokens),
		},
	}, nil
}

// logReasoningContent records only the length of a vendor reasoning trace.
func (p *openaiProvider) logReasoningContent(fields map[string]respjson.Field) {
	field, ok := fields["reasoning_content"]
	if !ok {
		return
	}
	var reasoning string
	if err := json.Unmarshal([]byte(field.Raw()), &reasoning); err != nil {
		if p.logger != nil {
			p.logger.Debug("%s reasoning_content could not be decoded: %v", p.dialect.name(), err)
		}
		return
	}
	if p.logger != nil {
		p.logger.Debug("%s reasoning_content length=%d", p.dialect.name(), len(reasoning))
	}
}

// IsRetryable classifies compatible API 429 and 5xx errors as transient.
func (p *openaiProvider) IsRetryable(err error) bool {
	if apiErr, ok := errors.AsType[*openai.Error](err); ok {
		return apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}
	return isRetryableByMessage(err)
}

// deepseekDialect maps the provider-neutral thinking setting to DeepSeek fields.
type deepseekDialect struct{}

func (deepseekDialect) name() string { return "deepseek" }

func (deepseekDialect) buildRequest(params *openai.ChatCompletionNewParams, req GenerationRequest) []option.RequestOption {
	if req.Thinking.Enabled {
		params.ReasoningEffort = shared.ReasoningEffort("max")
		return []option.RequestOption{option.WithJSONSet("thinking", map[string]any{"type": "enabled"})}
	}
	return []option.RequestOption{option.WithJSONSet("thinking", map[string]any{"type": "disabled"})}
}
