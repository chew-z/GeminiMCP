package main

import (
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// qwenResponsesDialect shapes Responses-API requests for Qwen models.
// thinkingForced marks thinking-only models (e.g. qwen3.8-max-preview) that
// reject enable_thinking=false with a 400. Their documented effort values are
// low/high/xhigh (default xhigh, 131072-token thinking budget), so they get
// high effort for real generations and low effort for cheap utility calls
// like prequalification — never none, and not max: top intensity burns
// minutes per request and overruns the GEMINI_TIMEOUT budget.
type qwenResponsesDialect struct {
	thinkingForced bool
}

func (qwenResponsesDialect) name() string { return "qwen" }

func (d qwenResponsesDialect) buildRequest(params *responses.ResponseNewParams, req GenerationRequest) []option.RequestOption {
	switch {
	case d.thinkingForced && req.Thinking.Enabled:
		params.Reasoning.Effort = shared.ReasoningEffortHigh
	case d.thinkingForced:
		params.Reasoning.Effort = shared.ReasoningEffortLow
	case req.Thinking.Enabled && req.ResponseFormat != "json_object":
		params.Reasoning.Effort = shared.ReasoningEffortMax
	default:
		params.Reasoning.Effort = shared.ReasoningEffortNone
	}
	return []option.RequestOption{option.WithHeader("x-dashscope-session-cache", "enable")}
}
