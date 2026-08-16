package main

import (
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// deepseekResponsesDialect shapes Responses-API requests for DeepSeek V4
// models (POST /responses on api.deepseek.com, OpenAI Responses format).
// The API documents `reasoning.effort` as supported without enumerating
// values; V4's release notes list low/high/max for enabled thinking, and
// disabled thinking maps to "none" following the OpenAI convention the
// endpoint emulates. If a future API revision rejects "none", prequalify
// falls back to its default category and the 400 is never retried. The
// endpoint is stateless, so no caching headers are sent.
type deepseekResponsesDialect struct{}

func (deepseekResponsesDialect) name() string { return "deepseek" }

func (deepseekResponsesDialect) buildRequest(params *responses.ResponseNewParams, req GenerationRequest) []option.RequestOption {
	if req.Thinking.Enabled {
		params.Reasoning.Effort = shared.ReasoningEffortMax
	} else {
		params.Reasoning.Effort = shared.ReasoningEffortNone
	}
	return nil
}
