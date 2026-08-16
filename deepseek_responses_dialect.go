package main

import (
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// deepseekResponsesDialect shapes Responses-API requests for DeepSeek V4
// models (POST /responses on api.deepseek.com, OpenAI Responses format).
// Effort floor is low, not none: V4's release notes enumerate low/high/max
// for reasoning.effort, and "none" — while accepted today — is undocumented.
// Thinking-disabled requests (prequalify) therefore use low. The endpoint
// is stateless, so no caching headers are sent.
type deepseekResponsesDialect struct{}

func (deepseekResponsesDialect) name() string { return "deepseek" }

func (deepseekResponsesDialect) buildRequest(params *responses.ResponseNewParams, req GenerationRequest) []option.RequestOption {
	if req.Thinking.Enabled {
		params.Reasoning.Effort = shared.ReasoningEffortMax
	} else {
		params.Reasoning.Effort = shared.ReasoningEffortLow
	}
	return nil
}
