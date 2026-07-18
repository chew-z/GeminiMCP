package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type responsesDialect interface {
	name() string
	buildRequest(*responses.ResponseNewParams, GenerationRequest) []option.RequestOption
}

type responsesProvider struct {
	client  openai.Client
	model   string
	dialect responsesDialect
	logger  Logger
}

func newResponsesProvider(cfg ProviderConfig, dialect responsesDialect, logger Logger) *responsesProvider {
	return &responsesProvider{
		client: openai.NewClient(
			option.WithAPIKey(cfg.APIKey),
			option.WithBaseURL(cfg.BaseURL),
			option.WithMaxRetries(0),
		),
		model:   cfg.Model,
		dialect: dialect,
		logger:  logger,
	}
}

func (p *responsesProvider) Generate(ctx context.Context, req GenerationRequest) (*GenerationResponse, error) {
	if p == nil || p.dialect == nil {
		return nil, errors.New("Responses provider not initialized")
	}
	params := p.buildResponseParams(req)
	resp, err := p.client.Responses.New(ctx, params, p.dialect.buildRequest(&params, req)...)
	if err != nil {
		return nil, err
	}
	p.logReasoningItems(resp)
	return convertResponse(resp)
}

func (p *responsesProvider) buildResponseParams(req GenerationRequest) responses.ResponseNewParams {
	var input strings.Builder
	for _, part := range req.Parts {
		if part.File != nil {
			if p.logger != nil {
				p.logger.Warn("binary file parts are not supported by this provider; skipping")
			}
			continue
		}
		input.WriteString(part.Text)
	}
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(p.model),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: param.NewOpt(input.String()),
		},
		Temperature: param.NewOpt(req.Temperature),
	}
	if req.SystemPrompt != "" {
		params.Instructions = param.NewOpt(req.SystemPrompt)
	}
	if req.MaxOutputTokens > 0 {
		params.MaxOutputTokens = param.NewOpt(int64(req.MaxOutputTokens))
	}
	if req.ResponseFormat == "json_object" {
		jsonParam := shared.NewResponseFormatJSONObjectParam()
		params.Text.Format = responses.ResponseFormatTextConfigUnionParam{OfJSONObject: &jsonParam}
	}
	return params
}

func convertResponse(resp *responses.Response) (*GenerationResponse, error) {
	if resp == nil {
		return nil, errors.New("provider returned nil response")
	}
	response := &GenerationResponse{Text: resp.OutputText(), Model: resp.Model, Usage: mapResponseUsage(resp.Usage)}
	switch resp.Status {
	case responses.ResponseStatusFailed:
		return nil, fmt.Errorf("response failed: %s", resp.Error.Message)
	case responses.ResponseStatusCancelled:
		return nil, errors.New("response was cancelled")
	case responses.ResponseStatusIncomplete:
		response.FinishReason = resp.IncompleteDetails.Reason
		return response, nil
	case responses.ResponseStatusCompleted:
		if len(resp.Output) == 0 {
			return nil, errors.New("provider returned empty output")
		}
		response.FinishReason = "stop"
		return response, nil
	default:
		return nil, fmt.Errorf("response returned unexpected status %q", resp.Status)
	}
}

func mapResponseUsage(usage responses.ResponseUsage) UsageInfo {
	return UsageInfo{
		PromptTokens:    int32(usage.InputTokens),
		OutputTokens:    int32(usage.OutputTokens),
		ReasoningTokens: int32(usage.OutputTokensDetails.ReasoningTokens),
		CachedTokens:    int32(usage.InputTokensDetails.CachedTokens),
		TotalTokens:     int32(usage.TotalTokens),
	}
}

func (p *responsesProvider) IsRetryable(err error) bool {
	if apiErr, ok := errors.AsType[*openai.Error](err); ok {
		return apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}
	return isRetryableByMessage(err)
}

// logReasoningItems records the count and summary lengths of reasoning output
// items, matching the debug-level observability of openaiProvider.logReasoningContent.
func (p *responsesProvider) logReasoningItems(resp *responses.Response) {
	if resp == nil || p.logger == nil {
		return
	}
	var totalSummaryLen int
	reasoningCount := 0
	for _, item := range resp.Output {
		if item.Type != "reasoning" {
			continue
		}
		reasoningCount++
		for _, s := range item.Summary {
			totalSummaryLen += len(s.Text)
		}
	}
	if reasoningCount > 0 {
		p.logger.Debug("%s reasoning items=%d summary_length=%d", p.dialect.name(), reasoningCount, totalSummaryLen)
	}
}
