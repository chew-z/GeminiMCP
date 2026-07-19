package main

import (
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// qwenResponsesDialect shapes Responses-API requests for Qwen models.
// thinkingForced marks models that reject enable_thinking=false (DashScope
// answers 400); for those, reasoning stays at max effort on every request.
type qwenResponsesDialect struct {
	thinkingForced bool
}

func (qwenResponsesDialect) name() string { return "qwen" }

func (d qwenResponsesDialect) buildRequest(params *responses.ResponseNewParams, req GenerationRequest) []option.RequestOption {
	switch {
	case d.thinkingForced:
		params.Reasoning.Effort = shared.ReasoningEffortMax
	case req.Thinking.Enabled && req.ResponseFormat != "json_object":
		params.Reasoning.Effort = shared.ReasoningEffortMax
	default:
		params.Reasoning.Effort = shared.ReasoningEffortNone
	}
	return []option.RequestOption{option.WithHeader("x-dashscope-session-cache", "enable")}
}
